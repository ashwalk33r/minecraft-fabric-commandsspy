# gen_matrix_test.go

Offline tests for `genMatrix` in `gen_matrix.go` — the generator that builds the CI e2e job matrix. No network, no real repo state: tests run against temp-dir fixtures, so `FORCE_BANDS` (the `bands` argument) alone decides which version bands exist.

## What it locks down

Every number in the file is a deliberate contract. If you change the matrix in `gen_matrix.go`, these tests are the tripwire:

- Which output keys exist (all 23 `<band>_java<N>` keys, always emitted).
- Exact version lists per band, and exact job counts per event type (`pull_request` = lean, `workflow_dispatch` = deep sweep). Forge and NeoForge rows have no newest-Java coverage rows beyond the forward-JVM rows (#58), but their floor rows do widen on the deep sweep.
- The sampling rule: `deep + excluded == declared` for every band in the coverage table, every exclusion carrying a non-empty reason, and the emitted grid booting exactly `sampled` on PR and exactly `deep` on dispatch.
- Totals: `TOTAL_JOBS = 2 x fabric pairs + forge pairs + neo pairs + a fixed-job constant` — fabric keys feed a `-fabric` and a `-quilt` caller job each, `forge_*`/`neo_*` keys one. The fixed jobs, and the smaller push-only count left after the event-skipped ones drop out, are enumerated in the comment above `fixedJobs` in `gen_matrix.go`. The expected numbers themselves are the pins below — read them there, not here.
- Output format: every value is a valid JSON string array; absent bands emit the literal `[]`, never a missing key.

## Main test functions

- `TestKeysAlwaysPresentAndBandLists` — all 23 keys present; exact version arrays for t0, mc1192, mc114, the six Forge rows and the four NeoForge rows (byte-pinned, formerly hand-listed in `e2e.yml`).
- `TestAbsentBandsEmitEmptyArrayLiteral` — bands not shipped still emit their keys as `[]`, on both event types.
- `TestSubmatrixCountsAndTotals` — per-key job counts against the `expected` table, plus the `GATED_PAIRS` / `TOTAL_JOBS` / `EVENT_NAME` summary lines.
- `TestOptionCombinationTotals` — totals for every band combination (`""` up through the full `allBands` list, `neo` included) on both triggers; the forge-less cases prove absent Forge bands add zero pairs, and the last case pins the NeoForge band at +7 pairs / +7 jobs.
- `TestNeoRowsCountAsOneJobEach` — the four `neo_*` lists byte-pinned, plus the regression tripwire: `LOADER=neoforge` has no quilt twin, so the band adds 7 jobs, not 14 (a `strings.Contains(name, "forge")` job predicate would match `neoforge` and double them).
- `TestCanariesMovedNotDuplicated` — canary versions 1.21.11 and 26.2 live in the e2e gate only; they must never also appear in a stage list.
- `TestOutputsAreJSONStringArrays` — every emitted value parses as `[]string`.
- `TestBandDetection` — real detection paths, no forcing: t0 turns on via a widened `minecraft_range_121` in `gradle.properties`; mc1192/mc114 via `src/<band>/java` source-set directories; forge/forge_legacy/forge_mc116/forge_eventbus7 via the `minecraft_range_modern`/`_legacy`/`_mc116`/`_eventbus7` lines in `forge/gradle.properties` (modern-only and file-absent fixtures prove the legacy rows stay `[]`, and that 1.20.4 — which boots the legacy jar — drops out of `forge_java21` without the legacy band); `neo` via `minecraft_range_neo_all` in `neoforge/gradle.properties`, with a file-absent fixture proving its rows stay `[]`.
- `TestCoverageTableAccountsForEveryDeclaredVersion` — the anti-drift assertion: per band, `sampled` is inside `deep`, `deep` is inside `declared`, nothing is both booted and excluded, every exclusion reason is non-empty, and `deep + excluded` equals `declared` exactly.
- `TestGridBootsTheSampleThenTheDeepList` — the table is not decoration: the rows a band owns boot exactly its `sampled` set on `pull_request` and exactly its `deep` set on `workflow_dispatch` (`forge_java21`'s 1.20.4 excepted — it is keyed on the legacy band).
- `TestEveryRowBelongsToABand` — every emitted key appears in `bandRows` exactly once, so a new row cannot escape the two tests above.
- `TestSummaryLineFormat` — the human summary keeps its exact `%-16s %3d` printf shape, `neo_*` names included (they fit the 16-column field; the long `forge_eventbus7_*` names already do not).

Helpers: `runGrid` runs `genMatrix` and parses its `GITHUB_OUTPUT` lines into a `name -> json` map; `versionsOf` decodes one value; `emptyRoot` gives a fixture tree with no bands.

## How to run

```sh
make go-test                 # repo way: race + cover over tools/
cd tools && go test -run 'TestSubmatrixCountsAndTotals' -v   # one test
```

Failures almost always mean the matrix contract changed. If the change is intentional, update the `expected` map and the hardcoded totals here to match.
