package main

// RCON client (Issue D). Hand-rolled per judge ruling — no gorcon dependency.
// Framing: little-endian int32 length | int32 request_id | int32 type | payload | \x00\x00.
// Fragmented responses (>4096B) are reassembled explicitly: after the command
// we send a sentinel packet with a distinct request_id and concatenate
// response payloads until the packet answering the sentinel arrives.

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	serverdataAuth        = 3
	serverdataExecCommand = 2
	rconTimeout           = 10 * time.Second // matches the python client it replaces
	authID                = 1
	cmdID                 = 2
	sentinelID            = 3
)

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

// rconExec connects, authenticates, runs cmd, and returns the reassembled response.
func rconExec(addr, password, cmd string) (string, error) {
	conn, err := net.DialTimeout("tcp", addr, rconTimeout)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(rconTimeout)); err != nil {
		return "", err
	}

	if err := writePacket(conn, authID, serverdataAuth, password); err != nil {
		return "", err
	}
	for { // servers may send an empty RESPONSE_VALUE before the AUTH_RESPONSE (type 2)
		id, typ, _, err := readPacket(conn)
		if err != nil {
			return "", err
		}
		if typ != 2 {
			continue
		}
		if id == -1 {
			return "", fmt.Errorf("authentication failed (wrong rcon.password?)")
		}
		break
	}

	if err := writePacket(conn, cmdID, serverdataExecCommand, cmd); err != nil {
		return "", err
	}
	// Vanilla closes the connection if two client packets share one TCP
	// segment (verified empirically on 1.21.11), so read the first response
	// packet before sending the sentinel.
	var out strings.Builder
	id, _, payload, err := readPacket(conn)
	if err != nil {
		return "", err
	}
	if id == cmdID {
		out.WriteString(payload)
	}
	// Sentinel: the server answers in order, so every packet before the
	// sentinel's answer belongs to cmd. Tolerates both an empty reply and an
	// echoed error string for the sentinel — we key on request_id only — and
	// the connection deadline bounds a server that never answers it.
	if err := writePacket(conn, sentinelID, serverdataExecCommand, ""); err != nil {
		return "", err
	}
	for {
		id, _, payload, err := readPacket(conn)
		if err != nil {
			return "", err
		}
		if id == sentinelID {
			return out.String(), nil
		}
		if id == cmdID {
			out.WriteString(payload)
		}
	}
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
