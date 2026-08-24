# bta_test.go

Unit tests for the protocol-32769 client in `tools/bta.go`. No Minecraft server is
needed: BTA has no length prefix and no state machine, so every serverbound packet is a
pure function of its arguments and can be asserted byte for byte.

That byte-for-byte assertion is the point. With no length prefix, **a packet IS its id
byte followed by its fields** — an off-by-one desynchronizes the rest of the stream
instead of failing cleanly at a frame boundary, so a wrong layout shows up as an
unrelated hang much later.

## Tests

### TestBtaString

BTA's string form is an `int16` count of UTF-8 **bytes** followed by those bytes.
`beta.go`'s `string16` counts UTF-16 **code units** — the two agree on ASCII and disagree
everywhere else, so importing the wrong one from the file next door is the easiest
mistake here. The five cases pull the readings apart:

- `"AB"` — the ASCII case both encodings agree on.
- `""` — the empty string is still two bytes of length.
- `"-"` — the literal the offline-mode handshake reply is compared against.
- `"é"` (U+00E9) — one code unit but **two** bytes: count 2, where `beta.go` writes 1.
- `"\U0001F600"` — **four** bytes, where `beta.go` writes two code units.

### TestBtaStringRoundTrip

`btaReadString(btaString(s)) == s` for the same five strings. Round-tripping is what the
handshake reply, the login reply's strings and any kick reason actually exercise: they
are read back with the same encoding they are written with.

### TestBtaPacketLayouts

The three packets the bot writes, spelled out. The login subtest writes its expectation
field by field — `0x01`, int32 32769, username, 16-byte uuid, key string, int64 0, int32
0, int32 0, int8 0 — rather than calling the production encoder, so it fails if a field
ORDER or a field WIDTH changes, not just if a value does. It pins the position of the two
fields the server actually uses: the UUID it keys the player by, and the RSA public key
it encrypts that player's AES key to.

The message subtest guards a three-byte header whose failures are all silent — the packet
is accepted and the command simply never runs. The type byte selects `TYPE_CHAT`, the
path that reaches `handleMessage`; the encrypted flag must be **false**, or the server
AES-decrypts a plaintext line into garbage; and the slash is carried **in the string**,
which is what makes the line a command rather than chat.

`TestBtaPacketLayouts` also pins the login tail width: `dimensionId` and `worldTypeId` are
bytes before 8.0 and int32s from 8.0 on, a six-byte difference.

### TestBtaMessagePacketPerEra

The message packet has three layouts across the seven releases and a wrong one is silent
on the wire — the server just drops the connection — so all three are asserted byte for
byte: 7.3 (`type`, UTF-8 string, `encrypted`), 7.3_01..7.3_04 (same order, UTF-16BE
string), 8.0+ (`type`, `encrypted`, UTF-8 string).

The last assertion in the test is the one that matters most: 7.3 and 7.3_04 must NOT
encode identically. 7.3's protocol number is 29472, which sorts ABOVE 7.3_04's 29444, so
anyone "tidying" the encoder's equality test into an ordered comparison silently puts 7.3
in the wrong era. This fails when they do.

### TestBtaPrintableStripsControlBytes

The kick reason is the only untrusted text this client prints, and the harness greps the
log it lands in — one control byte makes grep treat the whole capture as binary and skip
it, costing the run its verdict.

### TestBtaKeepAliveIsABareByte

Keep Alive is a bare `0x00` with no payload in both directions, inherited from Beta 1.7.3.
Every post-Netty protocol gives it an int body, so this is the assumption most likely to
be imported by mistake from `mc.go` — hence a test for a one-byte function.

### TestBtaNextPacketIDSkipsNoise

The first packet a BTA server sends is a `0xFA` custom payload, not the handshake reply;
skipping it by its declared length is the only way to stay in frame, and getting that
length arithmetic wrong desynchronizes everything after it. The first subtest therefore
feeds a stream of `0xFA` + keep-alive + `0xFA` + the real handshake reply and asserts
**both** the returned id and that the `"-"` after it still reads back correctly — the
second half is what proves the skip landed on a packet boundary.

The second subtest pins that a `0xFF` surfaces the server's own kick reason — decoded.
That string is the only diagnosis an e2e failure gets, and it is the one field on this
protocol encoded as UTF-16BE rather than UTF-8, so it asserts both that the text is there
and that no NUL survived: NULs make `grep` treat a captured log as binary and skip it.

### TestBtaPublicKeyFitsTheServerCap

The login packet's key must be a real 2048-bit X.509/SPKI key or the server's login
handler throws and the connection dies. Base64 of a 2048-bit SPKI is exactly 392 chars,
which is also the server's `MAX_AES_KEY_SIZE` — a length check catches both a wrong key
size and a wrong encoding.

### TestBtaProtocolRangeIsAboveEveryModernOne

Each BTA release has its own protocol number and kicks a client offering a different one,
so `bot.go` dispatches on the whole range rather than on 8.0.1's number alone. This walks
the seven declared numbers against the dispatch floor, and walks protocol 14 and every
row of `table.go` the other way — nothing that should reach another client may fall into
the BTA range.

### TestBtaOfflineUUIDIsPerPlayer

The one test protecting a failure mode that produces **no error at all**. The server keys
players by the login packet's UUID, so if both e2e players sent the same one the second
login would silently evict the first — the cross-check player kills the command player,
and the leg fails with an empty log rather than a message. This asserts the two players
differ, that the derivation is deterministic, and that it is a version-3 RFC 4122 UUID.

## Run them

```sh
cd tools && go test ./... -run TestBta -v
```
