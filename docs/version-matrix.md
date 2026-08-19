# Version matrix

One shared implementation (`src/main`), four era-correct jars. Each jar differs
only in its `CommandManagerMixin` source set and build settings.

## The four jars

| Source set | Jar covers | Mappings | Bytecode | Mixin compat | Hooked method |
|---|---|---|---|---|---|
| `src/mc114` | >=1.14 <1.19 | yarn/intermediary (1.16.5) | Java 8 | `JAVA_8` | `CommandManager.execute(ServerCommandSource, String)` returns `int` |
| `src/mc1192` | >=1.19.1 <1.20.3 | yarn/intermediary (1.20.1) | Java 17 | `JAVA_17` | `CommandManager.execute(ParseResults, String)` returns `int` |
| `src/mc121` | >=1.20.3 <1.22 | yarn/intermediary (1.21.1) | Java 21 | `JAVA_21` | `CommandManager.execute(ParseResults, String)` returns `void` |
| `src/mc26` | >=26.1 <26.3 | official Mojang names (unobfuscated) | Java 21 | `JAVA_21` | `Commands.performCommand(ParseResults, String)` returns `void` |

Each mixin unwraps the command source, decides player vs. other, and calls the
shared `CommandsSpy.handleCommand(fullCommand, isPlayer, sourceName)`.

## Why the boundaries sit where they do

`CommandManager.execute` (intermediary `class_2170.method_9249`) changed shape
twice:

- **1.19.1** wrapped the `ServerCommandSource` argument in `ParseResults`
  (still returning `int`). 1.19.0 is **unsupported**: its `execute()` still
  takes `(ServerCommandSource, String)`, so the mc1192 jar's mixin cannot
  apply there (e2e-proven: `InvalidInjectionException` on 1.19.0), and the
  mc114 jar's Java 8 hook was never built for the 1.19 runtime.
- **1.20.3** flipped the return type to `void`.

Mixin matches the injection target by name and then validates the descriptor,
so a callback of the wrong shape fails at load time ("CallbackInfoReturnable
is required!"). A jar built for one era boot-fails on the other — that is why
the mc1192 range stops mid-minor at 1.20.2, and why the callback type is the
entire difference between the mc1192 and mc121 hooks.

**26.x** ships unobfuscated and the Fabric runtime uses official Mojang names,
so intermediary-compiled mixins silently never apply. The mc26 source set
compiles directly against official names (`Commands.performCommand`); yarn
does not exist for 26.x.

Every intermediary member the mc114 hook touches is identical across
1.14.4–1.18.2, so the compile-against version (1.16.5) does not change the
output jar.

## Quilt: the pre-1.18 entrypoint gap

On Quilt Loader, `CommandsSpy.onInitialize()` is **never invoked on dedicated
servers below Minecraft 1.18** — silently, with no crash and no exception.
e2e-proven with quilt-loader 0.30.0: 1.14.4, 1.16.5 and 1.17.1 fail; 1.18.2,
1.19.2, 1.19.4, 1.20.2, 1.21.11 pass. The boundary is a Minecraft version, not
a jar boundary — 1.17.1 and 1.18.2 are served by the same mc114 jar and the
same Java 17 floor.

Nothing user-visible is lost. Mixins are applied by SpongePowered Mixin
independently of the loader's entrypoint invocation, so the mod's entire
function — console, RCON and player command logging — is asserted and passes
on all three versions. The only missing artifact is the startup banner.

Upstream, not ours, and not fixable by choosing a different loader version:
quilt-loader's `EntrypointPatch` bytecode-patches Minecraft's own main class to
inject the entrypoint call, and its `EnvType.SERVER` path is byte-identical
across every release from 0.23.0 to 0.30.1-beta.2 (only client/applet/
pre-classic paths changed), and identical to Fabric Loader 0.19.2's — which
works on these versions. Every in-patch failure mode throws loudly, so the hook
is provably injected and `Hooks.startServer` provably reached; the defect lies
further into quilt's mod-loading pipeline. No upstream issue reports it
(https://github.com/QuiltMC/quilt-loader/issues, searched for EntrypointPatch /
entrypoint / legacy / 1.16 / 1.17 / onInitialize), and quilt-loader publishes no
minimum-supported-Minecraft table.

These versions stay in the Quilt e2e matrix with every functional assertion
intact. `scripts/e2e-entrypoint.sh` asserts the banner **expected-absent** on
`LOADER=quilt` below 1.18 (`QUILT_ENTRYPOINT_GAP`), so CI fails and tells us to
update this section the day upstream fixes it.

## Java floors

A Minecraft version has a Java **floor**, not a pin: compatibility with newer
JVMs is asserted by the e2e grid, not assumed. The floor table's single
executable home is the `case` statement in `scripts/e2e-run-one.sh` (query it
with `--print-java <version>`); `tools/floors_test.go` pins it against drift.

| MC range | e2e floor JVM | Note |
|---|---|---|
| 1.14–1.16.x | 8 | |
| 1.17.x | 17 | historical floor is 16, but no Temurin 16 JRE image exists; the jar's own `java >=8` guard still admits Java 16 operators |
| 1.18–1.20.2 | 17 | |
| 1.20.3–1.21.x | 21 | 1.20.3/1.20.4's vanilla floor is 17, but they run the mc121 jar, which is Java 21 bytecode |
| 26.x | 25 | |

Java 11 is supported for manual override runs only
(`make e2e VERSIONS=... JAVA=11`) and never appears in a default matrix.

The `mixin_compat_*` values in `gradle.properties` are templated into
`commandsspy.mixins.json` and must not exceed the era's runtime JRE: a
`JAVA_21` compatibility level on a Java 17 JVM fails mixin bootstrap.

Which floors get the alpine base image vs. jammy, and why:
[ci.md](ci.md) → e2e server images.

## Loader floors

- mc121 / mc1192: loader >=0.16.5.
- mc26: loader >=0.19.3, the verified line for unobfuscated 26.x.
- mc114: deliberately a high floor (>=0.19.3), the line verified to serve
  working server launchers all the way down to 1.14.4. Do not lower it.

Quilt Loader natively loads a jar's `fabric.mod.json` — this project also
ships a `quilt.mod.json` (same jar, both loaders) purely for an accurate
platform badge, not because Quilt needs it to load the mod. All four eras
declare the same Quilt Loader floor, `>=0.30.0`, and the same Java floor as
their Fabric counterpart (Quilt Loader itself imposes no additional JVM
floor at any era). This is asserted, not assumed: the e2e matrix runs every
version in this file on both loaders (`LOADER=fabric`/`LOADER=quilt`); see
[e2e-harness.md](e2e-harness.md) → "Quilt server install".

## Default e2e version list

The default `VERSIONS` in the Makefile samples the matrix:

- **T0 band** (1.20.3–1.20.6) and every 1.21.x release: exhaustive — the band
  is its boundaries.
- **26.x**: 26.1 and 26.2 only. 26.1.1/26.1.2 are excluded: mapping breaks
  land on minor boundaries and each extra version costs a full server
  download. Suspect a 26.x patch? `make e2e VERSIONS="26.1 26.1.1 26.1.2 26.2"`.
- **mc1192 band**: 1.19.2, 1.19.4, 1.20.1, 1.20.2 — both ends of the 1.19
  line, the most-run legacy version, and the 1.20.2/1.20.3 boundary.
  The rest: `make e2e VERSIONS="1.19.1 1.19.3 1.20"`.
- **mc114 band**: 1.16.5, 1.17.1, 1.18.2. 1.14.4/1.15.2 ride the identical
  jar; their one distinguishing property is the older `Recon` RCON spelling
  (see [e2e-harness.md](e2e-harness.md)). Suspect the bottom of the range?
  `make e2e VERSIONS="1.14.4 1.15.2"`.

## Forge: a fifth jar, narrower by construction

Issue #23 phase 1 adds ONE Forge jar, `commandsspy-<ver>+mc1.21.x-forge.jar`,
built by a separate Gradle project in `forge/` (ForgeGradle 7, its own
`settings.gradle`/`build.gradle`/`gradle.properties`) that is not part of the
root build — `make build` still produces exactly the four jars above; the
Forge jar is `make build-forge`, on demand. It compiles the same shared core
from one copy (`sourceSets.main.java.srcDir '../src/main/java'`), against
Minecraft 1.21.1 / Forge 52.1.16, Java 21 toolchain.

Forge needs no Mixin: `forge/src/main/java/.../CommandsSpyForge.java`
registers a `net.minecraftforge.event.CommandEvent` listener on
`MinecraftForge.EVENT_BUS` instead. `CommandEvent` fires inside
`Commands#performCommand` — the same call site the Fabric mixins inject
into — carrying the same `ParseResults`; the raw command is recovered as
`event.getParseResults().getReader().getString()`, measured byte-identical
to what the mixins receive (every existing log-literal assertion passed
unchanged).

### Measured range

Booted as a real Forge dedicated server in the e2e harness with the full
assertion set, per Minecraft version. Measured with `minecraft_range`
temporarily widened to `[1.20.3,1.21.6)` so Forge's own metadata gate would
not be what limited the answer; the shipped range is the narrower one this
table produced.

| Minecraft | Forge build | Result |
|---|---|---|
| 1.20.4 | 49.2.0 | boots, banner logs, then the first executed command throws `NoSuchMethodError: CommandSourceStack.getEntity()` and the server crashes |
| 1.20.5 | — | Forge publishes no build for 1.20.5 at all |
| 1.20.6 | 50.2.0 | PASS |
| 1.21 | 51.0.33 | PASS |
| 1.21.1 | 52.1.0 | PASS (compile target) |
| 1.21.3 | 53.1.0 | PASS |
| 1.21.4 | 54.1.14 | PASS |
| 1.21.5 | 55.1.0 | PASS |

One jar spans Minecraft **1.20.6–1.21.5**: six Minecraft versions across six
consecutive Forge branches (50, 51, 52, 53, 54, 55). Shipped `mods.toml`
declares `minecraft = [1.20.6,1.21.6)`, `forge`/`loaderVersion = [50,)`.

### Why that range and not more

`javap` on the built jar shows zero SRG member names (`m_xxxxx_`) — only
plain Mojang official names (`CommandSourceStack.getEntity`,
`CommandSourceStack.getTextName`, `ServerPlayer.getName`,
`Component.getString`), plus brigadier and Forge API classes, which are
never obfuscated. Forge ships Mojang official mappings at runtime from
Minecraft 1.20.5 onward; within that era Forge has the same cross-version
member stability Fabric gets from intermediary. Below 1.20.5 the Forge
runtime is SRG-mapped, which is exactly why 1.20.4 fails.

- **1.20.6 is a hard floor**, not conservatism.
- **`<1.21.6` is declared, not measured**: Forge's EventBus 6 -> 7 API break
  lands between 1.21.5 and 1.21.8 (`MinecraftForge.EVENT_BUS.addListener` ->
  `CommandEvent.BUS.addListener`; `Event` -> `MutableEvent` + `Cancellable`)
  and needs a second entrypoint variant.
- A sub-1.20.5 Forge jar would need ForgeGradle's separate
  `net.minecraftforge.renamer` reobfuscation step, whose own cross-version
  range is unmeasured. Not supported: every Forge era outside
  1.20.6–1.21.5, including 1.16.5 and 1.20.1 — the largest legacy Forge
  server bases — which would require that unmeasured reobfuscated path.

For calibration: the Fabric mc121 jar covers 1.20.3–1.21.x (~13 versions).
Forge's era simply started later; both jars are era-maximal for their
loader.
