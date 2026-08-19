# CI

## The one gate: `make ci`

`make ci` is the single fast quality gate — the pre-commit hook and CI both
run exactly it, inside the pinned `Dockerfile.ci` container, so a local pass
means a CI pass. It covers shell lint and Go format/vet/lint/vuln/build/test,
and excludes e2e (that has its own workflow). `make ci-host` is the same
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
and JDK 25 (for the 26.x target's toolchain) alongside Go and shellcheck, so
`make build`, `make test`, `make lint-java`, and `make ci` together need only
`make` and `docker` on the host, locally and in CI. There is no
Docker-unavailable escape hatch for these three (unlike `ci-host`): run
`./gradlew` directly against a local JDK instead.

## gradle.yml

Unit tests run before static analysis on purpose: a behavioural regression
fails the run in the first minute, under a named per-target check. Each of the
four targets (`test114`, `test1192`, `test121`, `test26`) is a named step.
A new push to the same ref cancels the in-flight run.

`scripts/verify-action-pins.sh` asserts every third-party action is pinned to
a full commit SHA.

Reference: [Building and testing Java with
Gradle](https://docs.github.com/en/actions/automating-builds-and-tests/building-and-testing-java-with-gradle)
(GitHub Actions docs) — `gradle.yml` follows this template.

## e2e.yml — staged matrix

Stage order is popularity order: a failure in a widely-run version surfaces
before runner minutes are spent on the long tail.

1. **build-jars + unit-tests** — jars are built once and shared as artifacts;
   the offline routing contract (`scripts/test-jar-routing.sh`) and the grid
   count assertions run here, before anything boots.
2. **e2e-gate** — two canary pairs: 1.21.11/java21 and 26.2/java25. A broken
   build costs 2 e2e jobs instead of the whole fan-out. `fail-fast` is off so
   both canaries always report.
3. **Band stages** — one reusable submatrix (`e2e-stage.yml`) per
   {band, Java} pair: mc121, mc26, T0 (1.20.3–1.20.6), mc1192, mc114. Each
   submatrix additionally crosses every version with a static
   `loader: [fabric, quilt]` dimension inside `e2e-stage.yml` itself — the
   loader axis is orthogonal to band/version generation
   (`tools/gen_matrix.go` has no concept of it), so every `uses:` call site
   below doubles automatically with no per-band edits. `e2e-gate`'s two
   canary pairs become four the same way.

Lean grid on `pull_request` (floor rows exhaustive, newest-Java coverage rows
only at each band's ends), full cross-product on `workflow_dispatch`.
Rationale: Minecraft breaks are per-patch, JVM breaks are per-JVM, so a
band's ends cover the real variable on higher JVMs.

Stages chain via `needs:`. Each stage's job body is defined once, in
`e2e-stage.yml`, and reused by every stage — only the version list per stage
differs, sourced from `tools/gen_matrix.go`.

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
ever *downgrade* from caller to callee, never elevate, so both `e2e.yml`'s
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
a band is a change to that file plus one `uses:` block in `e2e.yml`.

Two non-obvious rules it must keep:

- **Every output name is emitted on every run**, as the literal `[]` when the
  band does not exist. A missing GitHub output evaluates to `''`, and
  `'' != '[]'` is true, which would feed `fromJSON('')` to a matrix and
  hard-error the run. The `[]` literal keeps the `!= '[]'` skip guard honest.
- **The gate canaries are moved to the gate, never duplicated** in the band
  lists.

Each band job's `if:` guard is `!cancelled() && no needed job failed &&
list != '[]'` — plain `success()` would skip the band when an unrelated
sibling failed, and `fromJSON` on an empty string would kill the run.

Job counts per band and trigger are pinned in `tools/gen_matrix_test.go`;
`tools/floors_test.go` pins the Java floors against
`scripts/e2e-run-one.sh`. Change the grid → those tests name the new numbers.

Grid policy: every version runs on its own floor JVM. Newest-Java coverage
rows sample only the band's ends on `pull_request` (lean) and the whole band
on `workflow_dispatch` (full) — floors and full rationale:
[version-matrix.md](version-matrix.md).

`tools/gen_matrix.go` env vars:

| Var | Meaning |
|---|---|
| `EVENT_NAME` | `pull_request` \| `workflow_dispatch` (default: `pull_request`) |
| `GITHUB_OUTPUT` | file to append `name=json` lines to (optional) |
| `FORCE_BANDS` | space-separated band names to treat as present (testing) |
| `REPO_ROOT` | repo root (default: current working directory) |
