# tools/rcon.go

A minimal, hand-rolled RCON client. No third-party library. It connects to a
Minecraft server's RCON port, authenticates, runs one command, and prints the
server's reply.

## What is RCON?

RCON is Source's remote-console protocol, which Minecraft adopted. It runs
over plain TCP. Each message is a packet framed like this (all ints are
little-endian int32):

```
length | request_id | type | payload | \x00\x00
```

- `length` counts everything after itself (so `10 + len(payload)`).
- `type` is 3 for auth (`SERVERDATA_AUTH`), 2 for a command
  (`SERVERDATA_EXECCOMMAND`) and for responses.
- Auth failure is signaled by a reply with `request_id == -1`.

One quirk: a single response packet caps its payload at ~4096 bytes. Long
command output arrives split across several packets, and the protocol has no
"end of response" marker. The standard trick, used here: after the real
command, send an empty "sentinel" command with a different `request_id`.
The server answers in order, so every packet that arrives before the
sentinel's reply belongs to the real command. Concatenate those; stop when
the sentinel's id shows up.

A second quirk: vanilla Minecraft drops the connection if two client packets
land in one TCP segment, so the code reads the first response before sending
the sentinel.

## Functions

- `writePacket(w, id, typ, payload)` — builds and writes one framed packet.
- `readPacket(r)` — reads one packet; returns id, type, payload. Rejects
  lengths under 10 or over 4 MiB as corrupt.
- `rconExec(addr, password, cmd)` — the whole conversation: dial (10 s
  timeout and deadline), authenticate (skipping any empty pre-auth packet
  some servers send), run `cmd`, reassemble fragments via the sentinel,
  return the joined output.
- `runRcon(args)` — CLI wrapper. Flags: `--host` (default `127.0.0.1`),
  `--port` (default `25575`), `--password`. Remaining args are joined into
  the command. Prints the response to stdout.

## Place in the package

`tools/` is one binary with subcommands (see `main.go`). This file provides
the `rcon` subcommand:

```
tools rcon --port 25575 --password S say hello
```

The harness uses it to poke a running test server without needing `mcrcon`
or similar installed.

## Retry policy

`rconExec` runs the whole conversation up to **3 times**, with a fixed **1 s**
pause between attempts, and each attempt gets a fresh connection — a
half-broken one is not reusable.

Only a connection that was **established and then broke** is retried. Two
failures are returned on the first attempt instead:

- a **refused dial** (phase `dial`) — this is the expected answer on the
  Babric/BTA e2e legs, which assert that Beta-era servers have no RCON at all
  by dialling a port nothing is on. Retrying it would slow that probe down for
  no information.
- **`authentication failed`** — a rejected password is deterministic.

The retry exists for [issue #89](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/issues/89):
a 1.14.4 e2e leg saw the server log `Rcon connection from:` and the client get
`EOF`, with no server-side error and a clean re-run.

Every failed attempt prints one line to **stderr**:

```
[rcon] attempt 1/3 failed after 812ms during auth-read: EOF; retrying in 1s
```

The phase (`dial`, `deadline`, `auth-write`, `auth-read`, `cmd-write`,
`sentinel-write`, `cmd-read`) is the diagnosis `rcon: EOF` never carried: it
separates a server-side accept race from the two-packets-in-one-TCP-segment
hazard above, and the elapsed time separates an instant hang-up from a stall.
`scripts/e2e-entrypoint.sh` greps for `^\[rcon\] attempt` and reports a retried
leg as the warning `rcon-retried` — a retry never fails a leg, but it is never
silent either.
