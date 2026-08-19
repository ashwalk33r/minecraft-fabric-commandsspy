# Version matrix

One shared implementation (`src/main`), eight jars: four era-correct
Fabric/Quilt jars, which differ only in their `CommandManagerMixin` source
set and build settings; two NeoForge jars built from the same core against a
different loader (see
[NeoForge](#neoforge-two-more-jars-on-a-different-contract) below); and two
Forge jars, modern and legacy, built from the same core against a third
loader (see [Forge](#forge-a-fifth-jar-narrower-by-construction) below).

## The four Fabric/Quilt jars

| Source set | Jar covers | Mappings | Bytecode | Mixin compat | Hooked method |
|---|---|---|---|---|---|
| `src/mc114` | >=1.14 <1.19 | yarn/intermediary (1.16.5) | Java 8 | `JAVA_8` | `CommandManager.execute(ServerCommandSource, String)` returns `int` |
| `src/mc1192` | >=1.19.1 <1.20.3 | yarn/intermediary (1.20.1) | Java 17 | `JAVA_17` | `CommandManager.execute(ParseResults, String)` returns `int` |
| `src/mc121` | >=1.20.3 <1.22 | yarn/intermediary (1.21.1) | Java 21 | `JAVA_21` | `CommandManager.execute(ParseResults, String)` returns `void` |
| `src/mc26` | >=26.1 <26.3 | official Mojang names (unobfuscated) | Java 21 | `JAVA_21` | `Commands.performCommand(ParseResults, String)` returns `void` |

Each mixin unwraps the command source, decides player vs. other, and calls the
shared `CommandsSpy.handleCommand(fullCommand, isPlayer, sourceName)`.

## The loader seam in the shared core

`src/main/java` references no loader type at all — not Fabric, not Quilt, not
NeoForge. Exactly one class per loader carries the coupling, and each loader
build compiles the same shared core against its own:

| Loader | Entrypoint class | Lives in |
|---|---|---|
| Fabric / Quilt | `CommandsSpyFabric implements ModInitializer` | `src/fabric/java` |
| NeoForge | `CommandsSpyNeoForge`, annotated `@Mod("commandsspy")` | `neoforge/src/main/java` |

Both call `CommandsSpy.init()`, which emits the
`Loading CommandsSpy by Ultra_MC.` banner and, by touching the class, pulls
`CONFIG` and `BLACKLIST` up at boot rather than on the first executed command.

`src/fabric/java` is added to the root build's source set unconditionally, on
all four targets — unlike the per-era `src/mcNNN/java` dirs, it is identical
everywhere. The NeoForge build source-includes `src/main/java` and **not**
`src/fabric/java`, which is the whole reason the two are separate directories.

The config file path is resolved without any loader API
(`Paths.get("config", "commands-spy.json")`): on a dedicated server the game
directory *is* the working directory, on every loader, so this is the same path
Fabric's `getConfigDir()` and NeoForge's `FMLPaths.CONFIGDIR` both produce. That
equivalence is asserted on NeoForge the same way it is on Fabric and Quilt — by
booting a real server in e2e and watching the mod read and write its config.

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

## NeoForge: two more jars, on a different contract

NeoForge is structurally unlike Quilt. Quilt rides the existing four jars for
free — one jar, both loaders, no new code. NeoForge needs its own jars, its own
build, and a **recurring** obligation this project did not have before.

### The two jars

| Jar | NeoForge line | Minecraft | Toolchain / bytecode | Hook |
|---|---|---|---|---|
| `+neoforge-mc1.21.1` | `21.1.248` | **1.21.1 only** | Java 21 | `CommandEvent` |
| `+neoforge-mc26.2` | `26.2.0.64` | **26.2 only** | Java 25 | `CommandEvent` |

The Minecraft part of the classifier is exact, not a range, and that is the
whole point of this section.

### Why one jar cannot span a range

Fabric's Intermediary mappings are stable across Minecraft versions, which is
why four jars cover 24 tested versions. NeoForge has no equivalent: it publishes
**one version line per Minecraft version**, each built against that version's
Mojang mappings. So a NeoForge jar covers exactly one Minecraft version, its
`neoforge.mods.toml` says so (`versionRange = "[1.21.1]"`), and every new
Minecraft release means a new NeoForge line, a new jar and a new e2e row —
forever. That cost is real and is the reason the shipped scope is two lines, not
sixteen.

Note the 26.2 line numbers as `26.2.0.<n>`, four components, not `26.2.<n>`.

### Hard floors

- **Minecraft 1.20.2 is the floor, permanently.** `net.neoforged:neoforge`
  starts at `20.2.12-beta`; there is no NeoForge for 1.14–1.20.1. The entire
  `src/mc114` era and most of `src/mc1192` are out of reach by construction.
- **1.20.1 is not NeoForge.** It is MinecraftForge `47.1.x` — `net.minecraftforge.*`
  packages, `META-INF/mods.toml` (not `neoforge.mods.toml`), manifest-declared
  mixin configs. A third metadata variant, deliberately out of scope here.
- **A Loom-built jar can never load on NeoForge**, at any version. The barrier is
  the loader contract (`fabric.mod.json` + `ModInitializer` vs
  `neoforge.mods.toml` + `@Mod`), not the mappings — it holds even on 26.x where
  both sides use real names.

### `CommandEvent`, not a mixin

The NeoForge jars contain no mixin. NeoForge fires
`net.neoforged.neoforge.event.CommandEvent` from `Commands.performCommand` —
the exact instruction `src/mc26`'s mixin injects at `@At("HEAD")` of. Same hook
point, so **coverage is identical** (player, console, RCON, command block) and
so is the blind spot: datapack functions and `/execute run` sub-commands have
gone through `Commands.executeCommandInContext` since 1.20.2 and are seen by
neither mechanism. That is a pre-existing gap in the Fabric behaviour, not a
NeoForge regression.

A mixin would buy nothing here and cost more: a second metadata format to
template, mixin compatibility levels, and a per-Minecraft-version descriptor
dependency — the thing that already forced four source sets on the Fabric side.
The event's signature is NeoForge API, so it does not move when Minecraft's
does. The Fabric mixins stay exactly as they are; this is a per-loader choice of
the cheapest hook reaching the same instruction, not a migration.

The event carries no raw command string; `getParseResults().getReader().getString()`
is the equivalent of the mixins' `fullCommand` parameter.

### Build

`neoforge/` is a **standalone Gradle build**, not a subproject:
`gradle -p neoforge build -PneoTarget=<121|26>`, wired into `make build`.
ModDevGradle and Fabric Loom are not supported in one Gradle project
([ModDevGradle#234](https://github.com/neoforged/ModDevGradle/issues/234)) and
the root build applies Loom unconditionally, so keeping them in separate builds
means the two plugins never meet — and the root build's four invocations, its
`org.gradle.jvmargs`, and the Fabric jars' output all stay untouched.

Two things that are not obvious and are both e2e-proven:

- **`modLoader` and `loaderVersion` in `neoforge.mods.toml` are mandatory**,
  despite being widely documented as optional. Omitting `modLoader` makes FML
  reject the jar with `InvalidModFileException: Missing ModLoader in file` and
  crash the server during pre-load. `loaderVersion` is the **FML** version — a
  third version axis, distinct from both the NeoForge and Minecraft versions
  (FML 4.x ships with NeoForge 21.1.x, FML 11.x with 26.2.x), hence the
  per-line `fml_range_*` properties.
- **`options.release` must match the line's own Java level**, unlike the root
  build's mc26 target which compiles 26.x to release 21. `release` also sets the
  project's `org.gradle.jvm.version` consumer attribute, and 26.2's
  `net.neoforged.fancymodloader:loader` publishes only a Java 25 variant —
  asking for 21 fails dependency resolution outright with "no matching variant",
  it does not silently downgrade.

The first build of each line runs ModDevGradle's NeoForm pipeline (decompile +
recompile Minecraft, ~8-9 minutes measured); it is cached in `GRADLE_USER_HOME`
afterwards and subsequent builds take seconds. Only `LOADER=neoforge` e2e runs
depend on these jars, so a Fabric or Quilt run never pays for it.

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

NeoForge is the third value of that same axis (`LOADER=neoforge`), but it is
**not** orthogonal to the version list the way Quilt is: it runs only the two
Minecraft versions its two jars target. The pins live in
`neoforge/gradle.properties` and are mirrored in `scripts/e2e-run-one.sh` —
change both together; `scripts/test-jar-routing.sh` fails if they drift, and any
other version under `LOADER=neoforge` is an explicit
`neoforge-unsupported-version` failure rather than a jar that cannot load.

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
- A sub-1.20.5 Forge jar needs ForgeGradle's separate
  `net.minecraftforge.renamer` reobfuscation step — see "Forge legacy jar"
  below for the second Forge jar this project ships to cover it.

For calibration: the Fabric mc121 jar covers 1.20.3–1.21.x (~13 versions).
Forge's era simply started later; both jars are era-maximal for their
loader.

## Forge legacy jar: SRG member ids are stable, class identities are not

Issue #28 task 1 asked a narrower question than "can a legacy Forge jar
exist": are SRG member ids (`m_xxxxx_`/`f_xxxxx_`) as stable *within* the
pre-1.20.5 SRG era as Mojang's official names are within the modern era —
stable enough that ForgeGradle's `net.minecraftforge.renamer` plugin, run
once, produces a jar that boots correctly across multiple legacy Minecraft
versions? Answer: **mostly yes, with one sharp, well-understood boundary.**

`forge/build.gradle` dispatches on `-PforgeTarget` (default `modern`, the
jar described above; `legacy` for this one) the same way the root build
dispatches on `-PmcTarget` — one Gradle project, two compile targets,
selected by a property rather than forked into a second directory. The
`legacy` target adds a second output alongside the plain `jar` task:
`renamer.classes(tasks.named('jar', Jar)) { map.from
minecraft.dependency.toSrgFile; archiveClassifier = 'forge' }`, pinned to
`net.minecraftforge.renamer` version `1.1.2` (exact, not a range — the
plugin is young and actively iterating). Compiled once against Minecraft
1.20.1 / Forge 47.4.20 official mappings, `renameJar` maps the output down
to that version's SRG names — the only mapping ForgeGradle 7's Mavenizer
resolves for a `minecraft.dependency(...)` declaration; there is no
per-target-version remapping built into the plugin. `javap` on the result
confirms it: `CommandSourceStack.getEntity` -> `CommandSourceStack.m_81373_`,
`ServerPlayer.getName` -> `ServerPlayer.m_7755_`,
`CommandSourceStack.getTextName` -> `CommandSourceStack.m_81368_` — brigadier
and Forge API calls (`ParseResults.getContext`, `CommandEvent.getParseResults`)
stay as-is, never obfuscated, same pattern the modern jar's own `javap`
check uses in reverse. `make build-forge-legacy` builds it;
`MOD_JAR_FORGE_LEGACY` in the Makefile names it.

### Measured range

One SRG-renamed jar, booted as a real Forge dedicated server with the full
e2e assertion set, `minecraft_range` widened to `[1.16,1.20.6)` during
measurement so mods.toml metadata was never the limiting factor:

| Minecraft | Forge build | Result |
|---|---|---|
| 1.16.4 | 35.1.4 | FAIL — `NoSuchMethodError` inside Forge's own `cpw.mods.modlauncher.SecureJarHandler`, before any mod code runs. An ancient ModLauncher/JDK incompatibility, not a signal about this mod or SRG stability. |
| 1.16.5 | 36.2.34 | FAIL — `NoClassDefFoundError: net/minecraft/commands/CommandSourceStack`. Decisive: see below. |
| 1.17.1 | 37.1.1 | PASS |
| 1.18 | 38.0.14 | PASS |
| 1.18.1 | 39.1.0 | PASS — all three e2e legs (console, RCON, player). Confirming this leg required fixing a harness gap first: the e2e bot's protocol table (`tools/table.go`) was missing protocol 757 (1.18/1.18.1), so this version's player-phase leg used to fail with an unrelated "unsupported protocol" error, never reaching a real assertion. Fixed alongside this jar (see docs/protocol-table.md); confirmed live with keep_alive `0x21`/`0x0F` and chat `0x03`, identical to 756 and 758. |
| 1.18.2 | 40.3.0 | PASS |
| 1.19.1 | 42.0.9 | PASS |
| 1.19.2 | 43.5.0 | PASS |
| 1.20.1 | 47.4.10 | PASS (compile target) |
| 1.20.2 | 48.1.0 | PASS |
| 1.20.3 | — | PASS |
| 1.20.4 | 49.2.0 | PASS |
| 1.20.5 | — | Forge publishes no build at all — same gap the modern jar hits at its floor. |

One jar spans Minecraft **1.17.1–1.20.4**: nine Minecraft versions across
seven consecutive Forge branches (37, 39, 40, 42, 43, 47, 48, 49) — directly
adjacent to the modern jar's own 1.20.6 floor, with only the
Forge-publishes-nothing 1.20.5 gap between them. Shipped
`forge/gradle.properties` (`_legacy` suffix): `minecraft_range =
[1.17.1,1.20.5)`, `forge_range`/`loader_range = [37,50)`. The jar itself is
`commandsspy-<ver>+mc1.17-1.20.4-forge.jar`.

### Why 1.16.x fails: a class rename, not an SRG rename

`m_81373_` etc. are stable across every branch above — SRG **member** ids
genuinely do not drift within this span, seven Forge majors and three years
of Minecraft releases. 1.16.x fails for a different, sharper reason: the
1.16.5 crash log shows the vanilla call site itself as
`net.minecraft.command.Commands.func_197059_a` — package `command`, class
`Commands`/`CommandSource`. By 1.17.1 (and every version above), the same
call site is `net.minecraft.commands.CommandSourceStack` — package
`commands`, class renamed. This is Mojang's own official-mapping vocabulary
changing shape, upstream of and unrelated to Forge's SRG obfuscation layer;
`renamer.mappings` remaps method/field ids inside a fixed class reference,
it cannot retarget a hardcoded class name to a class that did not exist yet
under that name when the jar was compiled. No amount of "point
`renamer.classes` at 1.16.5's own SRG file" fixes this on a single
compiled-once jar — the compiled bytecode already says
`net/minecraft/commands/CommandSourceStack`, a class absent from 1.16.5
entirely, so class loading fails before any renaming question is reached.
A 1.16.x-and-below jar would need its own compile pass against
1.16.5-shaped official mappings (a genuinely separate source variant, the
same shape the Fabric mc114/mc1192/mc121 source-set split already uses for
exactly this reason), not just a different renamer mapping file on the
existing compiled output. This is also why 1.16.x is excluded from the
shipped range rather than folded in: the fix is a third compile target, not
a wider range on this one, and nothing in the measured 1.17.1-1.20.4 span
needs it.

1.16.4's failure is a second, independent finding: even setting the
class-identity question aside, Forge 35.1.4's own `ModLauncher`
(2020-era `cpw.mods.modlauncher`) throws `NoSuchMethodError` inside its own
jar-signature verification on the JDK the 1.16.x e2e floor (Java 8) ships,
before FML ever inspects the mod jar. Old Forge branches carry their own
JDK-patch-level fragility independent of anything under this project's
control. Probing 1.16.x during measurement needed a temporary Java 8
compile pass (`--release 8`, plus a matching classic-cast rewrite of
`CommandsSpyForge.java`'s one pattern-matching `instanceof`, a Java 16+
feature); neither is part of the shipped `legacy` target, which compiles
straight to Java 17 like every other 1.17.1+ target in this project, since
1.16.x never entered the shipped range.

### Gate coverage

`.github/workflows/e2e.yml`'s `e2e-forge-legacy-java17` job hand-lists every
measured PASS version above plus the 1.16.5 guard leg — unlike the modern
jar's three-leg floor/ceiling/guard job, the whole point of this range was
proving SRG member-id stability *across* seven Forge major branches, so a
floor+ceiling-only gate would not exercise the thing being measured.
`scripts/e2e-run-one.sh` routes each Minecraft version to the legacy or
modern jar by a single case statement (`FORGE_JAR_BAND`, probed via
`--print-forge-routing`); `scripts/test-jar-routing.sh` asserts that
routing offline, including that versions outside both ranges come back
refused rather than silently handed either jar.
