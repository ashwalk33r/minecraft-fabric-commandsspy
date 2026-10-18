# upstream.go

The `upstream-drift` subcommand. It compares the stable versions that each
upstream project publishes with the versions this repository declares, and
fails when upstream has a stable version that no band declares or excludes.

## Why it exists

Nothing else in the repository notices a new Mojang or loader release. The
offline tests prove that the declared lists agree with each other, but they
can't see a version that upstream published after the lists were written.
This check fails on the first pull request after such a release.

## Inputs

- `coverage[band].declared` in `gen_matrix.go`. `upstreamBands` maps each
  backend to its bands, and the union of their declared lists is the
  backend's declared set.
- `upstreamExcluded` in `upstream.go`. Each entry maps a backend and a version
  to a reason. A version listed here passes without being declared. An empty
  reason fails the whole run.
- One upstream feed per backend, listed in `liveFeeds`. Quilt also reads the
  Mojang version manifest.

## Feeds and stable filters

| Backend | Feed | A version counts as stable when |
|---|---|---|
| `fabric` | Fabric meta `v2/versions/game` | `stable` is `true` |
| `quilt` | Quilt meta `v3/versions/game` | Mojang's manifest lists it with `type` `release` |
| `forge` | Forge `promotions_slim.json` | It has a `-latest` or `-recommended` promotion key |
| `neoforge` | NeoForge `maven-metadata.xml` | The build has no `-` suffix. `21.1.77` maps to `1.21.1` and `26.3.0.92` maps to `26.3` |
| `bta` | BTA `bta-client/release/` directory index | A `vN.N` directory exists. `v7.3_04` maps to `bta7.3_04` |
| `babric` | Babric meta `v2/versions/game` | `stable` is `true` |

## Pass rule

For each backend, the floor is the lowest declared version. Every stable
upstream version at or above the floor must be declared or excluded. Babric
has no floor, because its band is the one version `b1.7.3`, so every other
stable Babric version is drift.

The command exits with status 1 in these cases:

- A stable version is neither declared nor excluded. The output has one
  `DRIFT BACKEND VERSION` line for each gap.
- A feed is unreachable after two attempts, returns a non-200 status, can't
  be parsed, or lists no stable version.
- An `upstreamExcluded` entry has an empty reason.

## Usage

```
cd tools && go run . upstream-drift
make ci-upstream-drift
```

The `make` target runs the same command inside the pinned CI image. The
`upstream-drift` job in `.github/workflows/ci.yml` runs it on every event, and
`Build` needs that job.

## Fixing a failure

- If the jars support the new version, widen the `minecraft_range_*` line and
  update `mojangAxis` and `coverage` in `gen_matrix.go`.
- If the jars don't support it, add the version to `upstreamExcluded` with a
  reason that someone else can verify.
