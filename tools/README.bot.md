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
   Skipped entirely when `-protocol` is given (see below).
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
| `-protocol` | `0` | skip the status ping and assume this protocol; `14` = Beta 1.7.3, `29441`-`32769` = BTA (per release) |

`-protocol` exists for the two pre-Netty loaders. A Beta 1.7.3 server answers
the modern status ping with `0xFF` + `"Protocol error"` — the status handshake
postdates it — so protocol 14 cannot be negotiated and must be declared; BTA
forks that same framing and is declared for the same reason. Each BTA release
has its own protocol number and kicks a client that offers a different one, so
the whole range dispatches to `runBtaBot` and the number is passed through to
the login packet rather than assumed — the version→number table lives in
`scripts/e2e-run-one.sh`. Every other loader
still negotiates by ping, which is why the flag defaults to 0.

Each declared protocol short-circuits `runBot` into its own client, before the
status-ping phase:

| `-protocol` | Function | Wire code |
|---|---|---|
| `14` | `runBetaBot` | `beta.go` |
| `29441`-`32769` | `runBtaBot` | `bta.go` |

Both twins run the same phases as `runBot` — two players, one command, the same
attribution cross-check — but over framing that shares nothing with `mc.go`: no
VarInt frames, no compression, no login state machine. They are separate
functions, and separate from each other, because BTA and Beta 1.7.3 share only
their framing; every layout above it differs. See `README.bta.md`.

## Place in the tools/ package

`main.go` dispatches subcommands: `bot` → `runBot` (this file), plus `rcon`
and `gen-matrix`. bot.go owns only the orchestration; the Minecraft protocol
work (`ping`, `join`, `client`, `pump`, `sendCommand`) lives in `mc.go`, the
supported-protocol table (`row`, `rowFor`) lives in `table.go`, and the two
pre-Netty protocols live in `beta.go` and `bta.go`.

## Run it

```sh
go run ./tools bot -host 127.0.0.1 -port 25565 -command list
```
