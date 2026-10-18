# upstream_test.go

Offline tests for `upstream.go`. Each test starts an `httptest` server that
serves fixture feeds, so `make ci` makes no network call.

## Fixtures

- `driftFixtures` publishes 26.3 on Fabric, Quilt, Forge, and NeoForge, plus
  `26.3-rc-1`, `26.4-snapshot-1`, `26.3.0.5-beta`, and a Forge key with no
  promotion.
- `noiseOnly` keeps 26.3 only as an rc, a beta build, and an unpromoted Forge
  key, so the stable filters are the only thing that keeps the run green.
- `declaredTo262` stands in for the declared lists before the 26.3 bump.

## Tests

- `TestUpstreamDriftNamesEveryBackendBehindUpstream`: fails with exactly four
  `DRIFT` lines, one each for Fabric, Quilt, Forge, and NeoForge.
- `TestUpstreamDriftPassesWhenTheGapHasAReason`: the same fixtures pass when
  each 26.3 gap is in the excluded map with a reason.
- `TestUpstreamDriftRejectsAnEmptyReason`: an excluded entry with a blank
  reason fails.
- `TestUpstreamDriftIgnoresPrereleasesAndUnpromotedBuilds`: the negative
  control. `noiseOnly` passes, so a stable filter that stops filtering
  fails this test.
- `TestUpstreamDriftFailsOnABrokenFeed`: an unreachable feed, malformed JSON,
  malformed XML, an empty feed, a BTA index with no release directories, and a
  Mojang manifest with no releases each fail the run.
- `TestUpstreamExcludedHasReasonsAndKnownBackends`: the real `upstreamExcluded`
  map has a reason for every entry and names only known backends, and every
  band in `upstreamBands` exists in `coverage`.

## How to run

```
cd tools && go test -run 'Upstream' -v
```
