# bot_test.go

Unit tests for the fake-client ("bot") protocol code in `mc.go`. The bot pretends to
be a Minecraft player during e2e runs. These tests pin the exact bytes it sends, so a
protocol regression fails here instead of deep inside a live server run.

## What it tests

Three low-level building blocks, no network involved:

1. **VarInt encoding.** Minecraft's variable-length integer format. Every packet uses it.
2. **Offline-mode UUIDs.** The UUID a server assigns a player when auth is off.
3. **Command packet bytes.** The wire bytes for sending a `/list` command, on both
   sides of the 1.19 protocol split.

## Test functions

- `TestVarIntRoundTrip` — encodes a set of edge-case ints (0, 127, 128, max int32, -1)
  with `buf.varint`, decodes with `readVarint`, and expects the same value back.
- `TestOfflineUUID` — checks `offlineUUID("e2e_player1")` is a version-3 UUID with
  correct variant bits, and matches a hardcoded known-good hex value
  (MD5 of `OfflinePlayer:e2e_player1`).
- `TestCommandBytesModern1192` — protocol 760 (Minecraft 1.19.2). Commands use the
  `chat_command` packet: bare `"list"`, **no slash**, plus timestamp, salt, and empty
  signature fields. Asserts the exact byte sequence.
- `TestCommandBytesLegacy1182` — protocol 758 (Minecraft 1.18.2). Commands are plain
  chat packets: `"/list"`, **slash included**, nothing else. Also exact bytes.

The last two pin the era split ("list" vs "/list") that the e2e harness's log oracle
asserts on. If someone changes `commandPacket`, these fail before any server does.

## Fakes and mocks

None. Everything under test is pure byte manipulation. The only test double is
`bytes.NewReader` standing in for a network connection in the varint round trip.

## How to run

```sh
cd tools && go test -run 'TestVarInt|TestOfflineUUID|TestCommandBytes' -v
```

Or the whole package via the repo's standard gate:

```sh
make go-test    # cd tools && go test -race -count=1 -cover ./...
```

No server, Docker, or network needed; runs in under a second.
