# CI

## The one gate: `make ci`

`make ci` is the single fast quality gate — the pre-commit hook and CI both
run exactly it, inside the pinned `Dockerfile.ci` container, so a local pass
means a CI pass. It covers shell lint and Go format/vet/lint/vuln/build/test,
and excludes e2e (that has its own jobs later in the same `ci.yml`
pipeline). `make ci-host` is the same
chain on the host toolchain — an escape hatch when Docker is unavailable;
the container run is authoritative.

`Dockerfile.ci` pins every tool by exact version (Go, shellcheck, base image
by digest) because lint output is only stable per toolchain release. The image
tag is content-addressed: editing `Dockerfile.ci` triggers a rebuild.
`GOTOOLCHAIN=local` is load-bearing — without it Go could silently download a
different toolchain than the pinned one. `CI_CACHE_DIR` holds warm Go caches;
disposable, empty it for a cold run.

The image is also published to GHCR (repo is public: free, unlimited
storage/bandwidth), so `ci-image` can `docker pull` a real registry image
instead of rebuilding locally on every fresh clone or CI job — a plain
`docker pull` dedups layers properly, unlike a gzipped image tarball
round-tripped through an Actions cache.

The cache specifically keeps `go run golangci-lint@<version>` warm across
runs, on top of the general Go build/module cache.

## `make build` / `make test` / `make lint-java`

These also run inside the pinned `Dockerfile.ci` image — it carries JDK 21
and JDK 25 (for the 26.x and NeoForge 26.2 toolchains) alongside Go and
shellcheck, so
`make build`, `make test`, `make lint-java`, and `make ci` together need only
`make` and `docker` on the host, locally and in CI. There is no
Docker-unavailable escape hatch for these three (unlike `ci-host`): run
`./gradlew` directly against a local JDK instead.

`make build` builds all six jars (four Fabric/Quilt eras + two NeoForge
lines); the Forge jars (a separate Gradle build in `forge/`) are
`make build-forge`/`make build-forge-legacy`/`make build-forge-mc116`/
`make build-forge-eventbus7`, on demand — not part of the default
`make build`/`make ci` path.

## ci.yml — the one workflow

One workflow, `CI` (`ci.yml`), replaced the old `Build` (`gradle.yml`) +
`E2E` (`e2e.yml`) pair, which duplicated `make test` and `make build` on
every PR. Triggers: push to `main`, `pull_request`, `workflow_dispatch`; a
new push to the same ref cancels the in-flight run. The reusable submatrix
`e2e-stage.yml` is unchanged.

Tier 0 is four cheap parallel gates:

- **contracts** — `scripts/verify-action-pins.sh` (asserts every third-party
  action is pinned to a full commit SHA), Gradle wrapper validation, the grid
  count assertions (`make ci-tools-test`), the offline routing contract
  (`scripts/test-jar-routing.sh`), and `make ci-gen-matrix` — the single
  source of stage definitions, whose 18 band outputs every stage job reads
  as `needs.contracts.outputs.*`. It restores the `ci-go` cache read-only;
  `go-quality` owns the save (its `make ci` populates the richer cache, and
  a save race here would clobber it).
- **go-quality** — exactly `make ci`.
- **lint-java** — `make lint-java` (checkstyle + PMD).
- **unit-tests** — `make test`: the routing contract plus all four targets
  (`test114`, `test1192`, `test121`, `test26`), test reports uploaded as an
  artifact.

The aggregator job named `Build` is main's required status check — the same
context name the old `gradle.yml` job carried, so branch protection needed
no change. It fails on ANY failed, cancelled, or *skipped* dependency
(skipped too, so a future `if:` on a build job cannot vacuously green the
required check).

On push to `main` the jars are built and published (30-day retention) with
zero e2e: `tools/gen_matrix.go` emits `[]` for every band on
`EVENT_NAME=push`, and `e2e-gate` plus the two literal NeoForge jobs carry
an explicit `github.event_name != 'push'` guard.

## The staged e2e matrix

Stage order is popularity order: a failure in a widely-run version surfaces
before runner minutes are spent on the long tail.

1. **Tier 1: five parallel build jobs** (`needs: [contracts]`), replacing
   the old sequential ~25-minute `build-jars` job: `build-fabric-neoforge`
   (`make build` — all six jars, four Fabric/Quilt eras + two NeoForge
   lines), `build-forge-modern`, `build-forge-legacy`, `build-forge-mc116`,
   and `build-forge-eventbus7` — each Forge target a separate Gradle build
   (`forge/`), all in the same pinned CI image. One artifact per job:
   `commandsspy-jar-fabric-neoforge-<sha>` (six jars),
   `commandsspy-jar-forge-modern-<sha>`,
   `commandsspy-jar-forge-legacy-<sha>`,
   `commandsspy-jar-forge-mc116-<sha>`,
   `commandsspy-jar-forge-eventbus7-<sha>`; retention 30 days on push,
   1 day otherwise. The download side is unchanged: stage jobs fetch with
   pattern `commandsspy-jar-*-<sha>` + `merge-multiple`, so the split is
   invisible to them. Gradle cache keys are per-target
   (`ci-gradle-<target>-<hash>`) on purpose: a single shared key is saved
   by whichever job finishes first, so the other targets' (ForgeGradle
   decompile) caches would never persist and every run would rebuild cold.
   `forge/build.gradle` and `forge/gradle.properties` are in every key
   alongside the root build files, because the first Forge build of each
   target decompiles Minecraft and is slow on a cold cache.
2. **e2e-gate** — `needs` all four Tier 0 gates and all five build jobs:
   two canary pairs (1.21.11/java21, 26.2/java25), each
   crossed with `loader: fabric` and `loader: quilt` via `matrix.include`, so
   four canary jobs run. `fail-fast` is off so all four always report.
3. **Forge stages** — five caller jobs (`e2e-forge-java21`,
   `e2e-forge-legacy-java17`, `e2e-forge-mc116-java8`,
   `e2e-forge-eventbus7-java21`, `e2e-forge-eventbus7-java25`), each a normal
   `e2e-stage.yml` call with `loader: forge` reading its version list from a
   `tools/gen_matrix.go` output of the same name, like the Fabric/Quilt
   bands. Forge's loader-awareness in the generator is floor rows only — no
   newest-Java coverage rows, no lean/full split: the Forge jars' own
   bytecode floors (mc116 = 8, legacy = 17 uniform, modern/eventbus7 = 21,
   with eventbus7's 26.x half running 25 because those servers require it)
   are what matter, and the forward-JVM coverage-row pattern is a Fabric-jar
   concept that must not be reused with `loader: forge`. Per-leg rationale
   (why the modern band is edges-only, why the legacy/mc116/eventbus7 bands
   list every measured version, how 1.16.4 rides the mc116 leg via the
   install-time ModLauncher drop-in) lives in the generator's Forge stage
   comment.
4. **NeoForge stages** — two jobs, one per shipped NeoForge line
   (1.21.1/java21 and 26.2/java25), each a normal `e2e-stage.yml` call with a
   **literal** one-element version list. See "The NeoForge stages" below.
5. **Band stages** — one reusable submatrix call (`e2e-stage.yml`) per
   {band, Java, loader} triple: mc121, mc26, T0 (1.20.3-1.20.6), mc1192,
   mc114. Loader is a `uses:`-time input, not a dimension inside
   `e2e-stage.yml`'s own matrix — every band therefore has TWO separate
   `ci.yml` job entries (`-fabric`/`-quilt` suffix), so the Actions UI
   renders fabric and quilt as two independent, side-by-side job groups
   instead of interleaved rows in one shared group. The fabric/quilt axis is
   orthogonal to band/version generation - both loader variants of a band
   read the exact same `needs.contracts.outputs.*` version list, they just
   run as separate jobs. Each band's `needs:` lists both loader variants of every prior
   band, so stage ordering (popularity-first) still holds across both
   loaders; the two loader variants of the same band run fully in parallel
   with no dependency between them. `e2e-stage.yml`'s `loader` input
   generalized to `forge` for free; it gained one conditional step that
   moves the Forge jar from `build/libs` into `forge/build/libs` before the
   run, since the two Gradle builds place their output in different
   directories.

Lean grid on `pull_request` (floor rows exhaustive, newest-Java coverage rows
only at each band's ends), full cross-product on `workflow_dispatch`.
Rationale: Minecraft breaks are per-patch, JVM breaks are per-JVM, so a
band's ends cover the real variable on higher JVMs.

Stages chain via `needs:`. Each stage's job body is defined once, in
`e2e-stage.yml`, and reused by every stage — only the version list per stage
differs, sourced from `tools/gen_matrix.go`.

## The NeoForge stages

Two things about them are deliberate and worth not "fixing":

- **`tools/gen_matrix.go` is not involved.** Every other stage reads its version
  list from a generator output; the NeoForge jobs carry a literal
  `'["1.21.1"]'` / `'["26.2"]'`. NeoForge covers exactly two Minecraft versions
  because one NeoForge jar covers exactly one Minecraft version (see
  [version-matrix.md](version-matrix.md) → "NeoForge"), so this is the first
  thing in the project that makes the loader axis *non*-orthogonal to version
  generation. Teaching the generator a filtered, loader-dependent list to emit
  two constants is more machinery than the constants. Revisit if the list grows.
- **NeoForge is not in `e2e-gate`.** The gate exists so a broken build costs a
  handful of jobs instead of the whole fan-out; putting NeoForge there would let
  a NeoForge-only break block ~40 Fabric/Quilt jobs that have nothing to do with
  it. Its own job group also matches the reason Quilt got one: two clearly
  separate, independently-collapsible groups in the Actions UI.

Both jobs `need` `contracts`, `unit-tests`, all five build jobs, and
`e2e-gate`, and use the same `if:` guard as every other stage minus the
`!= '[]'` clause, which cannot fire on a literal list — plus the
`github.event_name != 'push'` guard, since their literal lists never go
empty on push the way the generated bands do. They are not in any other
job's `needs:`, so the popularity-first band ordering is untouched and they
run in parallel with it.

`build-fabric-neoforge` uploads all six jars in one artifact;
`e2e-stage.yml`'s "Verify prebuilt jars"
step checks for all six, so a jar that silently failed to build fails the stage
before a server boots rather than surfacing as `mod-not-loaded` later.

## e2e server images

`e2e-images` (called by `make e2e`/`make e2e-ci`) publishes each per-Java
server image to
`ghcr.io/ashwalk33r/commandsspy-e2e:java<N>-<variant>-<content-hash>`,
same pull-or-build-push pattern as `Dockerfile.ci`/`CI_IMAGE`: `docker image
inspect` (local) → `docker pull` (GHCR) → `docker build` + best-effort
`docker push` in CI, falling through on any miss. The hash covers
`Dockerfile`, `scripts/e2e-entrypoint.sh`, and every tracked file under
`tools/` — the inputs that actually determine the image's content — so it
invalidates correctly on a source change and never serves a stale image.
`<variant>` (`alpine` or `jammy`, decided by the `Makefile`'s `e2e-images`
recipe, not by the hashed files) is baked into the tag string itself rather
than into the hash, so changing which Java floors use which base still
busts exactly the right tags. Every job that calls `e2e-images` (`e2e-gate`
and every `e2e-stage.yml` caller) needs `permissions: packages: write` plus
a GHCR login step: reusable-workflow (`workflow_call`) permissions only
ever *downgrade* from caller to callee, never elevate, so both `ci.yml`'s
calling jobs and `e2e-stage.yml`'s `run` job need the grant, not just one
side.

Java floors 21/25/26 build on `eclipse-temurin:<N>-jre-alpine`; floors
8/11/17 stay on `-jre-jammy` — those three lack an arm64 alpine tag, which
would force local arm64 dev under qemu for no matching win. `bash` is
installed alongside `curl` on the alpine path: `scripts/e2e-entrypoint.sh`
has a `#!/bin/bash` shebang and the alpine Temurin JRE ships no bash by
default.

Measured (cold, no cached base images or build cache — the state a fresh
Actions runner starts from): `docker build --no-cache` averaged ~13.8s
(java21, 3 samples) and ~14.2s (java8, 2 samples); pulling an equivalent
already-built image averaged ~2.0s and ~3.9s respectively — roughly
75-85% faster per job, avoiding a redundant `golang:1.24-alpine` pull,
`apt-get update`/`apk add`, and `go build` on every one of the ~40 e2e
matrix jobs a PR that doesn't touch the Dockerfile/`tools/` would
otherwise pay for. The alpine swap itself measured ~10-36% faster
`docker build` and ~23% faster `docker pull` (java17) over the equivalent
jammy image, plus ~26-32% smaller final image size, on top of the
pull-vs-build saving above.

## The grid generator contract

`tools/gen_matrix.go` is the single source of stage definitions; every
submatrix reads its version list from an output it emits. Adding or removing
a band is a change to that file plus one `uses:` block in `ci.yml`.

Two non-obvious rules it must keep:

- **Every output name is emitted on every run**, as the literal `[]` when the
  band does not exist. A missing GitHub output evaluates to `''`, and
  `'' != '[]'` is true, which would feed `fromJSON('')` to a matrix and
  hard-error the run. The `[]` literal keeps the `!= '[]'` skip guard honest.
- **The gate canaries are moved to the gate, never duplicated** in the band
  lists.
- **Forge bands emit floor rows only** (`forge_java21`, `forge_legacy_java17`,
  `forge_mc116_java8`, `forge_eventbus7_java21`,
  `forge_eventbus7_java25`) — no coverage rows, no lean/full split. Presence
  is keyed off the `minecraft_range_modern`/`_legacy`/`_mc116`/`_eventbus7`
  lines in `forge/gradle.properties`; another Forge band is one more
  range-key case, one emit, and one `uses:` block. Within `forge_java21`,
  1.20.4 is keyed on the *legacy* band — it boots the legacy jar (see the
  rationale comment in `tools/gen_matrix.go`).

Each band job's `if:` guard is `!cancelled() && no needed job failed &&
list != '[]'` — plain `success()` would skip the band when an unrelated
sibling failed, and `fromJSON` on an empty string would kill the run.

Job counts per band and trigger are pinned in `tools/gen_matrix_test.go`;
`tools/floors_test.go` pins the Java floors against
`scripts/e2e-run-one.sh` (and the Forge rows against
`--print-forge-routing`). Change the grid → those tests name the new numbers.
`TOTAL_JOBS = 2 x fabric pairs + forge pairs + 16`: every fabric band key
feeds two caller jobs (`-fabric` and `-quilt`), Forge keys feed one
(single-loader), and the 16 fixed jobs are contracts, go-quality,
lint-java, unit-tests, the 5 build jobs, the `Build` aggregator, the 4
e2e-gate canaries (2 versions x fabric/quilt), and the 2 literal NeoForge
jobs. On `pull_request` that is 2x39 + 30 + 16 = 124 jobs; on push only 10
of the fixed jobs run (the gate and NeoForge jobs are event-skipped) and
every band is empty.

Grid policy: every version runs on its own floor JVM. Newest-Java coverage
rows sample only the band's ends on `pull_request` (lean) and the whole band
on `workflow_dispatch` (full) — floors and full rationale:
[version-matrix.md](version-matrix.md).

`tools/gen_matrix.go` env vars:

| Var | Meaning |
|---|---|
| `EVENT_NAME` | `push` \| `pull_request` \| `workflow_dispatch` (default: `pull_request`); `push` emits `[]` for every band |
| `GITHUB_OUTPUT` | file to append `name=json` lines to (optional) |
| `FORCE_BANDS` | space-separated band names to treat as present (testing) |
| `REPO_ROOT` | repo root (default: current working directory) |
