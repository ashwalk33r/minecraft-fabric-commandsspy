package main

import (
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeRCONSeq serves one handler per connection, in order, on a random
// loopback port. The returned counter reports how many connections were
// accepted — which is what the retry tests actually assert.
func fakeRCONSeq(t *testing.T, handlers ...func(c net.Conn)) (string, func() int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var accepted atomic.Int64
	go func() {
		for _, h := range handlers {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			h(c)
			c.Close()
		}
	}()
	return ln.Addr().String(), func() int { return int(accepted.Load()) }
}

func fakeRCON(t *testing.T, handler func(c net.Conn)) string {
	t.Helper()
	addr, _ := fakeRCONSeq(t, handler)
	return addr
}

func TestRconAuthFailure(t *testing.T) {
	addr := fakeRCON(t, func(c net.Conn) {
		readPacket(c)
		writePacket(c, -1, 2, "") // auth rejected
	})
	_, err := rconExec(addr, "wrong", "list")
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("want auth failure error, got %v", err)
	}
}

func TestRconFragmentedResponse(t *testing.T) {
	big := strings.Repeat("x", 4096) + strings.Repeat("y", 3000)
	addr := fakeRCON(t, func(c net.Conn) {
		id, _, _, _ := readPacket(c) // auth
		writePacket(c, id, 2, "")
		cid, _, cmd, _ := readPacket(c)
		if cmd != "help" {
			return
		}
		writePacket(c, cid, 0, big[:4096]) // client reads this before sending sentinel
		sid, _, _, _ := readPacket(c)
		writePacket(c, cid, 0, big[4096:])
		writePacket(c, sid, 0, "Unknown request") // vanilla-style echo for the sentinel
	})
	got, err := rconExec(addr, "pw", "help")
	if err != nil {
		t.Fatal(err)
	}
	if got != big {
		t.Fatalf("reassembly mismatch: got %d bytes, want %d", len(got), len(big))
	}
}

// serveSaveAll plays a healthy server: auth, one response packet, sentinel echo.
func serveSaveAll(c net.Conn) {
	id, _, _, _ := readPacket(c)
	writePacket(c, id, 2, "")
	cid, _, _, _ := readPacket(c)
	writePacket(c, cid, 0, "Saved the game")
	sid, _, _, _ := readPacket(c)
	writePacket(c, sid, 0, "")
}

// Issue #89: the server accepted the connection and hung up mid-exchange.
// The second attempt must carry the leg.
func TestRconRetriesAfterMidExchangeClose(t *testing.T) {
	rconRetryDelay = 0
	t.Cleanup(func() { rconRetryDelay = time.Second })
	addr, accepted := fakeRCONSeq(t,
		func(c net.Conn) {}, // accept, then hang up: what #89 saw as "rcon: EOF"
		serveSaveAll,
	)
	got, err := rconExec(addr, "pw", "save-all")
	if err != nil {
		t.Fatalf("want success on the second attempt, got %v", err)
	}
	if got != "Saved the game" {
		t.Fatalf("got %q, want %q", got, "Saved the game")
	}
	if n := accepted(); n != 2 {
		t.Fatalf("want exactly 2 connections, got %d", n)
	}
}

// A rejected password is deterministic. Retrying it would triple the noise and
// prove nothing.
func TestRconDoesNotRetryAuthFailure(t *testing.T) {
	rconRetryDelay = 0
	t.Cleanup(func() { rconRetryDelay = time.Second })
	addr, accepted := fakeRCONSeq(t,
		func(c net.Conn) {
			readPacket(c)
			writePacket(c, -1, 2, "") // auth rejected
		},
		serveSaveAll, // must never be reached
	)
	if _, err := rconExec(addr, "wrong", "list"); err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("want auth failure error, got %v", err)
	}
	if n := accepted(); n != 1 {
		t.Fatalf("auth failure must not be retried; got %d connections", n)
	}
}

// The Babric/BTA legs assert RCON's ABSENCE by dialling a port nothing is on.
// That probe must stay instant, so a refused dial is never retried.
func TestRconDoesNotRetryRefusedDial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens now
	start := time.Now()
	_, err = rconExec(addr, "pw", "save-all")
	if err == nil || !strings.Contains(err.Error(), "dial") {
		t.Fatalf("want a dial error, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("a refused dial must fail fast, took %v", elapsed)
	}
}
