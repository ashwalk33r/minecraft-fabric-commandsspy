package main

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"strings"
	"testing"
)

// BTA's only string form: a big-endian int16 counting UTF-8 BYTES, followed by those
// bytes. Beta 1.7.3's string16 counts UTF-16 code units instead, so the two agree on
// ASCII and disagree everywhere else — importing the wrong one is the easiest mistake
// this file can catch.
func TestBtaString(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []byte
	}{
		{"ascii", "AB", []byte{0x00, 0x02, 'A', 'B'}},
		{"empty", "", []byte{0x00, 0x00}},
		{"offline handshake reply", "-", []byte{0x00, 0x01, '-'}},
		// U+00E9 is one code unit but TWO UTF-8 bytes: the count is 2, which is what
		// separates this encoding from beta.go's.
		{"bmp non-ascii", "é", []byte{0x00, 0x02, 0xc3, 0xa9}},
		// U+1F600 is four UTF-8 bytes and two UTF-16 code units: count 4, not 2.
		{"astral", "\U0001F600", []byte{0x00, 0x04, 0xf0, 0x9f, 0x98, 0x80}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := btaString(c.in)
			if !bytes.Equal(got, c.want) {
				t.Fatalf("btaString(%q) = % x, want % x", c.in, got, c.want)
			}
		})
	}
}

// Round-tripping matters because the handshake reply, any kick reason and the login
// reply's own strings are read back with the same encoding they are written with.
func TestBtaStringRoundTrip(t *testing.T) {
	for _, s := range []string{"", "-", "e2e_player1", "é", "\U0001F600"} {
		encoded := btaString(s)
		got, err := btaReadString(bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("btaReadString(% x): %v", encoded, err)
		}
		if got != s {
			t.Fatalf("round trip of %q gave %q", s, got)
		}
	}
}

// The three packets the bot writes, spelled out field by field rather than by calling
// the production encoder, so a changed field ORDER or WIDTH fails here and not as an
// unrelated hang. BTA has no length prefix: a packet IS its id byte followed by its
// fields, so an off-by-one desynchronizes the whole stream.
func TestBtaPacketLayouts(t *testing.T) {
	t.Run("handshake", func(t *testing.T) {
		want := []byte{0x02, 0x00, 0x0b}
		want = append(want, "e2e_player1"...)
		if got := btaHandshakePacket("e2e_player1"); !bytes.Equal(got, want) {
			t.Fatalf("handshake = % x, want % x", got, want)
		}
	})

	// Two fields here are load-bearing, not decoration. The UUID is what the server
	// keys the player by, and the RSA public key is what it encrypts that player's AES
	// key to — it throws if it cannot. Their positions are what this asserts.
	t.Run("login request", func(t *testing.T) {
		const pub = "TESTKEY"
		var want []byte
		want = append(want, 0x01)
		want = append(want, 0x00, 0x00, 0x80, 0x01) // int32 protocol, as passed in
		want = append(want, 0x00, 0x0b)
		want = append(want, "e2e_player1"...)
		uuid := md5.Sum([]byte("OfflinePlayer:e2e_player1"))
		uuid[6] = uuid[6]&0x0f | 0x30
		uuid[8] = uuid[8]&0x3f | 0x80
		want = append(want, uuid[:]...) // 16-byte uuid, version 3 over the username
		want = append(want, 0x00, 0x07)
		want = append(want, pub...)
		want = append(want, 0, 0, 0, 0, 0, 0, 0, 0) // int64 worldSeed
		want = append(want, 0, 0, 0, 0)             // int32 dimensionId
		want = append(want, 0, 0, 0, 0)             // int32 worldTypeId
		want = append(want, 0x00)                   // int8 packetDelay
		if got := btaLoginPacket(btaProtocolVersion, "e2e_player1", pub); !bytes.Equal(got, want) {
			t.Fatalf("login = % x, want % x", got, want)
		}
	})

	// The header is the whole point of this subtest. The type byte selects TYPE_CHAT,
	// the path that reaches handleMessage; the encrypted flag MUST be false, or the
	// server AES-decrypts the plaintext line into garbage. Both failures are silent —
	// the packet is accepted and the command simply never runs.
	t.Run("message header is chat, unencrypted, and carries the slash", func(t *testing.T) {
		want := []byte{0x03, 0x00, 0x00}
		want = append(want, btaString("/me waves")...)
		got := btaMessagePacket("/me waves")
		if !bytes.Equal(got, want) {
			t.Fatalf("message = % x, want % x", got, want)
		}
		if !bytes.Contains(got, []byte("/me waves")) {
			t.Fatalf("message %q lost the leading slash that makes it a command", got)
		}
	})
}

// Keep Alive is a BARE byte in both directions — no payload at all, inherited from
// Beta 1.7.3. Every post-Netty protocol gives it a body, so this is the assumption most
// likely to be imported by mistake from mc.go.
func TestBtaKeepAliveIsABareByte(t *testing.T) {
	if got := btaKeepAlivePacket(); !bytes.Equal(got, []byte{0x00}) {
		t.Fatalf("keep alive = % x, want 00", got)
	}
}

// btaNextPacketID has to survive the 0xFA custom payloads HalpLibe injects during login:
// they can arrive before the handshake reply, and skipping them by their declared length
// is the only way to stay in frame. Bare keep-alives are swallowed the same way.
func TestBtaNextPacketIDSkipsNoise(t *testing.T) {
	customPayload := func(channel string, body []byte) []byte {
		out := append([]byte{0xFA}, btaString(channel)...)
		out = binary.BigEndian.AppendUint32(out, uint32(len(body)))
		return append(out, body...)
	}

	t.Run("skips custom payloads and keep-alives", func(t *testing.T) {
		var stream []byte
		stream = append(stream, customPayload("HalpLibe|Hello", []byte{1, 2, 3, 4})...)
		stream = append(stream, 0x00) // bare keep-alive
		stream = append(stream, customPayload("", nil)...)
		stream = append(stream, 0x02) // the handshake reply we actually want
		stream = append(stream, btaString("-")...)

		r := bytes.NewReader(stream)
		id, err := btaNextPacketID(r)
		if err != nil {
			t.Fatalf("btaNextPacketID: %v", err)
		}
		if id != btaPacketHandshake {
			t.Fatalf("got packet 0x%02x, want 0x02", id)
		}
		hash, err := btaReadString(r)
		if err != nil {
			t.Fatalf("btaReadString: %v", err)
		}
		if hash != "-" {
			t.Fatalf("handshake hash = %q, want %q — the skip left the stream out of frame", hash, "-")
		}
	})

	// A 0xFF must surface the server's own kick reason rather than a framing error:
	// that string is the only diagnosis an e2e failure gets.
	t.Run("reports a disconnect with its reason", func(t *testing.T) {
		stream := append([]byte{0xFF}, betaString16("Outdated server!")...)
		_, err := btaNextPacketID(bytes.NewReader(stream))
		if err == nil || !strings.Contains(err.Error(), "Outdated server!") {
			t.Fatalf("btaNextPacketID error = %v, want it to carry the kick reason", err)
		}
		if strings.ContainsRune(err.Error(), 0) {
			t.Fatalf("kick reason %q still carries NUL bytes — decoded as UTF-8, not UTF-16BE", err)
		}
	})
}

// The key the login packet carries must be a real 2048-bit X.509/SPKI key: the server
// RSA-encrypts a per-player AES key to it, and anything else kills the connection.
// Base64 of a 2048-bit SPKI is exactly 392 chars, the server's own MAX_AES_KEY_SIZE cap.
func TestBtaPublicKeyFitsTheServerCap(t *testing.T) {
	pub, err := btaPublicKey()
	if err != nil {
		t.Fatalf("btaPublicKey: %v", err)
	}
	if len(pub) != 392 {
		t.Fatalf("public key is %d chars, want 392 (the server's MAX_AES_KEY_SIZE)", len(pub))
	}
}

// The two e2e players MUST NOT share a UUID. The server keys players by the one the
// login packet carries, so a shared UUID makes the second login silently evict the
// first — the cross-check player kills the command player, and the leg fails with an
// empty log rather than an error. Measured against BTA 8.0.1; see bta.go.
func TestBtaOfflineUUIDIsPerPlayer(t *testing.T) {
	one, two := btaOfflineUUID("e2e_player1"), btaOfflineUUID("e2e_player2")
	if one == two {
		t.Fatalf("both e2e players got uuid % x — the second login would evict the first", one)
	}
	if one != btaOfflineUUID("e2e_player1") {
		t.Fatal("btaOfflineUUID is not deterministic")
	}
	if one[6]&0xf0 != 0x30 || one[8]&0xc0 != 0x80 {
		t.Fatalf("uuid % x is not a version-3 RFC 4122 uuid", one)
	}
}

// Every BTA release has its own protocol number and the server kicks a client that
// offers a different one, so the dispatch in bot.go routes the whole range to runBtaBot
// rather than the newest number alone. The gap between the two forks is enormous — three
// digits versus five — so a floor is enough to tell them apart.
func TestBtaProtocolRangeIsAboveEveryModernOne(t *testing.T) {
	// The seven declared versions, read out of each package's PacketHandlerLogin.
	for _, p := range []int{29472, 29441, 29442, 29443, 29444, 32768, btaProtocolVersion} {
		if p < btaMinProtocolVersion {
			t.Fatalf("BTA protocol %d is below the dispatch floor %d — bot.go would send it to the modern client", p, btaMinProtocolVersion)
		}
	}
	if betaProtocolVersion >= btaMinProtocolVersion {
		t.Fatalf("protocol %d would dispatch to the BTA client", betaProtocolVersion)
	}
	for _, r := range rows {
		if r.proto >= btaMinProtocolVersion {
			t.Fatalf("table row %s (protocol %d) would dispatch to the BTA client", r.name, r.proto)
		}
	}
}
