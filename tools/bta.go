package main

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
)

// Protocol 32769 — "Better than Adventure!", a THIRD client next to table.go's modern
// one and beta.go's protocol-14 one. BTA forks Beta 1.7.3's pre-Netty framing (a packet
// is its id byte followed by big-endian fields: no length prefix, no compression, no
// transport encryption) and then changes everything above it: strings are UTF-8 with an
// int16 BYTE count instead of UTF-16BE code units, login carries a UUID and an RSA
// public key, and chat is a message packet with a type byte and an encrypted flag.
//
// Not cited from a spec page: BTA publishes no protocol documentation. Every layout here
// was verified empirically against booted BTA servers, which logged the bot in and
// processed its command. See docs/bta-toolchain-spike.md §7.
//
// The protocol number is PER RELEASE, not per fork, and the caller supplies it: 7.3 is
// 29472, 7.3_01..7.3_04 are 29441..29444, 8.0 is 32768 and 8.0.1 is 32769 (read out of
// each package's PacketHandlerLogin equality check). Offering the wrong one gets the
// server to kick the bot during login with "Outdated server!". The version→number table
// lives in scripts/e2e-run-one.sh beside each package's hash, so it is not duplicated
// here; this file only needs to know where the range starts, to tell a BTA protocol
// number apart from a modern one (three digits) on the command line.
const (
	btaProtocolVersion    = 32769 // BTA 8.0.1, the newest declared version
	btaMinProtocolVersion = 29441 // BTA 7.3_01, the lowest number any declared version uses

	// The two wire boundaries, both measured with javap over all seven server jars.
	// Note the numbers do NOT sort by release: 7.3 is 29472, ABOVE 7.3_01..7.3_04's
	// 29441..29444, so the older-than-8.0 test is a comparison and the 7.3 test is an
	// equality. A predicate cannot express it; a range table would be three rows of
	// ceremony for two branches.
	btaProtocol73 = 29472 // BTA 7.3, the last release whose chat string is UTF-8
	btaProtocol80 = 32768 // BTA 8.0, where the chat fields swap and the login tail widens
)

// Packet ids, serverbound unless noted.
const (
	btaPacketKeepAlive     = 0x00 // both directions, BARE byte, no payload
	btaPacketLogin         = 0x01
	btaPacketHandshake     = 0x02
	btaPacketMessage       = 0x03
	btaPacketCustomPayload = 0xFA // clientbound, injected by HalpLibe during login
	btaPacketDisconnect    = 0xFF // clientbound
)

// Message packet type byte: TYPE_CHAT, the one that reaches handleMessage and so the
// slash-command seam this mod hooks.
const btaMessageTypeChat = 0x00

// btaString encodes BTA's only string form: an int16 count of UTF-8 BYTES, then those
// bytes. Beta 1.7.3's string16 counts UTF-16 code units instead — the two agree on ASCII
// and disagree everywhere else, which is the single easiest layout to import wrongly.
func btaString(s string) []byte {
	return append(binary.BigEndian.AppendUint16(nil, uint16(len(s))), s...)
}

func btaReadString(r io.Reader) (string, error) {
	var count int16
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return "", fmt.Errorf("string length: %w", err)
	}
	if count < 0 {
		return "", fmt.Errorf("string length is negative (%d)", count)
	}
	b := make([]byte, count)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", fmt.Errorf("string body (%d bytes): %w", count, err)
	}
	return string(b), nil
}

func btaHandshakePacket(username string) []byte {
	return append([]byte{btaPacketHandshake}, btaString(username)...)
}

// btaLoginPacket sends zero seed/dimension/world-type: the server ignores those on a
// serverbound login and replies with the real values. The UUID and publicKey are NOT
// ignored — see btaOfflineUUID and btaPublicKey.
//
// dimensionId and worldTypeId are BYTES before 8.0 and int32s from 8.0 on. Sending the
// wide form to a 7.3-line server leaves six stray zero bytes in the stream, which that
// server reads as six bare keep-alives — harmless by luck, not by design, and the luck
// runs out the moment a non-zero value is sent.
func btaLoginPacket(protocol int, username, publicKey string) []byte {
	out := []byte{btaPacketLogin}
	out = binary.BigEndian.AppendUint32(out, uint32(protocol))
	out = append(out, btaString(username)...)
	uuid := btaOfflineUUID(username)
	out = append(out, uuid[:]...)
	out = append(out, btaString(publicKey)...)
	out = binary.BigEndian.AppendUint64(out, 0) // worldSeed
	if protocol >= btaProtocol80 {
		out = binary.BigEndian.AppendUint32(out, 0) // dimensionId
		out = binary.BigEndian.AppendUint32(out, 0) // worldTypeId
	} else {
		out = append(out, 0x00, 0x00) // dimensionId, worldTypeId
	}
	out = append(out, 0x00) // packetDelay
	return out
}

// btaMessagePacket carries chat and, with a leading slash, commands. The encrypted flag
// is false, which tells the server to take the string verbatim rather than AES-decrypt
// it. Only the message FIELD is ever encrypted on this protocol, never the stream, and
// only client→server chat may opt out — which is why this bot never needs a cipher.
//
// This is the packet that changed most across releases — three shapes, all read off the
// server jars rather than guessed, because a wrong one is not an error: the server drops
// the connection the instant it arrives ("lost connection: disconnect.genericReason")
// and the command never reaches the command manager.
//
//	7.3            (29472)  PacketChat:    type, string UTF-8,    encrypted
//	7.3_01..7.3_04 (29441+)  PacketChat:    type, string UTF-16BE, encrypted
//	8.0, 8.0.1     (32768+)  PacketMessage: type, encrypted,       string UTF-8
//
// The 8.0 line also reads a format short between the flag and the string, but ONLY when
// the type byte's high bit is set. TYPE_CHAT never sets it, so this never writes one.
func btaMessagePacket(protocol int, message string) []byte {
	out := []byte{btaPacketMessage, btaMessageTypeChat}
	if protocol >= btaProtocol80 {
		out = append(out, 0x00 /* encrypted = false */)
		return append(out, btaString(message)...)
	}
	// 7.3_01 switched this one field to UTF-16BE — which is protocol 14's string16
	// exactly (an int16 count of code units, then UTF-16BE), so it borrows beta.go's
	// encoder rather than growing a second one here.
	if protocol == btaProtocol73 {
		out = append(out, btaString(message)...)
	} else {
		out = append(out, betaString16(message)...)
	}
	return append(out, 0x00 /* encrypted = false */)
}

func btaKeepAlivePacket() []byte {
	return []byte{btaPacketKeepAlive}
}

// btaOfflineUUID is the offline-mode UUID of a username: a version-3 (MD5) UUID over
// "OfflinePlayer:<name>", the same derivation vanilla uses.
//
// The server keys players by the UUID the login packet carries, and it is what makes the
// two-player attribution cross-check work at all: with a shared UUID (a zero one, say)
// the second login silently evicts the first, and the command the first player then
// sends is dropped with nothing logged at either end. Measured against BTA 8.0.1.
func btaOfflineUUID(username string) [16]byte {
	uuid := md5.Sum([]byte("OfflinePlayer:" + username))
	uuid[6] = uuid[6]&0x0f | 0x30 // version 3
	uuid[8] = uuid[8]&0x3f | 0x80 // RFC 4122 variant
	return uuid
}

// btaPublicKey returns the base64 X.509/SPKI form of a fresh 2048-bit RSA public key.
// The server RSA-encrypts a per-player AES key to whatever the login packet supplies, so
// this has to be a real key: anything else throws inside the server's login handler and
// the connection dies. The matching private key is discarded — the bot never reads the
// server's replies, so it never has to decrypt that AES key. The verdict comes from the
// server log, as it does for every other loader.
func btaPublicKey() (string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("generate rsa key: %w", err)
	}
	spki, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", fmt.Errorf("marshal public key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(spki), nil
}

// btaNextPacketID reads the id of the next packet the client cares about, swallowing the
// two that can appear anywhere: bare keep-alives, and the 0xFA custom payloads HalpLibe
// injects during login (skipped by length, without interpreting the channel — a mod pack
// may send any number of them). A 0xFF is reported with the server's own kick reason.
func btaNextPacketID(r io.Reader) (byte, error) {
	for {
		var id [1]byte
		if _, err := io.ReadFull(r, id[:]); err != nil {
			return 0, fmt.Errorf("read packet id: %w", err)
		}
		switch id[0] {
		case btaPacketKeepAlive: // bare byte, nothing to skip
		case btaPacketCustomPayload:
			if _, err := btaReadString(r); err != nil { // channel
				return 0, fmt.Errorf("custom payload channel: %w", err)
			}
			var size int32
			if err := binary.Read(r, binary.BigEndian, &size); err != nil {
				return 0, fmt.Errorf("custom payload size: %w", err)
			}
			if _, err := io.CopyN(io.Discard, r, int64(size)); err != nil {
				return 0, fmt.Errorf("custom payload body (%d bytes): %w", size, err)
			}
		case btaPacketDisconnect:
			// The kick reason is protocol 14's UTF-16BE string16, NOT the UTF-8 form
			// every other string on this protocol uses — measured, and the reason this
			// borrows beta.go's reader. Read as UTF-8 it comes out as " O u t d a t e d",
			// NUL-interleaved: unreadable, and enough to make grep call a captured log
			// binary and skip it.
			reason, _ := betaReadString16(r)
			return 0, fmt.Errorf("server disconnected us: %s", btaPrintable(reason))
		default:
			return id[0], nil
		}
	}
}

// btaPrintable drops control characters from a server-supplied string before it reaches
// a log line. The kick reason is the only untrusted text this client prints, and the e2e
// harness greps the log it lands in: one stray NUL makes grep call the whole capture
// binary and skip it, costing the run its verdict.
func btaPrintable(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// btaLogin runs the whole offline-mode login: handshake, then login request. The
// server's handshake reply is exactly "-" in offline mode; anything else means the
// server wants authentication and the leg's server.properties is wrong.
func btaLogin(conn net.Conn, protocol int, username string) error {
	if _, err := conn.Write(btaHandshakePacket(username)); err != nil {
		return fmt.Errorf("write handshake: %w", err)
	}
	id, err := btaNextPacketID(conn)
	if err != nil {
		return fmt.Errorf("read handshake reply: %w", err)
	}
	if id != btaPacketHandshake {
		return fmt.Errorf("handshake reply: got packet 0x%02x, want 0x02", id)
	}
	hash, err := btaReadString(conn)
	if err != nil {
		return fmt.Errorf("read handshake hash: %w", err)
	}
	if hash != "-" {
		return fmt.Errorf("server is not in offline mode (handshake hash %q, want %q)", hash, "-")
	}

	publicKey, err := btaPublicKey()
	if err != nil {
		return err
	}
	if _, err := conn.Write(btaLoginPacket(protocol, username, publicKey)); err != nil {
		return fmt.Errorf("write login: %w", err)
	}
	id, err = btaNextPacketID(conn)
	if err != nil {
		return fmt.Errorf("read login reply: %w", err)
	}
	if id != btaPacketLogin {
		return fmt.Errorf("login reply: got packet 0x%02x, want 0x01", id)
	}
	var entityID int32
	if err := binary.Read(conn, binary.BigEndian, &entityID); err != nil {
		return fmt.Errorf("read entity id: %w", err)
	}
	if _, err := btaReadString(conn); err != nil { // username, always empty clientbound
		return fmt.Errorf("read login username: %w", err)
	}
	var uuid [16]byte
	if _, err := io.ReadFull(conn, uuid[:]); err != nil {
		return fmt.Errorf("read login uuid: %w", err)
	}
	if _, err := btaReadString(conn); err != nil { // the server's own public key
		return fmt.Errorf("read server public key: %w", err)
	}
	// worldSeed (int64), dimensionId, worldTypeId, packetDelay (int8) — all discarded,
	// so only their WIDTH matters: dimensionId and worldTypeId are bytes before 8.0 and
	// int32s from 8.0 on. Reading the wide form off a 7.3-line server would swallow six
	// bytes of whatever came next.
	tail := int64(8 + 1 + 1 + 1)
	if protocol >= btaProtocol80 {
		tail = 8 + 4 + 4 + 1
	}
	if _, err := io.CopyN(io.Discard, conn, tail); err != nil {
		return fmt.Errorf("read login tail: %w", err)
	}
	return nil
}

func btaSendChat(conn net.Conn, protocol int, message string) error {
	if _, err := conn.Write(btaMessagePacket(protocol, message)); err != nil {
		return fmt.Errorf("write message %q: %w", message, err)
	}
	return nil
}
