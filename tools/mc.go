// Minimal offline-mode Minecraft Java protocol layer for the e2e bot:
// status ping + login + (configuration) + one command + keepalives.
// No encryption (online-mode=false is a harness invariant), no auth, no
// chat signing. Compression is off in the harness
// (network-compression-threshold=-1) but handled defensively anyway.
// Adapted from the proven feasibility prototype (proto/handrolled/main.go,
// verified live on 1.16.5/1.19.2/1.21.11/26.1/26.2).
package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"
)

// ---------- wire primitives ----------

type buf struct{ bytes.Buffer }

func (b *buf) varint(v int) {
	u := uint32(v)
	for {
		if u&^0x7f == 0 {
			b.WriteByte(byte(u))
			return
		}
		b.WriteByte(byte(u&0x7f | 0x80))
		u >>= 7
	}
}
func (b *buf) str(s string) { b.varint(len(s)); b.WriteString(s) }

// binary.Write to a bytes.Buffer cannot fail; the ignore is deliberate.
func (b *buf) u16(v uint16) { _ = binary.Write(b, binary.BigEndian, v) }
func (b *buf) i64(v int64)  { _ = binary.Write(b, binary.BigEndian, v) }
func (b *buf) boolean(v bool) {
	if v {
		b.WriteByte(1)
	} else {
		b.WriteByte(0)
	}
}

func readVarint(r io.ByteReader) (int, error) {
	var v, pos uint32
	for {
		c, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		v |= uint32(c&0x7f) << pos
		if c&0x80 == 0 {
			return int(int32(v)), nil
		}
		pos += 7
		if pos >= 32 {
			return 0, errors.New("varint too long")
		}
	}
}

func readString(r *bytes.Reader) (string, error) {
	n, err := readVarint(r)
	if err != nil {
		return "", err
	}
	if n < 0 || n > r.Len() {
		return "", fmt.Errorf("string length %d out of range (%d bytes left)", n, r.Len())
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return string(b), nil
}

// offlineUUID is the offline-mode UUID: version-3 MD5 of "OfflinePlayer:<name>".
func offlineUUID(name string) [16]byte {
	h := md5.Sum([]byte("OfflinePlayer:" + name))
	h[6] = (h[6] & 0x0f) | 0x30
	h[8] = (h[8] & 0x3f) | 0x80
	return h
}

// conn frames packets; threshold >= 0 means compression is on.
type conn struct {
	c         net.Conn
	r         *bufio.Reader
	threshold int
}

// dial connects and sets an absolute deadline on the socket, so every
// read/write in the whole run is bounded by the global timeout.
func dial(addr string, deadline time.Time) (*conn, error) {
	nc, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, err
	}
	if err := nc.SetDeadline(deadline); err != nil {
		_ = nc.Close()
		return nil, err
	}
	return &conn{c: nc, r: bufio.NewReader(nc), threshold: -1}, nil
}

// sendPacket frames an already-serialized [id][body] payload.
func (c *conn) sendPacket(payload []byte) error {
	var frame buf
	if c.threshold >= 0 {
		if len(payload) >= c.threshold {
			var z bytes.Buffer
			w := zlib.NewWriter(&z)
			if _, err := w.Write(payload); err != nil {
				return fmt.Errorf("compress: %w", err)
			}
			if err := w.Close(); err != nil {
				return fmt.Errorf("compress: %w", err)
			}
			frame.varint(len(payload))
			frame.Write(z.Bytes())
		} else {
			frame.varint(0)
			frame.Write(payload)
		}
	} else {
		frame.Write(payload)
	}
	var out buf
	out.varint(frame.Len())
	out.Write(frame.Bytes())
	_, err := c.c.Write(out.Bytes())
	return err
}

func (c *conn) send(id int, body []byte) error {
	var p buf
	p.varint(id)
	p.Write(body)
	return c.sendPacket(p.Bytes())
}

// recv returns packet id and body.
func (c *conn) recv() (int, *bytes.Reader, error) {
	n, err := readVarint(c.r)
	if err != nil {
		return 0, nil, err
	}
	if n < 0 {
		return 0, nil, fmt.Errorf("negative frame length %d", n)
	}
	raw := make([]byte, n)
	if _, err := io.ReadFull(c.r, raw); err != nil {
		return 0, nil, err
	}
	rd := bytes.NewReader(raw)
	if c.threshold >= 0 {
		size, err := readVarint(rd)
		if err != nil {
			return 0, nil, err
		}
		if size > 0 {
			zr, err := zlib.NewReader(rd)
			if err != nil {
				return 0, nil, err
			}
			dec, err := io.ReadAll(zr)
			if err != nil {
				return 0, nil, err
			}
			rd = bytes.NewReader(dec)
		}
	}
	id, err := readVarint(rd)
	return id, rd, err
}

func (c *conn) handshake(proto int, host string, port, next int) error {
	var b buf
	b.varint(proto)
	b.str(host)
	b.u16(uint16(port))
	b.varint(next)
	return c.send(0x00, b.Bytes())
}

// ---------- status ping ----------

// ping negotiates the version: it asks the server for its status and returns
// the advertised version name and protocol number.
func ping(host string, port int, deadline time.Time) (string, int, error) {
	c, err := dial(fmt.Sprintf("%s:%d", host, port), deadline)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = c.c.Close() }()
	// -1 in the status handshake means "protocol unknown"; the server answers
	// regardless — this is how the version is negotiated, never passed by name.
	if err := c.handshake(-1, host, port, 1); err != nil {
		return "", 0, err
	}
	if err := c.send(0x00, nil); err != nil {
		return "", 0, err
	}
	id, body, err := c.recv()
	if err != nil {
		return "", 0, err
	}
	if id != 0x00 {
		return "", 0, fmt.Errorf("unexpected status packet 0x%02x", id)
	}
	var st struct {
		Version struct {
			Name     string `json:"name"`
			Protocol int    `json:"protocol"`
		} `json:"version"`
	}
	js, err := readString(body)
	if err != nil {
		return "", 0, fmt.Errorf("status response: %w", err)
	}
	if err := json.Unmarshal([]byte(js), &st); err != nil {
		return "", 0, err
	}
	return st.Version.Name, st.Version.Protocol, nil
}

// ---------- login + play ----------

type client struct {
	*conn
	row  row
	name string
}

// join dials, handshakes and drives login -> (configuration) -> play.
func join(r row, host string, port int, name string, deadline time.Time) (*client, error) {
	cn, err := dial(fmt.Sprintf("%s:%d", host, port), deadline)
	if err != nil {
		return nil, err
	}
	cl := &client{conn: cn, row: r, name: name}
	if err := cl.handshake(r.proto, host, port, 2); err != nil {
		_ = cn.c.Close()
		return nil, err
	}
	if err := cl.runLogin(); err != nil {
		_ = cn.c.Close()
		return nil, err
	}
	return cl, nil
}

// loginStart's payload varies by era; see docs/protocol-table.md.
func (c *client) loginStart() error {
	var b buf
	b.str(c.name)
	u := offlineUUID(c.name)
	switch p := c.row.proto; {
	case p >= 764: // 1.20.2+: required raw UUID
		b.Write(u[:])
	case p >= 761: // 1.19.3-1.20.1: Option<UUID>
		b.boolean(true)
		b.Write(u[:])
	case p == 760: // 1.19.1/1.19.2: Option<sig> (absent) + Option<UUID>
		b.boolean(false)
		b.boolean(true)
		b.Write(u[:])
	case p == 759: // 1.19: Option<sig> (absent), no UUID field
		b.boolean(false)
	default: // <=1.18.2: name only
	}
	return c.send(0x00, b.Bytes())
}

// runLogin drives login -> (configuration on 1.20.2+) -> play, returning when
// the client has reached the play state.
func (c *client) runLogin() error {
	if err := c.loginStart(); err != nil {
		return err
	}
	cfg := c.row.cfg()
	state := "login"
	for {
		id, body, err := c.recv()
		if err != nil {
			return fmt.Errorf("%s state: %w", state, err)
		}
		if state == "login" {
			switch id {
			case 0x00:
				reason, rerr := readString(body)
				if rerr != nil {
					return fmt.Errorf("login disconnect (unreadable reason: %v)", rerr)
				}
				return fmt.Errorf("login disconnect: %s", reason)
			case 0x01:
				return errors.New("server requested encryption (online-mode=true?)")
			case 0x03: // set compression
				t, _ := readVarint(body)
				c.threshold = t
			case 0x02: // login success
				if !c.row.config() {
					return nil // straight to play
				}
				if err := c.send(0x03, nil); err != nil { // login_acknowledged
					return err
				}
				state = "config"
			}
			continue
		}
		// configuration state (1.20.2+); keepalives here have their OWN ids.
		// Reply-send errors are deliberately dropped: a dead socket surfaces
		// as an error on the next recv, which is the one place that reports.
		switch id {
		case cfg.kaCB:
			b, _ := io.ReadAll(body)
			_ = c.send(cfg.kaSB, b)
		case cfg.pingCB:
			b, _ := io.ReadAll(body)
			_ = c.send(cfg.pongSB, b)
		case cfg.knownCB: // select_known_packs (1.20.5+): reply "no packs known"
			var b buf
			b.varint(0)
			_ = c.send(cfg.knownSB, b.Bytes())
		case cfg.cocCB: // code_of_conduct (26.x era): accept defensively
			_ = c.send(cfg.acceptSB, nil)
		case cfg.discCB:
			reason, rerr := readString(body)
			if rerr != nil {
				return fmt.Errorf("config disconnect (unreadable reason: %v)", rerr)
			}
			return fmt.Errorf("config disconnect: %s", reason)
		case cfg.finishCB:
			return c.send(cfg.finishSB, nil) // acknowledge -> play
		}
	}
}

// commandPacket serializes [id][body] of the one command, per chat era.
// Pure so tests can assert exact bytes; ts is the millisecond timestamp used
// by the signed-era layouts.
func commandPacket(r row, cmd string, ts int64) []byte {
	var b buf
	b.varint(r.cmdID)
	switch r.era {
	case eraChat:
		// pre-1.19: plain chat packet, slash INCLUDED. The harness's log
		// oracle asserts the exact era literal ("/list" vs "list") and
		// deliberately never matches both — preserve the split.
		b.str("/" + cmd)
	case era759, era760, era761:
		b.str(cmd) // no slash: chat_command carries the bare command
		b.i64(ts)
		b.i64(0)    // salt
		b.varint(0) // empty argument-signature array (offline mode: unsigned)
		switch r.era {
		case era759:
			b.boolean(false) // signedPreview
		case era760:
			b.boolean(false) // signedPreview
			b.varint(0)      // previousMessages
			b.boolean(false) // lastRejectedMessage
		case era761:
			b.varint(0)              // messageCount
			b.Write([]byte{0, 0, 0}) // acknowledged: fixed 20-bit bitset
		}
	default: // eraPlain, 1.20.5+: the string is the whole packet
		b.str(cmd) // no slash
	}
	return b.Bytes()
}

func (c *client) sendCommand(cmd string) error {
	return c.sendPacket(commandPacket(c.row, cmd, time.Now().UnixMilli()))
}

// pump answers play-state keepalives until stop is closed, reporting any
// connection error. It also logs the server's /list reply as proof the
// command executed.
func (c *client) pump(stop <-chan struct{}, errc chan<- error) {
	for {
		select {
		case <-stop:
			return
		default:
		}
		id, body, err := c.recv()
		if err != nil {
			select {
			case <-stop:
			default:
				errc <- fmt.Errorf("%s: %w", c.name, err)
			}
			return
		}
		b, _ := io.ReadAll(body)
		if id == c.row.kaCB {
			// A failed reply surfaces as an error on the next recv.
			_ = c.send(c.row.kaSB, b)
		}
		// Proof the command ran: /list's reply arrives as a system message.
		// ponytail: substring sniff, decode system_chat properly if the reply
		// text ever needs structure.
		if i := bytes.Index(b, []byte("commands.list")); i >= 0 {
			end := min(i+200, len(b))
			log.Printf("%s: SERVER REPLY: %s", c.name, strings.Map(printable, string(b[i:end])))
		}
	}
}

func printable(r rune) rune {
	if r < 32 || r > 126 {
		return -1
	}
	return r
}
