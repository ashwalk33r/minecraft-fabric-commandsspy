# beta_test.go

Unit tests for the protocol-14 client in `tools/beta.go`. No Minecraft server is needed:
protocol 14 has no length prefix and no state machine, so every serverbound packet is a
pure function of its arguments and can be asserted byte for byte.

That byte-for-byte assertion is the point. With no length prefix, **a packet IS its id
byte followed by its fields** — an off-by-one desynchronizes the rest of the stream
instead of failing cleanly at a frame boundary, so a wrong layout shows up as an
unrelated hang much later.

## Tests

### TestBetaString16

The single most error-prone piece. The length field counts UTF-16 **code units**, not
bytes and not runes. The five cases exist to separate those three readings, which agree
on ASCII and disagree everywhere else:

- `"AB"` — the ASCII case all three readings agree on.
- `""` — the empty string is still two bytes of length.
- `"-"` — the literal the offline-mode handshake reply is compared against.
- `"é"` (U+00E9) — one code unit, **two** bytes: fails a byte-count implementation.
- `"\U0001F600"` — a surrogate pair: **two** code units, four bytes. Fails a rune-count
  implementation.

### TestBetaString16RoundTrip

`betaReadString16(betaString16(s)) == s` for the same five strings. Round-tripping is
what the handshake reply and any kick reason actually exercise: they are read back with
the same encoding they are written with.

### TestBetaPacketLayouts

The three packets the bot writes, spelled out. The login subtest writes its expectation
field by field (`0x01`, int32 14, username, int64 0, int8 0) rather than calling the
production encoder, so it fails if the field ORDER or a field WIDTH changes, not just if
a value does.

The chat subtest's name says what it is really protecting: the slash is carried **in the
string**, and it is what makes the line a command rather than chat. Dropping it there is
silent — the server accepts the packet and just says something in chat.

### TestBetaKeepAliveIsABareByte

Keep Alive on protocol 14 is a bare `0x00` with no payload in both directions. Every
later protocol gives it an int body, so this is the assumption most likely to be imported
by mistake from `mc.go` — hence a test for a one-byte function.

## Run them

```sh
cd tools && go test ./... -run TestBeta -v
```
