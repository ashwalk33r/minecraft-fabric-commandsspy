package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"unicode/utf16"
)

// Protocol 14 — Minecraft Beta 1.7.3, the version Babric targets. This is a SECOND,
// structurally different client from the one table.go drives: pre-Netty, so there is no
// VarInt length prefix, no compression, no encryption in offline mode, and no login
// state machine. A packet is its id byte followed by big-endian fields.
//
// Not cited from a spec page: no citable protocol-14 documentation could be retrieved
// (the live wiki documents the modern protocol, and the wiki.vg mirror documents 340).
// Every layout here was verified empirically against a booted b1.7.3 server, which
// logged the bot in and processed its command. See the spec.
const betaProtocolVersion = 14

// Packet ids, serverbound unless noted.
const (
	betaPacketKeepAlive  = 0x00 // both directions, BARE byte, no payload
	betaPacketLogin      = 0x01
	betaPacketHandshake  = 0x02
	betaPacketChat       = 0x03
	betaPacketDisconnect = 0xFF // clientbound
)

// betaString16 encodes protocol 14's only string form: an int16 count of UTF-16 CODE
// UNITS, then those units as UTF-16BE. The count is neither a byte count nor a rune
// count — a non-BMP rune is a surrogate pair and counts as two.
func betaString16(s string) []byte {
	units := utf16.Encode([]rune(s))
	out := make([]byte, 2+2*len(units))
	binary.BigEndian.PutUint16(out, uint16(len(units)))
	for i, u := range units {
		binary.BigEndian.PutUint16(out[2+2*i:], u)
	}
	return out
}

func betaReadString16(r io.Reader) (string, error) {
	var count uint16
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return "", fmt.Errorf("string16 length: %w", err)
	}
	units := make([]uint16, count)
	if err := binary.Read(r, binary.BigEndian, &units); err != nil {
		return "", fmt.Errorf("string16 body (%d units): %w", count, err)
	}
	return string(utf16.Decode(units)), nil
}

func betaHandshakePacket(username string) []byte {
	return append([]byte{betaPacketHandshake}, betaString16(username)...)
}

// betaLoginPacket sends mapSeed 0 and dimension 0: the server ignores both on a
// serverbound login and replies with the real values.
func betaLoginPacket(username string) []byte {
	out := []byte{betaPacketLogin}
	out = binary.BigEndian.AppendUint32(out, uint32(betaProtocolVersion))
	out = append(out, betaString16(username)...)
	out = binary.BigEndian.AppendUint64(out, 0) // mapSeed
	out = append(out, 0x00)                     // dimension
	return out
}

func betaChatPacket(message string) []byte {
	return append([]byte{betaPacketChat}, betaString16(message)...)
}

func betaKeepAlivePacket() []byte {
	return []byte{betaPacketKeepAlive}
}

// betaLogin runs the whole offline-mode login: handshake, then login request. The
// server's handshake reply is exactly "-" in offline mode; anything else means the
// server wants authentication and the leg's server.properties is wrong.
func betaLogin(conn net.Conn, username string) error {
	if _, err := conn.Write(betaHandshakePacket(username)); err != nil {
		return fmt.Errorf("write handshake: %w", err)
	}

	id := make([]byte, 1)
	if _, err := io.ReadFull(conn, id); err != nil {
		return fmt.Errorf("read handshake reply id: %w", err)
	}
	if id[0] == betaPacketDisconnect {
		reason, _ := betaReadString16(conn)
		return fmt.Errorf("server refused at handshake: %s", reason)
	}
	if id[0] != betaPacketHandshake {
		return fmt.Errorf("handshake reply: got packet 0x%02x, want 0x02", id[0])
	}
	hash, err := betaReadString16(conn)
	if err != nil {
		return fmt.Errorf("read handshake hash: %w", err)
	}
	if hash != "-" {
		return fmt.Errorf("server is not in offline mode (handshake hash %q, want %q)", hash, "-")
	}

	if _, err := conn.Write(betaLoginPacket(username)); err != nil {
		return fmt.Errorf("write login: %w", err)
	}
	if _, err := io.ReadFull(conn, id); err != nil {
		return fmt.Errorf("read login reply id: %w", err)
	}
	if id[0] == betaPacketDisconnect {
		reason, _ := betaReadString16(conn)
		return fmt.Errorf("server refused at login: %s", reason)
	}
	if id[0] != betaPacketLogin {
		return fmt.Errorf("login reply: got packet 0x%02x, want 0x01", id[0])
	}
	var entityID int32
	if err := binary.Read(conn, binary.BigEndian, &entityID); err != nil {
		return fmt.Errorf("read entity id: %w", err)
	}
	if _, err := betaReadString16(conn); err != nil { // always empty on b1.7.3
		return fmt.Errorf("read login string: %w", err)
	}
	var tail struct {
		MapSeed   int64
		Dimension int8
	}
	if err := binary.Read(conn, binary.BigEndian, &tail); err != nil {
		return fmt.Errorf("read login tail: %w", err)
	}
	return nil
}

func betaSendChat(conn net.Conn, message string) error {
	if _, err := conn.Write(betaChatPacket(message)); err != nil {
		return fmt.Errorf("write chat %q: %w", message, err)
	}
	return nil
}
