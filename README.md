[![Build](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/actions/workflows/gradle.yml/badge.svg)](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/actions/workflows/gradle.yml)

# CommandsSpy

A server mod that logs every executed command with its source (player, console,
RCON, function, command block). One shared implementation ships as four
era-correct Fabric/Quilt jars covering Minecraft 1.14.4–26.2, plus two NeoForge
jars for Minecraft 1.21.1 and 26.2.

User documentation: [MOD.md](./MOD.md). Official releases:
[Modrinth](https://modrinth.com/mod/commandsspy/versions).

## Docs

- [docs/version-matrix.md](docs/version-matrix.md) — the six jars, the loader
  seam in the shared core, version boundaries, Java floors, default e2e matrix.
- [docs/testing.md](docs/testing.md) — unit suite: how it boots, isolation,
  known quirks, era-specific wiring.
- [docs/e2e-harness.md](docs/e2e-harness.md) — Docker e2e: phases,
  assertions, verdict codes, jar cache, routing drift protection.
- [docs/ci.md](docs/ci.md) — `make ci` gate, workflows, staged e2e grid.
- [docs/protocol-table.md](docs/protocol-table.md) — the Go bot's per-version
  protocol reference.

## Build

`make build` needs only `make` and `docker` — it runs Gradle (JDK 21, plus
JDK 25 for the 26.x and NeoForge 26.2 targets) inside the pinned image from
`Dockerfile.ci`, the same image `make ci` already used for Go/shellcheck. No
local JDK required.

```bash
make build                      # all six jars, dockerized
```

The first NeoForge build of each line runs ModDevGradle's NeoForm pipeline
(decompile + recompile Minecraft, ~8-9 minutes); it is cached in
`GRADLE_USER_HOME` afterwards. Only `LOADER=neoforge` e2e runs depend on those
jars, so a Fabric or Quilt run never pays for it.

For IDE use (IntelliJ: reload Gradle projects, then `./gradlew genSources`)
or a one-off manual build, a local JDK 21+ still works directly against the
wrapper; era toolchains (e.g. JDK 25 for 26.x) are then auto-provisioned:

```bash
./gradlew build                 # mc1.21.x jar   (1.20.3–1.21.11, Java 21 bytecode)
./gradlew build -PmcTarget=1192 # mc1.19-1.20.2  (1.19.1–1.20.2, Java 17 bytecode)
./gradlew build -PmcTarget=114  # mc1.14.x       (1.14–1.18.2, Java 8 bytecode)
./gradlew build -PmcTarget=26   # mc26.x         (26.1–26.2)
```

`neoforge/` is a standalone Gradle build (ModDevGradle and Fabric Loom are not
supported in one project), driven with `-p`:

```bash
./gradlew -p neoforge build -PneoTarget=121 # +neoforge-mc1.21.1 (NeoForge 21.1.x)
./gradlew -p neoforge build -PneoTarget=26  # +neoforge-mc26.2   (NeoForge 26.2.x)
```

All six jars land in `build/libs/`. An unknown `-PmcTarget=`/`-PneoTarget=`
fails the build with the list of supported values. Bare `make` prints the full
target catalog.

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

`make e2e` boots every supported version as a real server in Docker and asserts
console, RCON, and player command logging — including a negative cross-check
that a silent second player is never attributed a command.

```bash
make e2e                            # full default matrix, all at once
make e2e VERSIONS="1.21 26.2"       # quick check
make e2e-ci                         # bounded variant (PARALLEL=4)
make e2e VERSIONS="1.21.11" JAVA=25 # prove a newer JVM
make e2e LOADER=quilt               # same assertions on Quilt Loader
make e2e VERSIONS="1.21.1" LOADER=neoforge JAVA=21  # NeoForge 21.1.x
make e2e VERSIONS="26.2"   LOADER=neoforge JAVA=25  # NeoForge 26.2.x
make e2e-times                      # per-version boot times
make clean-e2e                      # remove logs, results, containers, images
```

Knobs: `VERSIONS`, `PARALLEL` (alias `J`), `BOOT_TIMEOUT` (default 180s),
`JAVA` (override JVM; a version has a Java floor, not a pin), `LOADER`
(`fabric` | `quilt` | `neoforge`). Output:
`build/e2e-logs/<key>.log` and `build/e2e-results/<key>.result`; any failure
makes the run exit non-zero. Harness internals and verdict codes:
[docs/e2e-harness.md](docs/e2e-harness.md). The CI grid (canary gate, staged
bands, lean vs full shapes): [docs/ci.md](docs/ci.md).

## Compatibility

### Fabric Loader / Quilt Loader

| Minecraft | Jar | Java | Fabric Loader | Quilt Loader |
| --- | --- | --- | --- | --- |
| 1.14–1.18.2 | mc1.14.x | 8+ | 0.19.3+ | 0.30.0+ |
| 1.19.1–1.20.2 | mc1.19-1.20.2 | 17+ | 0.16.5+ | 0.30.0+ |
| 1.20.3–1.21.11 | mc1.21.x | 21+ | 0.16.5+ | 0.30.0+ |
| 26.1–26.2 | mc26.x | 25+ | 0.19.3+ | 0.30.0+ |

1.19.0 is unsupported. Fabric API is not required. One jar serves both
loaders (Quilt reads its bundled `quilt.mod.json`; Fabric reads
`fabric.mod.json`) — every version above is e2e-tested against real server
boots on both. Why the boundaries sit where they do:
[docs/version-matrix.md](docs/version-matrix.md).

### Forge

Four separately-built jars cover Forge from Minecraft 1.14.4 through 26.2,
one per mapping/EventBus era: `mc116` (1.14-1.16.5, SRG incl. classes,
Java 8), `legacy` (1.17.1-1.20.4, SRG members, Java 17), `modern`
(1.20.6-1.21.5, official names, Java 21) and `eventbus7` (1.21.6-26.2,
EventBus 7). Built on demand: `make build-forge`,
`build-forge-legacy`, `build-forge-mc116`, `build-forge-eventbus7`. Details,
measured boot tables, and why each boundary sits where it does:
[docs/version-matrix.md](docs/version-matrix.md) -> the Forge sections.
Note for 1.16.4 admins: stock Forge 35.x cannot boot a current JDK 8
(upstream ModLauncher/JDK 8u321+ issue) — swap the install's ModLauncher
8.0.x jar for 8.1.3, as CI does; the mod then passes the full e2e suite
(docs/version-matrix.md, mc116 gate 1).

### NeoForge

| Minecraft | Jar | Java | NeoForge |
| --- | --- | --- | --- |
| 1.21.1 | neoforge-mc1.21.1 | 21+ | 21.1.x |
| 26.2 | neoforge-mc26.2 | 25+ | 26.2.x |

**One NeoForge jar covers exactly one Minecraft version** — NeoForge publishes
one version line per Minecraft version and has no Intermediary-equivalent stable
mapping to ride, so it cannot span a range the way the Fabric jars do. Minecraft
1.20.2 is NeoForge's own permanent floor; 1.20.1 and below are MinecraftForge or
nothing. Both jars are e2e-tested against real NeoForge server boots. They use
NeoForge's native `CommandEvent` rather than a mixin — same hook point, same
coverage. Details and the ongoing cost:
[docs/version-matrix.md](docs/version-matrix.md) → "NeoForge".
