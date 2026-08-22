package main

import (
	"bytes"
	"testing"
)

// string16 is protocol 14's only string form: a big-endian int16 counting UTF-16 code
// units, followed by that many code units as UTF-16BE. The count is NOT a byte count
// and NOT a rune count, which is where an implementation usually goes wrong.
func TestBetaString16(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []byte
	}{
		{"ascii", "AB", []byte{0x00, 0x02, 0x00, 'A', 0x00, 'B'}},
		{"empty", "", []byte{0x00, 0x00}},
		{"offline handshake reply", "-", []byte{0x00, 0x01, 0x00, '-'}},
		// U+00E9 is one code unit; the byte length is 2 while the count stays 1.
		{"bmp non-ascii", "é", []byte{0x00, 0x01, 0x00, 0xe9}},
		// U+1F600 is a surrogate pair: TWO code units, count 2, four bytes.
		{"surrogate pair", "\U0001F600", []byte{0x00, 0x02, 0xd8, 0x3d, 0xde, 0x00}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := betaString16(c.in)
			if !bytes.Equal(got, c.want) {
				t.Fatalf("betaString16(%q) = % x, want % x", c.in, got, c.want)
			}
		})
	}
}

// Round-tripping matters because the handshake reply and any kick reason are read back
// with the same encoding they are written with.
func TestBetaString16RoundTrip(t *testing.T) {
	for _, s := range []string{"", "-", "SpyBot", "é", "\U0001F600"} {
		encoded := betaString16(s)
		got, err := betaReadString16(bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("betaReadString16(% x): %v", encoded, err)
		}
		if got != s {
			t.Fatalf("round trip of %q gave %q", s, got)
		}
	}
}

// The three packets the bot writes, byte for byte. Protocol 14 has no length prefix, so
// a packet IS its id byte followed by its fields — an off-by-one here desynchronizes the
// whole stream rather than failing cleanly.
func TestBetaPacketLayouts(t *testing.T) {
	t.Run("handshake", func(t *testing.T) {
		want := append([]byte{0x02}, betaString16("SpyBot")...)
		if got := betaHandshakePacket("SpyBot"); !bytes.Equal(got, want) {
			t.Fatalf("handshake = % x, want % x", got, want)
		}
	})

	t.Run("login request", func(t *testing.T) {
		var want []byte
		want = append(want, 0x01)
		want = append(want, 0x00, 0x00, 0x00, 0x0e) // int32 protocol 14
		want = append(want, betaString16("SpyBot")...)
		want = append(want, 0, 0, 0, 0, 0, 0, 0, 0) // int64 map seed 0
		want = append(want, 0x00)                   // int8 dimension 0
		if got := betaLoginPacket("SpyBot"); !bytes.Equal(got, want) {
			t.Fatalf("login = % x, want % x", got, want)
		}
	})

	t.Run("chat carries the slash so it reaches the command seam", func(t *testing.T) {
		want := append([]byte{0x03}, betaString16("/me waves")...)
		if got := betaChatPacket("/me waves"); !bytes.Equal(got, want) {
			t.Fatalf("chat = % x, want % x", got, want)
		}
	})
}

// Keep Alive is a BARE byte in both directions — no payload at all. Every later protocol
// gives it an int body, so this is the assumption most likely to be imported by mistake.
func TestBetaKeepAliveIsABareByte(t *testing.T) {
	if got := betaKeepAlivePacket(); !bytes.Equal(got, []byte{0x00}) {
		t.Fatalf("keep alive = % x, want 00", got)
	}
}
