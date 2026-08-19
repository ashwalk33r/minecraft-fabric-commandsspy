# tools/main.go

The entry point of the `tools` binary. One binary, three subcommands — a small
toolbox for the e2e harness instead of three separate programs.

## What it does

`main.go` is a dispatcher and nothing else. It:

1. Reads the first CLI argument (`os.Args[1]`) as the subcommand name.
2. Looks it up in the `subcommands` map, which wires names to functions
   defined in the other files of this package:
   - `bot` → `runBot` (bot.go) — e2e player phase: two Minecraft bot clients
     join a server and exercise command attribution.
   - `rcon` → `runRcon` (rcon.go) — hand-rolled RCON client for sending
     server commands.
   - `gen-matrix` → `runGenMatrix` (gen_matrix.go) — emits the e2e version
     matrix consumed by `.github/workflows/e2e.yml`.
3. Passes the remaining arguments (`os.Args[2:]`) to that function untouched.

Each subcommand parses its own flags with its own `flag.FlagSet`. `main.go`
defines no flags of its own.

## Usage

```
tools <bot|rcon|gen-matrix> [args]
```

## Exit codes

- `0` — subcommand returned nil.
- `1` — subcommand returned an error (printed to stderr as `name: err`).
- `2` — no subcommand given, or unknown subcommand (usage printed to stderr).

## Adding a subcommand

Write a `func runFoo(args []string) error` in a new file, add one entry to the
`subcommands` map, and update the `usage()` string. That is the whole wiring.
