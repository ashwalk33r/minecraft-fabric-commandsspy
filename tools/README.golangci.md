# .golangci.yml

Config for [golangci-lint](https://golangci-lint.run/), the standard Go lint runner. It runs many static checkers over the Go code in `tools/` in one pass and fails on findings.

## What this config does

- `version: "2"` — uses the v2 config schema (requires golangci-lint v2.x).
- `linters.enable: misspell` — adds the `misspell` linter (catches misspelled English words in comments and strings) on top of golangci-lint's default set (`errcheck`, `govet`, `staticcheck`, `ineffassign`, `unused`).
- `exclusions.rules` — silences `errcheck` (the "you ignored an error return" linter) only in `_test.go` files. Tests routinely ignore errors on cleanup calls; production code may not. The header comment says why: in protocol code an ignored error is a silent failure.
- `run.timeout: 3m` — aborts the whole run if it takes longer than 3 minutes, so CI never hangs.

Everything else is golangci-lint defaults. Strictness is deliberate.

## How to run it

```sh
cd tools
golangci-lint run ./...
```

The config is picked up automatically from `tools/.golangci.yml`. Install: `brew install golangci-lint` (needs v2.x for this config).
