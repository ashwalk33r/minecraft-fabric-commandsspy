[![CI](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/actions/workflows/ci.yml/badge.svg)](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/actions/workflows/ci.yml)

# CommandsSpy

A server mod that logs every executed command with its source (player, console,
RCON, function, command block). One shared implementation ships as four
era-correct Fabric/Quilt jars covering Minecraft 1.14.4–26.2, plus one NeoForge
jar covering Minecraft 1.20.2–26.2 and four Forge jars covering 1.14.4–26.2.

User documentation: [MOD.md](./MOD.md). Official releases:
[Modrinth](https://modrinth.com/mod/commandsspy/versions).

## Docs

- [docs/version-matrix.md](docs/version-matrix.md) — the nine jars, the loader
  seam in the shared core, version boundaries, Java floors, default e2e matrix.
- [docs/testing.md](docs/testing.md) — unit suite: how it boots, isolation,
  known quirks, era-specific wiring.
- [docs/e2e-harness.md](docs/e2e-harness.md) — Docker e2e: phases,
  assertions, verdict codes, jar cache, routing drift protection.
- [docs/ci.md](docs/ci.md) — `make ci` gate, workflows, staged e2e grid.
- [docs/protocol-table.md](docs/protocol-table.md) — the Go bot's per-version
  protocol reference.
- [docs/quilt-entrypoint-gap-upstream.md](docs/quilt-entrypoint-gap-upstream.md)
  — an unfiled, paste-ready bug report for quilt-loader's pre-1.18 entrypoint
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
`build-forge-mc116`, `build-forge-eventbus7`).

The first NeoForge build runs ModDevGradle's NeoForm pipeline (decompile +
recompile Minecraft, ~8-9 minutes); it is cached in `GRADLE_USER_HOME`
afterwards. Only `LOADER=neoforge` e2e runs depend on that jar, so a Fabric or
Quilt run never pays for it.

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
`JAVA` (override JVM; a version has a Java floor, not a pin), `LOADER`
(`fabric` | `quilt` | `forge` | `neoforge`). Output:
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
`fabric.mod.json`) — and every version CI boots, it boots on **both**, with
the identical assertion set. CI samples each range rather than enumerating it
(27 versions per loader); run any other in-range version yourself with
`make e2e VERSIONS=...`.

On Quilt below Minecraft 1.18 the startup banner never prints — Quilt Loader
does not invoke the mod's entrypoint on dedicated servers there. Logging is
unaffected: the mixin still applies, so console, RCON and player commands are
captured exactly as on Fabric. The missing banner is the only symptom: the
config file is still written at startup, which the e2e harness asserts before
any command runs. Why the boundaries sit where they do, and the detail on that
gap: [docs/version-matrix.md](docs/version-matrix.md).

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
| 1.20.2–1.20.4 | mc1.20.2-26.2-neoforge | 17+ | 20.2.x–20.4.x |
| 1.20.5–1.21.11 | mc1.20.2-26.2-neoforge | 21+ | 20.5.x–21.11.x |
| 26.1–26.2 | mc1.20.2-26.2-neoforge | 25+ | 26.1.x–26.2.x |

**One NeoForge jar covers every Minecraft version NeoForge publishes for.**
The Java column is NeoForge's own floor per Minecraft line, not the jar's: the
jar is Java 17 bytecode throughout and bytecode binds only downward, so it runs
on the Java 21 and Java 25 runtimes the upper lines require. NeoForge has
shipped Mojang official names since its first release, so there is no mapping
wall to split the jar at; the only seam is FML's metadata format, and the jar
carries both `META-INF/mods.toml` (FML 1.x/2.x) and
`META-INF/neoforge.mods.toml` (FML 3.x+) so each loader major reads the one it
knows. Minecraft 1.20.2 is NeoForge's own permanent floor; 1.20.1 and below are
MinecraftForge or nothing. The jar uses NeoForge's native `CommandEvent` rather
than a mixin — same hook point, same coverage. The range is measured, not
declared: the one jar boots real NeoForge servers on 1.20.2, 1.20.4, 1.20.6,
1.21.1, 1.21.11 and 26.2 in CI, spanning FML 1.x through 11.x. Evidence, the
metadata seams and the measured boot table:
[docs/version-matrix.md](docs/version-matrix.md) → "NeoForge".
