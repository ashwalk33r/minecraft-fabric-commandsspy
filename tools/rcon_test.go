package main

import (
	"net"
	"strings"
	"testing"
)

func fakeRCON(t *testing.T, handler func(c net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		handler(c)
	}()
	return ln.Addr().String()
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
