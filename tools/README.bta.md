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
| string form | `int16` UTF-16 **code units** + UTF-16BE | `int16` UTF-8 **bytes** + UTF-8 (except the kick reason) |
| protocol number | 14, one fork-wide | **one per release**, 29441..32769 |
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
| `0x01` Login | C→S | `byte 0x01` + `int protocol (per release)` + `string username` + `16 bytes uuid` + `string publicKey` + `long worldSeed (0)` + `dimensionId` + `worldTypeId` + `byte packetDelay (0)` — the two ids are **bytes before 8.0, int32s from 8.0** |
| `0x01` Login | S→C | `byte 0x01` + `int entityId` + `string (empty)` + `16 bytes uuid` + `string serverPublicKey` + `long worldSeed` + `int dimensionId` + `int worldTypeId` + `byte packetDelay` |
| `0x00` Keep Alive | both | a **bare single byte**, no payload |
| `0x03` Message | C→S | three shapes, see below |
| `0x88` AES Send Key | S→C | `byte 0x88` + `string` — the player's AES key, RSA-encrypted to the client's public key |
| `0xFA` Custom Payload | S→C | `byte 0xFA` + `string channel` + `int size` + `size` bytes |
| `0xFF` Disconnect | S→C | `byte 0xFF` + **`string16 reason`** — UTF-16BE, not this protocol's UTF-8 |

`worldSeed` / `dimensionId` / `worldTypeId` on the serverbound login are ignored by the
server, which replies with the real values. Two fields on that packet are **not** ignored,
and each cost a failed run to learn:

## The protocol number is per RELEASE

There is no single BTA protocol number. Each release compares the client's against its
own and kicks a mismatch during login, before anything can be sent:

| Version | Protocol | | Version | Protocol |
|---|---|---|---|---|
| 7.3 | 29472 | | 7.3_04 | 29444 |
| 7.3_01 | 29441 | | 8.0 | 32768 |
| 7.3_02 | 29442 | | 8.0.1 | 32769 |
| 7.3_03 | 29443 | | | |

(read out of each package's `PacketHandlerLogin` equality check). So the number is a
**parameter**, not a constant: `bot.go` routes the whole range to `runBtaBot` and passes
the caller's number straight into `btaLoginPacket`. The version→number table lives in
`scripts/e2e-run-one.sh` next to each package's hash and is deliberately not duplicated
here; `bta.go` only knows where the range starts (`btaMinProtocolVersion`), which is all
the dispatch needs — modern protocol numbers are three digits, BTA's are five.

## The message packet has three shapes

This is the packet that changed most, and a wrong shape is **not an error**: the server
drops the connection the instant it arrives (`lost connection: disconnect.genericReason`)
and the command never reaches the command manager. All three were read off the seven
server jars with `javap`, not guessed:

| Versions | Class | Layout |
|---|---|---|
| 7.3 | `PacketChat` | `type`, `string UTF-8`, `encrypted` |
| 7.3_01 … 7.3_04 | `PacketChat` | `type`, **`string UTF-16BE`**, `encrypted` |
| 8.0, 8.0.1 | `PacketMessage` | `type`, `encrypted`, `string UTF-8` |

Two things worth spelling out. **7.3 sorts above the releases that follow it** (29472 >
29444), so the test for it is an equality and only the 8.0 test is a comparison — an
ordered predicate over all three would put 7.3 in the wrong era. And the UTF-16BE form is
protocol 14's `string16` *exactly* — `writeShort(String.length())` then the UTF-16BE bytes
— so it borrows `beta.go`'s encoder instead of growing a second codec here.

The 8.0 line also reads a format `short` between the flag and the string, but **only when
the type byte's high bit is set**. `TYPE_CHAT` never sets it, so the bot never writes one.

The login tail moved on the same boundary: `dimensionId` and `worldTypeId` are bytes
before 8.0 and int32s from 8.0 on. The wide form sent to a 7.3-line server leaves six
stray zero bytes, which that server reads as six bare keep-alives — harmless by luck, not
by design, and the luck ends the moment a non-zero value is sent.

## The kick reason is UTF-16BE

Every string on this protocol is `int16` byte count + UTF-8 — **except** the `0xFF`
disconnect reason, which is protocol 14's UTF-16BE `string16`. That is why
`btaNextPacketID` reaches over to `beta.go`'s `betaReadString16` for that one field.
Read with the wrong decoder, `Outdated server!` prints as:

```
server disconnected us:  O u t d a t e d
```

— half the string, NUL-interleaved. Unreadable, and the NULs are enough to make `grep`
call a captured e2e log binary and skip it, which costs the run its verdict line. The
decode is the fix; `btaPrintable` then strips any control byte that survives, because
this is the only server-supplied text the bot prints.

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
  function per serverbound packet, so `bta_test.go` can assert them byte for byte. The
  last two take the protocol number, because their layout depends on it.
- `btaPrintable(s)` — strips control bytes from the one untrusted string this client
  prints.
- `btaOfflineUUID(username)` / `btaPublicKey()` — the two load-bearing login fields.
- `btaNextPacketID(r)` — the next packet id, past the noise.
- `btaLogin(conn, username)` — the whole offline-mode login: handshake, hash check, login
  request, login reply. A hash other than `-` means the server is not in offline mode and
  the leg's `server.properties` is wrong.
- `btaSendChat(conn, protocol, message)` — one Message packet.

## Place in the package

`bot.go` dispatches here when `--protocol` names any number in the BTA range, next to its
protocol-14 branch. That flag exists because this protocol **cannot be negotiated**: like Beta 1.7.3, BTA
predates the modern status handshake, so `ping()` in `mc.go` can never learn the version.
Every other loader still negotiates by ping.
