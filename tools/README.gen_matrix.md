# gen_matrix.go

Generates the e2e test matrix for CI. It is the single source of truth for
which {Minecraft version, Java version} pairs get tested. The GitHub workflow
(`.github/workflows/e2e.yml`) never hardcodes version lists — it reads them
from this program's output.

## What it does

1. Looks at the repo to see which compatibility "bands" exist (old-version
   source sets, version ranges in `gradle.properties`).
2. Builds a version list per {band, Java} pair from the coverage table — the
   band's `sampled` list on a lean run (pull request), its `deep` list on the
   deep sweep (manual dispatch), which is every version the band declares bar
   the ones excluded with a written reason.
3. Writes each list as a `name=["1.21", ...]` JSON line into `$GITHUB_OUTPUT`,
   plus a human-readable summary and job counts on stdout.

Key contract: the coverage table, not the emit calls, decides which versions
a row lists. Each band states its `declared` range, its `sampled` list, its
`deep` list and a reason for every declared version booted by neither;
`gen_matrix_test.go` asserts `deep + excluded == declared` exactly, so a
version can only leave the grid by acquiring a reason. Since issue #84
`declared` is every Mojang release the range covers — the same set published as
Modrinth `game_versions` — so what is advertised and what CI boots are one list
with one set of written exceptions. Background:
["The denominator, settled"](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/Supported-Versions#the-denominator-settled).

Second contract: every output name is written on every run, even as a literal
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
- `coverage` / `booted(band, full)` — the version contract per band, and the
  list to boot for this event.
- `mojangAxis` / `releasesIn(spec)` — the Minecraft release list, and the one
  place a declared range (`>=1.14 <1.19`, `[1.14.4,1.17)`) becomes a version
  list. Every band's `declared` is `releasesIn()` over its `minecraft_range_*`,
  and `gen_matrix_test.go` re-derives all of them from the real
  gradle.properties files, so a widened range cannot silently keep an old list.
- `printPublish(w)` / `publishedJars` — `gen-matrix --publish`, one
  `jar<TAB>loaders<TAB>game_versions` row per uploaded Modrinth version. The
  list is the band's declared range minus what no one can install: a version
  the loader project published no build for, and (on the quilt row) the
  versions in `quiltUnavailable`. The `mc1.14.x` jar is uploaded twice, `fabric`
  and `fabric,quilt`, because Modrinth cannot exclude one loader from one
  version. `docs/modrinth-versions.tsv` is this output committed, and
  `gen_matrix_test.go` fails when the two differ. Regenerate, never hand-edit:

  ```
  cd tools && REPO_ROOT=.. go run . gen-matrix --publish > ../docs/modrinth-versions.tsv
  ```
- `printCoverage(w)` — `gen-matrix --coverage`, a `band<TAB>state<TAB>version<TAB>reason`
  dump so `scripts/test-jar-routing.sh` can re-probe the exclusions rather
  than restate them.
- `neoFloor(v)` / `pick(list, keep)` — the row splits: NeoForge's own Java
  floor per version, and a filter.

## Inputs (env vars)

| Var | Meaning |
|---|---|
| `EVENT_NAME` | `pull_request` (lean, default), `workflow_dispatch` (deep sweep) or `push` (all bands empty) |
| `GITHUB_OUTPUT` | file to append `name=json` lines to (optional) |
| `FORCE_BANDS` | space-separated band names to pretend exist (testing) |
| `REPO_ROOT` | repo root, default `.` |

## Outputs

- To `$GITHUB_OUTPUT`: one `name=json` line per {band, Java} pair,
  e.g. `mc121_java21=["1.21","1.21.1",...]`.
- To stdout: aligned summary of each row, then `EVENT_NAME`, `GATED_PAIRS`
  (sum of all list lengths), and `TOTAL_JOBS` (what the workflow spawns:
  2 caller jobs per fabric pair — `-fabric` and `-quilt` — 1 per
  single-loader `forge_*` or `neo_*` pair, plus a fixed-job constant that
  shrinks on push, where the gate canaries, the config-behaviors legs and
  the refusal guards are event-skipped). The constant and its per-job
  breakdown are enumerated in the comment above `fixedJobs` in
  `gen_matrix.go`; for the current totals, run the generator — the expected
  values are pinned in `gen_matrix_test.go`.

## Place in the tools/ package

`tools/` is one Go binary with subcommands (`bot`, `rcon`, `gen-matrix`);
`main.go` dispatches to `runGenMatrix`. Expected job counts are pinned in
`gen_matrix_test.go`; `floors_test.go` cross-checks Java floors. Policy and
rationale live in `docs/ci.md` and the wiki's
[Version boundaries and root causes](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/Version-Boundaries-And-Root-Causes).
