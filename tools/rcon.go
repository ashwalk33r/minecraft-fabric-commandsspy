package main

// RCON client, hand-rolled (no gorcon dependency).
// Framing: little-endian int32 length | int32 request_id | int32 type | payload | \x00\x00.
// Fragmented responses (>4096B) are reassembled explicitly: after the command
// we send a sentinel packet with a distinct request_id and concatenate
// response payloads until the packet answering the sentinel arrives.

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

const (
	serverdataAuth        = 3
	serverdataExecCommand = 2
	rconTimeout           = 10 * time.Second
	rconAttempts          = 3
	authID                = 1
	cmdID                 = 2
	sentinelID            = 3
)

// A var, not a const, purely so the retry tests do not pay three seconds.
var rconRetryDelay = time.Second

// Sentinel so the retry loop can recognise the one failure that is a verdict
// rather than a transport hiccup.
var errAuthFailed = errors.New("authentication failed (wrong rcon.password?)")

func writePacket(w io.Writer, id, typ int32, payload string) error {
	pkt := make([]byte, 14+len(payload)) // len | id | type | payload | \x00\x00
	binary.LittleEndian.PutUint32(pkt[0:], uint32(10+len(payload)))
	binary.LittleEndian.PutUint32(pkt[4:], uint32(id))
	binary.LittleEndian.PutUint32(pkt[8:], uint32(typ))
	copy(pkt[12:], payload) // trailing two NULs are already zero
	_, err := w.Write(pkt)
	return err
}

func readPacket(r io.Reader) (id, typ int32, payload string, err error) {
	var hdr [4]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return
	}
	n := int32(binary.LittleEndian.Uint32(hdr[:]))
	if n < 10 || n > 1<<22 {
		return 0, 0, "", fmt.Errorf("bad rcon packet length %d", n)
	}
	body := make([]byte, n)
	if _, err = io.ReadFull(r, body); err != nil {
		return
	}
	id = int32(binary.LittleEndian.Uint32(body[0:]))
	typ = int32(binary.LittleEndian.Uint32(body[4:]))
	return id, typ, string(body[8 : n-2]), nil
}

// rconOnce is one whole conversation on one fresh connection: dial, auth, the
// command, the sentinel. The middle return value names the phase that failed,
// which is what makes an EOF diagnosable (issue #89 could not tell where the
// exchange died).
func rconOnce(addr, password, cmd string) (string, string, error) {
	conn, err := net.DialTimeout("tcp", addr, rconTimeout)
	if err != nil {
		return "", "dial", err
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(rconTimeout)); err != nil {
		return "", "deadline", err
	}

	if err := writePacket(conn, authID, serverdataAuth, password); err != nil {
		return "", "auth-write", err
	}
	for { // servers may send an empty RESPONSE_VALUE before the AUTH_RESPONSE (type 2)
		id, typ, _, err := readPacket(conn)
		if err != nil {
			return "", "auth-read", err
		}
		if typ != 2 {
			continue
		}
		if id == -1 {
			return "", "auth-read", errAuthFailed
		}
		break
	}

	if err := writePacket(conn, cmdID, serverdataExecCommand, cmd); err != nil {
		return "", "cmd-write", err
	}
	// Vanilla closes the connection if two client packets share one TCP
	// segment, so read the first response packet before sending the sentinel.
	var out strings.Builder
	id, _, payload, err := readPacket(conn)
	if err != nil {
		return "", "cmd-read", err
	}
	if id == cmdID {
		out.WriteString(payload)
	}
	// The server answers in order: everything before the sentinel's reply
	// belongs to cmd; matched by request_id.
	if err := writePacket(conn, sentinelID, serverdataExecCommand, ""); err != nil {
		return "", "sentinel-write", err
	}
	for {
		id, _, payload, err := readPacket(conn)
		if err != nil {
			return "", "cmd-read", err
		}
		if id == sentinelID {
			return out.String(), "", nil
		}
		if id == cmdID {
			out.WriteString(payload)
		}
	}
}

// rconExec runs the conversation, retrying a connection that was ESTABLISHED
// and then broke — the failure issue #89 saw once on 1.14.4, where the server
// logged "Rcon connection from" and the client got EOF with no server-side
// error. A refused dial and a rejected password are deterministic answers and
// are returned on the first attempt: the Babric/BTA legs assert RCON's absence
// by dialling a dead port, and that probe must stay instant.
//
// Every failed attempt is announced on stderr with its phase, its elapsed
// time and the underlying error, so the flake stays countable in CI logs
// instead of being papered over. scripts/e2e-entrypoint.sh greps for exactly
// that line.
//
// If the break happens after the write already reached the server, the
// server has likely already run cmd, so a retry can run it twice — callers
// must only pass an idempotent cmd.
func rconExec(addr, password, cmd string) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= rconAttempts; attempt++ {
		start := time.Now()
		out, phase, err := rconOnce(addr, password, cmd)
		if err == nil {
			return out, nil
		}
		lastErr = fmt.Errorf("%s: %w", phase, err)
		if phase == "dial" || errors.Is(err, errAuthFailed) || attempt == rconAttempts {
			break
		}
		fmt.Fprintf(os.Stderr, "[rcon] attempt %d/%d failed after %dms during %v; retrying in %v\n",
			attempt, rconAttempts, time.Since(start).Milliseconds(), lastErr, rconRetryDelay)
		time.Sleep(rconRetryDelay)
	}
	return "", lastErr
}

func runRcon(args []string) error {
	fs := flag.NewFlagSet("rcon", flag.ContinueOnError)
	host := fs.String("host", "127.0.0.1", "rcon host")
	port := fs.Int("port", 25575, "rcon port")
	password := fs.String("password", "", "rcon password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: tools rcon --port P --password S <command>")
	}
	out, err := rconExec(net.JoinHostPort(*host, fmt.Sprint(*port)), *password, strings.Join(fs.Args(), " "))
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}
