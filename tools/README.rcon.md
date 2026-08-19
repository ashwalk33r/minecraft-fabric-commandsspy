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
