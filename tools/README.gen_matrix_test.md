# gen_matrix_test.go

Offline tests for `genMatrix` in `gen_matrix.go` — the generator that builds the CI e2e job matrix. No network, no real repo state: tests run against temp-dir fixtures, so `FORCE_BANDS` (the `bands` argument) alone decides which version bands exist.

## What it locks down

Every number in the file is a deliberate contract. If you change the matrix in `gen_matrix.go`, these tests are the tripwire:

- Which output keys exist (all 18 `<band>_java<N>` keys, always emitted).
- Exact version lists per band, and exact job counts per event type (`pull_request` = lean, `workflow_dispatch` = full). Forge rows have no lean/full split.
- Totals: 63 gated pairs on PR, 92 on dispatch. `TOTAL_JOBS = 2 x fabric pairs + forge pairs + 8` (fabric keys feed a `-fabric` and a `-quilt` caller job each, Forge keys one; the 8 fixed jobs are build-jars, unit-tests, 4 gate canaries, 2 literal NeoForge jobs): 110 on PR, 168 on dispatch.
- Output format: every value is a valid JSON string array; absent bands emit the literal `[]`, never a missing key.

## Main test functions

- `TestKeysAlwaysPresentAndBandLists` — all 18 keys present; exact version arrays for t0, mc1192, mc114, and the five Forge rows (byte-pinned, formerly hand-listed in `e2e.yml`).
- `TestAbsentBandsEmitEmptyArrayLiteral` — bands not shipped still emit their keys as `[]`, on both event types.
- `TestSubmatrixCountsAndTotals` — per-key job counts against the `expected` table, plus the `GATED_PAIRS` / `TOTAL_JOBS` / `EVENT_NAME` summary lines.
- `TestOptionCombinationTotals` — totals for every band combination (`""` up through `t0 mc1192 mc114 forge forge_legacy forge_eventbus7`) on both triggers; the forge-less cases prove absent Forge bands add zero pairs.
- `TestCanariesMovedNotDuplicated` — canary versions 1.21.11 and 26.2 live in the e2e gate only; they must never also appear in a stage list.
- `TestOutputsAreJSONStringArrays` — every emitted value parses as `[]string`.
- `TestBandDetection` — real detection paths, no forcing: t0 turns on via a widened `minecraft_range_121` in `gradle.properties`; mc1192/mc114 via `src/<band>/java` source-set directories; forge/forge_legacy/forge_eventbus7 via the `minecraft_range_modern`/`_legacy`/`_eventbus7` lines in `forge/gradle.properties` (modern-only and file-absent fixtures prove the legacy rows stay `[]`, and that 1.20.4 — which boots the legacy jar — drops out of `forge_java21` without the legacy band).
- `TestSummaryLineFormat` — the human summary keeps its exact `%-16s %3d` printf shape.

Helpers: `runGrid` runs `genMatrix` and parses its `GITHUB_OUTPUT` lines into a `name -> json` map; `versionsOf` decodes one value; `emptyRoot` gives a fixture tree with no bands.

## How to run

```sh
make go-test                 # repo way: race + cover over tools/
cd tools && go test -run 'TestSubmatrixCountsAndTotals' -v   # one test
```

Failures almost always mean the matrix contract changed. If the change is intentional, update the `expected` map and the hardcoded totals here to match.
