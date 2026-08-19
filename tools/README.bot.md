# bot.go

The `bot` subcommand of the e2e tool. It connects two fake Minecraft players
to a running server, has one of them send a command, and exits. The mod under
test (CommandsSpy) should log that command; a later phase of the e2e run
checks the log. Exit code 0 means every phase succeeded.

## What it does, step by step

`runBot(args)` is the whole file. It:

1. Parses flags (see below).
2. Computes one absolute deadline (`now + timeout`). Every socket gets this
   deadline, so no phase can hang past it.
3. **Status-ping phase.** Calls `ping()` (from `mc.go`) to ask the server its
   name and protocol number, then `rowFor()` (from `table.go`) to find the
   matching protocol-table row. Unsupported protocol = error out.
4. **Login phase.** Joins two players, `e2e_player1` and `e2e_player2`, via
   `join()`. Each gets a background `pump` goroutine that keeps the
   connection alive and reports errors on a channel.
5. **Settle.** Waits `-settle` (capped by the deadline) so the joins land.
6. **Command phase.** `e2e_player1` sends `/<command>` (default `/list`).
   `e2e_player2` sends nothing on purpose — it is the attribution
   cross-check: the mod must log player1 as the sender, not player2.
7. **Settle again**, then closes the stop channel; deferred `Close()` calls
   disconnect both players cleanly.

Any error is wrapped with the protocol row, protocol number, and phase name,
so a failure line tells you exactly where it died.

## Flags

| Flag | Default | Meaning |
|------|---------|---------|
| `-host` | `127.0.0.1` | server host |
| `-port` | `25565` | server port |
| `-command` | `list` | command to send, without the slash |
| `-timeout` | `150s` | hard deadline for the entire run |
| `-settle` | `3s` | pause after joins and after the command |

## Place in the tools/ package

`main.go` dispatches subcommands: `bot` → `runBot` (this file), plus `rcon`
and `gen-matrix`. bot.go owns only the orchestration; the Minecraft protocol
work (`ping`, `join`, `client`, `pump`, `sendCommand`) lives in `mc.go`, and
the supported-protocol table (`row`, `rowFor`) lives in `table.go`.

## Run it

```sh
go run ./tools bot -host 127.0.0.1 -port 25565 -command list
```
