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
   {band, Java} pair: mc121, mc26, T0 (1.20.3–1.20.6), mc1192, mc114.

Lean grid on `pull_request` (floor rows exhaustive, newest-Java coverage rows
only at each band's ends), full cross-product on `workflow_dispatch`.
Rationale: Minecraft breaks are per-patch, JVM breaks are per-JVM, so a
band's ends cover the real variable on higher JVMs.

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
