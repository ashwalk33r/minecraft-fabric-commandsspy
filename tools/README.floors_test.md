# floors_test.go

Guards the Minecraft-version-to-Java-floor mapping against silent drift.

## The problem it solves

The real Java-floor table lives in one place: `scripts/e2e-run-one.sh`.
Each Minecraft version needs a minimum ("floor") Java version, e.g. 1.21.x
needs Java 21, 26.x needs Java 25, 1.16.5 needs Java 8.

If someone edits the script's table, nothing else would notice. This test
hardcodes a copy of the expected floors on purpose. A mismatch turns a
silent change into a test failure a human has to look at.

## Test functions

### TestPrintJavaFloors

Runs `bash scripts/e2e-run-one.sh --print-java <version>` for every version
in the Makefile's default VERSIONS list (25 versions, in
`defaultVersionFloors`). Fails if the script prints a Java version that
differs from the hardcoded expectation.

### TestGenMatrixFloorRowsAgreeWithPrintJava

Cross-checks the CI matrix generator against the script. It calls
`genMatrix` (from `gen_matrix.go`) in-process, parses its `name=[...]`
GitHub-output lines, and looks only at the six floor rows
(`mc121_java21`, `mc26_java25`, `t0_java21`, `mc1192_java17`,
`mc114_java8`, `mc114_java17`).

For every Minecraft version listed in a floor row, it asks the script
`--print-java` and asserts the answer matches the Java version in the
row's name. It also fails if any expected floor row is missing from the
generator's output.

Coverage rows (runs on newer Java than the floor) are intentionally not
checked. Forge and NeoForge rows are also not in `floorRows`: `--print-java`
is the Fabric floor table, the Forge jars carry their own bytecode floors,
and NeoForge carries its own per-line floors.

### TestGenMatrixForgeRowsAgreeWithForgeRouting

The Forge analogue, against `--print-forge-routing` instead of
`--print-java`: every version the generator emits in `forge_legacy_java17`
must route "legacy 0"; `forge_java21` versions route "modern 0" except
1.20.4 ("legacy 0" — the legacy jar's ceiling riding in the modern job);
`forge_eventbus7_java21`/`forge_eventbus7_java25` versions route
"eventbus7 0"; `forge_mc116_java8` versions route "mc116 0" — 1.16.4
included, made known-good by `e2e-run-one.sh`'s install-time ModLauncher
8.1.3 drop-in (see docs/version-matrix.md).

### TestGenMatrixNeoRowsAgreeWithNeoRouting

The NeoForge analogue, against `--print-neo-routing` (which prints
`<neoforge-build> <java-floor>`, or `unsupported 0` for a Minecraft version
with no NeoForge line). Every version in `neo_java17` / `neo_java21` /
`neo_java25` must report the floor its row name promises — NeoForge's own
floors, which differ from the Fabric table (1.20.4 is `--print-java` 21 but a
NeoForge Java-17 line). All three rows must be non-empty, and there must be
exactly three; only floors are checked, since the one band jar serves them
all.

## How to run

```sh
make go-test                 # full suite: go test -race -count=1 -cover ./...
cd tools && go test -run 'Floor|Routing' -v   # just these four tests
```

Requires `bash` and a POSIX environment, because the tests shell out to
`scripts/e2e-run-one.sh` via the relative path `../scripts/` — run them
from inside `tools/`.
