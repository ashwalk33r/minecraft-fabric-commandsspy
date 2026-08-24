# bta.go

The protocol-32769 client — **"Better than Adventure!"**, the fork this repository
supports as its sixth loader. It is a *third* client next to `mc.go`/`table.go`'s modern
one and `beta.go`'s protocol-14 one, not a table row and not a parameter on `beta.go`.

BTA inherits Beta 1.7.3's pre-Netty framing — **no packet-length prefix, no compression,
no transport encryption, no login state machine**; a packet is its id byte followed by
big-endian fields — and then changes everything above it. That inheritance is why it
cannot be a table row; the changes are why it is not a flag on `beta.go`:

| | protocol 14 (`beta.go`) | protocol 32769 (`bta.go`) |
|---|---|---|
| string form | `int16` UTF-16 **code units** + UTF-16BE | `int16` UTF-8 **bytes** + UTF-8 |
| login fields | protocol, name, seed, dimension | protocol, name, **uuid**, **RSA public key**, seed, dimension, world type, packet delay |
| chat packet | `0x03` + string | `0x03` + **type byte** + **encrypted flag** + string |
| login noise | none | unsolicited `0xFA` custom payloads |

Two straight-line clients are cheaper to read than one abstraction over both, so
`beta.go` is untouched.

## Provenance: measured, not cited

BTA publishes no protocol documentation, so nothing here is cited: every layout was
**verified empirically against a booted BTA 8.0.1 server** with this mod installed, which
logged the bot in and processed its command. The wire facts, and the failures that
produced them, are written up in `docs/bta-toolchain-spike.md` §7 (tracked, ships in the
same PR). The proof line the live run produces is:

```
[CommandsSpy/INFO]: [CommandsSpy] [Player: e2e_player1] me
```

with no second line for `e2e_player2` — the attribution cross-check.

## Packet layouts

| Packet | Dir | Layout |
|---|---|---|
| `0x02` Handshake | C→S | `byte 0x02` + `string username` |
| `0x02` Handshake | S→C | `byte 0x02` + `string hash` — offline mode returns exactly `-` |
| `0x01` Login | C→S | `byte 0x01` + `int protocol (=32769)` + `string username` + `16 bytes uuid` + `string publicKey` + `long worldSeed (0)` + `int dimensionId (0)` + `int worldTypeId (0)` + `byte packetDelay (0)` |
| `0x01` Login | S→C | `byte 0x01` + `int entityId` + `string (empty)` + `16 bytes uuid` + `string serverPublicKey` + `long worldSeed` + `int dimensionId` + `int worldTypeId` + `byte packetDelay` |
| `0x00` Keep Alive | both | a **bare single byte**, no payload |
| `0x03` Message | C→S | `byte 0x03` + `byte type (0 = chat)` + `byte encrypted (0)` + `string message` (a leading `/` makes it a command) |
| `0x88` AES Send Key | S→C | `byte 0x88` + `string` — the player's AES key, RSA-encrypted to the client's public key |
| `0xFA` Custom Payload | S→C | `byte 0xFA` + `string channel` + `int size` + `size` bytes |
| `0xFF` Disconnect | S→C | `byte 0xFF` + `string reason` |

`worldSeed` / `dimensionId` / `worldTypeId` on the serverbound login are ignored by the
server, which replies with the real values. Two fields on that packet are **not** ignored,
and each cost a failed run to learn:

## The two fields that are load-bearing

**The RSA public key must be real.** The server generates a per-player AES key and
RSA-encrypts it to whatever the login packet supplied; a junk string throws inside the
login handler and the connection dies with nothing useful logged. `btaPublicKey` generates
a fresh 2048-bit key and sends its base64 X.509/SPKI form — exactly 392 characters, which
is the server's own `MAX_AES_KEY_SIZE` cap. **The private half is discarded**: only the
`message` field of a Message packet is ever encrypted, never the stream, and only
server→client chat is required to be — so a bot that sends `encrypted = false` and never
reads a reply needs no cipher at all. (The spike prototype did decrypt, to prove the
command had run; the harness reads the *server log* instead, exactly like every other
loader, so that code is deliberately not ported.)

**The UUID must differ per player.** The server keys players by the UUID the login packet
carries, not by name. With a shared UUID — a zero one, say — the second login **silently
evicts the first**: no kick reason on the wire, no line in the server log, and the command
the first player then sends is simply dropped. That failure looks exactly like a broken
Message packet, which is what makes it worth a paragraph. `btaOfflineUUID` derives a
version-3 (MD5) UUID over `"OfflinePlayer:<name>"`, the same derivation vanilla uses, so
`e2e_player1` and `e2e_player2` coexist and the attribution cross-check means something.

## The 0xFA problem

The first packet a BTA server sends is not the handshake reply — it is a `0xFA` custom
payload, a HalpLibe artifact rather than protocol. Pointing `beta.go` at a BTA server
fails on exactly that:

```
bot: beta (protocol 14): login phase, e2e_player1: handshake reply: got packet 0xfa, want 0x02
```

`btaNextPacketID` is the answer: it skips any `0xFA` **by its declared length** without
interpreting the channel (a mod pack may send any number of them), swallows bare
keep-alives, surfaces a `0xFF` with the server's own kick reason, and returns anything
else. Every read in `btaLogin` goes through it.

## Read-loop gotcha

Same as protocol 14's: after login the server floods chunk, registry and AES-key packets
this client has no parser for, so the stream **cannot be read naively looking for a chat
echo**. `runBtaBot` (in `bot.go`) drains the socket to `io.Discard` — unread, the socket
buffer fills and stalls the server's writer for that connection. Nothing is asserted from
the stream: **the verdict comes from the server log.**

## Functions

- `btaString(s)` / `btaReadString(r)` — the string form above, both directions.
- `btaHandshakePacket`, `btaLoginPacket`, `btaMessagePacket`, `btaKeepAlivePacket` — one
  function per serverbound packet, so `bta_test.go` can assert them byte for byte.
- `btaOfflineUUID(username)` / `btaPublicKey()` — the two load-bearing login fields.
- `btaNextPacketID(r)` — the next packet id, past the noise.
- `btaLogin(conn, username)` — the whole offline-mode login: handshake, hash check, login
  request, login reply. A hash other than `-` means the server is not in offline mode and
  the leg's `server.properties` is wrong.
- `btaSendChat(conn, message)` — one Message packet.

## Place in the package

`bot.go` dispatches here when `--protocol 32769` is given, next to its protocol-14 branch.
That flag exists because this protocol **cannot be negotiated**: like Beta 1.7.3, BTA
predates the modern status handshake, so `ping()` in `mc.go` can never learn the version.
Every other loader still negotiates by ping.
