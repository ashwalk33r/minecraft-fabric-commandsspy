[![Build](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/actions/workflows/gradle.yml/badge.svg)](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/actions/workflows/gradle.yml)

# CommandsSpy

A Fabric server mod that logs every executed command with its source (player,
console, RCON, function, command block). One shared implementation ships as
four era-correct jars covering Minecraft 1.14.4–26.2.

User documentation: [MOD.md](./MOD.md). Official releases:
[Modrinth](https://modrinth.com/mod/commandsspy/versions).

## Docs

- [docs/version-matrix.md](docs/version-matrix.md) — the four jars, version
  boundaries, Java floors, default e2e matrix.
- [docs/testing.md](docs/testing.md) — unit suite: how it boots, isolation,
  known quirks, era-specific wiring.
- [docs/e2e-harness.md](docs/e2e-harness.md) — Docker e2e: phases,
  assertions, verdict codes, jar cache, routing drift protection.
- [docs/ci.md](docs/ci.md) — `make ci` gate, workflows, staged e2e grid.
- [docs/protocol-table.md](docs/protocol-table.md) — the Go bot's per-version
  protocol reference.

## Build

`make build` needs only `make` and `docker` — it runs Gradle (JDK 21, plus
JDK 25 for the 26.x target) inside the pinned image from `Dockerfile.ci`, the
same image `make ci` already used for Go/shellcheck. No local JDK required.

```bash
make build                      # all four jars, dockerized
```

For IDE use (IntelliJ: reload Gradle projects, then `./gradlew genSources`)
or a one-off manual build, a local JDK 21+ still works directly against the
wrapper; era toolchains (e.g. JDK 25 for 26.x) are then auto-provisioned:

```bash
./gradlew build                 # mc1.21.x jar   (1.20.3–1.21.11, Java 21 bytecode)
./gradlew build -PmcTarget=1192 # mc1.19-1.20.2  (1.19.1–1.20.2, Java 17 bytecode)
./gradlew build -PmcTarget=114  # mc1.14.x       (1.14–1.18.2, Java 8 bytecode)
./gradlew build -PmcTarget=26   # mc26.x         (26.1–26.2)
```

Jars land in `build/libs/`. An unknown `-PmcTarget=` fails the build with the
list of supported values. Bare `make` prints the full target catalog.

## Unit tests

`./gradlew test` runs one target locally against a JDK you already have;
`make test` runs the offline routing contract plus all four targets (each
resolves its own loader and compile level) inside Docker, needing no local
JDK. The suite boots a real FabricLoader via `fabric-loader-junit` and
asserts log output character for character; mixins themselves are proven by
the e2e suite, not here. Details, isolation rules, and the pinned
`FINDING` quirks: [docs/testing.md](docs/testing.md).

CI runs the suite per target before static analysis, and again as the
`unit-tests` job gating the e2e fan-out.

## E2E tests

`make e2e` boots every supported version as a real Fabric server in Docker
and asserts console, RCON, and player command logging — including a negative
cross-check that a silent second player is never attributed a command.

```bash
make e2e                            # full default matrix, all at once
make e2e VERSIONS="1.21 26.2"       # quick check
make e2e-ci                         # bounded variant (PARALLEL=4)
make e2e VERSIONS="1.21.11" JAVA=25 # prove a newer JVM
make e2e-times                      # per-version boot times
make clean-e2e                      # remove logs, results, containers, images
```

Knobs: `VERSIONS`, `PARALLEL` (alias `J`), `BOOT_TIMEOUT` (default 180s),
`JAVA` (override JVM; a version has a Java floor, not a pin). Output:
`build/e2e-logs/<key>.log` and `build/e2e-results/<key>.result`; any failure
makes the run exit non-zero. Harness internals and verdict codes:
[docs/e2e-harness.md](docs/e2e-harness.md). The CI grid (canary gate, staged
bands, lean vs full shapes): [docs/ci.md](docs/ci.md).

## Compatibility

| Minecraft | Jar | Java | Fabric Loader |
| --- | --- | --- | --- |
| 1.14–1.18.2 | mc1.14.x | 8+ | 0.19.3+ |
| 1.19.1–1.20.2 | mc1.19-1.20.2 | 17+ | 0.16.5+ |
| 1.20.3–1.21.11 | mc1.21.x | 21+ | 0.16.5+ |
| 26.1–26.2 | mc26.x | 25+ | 0.19.3+ |

1.19.0 is unsupported. Fabric API is not required. Why the boundaries sit
where they do: [docs/version-matrix.md](docs/version-matrix.md).
