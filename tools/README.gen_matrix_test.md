# gen_matrix_test.go

Offline tests for `genMatrix` in `gen_matrix.go` — the generator that builds the CI e2e job matrix. No network, no real repo state: tests run against temp-dir fixtures, so `FORCE_BANDS` (the `bands` argument) alone decides which version bands exist.

## What it locks down

Every number in the file is a deliberate contract. If you change the matrix in `gen_matrix.go`, these tests are the tripwire:

- Which output keys exist (all 13 `<band>_java<N>` keys, always emitted).
- Exact version lists per band, and exact job counts per event type (`pull_request` = lean, `workflow_dispatch` = full).
- Totals: 39 gated pairs on PR, 68 on dispatch, plus 4 fixed jobs (`TOTAL_JOBS = gated + 4`).
- Output format: every value is a valid JSON string array; absent bands emit the literal `[]`, never a missing key.

## Main test functions

- `TestKeysAlwaysPresentAndBandLists` — all 13 keys present; exact version arrays for t0, mc1192, mc114.
- `TestAbsentBandsEmitEmptyArrayLiteral` — bands not shipped still emit their keys as `[]`, on both event types.
- `TestSubmatrixCountsAndTotals` — per-key job counts against the `expected` table, plus the `GATED_PAIRS` / `TOTAL_JOBS` / `EVENT_NAME` summary lines.
- `TestOptionCombinationTotals` — totals for every band combination (`""`, `t0`, `t0 mc1192`, `t0 mc1192 mc114`) on both triggers.
- `TestCanariesMovedNotDuplicated` — canary versions 1.21.11 and 26.2 live in the e2e gate only; they must never also appear in a stage list.
- `TestOutputsAreJSONStringArrays` — every emitted value parses as `[]string`.
- `TestBandDetection` — real detection paths, no forcing: t0 turns on via a widened `minecraft_range_121` in `gradle.properties`; mc1192/mc114 via `src/<band>/java` source-set directories.
- `TestSummaryLineFormat` — the human summary keeps its exact `%-16s %3d` printf shape.

Helpers: `runGrid` runs `genMatrix` and parses its `GITHUB_OUTPUT` lines into a `name -> json` map; `versionsOf` decodes one value; `emptyRoot` gives a fixture tree with no bands.

## How to run

```sh
make go-test                 # repo way: race + cover over tools/
cd tools && go test -run 'TestSubmatrixCountsAndTotals' -v   # one test
```

Failures almost always mean the matrix contract changed. If the change is intentional, update the `expected` map and the hardcoded totals here to match.
