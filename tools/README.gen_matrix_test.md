# gen_matrix_test.go

Offline tests for `genMatrix` in `gen_matrix.go` — the generator that builds the CI e2e job matrix. No network, no real repo state: tests run against temp-dir fixtures, so `FORCE_BANDS` (the `bands` argument) alone decides which version bands exist.

## What it locks down

Every number in the file is a deliberate contract. If you change the matrix in `gen_matrix.go`, these tests are the tripwire:

- Which output keys exist (all 21 `<band>_java<N>` keys, always emitted).
- Exact version lists per band, and exact job counts per event type (`pull_request` = lean, `workflow_dispatch` = full). Forge and NeoForge rows have no lean/full split.
- Totals: 75 gated pairs on PR, 104 on dispatch. `TOTAL_JOBS = 2 x fabric pairs + forge pairs + neo pairs + 22` (fabric keys feed a `-fabric` and a `-quilt` caller job each, `forge_*`/`neo_*` keys one; the 22 fixed jobs are contracts, go-quality, lint-java, unit-tests, the 9 build jobs, the Build aggregator, 4 gate canaries and 4 config-behaviors legs): 136 on PR, 194 on dispatch. On push only the 14 fixed jobs that are not event-skipped remain.
- Output format: every value is a valid JSON string array; absent bands emit the literal `[]`, never a missing key.

## Main test functions

- `TestKeysAlwaysPresentAndBandLists` — all 21 keys present; exact version arrays for t0, mc1192, mc114, the five Forge rows and the three NeoForge rows (byte-pinned, formerly hand-listed in `e2e.yml`).
- `TestAbsentBandsEmitEmptyArrayLiteral` — bands not shipped still emit their keys as `[]`, on both event types.
- `TestSubmatrixCountsAndTotals` — per-key job counts against the `expected` table, plus the `GATED_PAIRS` / `TOTAL_JOBS` / `EVENT_NAME` summary lines.
- `TestOptionCombinationTotals` — totals for every band combination (`""` up through the full `allBands` list, `neo` included) on both triggers; the forge-less cases prove absent Forge bands add zero pairs, and the last case pins the NeoForge band at +6 pairs / +6 jobs.
- `TestNeoRowsCountAsOneJobEach` — the three `neo_*` lists byte-pinned, plus the regression tripwire: `LOADER=neoforge` has no quilt twin, so the band adds 6 jobs, not 12 (a `strings.Contains(name, "forge")` job predicate would match `neoforge` and double them).
- `TestCanariesMovedNotDuplicated` — canary versions 1.21.11 and 26.2 live in the e2e gate only; they must never also appear in a stage list.
- `TestOutputsAreJSONStringArrays` — every emitted value parses as `[]string`.
- `TestBandDetection` — real detection paths, no forcing: t0 turns on via a widened `minecraft_range_121` in `gradle.properties`; mc1192/mc114 via `src/<band>/java` source-set directories; forge/forge_legacy/forge_mc116/forge_eventbus7 via the `minecraft_range_modern`/`_legacy`/`_mc116`/`_eventbus7` lines in `forge/gradle.properties` (modern-only and file-absent fixtures prove the legacy rows stay `[]`, and that 1.20.4 — which boots the legacy jar — drops out of `forge_java21` without the legacy band); `neo` via `minecraft_range_neo_all` in `neoforge/gradle.properties`, with a file-absent fixture proving its rows stay `[]`.
- `TestSummaryLineFormat` — the human summary keeps its exact `%-16s %3d` printf shape, `neo_*` names included (they fit the 16-column field; the long `forge_eventbus7_*` names already do not).

Helpers: `runGrid` runs `genMatrix` and parses its `GITHUB_OUTPUT` lines into a `name -> json` map; `versionsOf` decodes one value; `emptyRoot` gives a fixture tree with no bands.

## How to run

```sh
make go-test                 # repo way: race + cover over tools/
cd tools && go test -run 'TestSubmatrixCountsAndTotals' -v   # one test
```

Failures almost always mean the matrix contract changed. If the change is intentional, update the `expected` map and the hardcoded totals here to match.
