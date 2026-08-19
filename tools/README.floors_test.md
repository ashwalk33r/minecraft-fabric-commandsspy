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
in the Makefile's default VERSIONS list (24 versions, in
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
checked. Forge rows are also not in `floorRows`: `--print-java` is the
Fabric floor table, and the Forge jars carry their own bytecode floors.

### TestGenMatrixForgeRowsAgreeWithForgeRouting

The Forge analogue, against `--print-forge-routing` instead of
`--print-java`: every version the generator emits in `forge_legacy_java17`
must route "legacy 0"; `forge_java21` versions route "modern 0" except
1.20.4 ("legacy 0" — the legacy jar's ceiling riding in the modern job);
`forge_eventbus7_java21`/`forge_eventbus7_java25` versions route
"eventbus7 0"; the 1.16.5 guard routes "modern 1" (outside every jar's
range, expect refused).

## How to run

```sh
make go-test                 # full suite: go test -race -count=1 -cover ./...
cd tools && go test -run Floor -v   # just these two tests
```

Requires `bash` and a POSIX environment, because the tests shell out to
`scripts/e2e-run-one.sh` via the relative path `../scripts/` — run them
from inside `tools/`.
