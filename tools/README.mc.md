# mc.go — minimal Minecraft protocol client

`mc.go` is the wire layer of the e2e bot. It speaks just enough offline-mode
Minecraft Java protocol to: ping a server, log in, send one command, and
answer keepalives. No encryption, no Mojang auth, no chat signing.
Compression is handled if the server turns it on.

Per-version facts (packet ids, eras) come from `row` in `table.go`; this file
only knows how to encode and drive the wire.

## Wire primitives

- `buf` — a `bytes.Buffer` with protocol writers: `varint`, `str`
  (varint-length-prefixed string), `u16`, `i64`, `boolean`.
- `readVarint` / `readString` — the matching readers, with bounds checks.
- `offlineUUID(name)` — version-3 MD5 UUID of `"OfflinePlayer:<name>"`,
  exactly what an offline-mode server computes.

## Framing: `conn`

Every packet is `[length varint][id varint][body]`. With compression on
(threshold >= 0), the frame becomes `[length][uncompressed-size][zlib data]`,
where size 0 means "not compressed".

- `dial` — TCP connect with one absolute deadline covering the whole session.
- `sendPacket` / `send` — frame (and compress if needed) and write.
- `recv` — read one frame, decompress if needed, return `(id, body)`.
- `handshake` — packet 0x00: protocol, host, port, next state (1=status, 2=login).

## Status ping

`ping(host, port, deadline)` sends a status handshake with protocol `-1`
("unknown") and returns the server's advertised version name and protocol
number from its JSON status reply. This is how the bot discovers which `row`
to use — the version is negotiated, never configured by name.

## Login and play: `client`

- `join` — dial, handshake, run login; returns a `client` in the play state.
- `loginStart` — the login payload varies by era (UUID optional/required,
  signature fields); the `switch` on protocol number mirrors
  `docs/protocol-table.md`.
- `runLogin` — the login state machine. Handles disconnect, set-compression,
  and login-success; on 1.20.2+ it also walks the configuration state,
  answering keepalives, ping, `select_known_packs` ("none"), and the 26.x
  code-of-conduct prompt, until the server says finish.
- Encryption request = hard error: the server is in online mode.

## Sending the command

- `commandPacket(row, cmd, ts)` — pure function serializing the command per
  chat era: plain `/cmd` chat packet (<=1.19), signed-era layouts with empty
  signatures (1.19.x), or just the bare string (1.20.5+). Pure so tests can
  assert exact bytes.
- `sendCommand` — wraps it with the current timestamp.
- `pump` — after the command, answers play-state keepalives until stopped,
  and logs any `commands.list` text in incoming packets as proof the
  command executed (substring sniff, not a real chat decode).
