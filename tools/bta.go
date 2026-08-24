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
)

// Protocol 32769 — "Better than Adventure!", a THIRD client next to table.go's modern
// one and beta.go's protocol-14 one. BTA forks Beta 1.7.3's pre-Netty framing (a packet
// is its id byte followed by big-endian fields: no length prefix, no compression, no
// transport encryption) and then changes everything above it: strings are UTF-8 with an
// int16 BYTE count instead of UTF-16BE code units, login carries a UUID and an RSA
// public key, and chat is a message packet with a type byte and an encrypted flag.
//
// Not cited from a spec page: BTA publishes no protocol documentation. Every layout here
// was verified empirically against a booted BTA 8.0.1 server, which logged the bot in and
// processed its command. See docs/bta-toolchain-spike.md §7.
const btaProtocolVersion = 32769

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
func btaLoginPacket(username, publicKey string) []byte {
	out := []byte{btaPacketLogin}
	out = binary.BigEndian.AppendUint32(out, uint32(btaProtocolVersion))
	out = append(out, btaString(username)...)
	uuid := btaOfflineUUID(username)
	out = append(out, uuid[:]...)
	out = append(out, btaString(publicKey)...)
	out = binary.BigEndian.AppendUint64(out, 0) // worldSeed
	out = binary.BigEndian.AppendUint32(out, 0) // dimensionId
	out = binary.BigEndian.AppendUint32(out, 0) // worldTypeId
	out = append(out, 0x00)                     // packetDelay
	return out
}

// btaMessagePacket carries chat and, with a leading slash, commands. The encrypted flag
// is false, which tells the server to take the string verbatim rather than AES-decrypt
// it. Only the message FIELD is ever encrypted on this protocol, never the stream, and
// only client→server chat may opt out — which is why this bot never needs a cipher.
func btaMessagePacket(message string) []byte {
	out := []byte{btaPacketMessage, btaMessageTypeChat, 0x00 /* encrypted = false */}
	return append(out, btaString(message)...)
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
			reason, _ := btaReadString(r)
			return 0, fmt.Errorf("server disconnected us: %s", reason)
		default:
			return id[0], nil
		}
	}
}

// btaLogin runs the whole offline-mode login: handshake, then login request. The
// server's handshake reply is exactly "-" in offline mode; anything else means the
// server wants authentication and the leg's server.properties is wrong.
func btaLogin(conn net.Conn, username string) error {
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
	if _, err := conn.Write(btaLoginPacket(username, publicKey)); err != nil {
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
	var tail struct {
		WorldSeed   int64
		DimensionID int32
		WorldTypeID int32
		PacketDelay int8
	}
	if err := binary.Read(conn, binary.BigEndian, &tail); err != nil {
		return fmt.Errorf("read login tail: %w", err)
	}
	return nil
}

func btaSendChat(conn net.Conn, message string) error {
	if _, err := conn.Write(btaMessagePacket(message)); err != nil {
		return fmt.Errorf("write message %q: %w", message, err)
	}
	return nil
}
