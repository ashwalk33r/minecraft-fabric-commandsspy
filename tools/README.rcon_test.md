# rcon_test.go

Unit tests for the RCON client in `tools/rcon.go` (`rconExec`, `readPacket`, `writePacket`).
RCON is the TCP protocol Minecraft servers expose for remote console commands.
No real Minecraft server is needed — everything runs against an in-process fake.

## The fake server

`fakeRCON(t, handler)` starts a TCP listener on `127.0.0.1` with a random port,
accepts exactly one connection, and hands it to your `handler` function.
The handler plays the server side of the protocol by hand, using the same
`readPacket`/`writePacket` helpers as the real client. The listener is closed
automatically via `t.Cleanup`.

## Tests

### TestRconAuthFailure

The fake server reads the client's auth packet and replies with request ID `-1`,
which is the RCON convention for "wrong password". The test asserts that
`rconExec` returns an error containing `authentication failed` instead of
proceeding.

### TestRconFragmentedResponse

RCON caps a single response packet at 4096 bytes of payload, so long command
output arrives split across multiple packets. The client detects the end of
output by sending a sentinel (dummy) request after the command; when the
server echoes the sentinel's ID back, the client knows all fragments arrived.

The fake server:

1. Accepts auth.
2. Receives the `help` command.
3. Sends the first 4096 bytes of a ~7 KB response.
4. Reads the client's sentinel request.
5. Sends the remaining bytes, then a vanilla-style `Unknown request` reply
   tagged with the sentinel's ID.

The test asserts `rconExec` reassembles the fragments into the exact
original ~7 KB string.

## How to run

```sh
cd tools
go test -run 'TestRcon' -v
```

Or run the whole tools suite with `go test ./...` from `tools/`.
Requires Go >= 1.23 (see the note in `tools/go.mod`).
