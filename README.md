[![CI](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/actions/workflows/ci.yml/badge.svg)](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/actions/workflows/ci.yml)

# CommandsSpy

A server mod that logs every executed command with its source (player, console,
RCON, function, command block). One shared implementation ships as eleven jars —
four era-correct Fabric/Quilt jars, four Forge jars split by mapping era, a
single NeoForge band jar, a Babric jar for Beta 1.7.3, and a BTA jar for
"Better than Adventure!", the Beta 1.7.3 fork. Which versions each
covers, and which are proven by a booted server in CI: [Supported Versions](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/Supported-Versions).

User documentation: [MOD.md](./MOD.md). Official releases:
[Modrinth](https://modrinth.com/mod/commandsspy/versions).

## Docs

- [Version boundaries and root causes](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/Version-Boundaries-And-Root-Causes) — the eleven jars, the loader
  seam in the shared core, version boundaries, Java floors, default e2e matrix.
- [docs/testing.md](docs/testing.md) — unit suite: how it boots, isolation,
  known quirks, era-specific wiring.
- [docs/e2e-harness.md](docs/e2e-harness.md) — Docker e2e: phases,
  assertions, verdict codes, jar cache, routing drift protection.
- [docs/ci.md](docs/ci.md) — `make ci` gate, workflows, staged e2e grid.
- [docs/protocol-table.md](docs/protocol-table.md) — the Go bot's per-version
  protocol reference.
- [docs/quilt-entrypoint-gap-upstream.md](docs/quilt-entrypoint-gap-upstream.md)
  — an unfiled, paste-ready bug report for quilt-loader's pre-1.18.2 entrypoint
  gap, with two reproduction recipes.

## Build

`make build` needs only `make` and `docker` — it runs Gradle (JDK 21, plus
JDK 25 for the 26.x target) inside the pinned image from
`Dockerfile.ci`, the same image `make ci` already used for Go/shellcheck. No
local JDK required.

```bash
make build                      # all five default jars, dockerized
```

That is the four Fabric/Quilt era jars plus the one NeoForge band jar; the four
Forge jars are built on demand (`make build-forge`, `build-forge-legacy`,
`build-forge-mc116`, `build-forge-eventbus7`), as are the two Beta-1.7.3-era jars
(`make build-babric`, `make build-bta`) — each of those resolves from community
mavens the default build never touches.

The first NeoForge build runs ModDevGradle's NeoForm pipeline (decompile +
recompile Minecraft, ~8-9 minutes); it is cached in `GRADLE_USER_HOME`
afterwards. Only `LOADER=neoforge` e2e runs depend on that jar, so a Fabric or
Quilt run never pays for it.

For IDE use (IntelliJ: reload Gradle projects, then `./gradlew genSources`)
or a one-off manual build, a local JDK 21+ still works directly against the
wrapper; era toolchains (e.g. JDK 25 for 26.x) are then auto-provisioned:

```bash
./gradlew build                 # mc1.21.x jar   (Java 21 bytecode)
./gradlew build -PmcTarget=1192 # mc1.19-1.20.2  (Java 17 bytecode)
./gradlew build -PmcTarget=114  # mc1.14.x       (Java 8 bytecode)
./gradlew build -PmcTarget=26   # mc26.x         (Java 21 bytecode, Java 25 runtime)

# each target's declared range is in gradle.properties (minecraft_range_*)
```

`neoforge/` is a standalone Gradle build (ModDevGradle and Fabric Loom are not
supported in one project), driven with `-p`:

```bash
./gradlew -p neoforge build -PneoTarget=all # +mc1.20.2-26.2-neoforge (the band jar)
```

`all` is the default and the only jar that ships. `-PneoTarget=121`/`=26` still
build single-line jars against NeoForge 21.1.x / 26.2.x; they exist for
bisecting a suspected per-line break, not for release.

All five jars land in `build/libs/`. An unknown `-PmcTarget=`/`-PneoTarget=`
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
make e2e VERSIONS="1.20.2" LOADER=neoforge JAVA=17  # NeoForge band floor
make e2e VERSIONS="1.21.1" LOADER=neoforge JAVA=21  # same jar, mid-band
make e2e VERSIONS="26.2"   LOADER=neoforge JAVA=25  # same jar, band ceiling
make e2e-times                      # per-version boot times
make clean-e2e                      # remove logs, results, containers, images
```

Knobs: `VERSIONS`, `PARALLEL` (alias `J`), `BOOT_TIMEOUT` (default 180s),
`JAVA` (override JVM; a version has a Java floor, not a pin — except the Forge
`modern` band, java 21 only, [#66](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/issues/66)), `LOADER`
(`fabric` | `quilt` | `forge` | `neoforge` | `babric` | `bta`). Output:
`build/e2e-logs/<key>.log` and `build/e2e-results/<key>.result`; any failure
makes the run exit non-zero. Harness internals and verdict codes:
[docs/e2e-harness.md](docs/e2e-harness.md). The CI grid (canary gate, staged
bands, lean vs full shapes): [docs/ci.md](docs/ci.md).

## Compatibility

Which Minecraft versions, loaders and Java versions are supported — and, separately,
which of them are actually proven by a booted server in CI — is stated in one place:

**[Supported Versions](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/Supported-Versions)** (wiki)

User-facing installation guidance, including which jar to pick, is in
[MOD.md](./MOD.md#compatibility).

This file deliberately states no version ranges. They were previously duplicated
here, in `MOD.md` and across `docs/`, and drifted: the same fact appeared with
different values in different files, and nothing could fail when one of them went
stale. The declared ranges themselves live in `gradle.properties`,
`forge/gradle.properties`, `neoforge/gradle.properties`, `babric/gradle.properties`
and `bta/gradle.properties`, which the build and the tests actually consume; the wiki page cites the commands that regenerate every
figure from them.

