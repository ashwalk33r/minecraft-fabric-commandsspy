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
and JDK 25 (for the 26.x toolchain) alongside Go and
shellcheck, so
`make build`, `make test`, `make lint-java`, and `make ci` together need only
`make` and `docker` on the host, locally and in CI. There is no
Docker-unavailable escape hatch for these three (unlike `ci-host`): run
`./gradlew` directly against a local JDK instead.

`make build` builds all five jars (four Fabric/Quilt eras + the single
NeoForge band jar, Minecraft 1.20.2-26.2); the Forge jars (a separate Gradle build in `forge/`) are
`make build-forge`/`make build-forge-legacy`/`make build-forge-mc116`/
`make build-forge-eventbus7`, on demand — not part of the default
`make build`/`make ci` path.

## ci.yml — the one workflow

One workflow, `CI` (`ci.yml`), replaced the old `Build` (`gradle.yml`) +
`E2E` (`e2e.yml`) pair, which duplicated `make test` and `make build` on
every PR. Triggers: push to `main`, `pull_request`, `workflow_dispatch`
(the deep sweep — see [Grid policy](#grid-policy) below); a
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
`EVENT_NAME=push` (the NeoForge legs included, since they are generated like
every other band), and `e2e-gate` plus the four config-behaviors legs, whose
version lists are literal and so never go empty, carry an explicit
`github.event_name != 'push'` guard.

## The staged e2e matrix

Stage order is popularity order: a failure in a widely-run version surfaces
before runner minutes are spent on the long tail.

1. **Tier 1: ten parallel build jobs, one jar each** (`needs: [contracts]`),
   replacing the old sequential ~25-minute `build-jars` job:
   `build-mc121x`/`build-mc1192`/`build-mc114x`/`build-mc26x` (per-era
   `make build-121`/`-1192`/`-114`/`-26`), `build-neo`
   (the one NeoForge band jar), and `build-forge-modern`, `build-forge-legacy`,
   `build-forge-mc116`, `build-forge-eventbus7` — each Forge target a
   separate Gradle build (`forge/`), all in the same pinned CI image — and
   `build-babric` (the one Babric jar, from the separate `babric/` build; see
   item 7). One
   artifact per job:
   `commandsspy-jar-mc1.21.x-<sha>`, `commandsspy-jar-mc1.19-1.20.2-<sha>`,
   `commandsspy-jar-mc1.14.x-<sha>`, `commandsspy-jar-mc26.x-<sha>`,
   `commandsspy-jar-neoforge-<sha>`,
   `commandsspy-jar-forge-modern-<sha>`,
   `commandsspy-jar-forge-legacy-<sha>`,
   `commandsspy-jar-forge-mc116-<sha>`,
   `commandsspy-jar-forge-eventbus7-<sha>`, `commandsspy-jar-babric-<sha>`;
   retention 30 days on push,
   1 day otherwise. The download side is unchanged: stage jobs fetch with
   pattern `commandsspy-jar-*-<sha>` + `merge-multiple`, so the split is
   invisible to them. Gradle cache keys are per-target
   (`ci-gradle-<target>-<hash>`) on purpose: a single shared key is saved
   by whichever job finishes first, so the other targets' (ForgeGradle
   decompile) caches would never persist and every run would rebuild cold.
   `forge/build.gradle` and `forge/gradle.properties` are in every key
   alongside the root build files, because the first Forge build of each
   target decompiles Minecraft and is slow on a cold cache.
2. **e2e-gate** — `needs` all four Tier 0 gates and all ten build jobs:
   two canary pairs (1.21.11/java21, 26.2/java25), each
   crossed with `loader: fabric` and `loader: quilt` via `matrix.include`, so
   four canary jobs run. `fail-fast` is off so all four always report.
3. **Forge stages** — six caller jobs (`e2e-forge-java21`,
   `e2e-forge-legacy-java17`, `e2e-forge-mc116-java8`,
   `e2e-forge-eventbus7-java21`, `e2e-forge-eventbus7-java25`,
   `e2e-forge-java26`), each a normal
   `e2e-stage.yml` call with `loader: forge` reading its version list from a
   `tools/gen_matrix.go` output of the same name, like the Fabric/Quilt
   bands. Forge's loader-awareness in the generator is floor rows plus **one**
   forward-JVM row, and no lean/full split: the Forge jars' own
   bytecode floors (mc116 = 8, legacy = 17 uniform, modern/eventbus7 = 21,
   with eventbus7's 26.x half running 25 because those servers require it)
   are what decide the floor rows, and the Fabric per-band coverage-row
   pattern — a whole band re-run one JVM up — still must not be reused with
   `loader: forge`. What `forge_java26` adds is narrower: one above-floor
   boot for the eventbus7 era (26.2 on java 26). The modern band gets none:
   its bootstrap cannot start on **java 24+** at all (measured boot-failed on
   both 25 and 26), because `nimbus-jose-jwt` requires `jdk.crypto.ec`,
   removed from the JDK in 24 (issue #66) — the same class of JVM-internals
   breakage as the
   `ManifestEntryVerifier` case, found by the row added to look for it.
   Bytecode binds downward but JVM *internals* do not: Forge 35.x cannot boot
   a stock current JDK 8 at all (issue #58, the `ManifestEntryVerifier` case
   in the wiki, [Forge `modern` is Java 21 only](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/Supported-Versions#forge-modern-is-java-21-only)). Per-leg rationale
   (why the modern band is the two edges plus 1.21.1 and nothing else, why
   the legacy/mc116/eventbus7 bands list every measured version, how 1.16.4
   rides the mc116 leg via the install-time ModLauncher drop-in) lives in
   the generator's Forge stage comment.
4. **NeoForge stages** — four jobs (`e2e-neoforge-java17`,
   `e2e-neoforge-java21`, `e2e-neoforge-java25`, `e2e-neoforge-fwd-java25`),
   each a normal
   `e2e-stage.yml` call with `loader: neoforge` reading its version list from
   the `tools/gen_matrix.go` output of the same name. One band jar serves all
   four; the first three split on NeoForge's **own** Java floor (17 up to line
   20.4, 21 through 21.11, 25 on 26.x — `scripts/e2e-run-one.sh
   --print-neo-routing`, not the Fabric era table) and the fourth boots
   1.21.1, a java-21 line, on java 25 — the loader's only above-floor leg
   (#58). See "The NeoForge stages" below.
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

6. **config-behaviors stages** — four caller jobs
   (`e2e-config-behaviors-fabric`, `e2e-config-behaviors-quilt`,
   `e2e-config-behaviors-forge`, `e2e-config-behaviors-neoforge`), one per
   loader, each a normal `e2e-stage.yml` call with `CONFIG_VARIANT=1` and a
   pinned single-version list. These legs seed `config/commands-spy.json`
   before boot and assert config-driven behaviors (blacklist suppression,
   `logArgs` handling, autocreation of a missing config) rather than the
   default leg's command-logging path; they report their own verdict line
   (`E2E <version> PASS config-behaviors`, see [e2e-harness.md](e2e-harness.md))
   and skip the player-bot phase since no player assertion runs on this leg.

7. **Babric stage** — one caller job, `e2e-babric-java21` ("e2e b1.7.3 java 21
   (babric)"), a normal `e2e-stage.yml` call with `loader: babric` reading the
   `babric_java21` output of `tools/gen_matrix.go`. Its jar comes from its own
   Tier 1 build job, `build-babric` ("Build: Babric (MC b1.7.3)"), a separate
   Gradle build in `babric/` for the same reason `forge/` and `neoforge/` are
   separate — a different plugin stack (ploceus + Loom remap), a different
   mappings namespace, and a reverse-conversion step the root build has no
   notion of. It uploads `commandsspy-jar-babric-<sha>` and caches on
   `ci-gradle-babric-<hash>` over `babric/`'s own build files. The Babric band
   is single-version by construction — b1.7.3 is the only Minecraft version the
   loader exists for — so `declared`, `sampled` and `deep` are the same
   one-element list and the `pull_request` and `workflow_dispatch` grids for
   this band are identical. That is the intended shape for a single-version
   Tier 1 band, not a missing deep sweep.

Lean grid on `pull_request` (floor rows boot each band's sample, newest-Java
coverage rows only that sample's ends), deep sweep on `workflow_dispatch`
(floor rows boot each band's whole declared range bar the written exclusions;
coverage rows widen to the whole sample). Rationale: Minecraft breaks are
per-patch, JVM breaks are per-JVM, so a band's ends cover the real variable on
higher JVMs. Which versions are in the sample, which are deep-only, and why a
declared version is in neither, are data in `tools/gen_matrix.go`'s coverage
table — see the wiki, ["The denominator, settled"](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/Supported-Versions#the-denominator-settled).

Stages chain via `needs:`. Each stage's job body is defined once, in
`e2e-stage.yml`, and reused by every stage — only the version list per stage
differs, sourced from `tools/gen_matrix.go`.

## The NeoForge stages

The legs are **generated**, exactly like the Forge bands: `neo_java17`,
`neo_java21`, `neo_java25` and `neo_fwd_java25` come out of
`tools/gen_matrix.go`, keyed on the
`minecraft_range_neo_all` line in `neoforge/gradle.properties`, and each leg is
an ordinary `e2e-stage.yml` call reading `needs.contracts.outputs.*`. The first
three are
floor rows, not a cross-jar stability proof: one band jar covers 1.20.2-26.2, so
there is no second jar whose overlap could drift. What those three lists sample
is
the band's edges (1.20.2, 26.2), the three Java floors NeoForge itself changes
at, and 1.21.1 as the modpack-dominant interior line. `neo_fwd_java25` is the
one above-floor leg (#58): 1.21.1, a java-21 line, booted on java 25, because
until it existed the band jar's "java-17 bytecode boots anywhere across the
17/21/25 spread" claim was asserted only at the floors themselves. Per-row
rationale lives in
the generator's NeoForge stage comment. They carry the standard band `if:`
guard including the `!= '[]'` clause and need no `github.event_name != 'push'`
guard — a generated list is already `[]` on push.

**NeoForge is still not in `e2e-gate`,** and that part is deliberate. The gate
exists so a broken build costs a handful of jobs instead of the whole fan-out;
putting NeoForge there would let a NeoForge-only break block ~40 Fabric/Quilt
jobs that have nothing to do with it. Its own job groups also match the reason
Quilt got one: separate, independently-collapsible groups in the Actions UI.
The three legs `need` `contracts`, `unit-tests`, all ten build jobs and
`e2e-gate`, and are in no other job's `needs:`, so the popularity-first band
ordering is untouched and they run in parallel with it.

Each build job uploads its one jar as its own artifact, and the `Build`
aggregator prints one grouped log with every artifact link;
`e2e-stage.yml`'s "Verify prebuilt jars"
step checks for all five (the four Fabric/Quilt eras plus the NeoForge band),
so a jar that silently failed to build fails the stage
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
- **Forge and NeoForge bands emit floor rows** (`forge_java21`,
  `forge_legacy_java17`, `forge_mc116_java8`, `forge_eventbus7_java21`,
  `forge_eventbus7_java25`, `neo_java17`, `neo_java21`, `neo_java25`) **plus
  one forward-JVM row each** (`forge_java26`, `neo_fwd_java25`) — no per-band
  coverage rows, no lean/full split: the above-floor rows run on every event,
  because the drift they catch comes from the JVM, not from the PR under
  test. `neo_fwd_java25` is deliberately outside the `neo_java<N>` naming, so
  that `tools/floors_test.go` can keep pinning every `neo_java<N>` row to
  "every version here has NeoForge floor exactly N" while asserting the
  opposite — a floor strictly below 25 — for the forward row.
  Presence is keyed off the
  `minecraft_range_modern`/`_legacy`/`_mc116`/`_eventbus7` lines in
  `forge/gradle.properties` and `minecraft_range_neo_all` in
  `neoforge/gradle.properties`; another band is one more
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
`TOTAL_JOBS = 2 x fabric pairs + forge pairs + neo pairs + a fixed-job
constant`: every fabric band key feeds two caller jobs (`-fabric` and
`-quilt`), Forge and NeoForge keys feed one (single-loader — neither has a
Quilt twin). The NeoForge e2e legs are not among the fixed jobs — they are
generated pairs. The constant, its per-job breakdown and the smaller count
left on push (where the gate canaries, the config-behaviors legs and the
refusal guards are event-skipped, and every band is empty) are enumerated in
the comment above `fixedJobs` in `gen_matrix.go`; the totals themselves are
the pins in `gen_matrix_test.go`. Run the generator for the current numbers.

The dispatch figure jumped when `workflow_dispatch` became a real deep sweep.
Every one of those extra jobs is a Minecraft version the pull-request grid
never boots; the pull-request figure did not move.

The fixed count went 25 -> 22 when the NeoForge legs were generated, and is
worth spelling out because two separate things shrank it: the two per-version
NeoForge build jobs collapsed into the one `build-neo` band job (-1), and the
two literal NeoForge e2e jobs became generated pairs counted in the grid
instead (-2). It went back to 24 when the two out-of-range refusal guards
landed.

## Grid policy

Every version runs on its own floor JVM. Newest-Java coverage rows sample only
the band's ends on `pull_request` (lean) and the whole sample on
`workflow_dispatch` (deep). The floor rows differ by event too: the sample on
`pull_request`, the band's whole declared range minus its written exclusions on
`workflow_dispatch`. Floors and full rationale:
the wiki's [Version boundaries and root causes](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/Version-Boundaries-And-Root-Causes), and
["The denominator, settled"](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/Supported-Versions#the-denominator-settled) for the
declared-versus-proven distinction and the offline checks that enforce it.

There is deliberately no `schedule:` trigger. A cron sweep would need someone
watching it: GitHub disables scheduled workflows after 60 days of repository
inactivity, hobby-tier cron runs queue unpredictably, and an unattended sweep
that fails with no alerting turns a known gap into a false green. The deep
sweep is a button instead.

`tools/gen_matrix.go` env vars:

| Var | Meaning |
|---|---|
| `EVENT_NAME` | `push` \| `pull_request` \| `workflow_dispatch` (default: `pull_request`); `push` emits `[]` for every band, `workflow_dispatch` runs the deep sweep |
| `GITHUB_OUTPUT` | file to append `name=json` lines to (optional) |
| `FORCE_BANDS` | space-separated band names to treat as present (testing) |
| `REPO_ROOT` | repo root (default: current working directory) |

`go run . gen-matrix --coverage` dumps the coverage table itself as
`band<TAB>state<TAB>version<TAB>reason`, which is how
`scripts/test-jar-routing.sh` re-probes the exclusions instead of restating
them.

## Publishing: the version lists are generated

`go run . gen-matrix --publish` prints one
`jar<TAB>loaders<TAB>game_versions` row per Modrinth version, and
`docs/modrinth-versions.tsv` is that output committed. **Publish from that
file** — copy each row's third column into the matching Modrinth version's
game_versions field. Never hand-carry the lists from the previous release: that
is how issue #84 happened, 40 of 1.7.0's 113 published loader-and-version claims
backed by no CI leg and one (Quilt `1.14`) not installable at all.

Two rules the file encodes, so the release does not have to re-decide them:

- `game_versions` is the jar's **declared range** — every Mojang release the
  loader will accept it on — which since #84 is also the set the deep sweep
  boots. A version is dropped only when the loader project published no build
  for it, because then there is nothing to install.
- `mc1.14.x` is uploaded **twice** — the same jar file, two Modrinth versions:
  `<ver>+mc1.14.x` tagged `fabric,quilt`, and `<ver>+mc1.14.x-fabric` tagged
  `fabric`, carrying the four versions Quilt Loader has no build for. Modrinth
  cannot exclude one loader from one version, and its version numbers must be
  distinct, so the `-fabric` suffix in the first column of the snapshot IS the
  version number to type. Eleven rows, ten jar files.

Regenerate after any change to a declared range or the coverage table
(`gen_matrix_test.go` fails until you do):

```
cd tools && REPO_ROOT=.. go run . gen-matrix --publish > ../docs/modrinth-versions.tsv
```

Caveats that belong in the Modrinth **description**, not in a version list:
`mc1.21.x-forge` is Java 21 only (Java 25+ crashes before Minecraft starts,
issue #66), and the NeoForge lines whose newest build is a `-beta` install fine
but are not booted by CI.
