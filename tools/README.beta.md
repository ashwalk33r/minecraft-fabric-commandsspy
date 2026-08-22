# beta.go

The protocol-14 client — Minecraft **Beta 1.7.3**, the version Babric targets. It is a
second, structurally different client from the one `mc.go`/`table.go` drive, not a table
row: protocol 14 is pre-Netty, so there is **no packet-length prefix, no compression, no
encryption in offline mode, and no login state machine**. A packet is its id byte
followed by big-endian fields. That is why it is ~130 lines rather than a port.

## Provenance: measured, not cited

The protocol *number* is citable — protocol 14 is Beta 1.7.3, per
<https://minecraft.wiki/w/Protocol_version>. The packet *layouts* are not: the live
minecraft.wiki protocol page documents only the modern post-Netty protocol, the wiki.vg
mirror documents protocol 340, and wiki.vg's own pre-Netty archive is gone. No citable
protocol-14 packet page could be retrieved.

So every layout here was **verified empirically against a booted b1.7.3 server**. Three
observations, all reproduced by this client:

- the handshake reply in offline mode is exactly `"-"`;
- the login reply carries `eid` / `seed` / `dim` — observed `login ok: eid=1211
  seed=2726119088588108853 dim=0` against the server's own
  `SpyBot [/127.0.0.1:41591] logged in with entity id 1211`;
- `0x03` + `string16("/me waves")` produced `* SpyBot2 waves`, which is what proves the
  packet reaches the player command seam this mod hooks.

See `docs/superpowers/specs/2026-08-22-babric-loader-support-spec.md`.

## string16 — the one error-prone piece

Protocol 14 has exactly one string form: a big-endian `int16` giving the number of UTF-16
**code units**, then that many code units as UTF-16BE (2 × n bytes). The count is
**neither a byte count nor a rune count** — a non-BMP rune is a surrogate pair and counts
as two. `betaString16`/`betaReadString16` implement it; `beta_test.go` pins all three
cases apart.

## Packet layouts

| Packet | Dir | Layout |
|---|---|---|
| `0x02` Handshake | C→S | `byte 0x02` + `string16 username` |
| `0x02` Handshake | S→C | `byte 0x02` + `string16 hash` — offline mode returns exactly `-` |
| `0x01` Login Request | C→S | `byte 0x01` + `int protocol (=14)` + `string16 username` + `long mapSeed (0)` + `byte dimension (0)` |
| `0x01` Login Response | S→C | `byte 0x01` + `int entityId` + `string16 (empty)` + `long mapSeed` + `byte dimension` |
| `0x00` Keep Alive | both | a **bare single byte**, no payload |
| `0x03` Chat Message | C→S | `byte 0x03` + `string16 message` (a leading `/` makes it a command) |
| `0xFF` Disconnect | S→C | `byte 0xFF` + `string16 reason` |

Keep Alive being a bare byte is the assumption most likely to be imported by mistake:
every later protocol gives it an int body. `TestBetaKeepAliveIsABareByte` exists for
exactly that reason.

`mapSeed 0` / `dimension 0` on the serverbound login are ignored by the server, which
replies with the real values.

## Read-loop gotcha

Immediately after login the server floods entity and chunk packets this client has no
parser for, so **the stream cannot be read naively looking for a chat echo**. `runBetaBot`
(in `bot.go`) drains the socket to `io.Discard` instead — unread, the socket buffer fills
and stalls the server's writer for that connection. Nothing is asserted from the stream:
**the verdict comes from the server log**, which is how every other loader's e2e leg
already works.

## Functions

- `betaString16(s)` / `betaReadString16(r)` — the encoding above, both directions.
- `betaHandshakePacket`, `betaLoginPacket`, `betaChatPacket`, `betaKeepAlivePacket` —
  one function per serverbound packet, so `beta_test.go` can assert them byte for byte.
- `betaLogin(conn, username)` — the whole offline-mode login: handshake, hash check,
  login request, login reply. A hash other than `-` means the server is not in offline
  mode and the leg's `server.properties` is wrong; a `0xFF` in either position is
  reported with the server's own kick reason.
- `betaSendChat(conn, message)` — one chat packet.

## Place in the package

`bot.go` dispatches here when `--protocol 14` is given. That flag exists because
protocol 14 **cannot be negotiated**: a b1.7.3 server answers the modern status ping with
`0xFF` + `"Protocol error"` (the status handshake postdates it), so `ping()` in `mc.go`
can never learn the version. Every other loader still negotiates by ping.
