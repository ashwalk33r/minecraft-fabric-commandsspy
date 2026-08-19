package main

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestVarIntRoundTrip(t *testing.T) {
	for _, v := range []int{0, 1, 127, 128, 255, 2097151, 2147483647, -1} {
		var b buf
		b.varint(v)
		got, err := readVarint(bytes.NewReader(b.Bytes()))
		if err != nil || got != v {
			t.Fatalf("varint %d -> %d, err %v", v, got, err)
		}
	}
}

func TestOfflineUUID(t *testing.T) {
	u := offlineUUID("e2e_player1")
	if u[6]>>4 != 3 {
		t.Fatalf("not a version-3 UUID: %x", u)
	}
	if u[8]>>6 != 2 {
		t.Fatalf("bad variant bits: %x", u)
	}
	// Known value: MD5("OfflinePlayer:e2e_player1") with version/variant bits set.
	if got := hex.EncodeToString(u[:]); got != "ff97cd301ce436a6afbe97d87cf4447c" {
		t.Fatalf("uuid = %s", got)
	}
}

// The 1.19+ side of the era boundary: chat_command carries the bare command,
// WITHOUT the slash. The harness's log oracle asserts this exact literal.
func TestCommandBytesModern1192(t *testing.T) {
	const ts = 0x0123456789ABCDEF
	got := commandPacket(rows[760], "list", ts)
	want := []byte{
		0x04,                     // packet id chat_command (proto 760)
		0x04, 'l', 'i', 's', 't', // String "list" — no slash
		0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, // i64 timestamp
		0, 0, 0, 0, 0, 0, 0, 0, // i64 salt
		0x00, // empty argument-signature array
		0x00, // signedPreview = false
		0x00, // previousMessages: none
		0x00, // lastRejectedMessage: absent
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("chat_command bytes\n got %x\nwant %x", got, want)
	}
}

// The pre-1.19 side of the boundary: a plain chat packet whose payload is
// "/list", slash INCLUDED. Together with TestCommandBytesModern1192 this
// pins the exact era-literal split ("/list" vs "list") that the harness's
// log oracle asserts on.
func TestCommandBytesLegacy1182(t *testing.T) {
	got := commandPacket(rows[758], "list", 0x0123456789ABCDEF)
	want := []byte{
		0x03,                          // packet id chat (proto 758)
		0x05, '/', 'l', 'i', 's', 't', // String "/list" — slash included, nothing else
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("chat bytes\n got %x\nwant %x", got, want)
	}
}
