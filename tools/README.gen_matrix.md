# gen_matrix.go

Generates the e2e test matrix for CI. It is the single source of truth for
which {Minecraft version, Java version} pairs get tested. The GitHub workflow
(`.github/workflows/e2e.yml`) never hardcodes version lists — it reads them
from this program's output.

## What it does

1. Looks at the repo to see which compatibility "bands" exist (old-version
   source sets, version ranges in `gradle.properties`).
2. Builds a version list per {band, Java} pair. Lean runs (pull requests) test
   only each band's oldest and newest version; full runs (manual dispatch)
   test everything.
3. Writes each list as a `name=["1.21", ...]` JSON line into `$GITHUB_OUTPUT`,
   plus a human-readable summary and job counts on stdout.

Key contract: every output name is written on every run, even as a literal
`[]`. A missing output becomes `''` in GitHub expressions and `fromJSON('')`
breaks the workflow. Two "gate canary" versions (1.21.11/java21, 26.2/java25)
live in the workflow's gate job and are deliberately excluded from the lists
here.

## Main functions

- `runGenMatrix(args)` — entry point, wired to the `gen-matrix` subcommand in
  `main.go`. Reads env vars, opens `$GITHUB_OUTPUT`, calls `genMatrix`.
- `genMatrix(repoRoot, eventName, forceBands, stdout, ghOut)` — the real work.
  Defines the stages (mc121, mc26, t0, mc1192, mc114, forge, forge_legacy,
  forge_mc116, forge_eventbus7, neo) and emits every row. Forge and NeoForge
  rows are floor-only: no coverage rows, no lean/full split (those jars' own
  bytecode floors and, for NeoForge, its own per-line Java floors govern — not
  the Fabric era table). Testable: writers are injected.
- `bandPresent(repoRoot, name, forced)` — is a band buildable in this tree?
  `t0` checks a range line in `gradle.properties`; `mc1192`/`mc114` check that
  `src/<band>/java` exists; `forge`/`forge_legacy`/`forge_mc116`/
  `forge_eventbus7` check the
  `minecraft_range_modern`/`_legacy`/`_mc116`/`_eventbus7` lines in
  `forge/gradle.properties`; `neo` checks `minecraft_range_neo_all` in
  `neoforge/gradle.properties` (one band jar, so one range key).
  `FORCE_BANDS` overrides for offline tests.
- `ends(list)` — first and last element; the "lean" shrink.

## Inputs (env vars)

| Var | Meaning |
|---|---|
| `EVENT_NAME` | `pull_request` (lean, default) or `workflow_dispatch` (full) |
| `GITHUB_OUTPUT` | file to append `name=json` lines to (optional) |
| `FORCE_BANDS` | space-separated band names to pretend exist (testing) |
| `REPO_ROOT` | repo root, default `.` |

## Outputs

- To `$GITHUB_OUTPUT`: one `name=json` line per {band, Java} pair,
  e.g. `mc121_java21=["1.21","1.21.1",...]`.
- To stdout: aligned summary of each row, then `EVENT_NAME`, `GATED_PAIRS`
  (sum of all list lengths), and `TOTAL_JOBS` (what the workflow spawns:
  2 caller jobs per fabric pair — `-fabric` and `-quilt` — 1 per
  single-loader `forge_*` or `neo_*` pair, plus 23 fixed jobs: contracts,
  go-quality, lint-java, unit-tests, the 10 build jobs, the Build aggregator,
  the 4 gate canaries and the 4 config-behaviors legs; on push only 15 of
  those run — gate and config-behaviors are event-skipped). Against the real
  tree today: 75 gated pairs / 137 jobs on a PR, 104 / 195 on dispatch.

## Place in the tools/ package

`tools/` is one Go binary with subcommands (`bot`, `rcon`, `gen-matrix`);
`main.go` dispatches to `runGenMatrix`. Expected job counts are pinned in
`gen_matrix_test.go`; `floors_test.go` cross-checks Java floors. Policy and
rationale live in `docs/ci.md` and `docs/version-matrix.md`.
