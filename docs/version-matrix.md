# Version matrix

One shared implementation (`src/main`), nine jars: four era-correct
Fabric/Quilt jars, which differ only in their `CommandManagerMixin` source
set and build settings; one NeoForge band jar covering Minecraft 1.20.2-26.2,
built from the same core against a different loader (see
[NeoForge](#neoforge-one-band-jar-on-a-different-contract) below); and four
Forge jars — mc116, legacy, modern and eventbus7 — built from the same core
against a third loader (see [Forge](#forge-four-jars-narrower-by-construction)
below).

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

On Quilt Loader, `CommandsSpyFabric.onInitialize()` is **never invoked on
dedicated servers below Minecraft 1.18** — silently, no crash, no exception.
e2e-proven with quilt-loader 0.30.0: **four** versions fail — 1.14.4, 1.15.2,
1.16.5 and 1.17.1; 1.18.2, 1.19.2, 1.19.4, 1.20.2, 1.21.11 pass. All four are
booted on Quilt in CI and gap-gated there: `scripts/e2e-entrypoint.sh:357`
matches `1.14*`/`1.15*`/`1.16*`/`1.17*`, and the versions come from
`tools/gen_matrix.go`'s `mc114_java8` (1.14.4, 1.15.2, 1.16.5) and
`mc114_java17` (1.17.1, 1.18.2) outputs, consumed by the `e2e 1.14-1.16 java 8
(quilt)` and `e2e 1.17-1.18 java 17 (quilt)` jobs. The boundary is a Minecraft
version, not a jar boundary — 1.17.1 and 1.18.2 are served by the same mc114
jar and the same Java 17 floor.

Command logging is unaffected: mixins are applied by SpongePowered Mixin
independently of the loader's entrypoint invocation, so console, RCON and
player command logging is asserted and passes on all four versions. The banner
is **not**, however, the only thing lost. `CONFIG` and `BLACKLIST` are
`static final` fields, initialised by class initialisation
(`CommandsSpy.java:13-14`), and `init()` exists for no other reason than to
touch the class at boot — its own javadoc says so (`CommandsSpy.java:16-23`),
as does [the loader seam](#the-loader-seam-in-the-shared-core) above. Nothing
else in the mod references `CommandsSpy`; the only other call sites anywhere
are the four mixins' `handleCommand`. So with the entrypoint never invoked, the
class is first initialised by the mixin's first `handleCommand` — by the first
command any source executes. Two user-facing consequences follow:

- **Config auto-creation moves from boot to the first executed command.**
  `CommandsSpyConfig.load()` writes the file only on the does-not-exist path
  (`CommandsSpyConfig.java:27-42`), and that path now runs at first command
  rather than at startup. An admin who boots a Quilt 1.16.5 server, stops it,
  and opens `config/commands-spy.json` to set a blacklist finds **no file**.
  That contradicts MOD.md's "On startup, the config file will be created
  automatically" — on a stock install, with no malformed input needed.
- **A malformed config fails late and silently instead of at boot.** A JSON
  syntax error leaves `load()` as an uncaught `JsonSyntaxException` (pinned by
  `CommandsSpyConfigTest.malformedJsonThrowsOnLoad`), which at the `static
  final` call site becomes an `ExceptionInInitializerError` thrown from inside
  the mixin injection on the first command, and a `NoClassDefFoundError` on
  every command after it. The server is up and looks healthy, command
  execution is broken, and there was no boot-time signal. On every loader where
  the entrypoint does fire, the same file is a clean, immediate boot failure.

Failing loud on a broken config is deliberate and stays that way: `blacklist`
is a privacy control — MOD.md documents suppressing `tell`/`t` to keep player
conversations out of the log — so defaulting silently would resume logging
exactly what an admin configured hidden. Only the *timing* of the failure moves
here, and that is a property of the loader, not something the mod chooses.

The default e2e leg cannot see the first consequence. Its config assertion
(`scripts/e2e-entrypoint.sh:396-405`) runs at the end of the run, after the
console, unknown-command and RCON commands have already been sent, so on Quilt
below 1.18 it passes because the first command created the file — not because
boot did. The comment there cites MOD.md's "On startup" promise, but the check
has never asserted the *startup* half of it, on any loader.

The declaration quilt-loader fails to honour is `quilt_loader.entrypoints.main`
in `src/main/resources/quilt.mod.json` (`pl.m2x.commandsspy.CommandsSpyFabric`)
— Quilt reads that file for this jar, not `fabric.mod.json`, so
`fabric.mod.json`'s identical `main` entrypoint is not the one being skipped.

Upstream, not ours, and not fixable by choosing a different loader version:
quilt-loader's `EntrypointPatch` bytecode-patches Minecraft's own main class to
inject the entrypoint call, and its `EnvType.SERVER` path is byte-identical
across every release from 0.23.0 to 0.30.1-beta.2 (only client/applet/
pre-classic paths changed), and identical to Fabric Loader 0.19.2's — which
works on these versions. Every in-patch failure mode throws loudly and none did,
which is consistent with the hook being injected and `Hooks.startServer`
reached — but that is an inference from an absence of errors, read from the
source, with no artifact retained and no debugger attached. It does not rule the
injected path out, and the measurement in "Why a Quilt-native entrypoint cannot
close this gap" below is a reason to suspect it: `main` reaches its call site
only through this injection, while `preLaunch` bypasses it entirely — and
`preLaunch` is the stage that works. No upstream issue reports it
(https://github.com/QuiltMC/quilt-loader/issues, searched for EntrypointPatch /
entrypoint / legacy / 1.16 / 1.17 / onInitialize), and quilt-loader publishes no
minimum-supported-Minecraft table.

A bug report for quilt-loader, written to be pasted into QuiltMC's tracker, is
kept at [docs/quilt-entrypoint-gap-upstream.md](quilt-entrypoint-gap-upstream.md).
**It has not been filed.** Nothing has been sent to QuiltMC or to any
maintainer; whether to file it is a human decision, and the file carries a
pre-filing checklist because every fact in it goes stale.

These versions stay in the Quilt e2e matrix with every functional assertion
intact. `scripts/e2e-entrypoint.sh` asserts the banner **expected-absent** on
`LOADER=quilt` below 1.18 (`QUILT_ENTRYPOINT_GAP`), so CI fails and tells us to
update this section the day upstream fixes it. It separately asserts the preLaunch
line **present** on every Fabric and Quilt leg (`prelaunch-entrypoint-not-invoked`,
see below), under its own gate rather than `QUILT_ENTRYPOINT_GAP` — the two are
independent upstream facts that only happen to share a version boundary today,
and that flag's failure text tells whoever sees the `main` gap close to delete it.

### The gap is specific to the `main` call site

`preLaunch` is not affected. quilt-loader invokes the
`net.fabricmc.loader.api.entrypoint.PreLaunchEntrypoint` declared in
`quilt.mod.json` below 1.18 — a different call site from the one the `main` gap
lives on: Knot runs preLaunch before the game's main class is loaded, not from
the EntrypointPatch-injected `Hooks.startServer` path.

Both stages were measured in the **same boot**, quilt-loader 0.30.0 on
Minecraft 1.16.5: `CommandsSpy preLaunch: config loaded.` 25 seconds ahead of
`Done (17.977s)!`, and the `main` banner never at all. The Fabric legs on the
same versions are the control — without them a silent Quilt result could not be
told apart from a broken class or a broken declaration, since Quilt reads
`quilt.mod.json` and never falls back to `fabric.mod.json` for this jar.

So on Quilt below 1.18 the class is initialised at boot after all, and both
consequences listed above are the `main` entrypoint's, not the loader's:
`config/commands-spy.json` is created at startup as MOD.md promises, and a
malformed config fails at boot rather than on the first command. Only the
banner itself is still missing. (The config file's boot-time creation is
asserted; the malformed-config timing follows from the same class
initialisation but is not separately measured.)

The probe logs its own string precisely because `Loading CommandsSpy` is the
expected-absent tripwire above — emitting that literal from a working preLaunch
would report a closed upstream bug that has not closed.

### Why a Quilt-native entrypoint cannot close this gap

Do not re-propose one. The jar already ships a native Quilt entrypoint
declaration — `quilt.mod.json`'s `quilt_loader.entrypoints.main` names
`CommandsSpyFabric` — and under quilt-loader 0.30.0, the version the e2e
harness pins, that declaration is what the loader honours on every version
where the banner *does* print: once the Quilt plugin returns a load option
for a jar, the Fabric plugin is never consulted for it. The same class, the
same `net.fabricmc.api.ModInitializer` interface and the same declaration
are invoked on 1.18.2 and silently skipped on 1.17.1, out of the same mc114
jar on the same Java 17 floor. Everything on the entrypoint side is constant
across the boundary; only the Minecraft version changes, so nothing on the
entrypoint side can be the cause.

Swapping to `org.quiltmc.loader.api.ModInitializer` is not merely useless, it
is rejected outright, and it costs a great deal to try: one jar serves both
loaders, so a Quilt superinterface on the shared entrypoint class becomes a
`NoClassDefFoundError` under Fabric Loader, and avoiding that needs a
Quilt-only class plus an `org.quiltmc:quilt-loader` dependency plus the Maven
repository declaration `build.gradle`'s empty `repositories { }` block does not
have — to restore one banner line on versions where every command is already
logged. The defect sits upstream of entrypoint dispatch: the type of an object
that is never dispatched to cannot decide whether the dispatch happens.

Measured, not merely argued: declaring the same class under `quilt.mod.json`'s
`pre_launch` stage crashes the server at boot on 1.16.5 with
`LanguageAdapterException: ... cannot be cast to
org.quiltmc.loader.api.entrypoint.PreLaunchEntrypoint`. quilt-loader's
`QuiltLoaderImpl.invokePreLaunch` dispatches `pre_launch` against Quilt's own
interface and `preLaunch` against Fabric's, and its `main`, `client` and
`server` stages all require the Fabric types — so a Quilt-typed initializer
under `main` would be rejected on every version, including the ones where the
banner works today. The two stages also reach their call sites by different
routes: `preLaunch` is invoked directly from `Knot.init`, while `main` is
dispatched from `Hooks.startServer` — which is reached only through the
`EntrypointPatch` bytecode injection into Minecraft's own main class. So a
working `preLaunch` below 1.18 shows the entrypoint storage and language-adapter
layers are sound there; it does not show that the patch fired or that
`Hooks.startServer` was reached, and those remain the open suspects.

## NeoForge: one band jar on a different contract

NeoForge is structurally unlike Quilt. Quilt rides the existing four jars for
free — one jar, both loaders, no new code. NeoForge needs its own entrypoint
class, its own metadata and its own standalone Gradle build. What it does *not*
need is a jar per Minecraft version: **one** jar,
`commandsspy-<ver>+mc1.20.2-26.2-neoforge.jar`, covers every Minecraft version
NeoForge has ever published for. `make build-neo` builds it
(`gradle -p neoforge build -PneoTarget=all`).

### Why one jar spans the whole NeoForge history

Verified twice over, independently: `javap` over 13 published
`neoforge-<v>-universal.jar` artifacts spanning `20.2.93` to `26.2.0.64`, and
reading the NeoForge / FancyModLoader / EventBus sources across their branches.

- **There is no SRG era for NeoForge at all.** NeoForge has shipped Mojang
  official names since `20.2`, its first release. The mapping wall that forces
  the Forge jars apart — SRG member ids, SRG class identities, a reobfuscating
  renamer — has no analogue here, so nothing in the jar's bytecode is pinned to
  one Minecraft version.
- **`net.neoforged.neoforge.event.CommandEvent` is identical on every line**:
  same package, same class, `public ParseResults<CommandSourceStack>
  getParseResults()`, `extends Event implements ICancellableEvent`, fired from
  `Commands.performCommand`.
- **`NeoForge.EVENT_BUS` is `public static final IEventBus` on every line.**
  NeoForge never adopted Forge's per-event `CommandEvent.BUS` — the exact seam
  that forced the Forge eventbus7 jar into existence. The EventBus library bump
  from 7.2.0 to 8.0.x at Minecraft 1.20.6 leaves the `addListener` overload set
  byte-identical.
- **The no-arg `@Mod` constructor is accepted by FML 1.x through 11.x.**
- **Every Minecraft symbol this mod touches is unchanged 1.20.2 to 26.2**:
  `CommandSourceStack.getTextName`/`getEntity`, `ServerPlayer.getName`, and
  brigadier's `ParseResults.getReader().getString()` /
  `getContext().getSource()`. That holds through the 26.x rename wave, which
  renamed plenty of other things (`ResourceLocation` -> `Identifier`) but not
  these.

Fabric needs four jars because the *Fabric toolchain* axis moves (intermediary
mappings, mixin compat levels, bytecode floors). Forge needs four because its
*mapping* and *EventBus* axes move. NeoForge moves neither, so the only thing
that can split a NeoForge jar is metadata this repo writes itself.

### The two metadata seams, and why one jar still crosses them

FML's mod-metadata contract changed twice, and both changes are pure file
format — no code, no mappings:

1. **Filename.** FML 1.x/2.x (Minecraft 1.20.2-1.20.4) read only
   `META-INF/mods.toml`; FML 3.x and later read `META-INF/neoforge.mods.toml`.
2. **Dependency key.** FML 1.x/2.x require `mandatory = true` and reject
   `type`; FML 4.x and later dropped `mandatory` and require `type =
   "required"`.

The jar ships **both files**, templated from the same values by
`neoforge/build.gradle`'s `processResources`. Each FML major reads only the
filename it knows and never looks at the other, so the incompatible dependency
keys never meet. That is the whole trick, and it is why one jar spans the seam
instead of two jars straddling it.

### Measured boot table

Real NeoForge dedicated servers, full e2e assertion set (mod banner, console
`list`, RCON `save-all`, player `list`), one row per Minecraft version NeoForge
publishes for. Rows marked `(beta)` run a beta loader build because that line
never published a stable one — the compile anchor is never one of them (see
"Build" below). The 21-row version -> loader-build -> Java-floor mapping this
table is measured against lives in `scripts/e2e-run-one.sh`, queryable with
`--print-neo-routing <mcver>`; `scripts/test-jar-routing.sh` pins it.

Beta-only Minecraft versions: 1.20.3, 1.20.5, 1.21.2, 1.21.6, 1.21.7, 1.21.9,
26.1, 26.1.1.

| Minecraft | NeoForge | FML | Java | Result |
|---|---|---|---|---|
| 1.20.2 | `20.2.93` | 1.0.16 | 17 | PASS |
| 1.20.4 | `20.4.251` | 2.0.17 | 17 | PASS (compile anchor) |
| 1.20.6 | `20.6.139` | 3.0.45 | 21 | PASS |
| 1.21.1 | `21.1.248` | 4.0.43 | 21 | PASS |
| 1.21.11 | `21.11.45` | 10.0.36 | 21 | PASS |
| 26.2 | `26.2.0.64` | 11.0.16 | 25 | PASS |

**One jar, six measured rows, six FML majors — 1 through 11.** 1.20.2 runs
FML 1.0.16, which reads `META-INF/mods.toml` and demands `mandatory`; 26.2 runs
FML 11.0.16, which reads `META-INF/neoforge.mods.toml` and demands `type`. The
same jar file satisfies both, which is the dual-metadata trick working exactly
as the file-format reading predicted. 1.20.6 is the row that mattered most
beyond the edges: FML 3.x is the major whose dependency-key handling was
documented but never observed here, and it passes.

Java-17 bytecode was also confirmed to run all the way up: the jar is compiled
`--release 17` and boots unchanged on the Java-21 (1.20.6-1.21.11) and Java-25
(26.2) runtimes. Bytecode binds only downward, and this is the measurement of
that claim rather than an assumption.

Measured on GitHub Actions in the PR for #33 (run 32386749463), full assertion
set per row. `config/commands-spy.json` behaviors (blacklist suppression,
`logArguments`, auto-creation) are asserted separately by the
`CONFIG_VARIANT=1` leg on 1.21.1, which also passes.

### Hard floors

- **Minecraft 1.20.2 is the floor, permanently.** `net.neoforged:neoforge`
  starts at `20.2.12-beta`; there is no NeoForge for 1.14-1.20.1. The entire
  `src/mc114` era and most of `src/mc1192` are out of reach by construction.
  A version below the floor under `LOADER=neoforge` is an explicit
  `neoforge-unsupported-version` failure, not a jar that quietly fails to load.
- **1.20.1 is not NeoForge.** It is MinecraftForge `47.1.x` — `net.minecraftforge.*`
  packages, manifest-declared mixin configs. Covered by the Forge legacy jar,
  not by this one.
- **A Loom-built jar can never load on NeoForge**, at any version. The barrier is
  the loader contract (`fabric.mod.json` + `ModInitializer` vs
  `neoforge.mods.toml` + `@Mod`), not the mappings — it holds even on 26.x where
  both sides use real names.

### `CommandEvent`, not a mixin

The NeoForge jar contains no mixin. NeoForge fires
`net.neoforged.neoforge.event.CommandEvent` from `Commands.performCommand` —
the exact instruction `src/mc26`'s mixin injects at `@At("HEAD")` of. Same hook
point, so **coverage is identical** (player, console, RCON, command block) and
so is the blind spot: datapack functions and `/execute run` sub-commands have
gone through `Commands.executeCommandInContext` since 1.20.2 and are seen by
neither mechanism. That is a pre-existing gap in the Fabric behaviour, not a
NeoForge regression.

A mixin would buy nothing here and cost more: mixin compatibility levels and a
per-Minecraft-version descriptor dependency — the thing that already forced four
source sets on the Fabric side, and the thing that would have re-split this jar.
The event's signature is NeoForge API, so it does not move when Minecraft's
does. The Fabric mixins stay exactly as they are; this is a per-loader choice of
the cheapest hook reaching the same instruction, not a migration.

The event carries no raw command string; `getParseResults().getReader().getString()`
is the equivalent of the mixins' `fullCommand` parameter.

`@EventBusSubscriber` is deliberately not used: the annotation *infers* which bus
to dispatch on and that inference has moved across these lines, and a wrong guess
fails silently — no listener, no error, no log line. An explicit
`NeoForge.EVENT_BUS.addListener` is unambiguous and compiled.

### Build

`neoforge/` is a **standalone Gradle build**, not a subproject:
`gradle -p neoforge build -PneoTarget=all`, wired into `make build`.
ModDevGradle and Fabric Loom are not supported in one Gradle project
([ModDevGradle#234](https://github.com/neoforged/ModDevGradle/issues/234)) and
the root build applies Loom unconditionally, so keeping them in separate builds
means the two plugins never meet — and the root build's four invocations, its
`org.gradle.jvmargs`, and the Fabric jars' output all stay untouched.

**The compile anchor is `20.4.251`, not the band's floor line `20.2.93`.**
ModDevGradle 2.x resolves `net.neoforged:neoforge` through the
`neoforge-moddev-bundle` capability, and the 20.2/20.3 lines predate it —
asking for them fails resolution outright with "Unable to find a variant ...
with the requested capability". `20.4.251` is the oldest line that publishes the
capability and is still a Java-17 line, which is what the band floor needs.
`CommandEvent` and `NeoForge.EVENT_BUS` are byte-identical on `20.2.93` and
`20.4.251`, so the anchor choice costs no API; the floor of the shipped range is
still 1.20.2, established by the boot table, not by the anchor.

Anchors must be **stable** builds. An e2e leg may run a beta build where the
line never published a stable one — those rows are flagged in the boot table —
but nothing this repo compiles against is a beta.

**Toolchain and `--release` are separate axes.** `Dockerfile.ci` bakes JDK 21
and JDK 25 only, with toolchain auto-download disabled, and Checkstyle 13.10
refuses to run on a JVM below 21 — so the band compiles **on** JDK 21 and
**targets** release 17 (`java_release_all=17`). Bytecode binds only downward:
Java-17 bytecode runs unchanged on the Java-21 and Java-25 runtimes the upper
half of the band uses. `options.release` is not cosmetic here — it also sets the
project's `org.gradle.jvm.version` consumer attribute, and a line's
`net.neoforged.fancymodloader:loader` publishes exactly one Java variant, so a
mismatch fails dependency resolution with "no matching variant" rather than
silently downgrading.

**`modLoader` and `loaderVersion` in both toml files are mandatory**, despite
being widely documented as optional. Omitting `modLoader` makes FML reject the
jar with `InvalidModFileException: Missing ModLoader in file` and crash the
server during pre-load. `loaderVersion` is the **FML** version — a third version
axis, distinct from both the NeoForge and Minecraft versions (FML 1.x ships with
NeoForge 20.2.x, 4.x with 21.1.x, 11.x with 26.2.x), hence `fml_range_all=[1,)`.

Two pinning traps, both easy to walk into:

- **26.x NeoForge versions have four components** (`26.2.0.<n>`), not three.
- **Maven's `<release>` marker for `net.neoforged:neoforge` resolves to
  `26.1.2.97`**, which sorts *above* `26.2.0.64` under NeoForge's own scheme.
  Never auto-pin from it.

The first build of the band runs ModDevGradle's NeoForm pipeline (decompile +
recompile Minecraft, ~8-9 minutes measured); it is cached in `GRADLE_USER_HOME`
afterwards and subsequent builds take seconds. Only `LOADER=neoforge` e2e runs
depend on this jar, so a Fabric or Quilt run never pays for it.

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

Quilt Loader's Fabric-compatibility layer will load a jar that carries only a
`fabric.mod.json` — that is why this mod already ran on Quilt before it
shipped any Quilt metadata. But once a `quilt.mod.json` is present in the jar,
that file takes precedence and Quilt ignores the jar's `fabric.mod.json`;
Fabric Loader ignores `quilt.mod.json` entirely. This project ships both in
the same jar, so on Quilt the metadata actually in force — declared ranges and
entrypoint alike — is `quilt.mod.json`'s, and an accurate Quilt platform badge
is a consequence of that, not the reason for it. All four eras
declare the same Quilt Loader floor, `>=0.30.0`, and the same Java floor as
their Fabric counterpart (Quilt Loader itself imposes no additional JVM
floor at any era). This is asserted, not assumed: every version the e2e matrix
runs, it runs on **both** loaders — each Fabric leg has a `-quilt` twin with an
identical version list and the identical assertion set (`LOADER=fabric`/
`LOADER=quilt`). That matrix is a per-band sample of the declared ranges, not
every version in them — see
[Default e2e version list](#default-e2e-version-list) below for which versions
are sampled and which you must run by hand. See also
[e2e-harness.md](e2e-harness.md) → "Quilt server install".

NeoForge is another value of that same axis (`LOADER=neoforge`), and since the
band jar covers 1.20.2-26.2 it is orthogonal to the version list from 1.20.2
up — but no further: NeoForge publishes nothing below 1.20.2, so every earlier
version under `LOADER=neoforge` is an explicit `neoforge-unsupported-version`
failure rather than a jar that cannot load. Which loader build each Minecraft
version installs, and NeoForge's own Java floor for it, come from the routing
table in `scripts/e2e-run-one.sh` (`--print-neo-routing <mcver>`), not from
`neoforge/gradle.properties`, which pins only the band's compile anchor and
metadata ranges; `scripts/test-jar-routing.sh` fails if the table drifts.

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

## Forge: four jars, narrower by construction

Forge ships **four** jars — mc116, legacy, modern and eventbus7 — one per
mapping/EventBus era; together they cover Minecraft 1.14 through 26.2 apart from
two interior holes, 1.17 and 1.20.5, that no jar claims — Forge published no
server build for either version, which is why the ranges skip them. This
section covers the `modern` jar, `commandsspy-<ver>+mc1.21.x-forge.jar`, which
landed first (issue #23 phase 1); the legacy, eventbus7 and mc116 sections
below cover the other three.

All four are built by a separate Gradle project in `forge/` (ForgeGradle 7, its
own `settings.gradle`/`build.gradle`/`gradle.properties`) that is not part of
the root build — `make build` still produces exactly the four jars above; the
Forge jars are `make build-forge`, `build-forge-legacy`, `build-forge-mc116`
and `build-forge-eventbus7`, on demand. The modern jar compiles the same
shared core
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
- **`<1.21.6` is a hard ceiling, now measured**: Forge's EventBus 6 -> 7 API
  break lands exactly at 1.21.6/Forge 56 (`MinecraftForge.EVENT_BUS.addListener`
  -> `CommandEvent.BUS.addListener`; `Event` -> `MutableEvent` + `Cancellable`)
  and needs a second entrypoint variant — the EventBus-7 jar below, whose
  measured floor is exactly this jar's ceiling.
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
| 1.16.4 | 35.1.4 | FAIL — `NoSuchMethodError` inside Forge's own `cpw.mods.modlauncher.SecureJarHandler`, before any mod code runs. Root-caused under issue #30: the JDK 8u321+ `ManifestEntryVerifier` change vs 2020-era ModLauncher, see the mc116 section below. |
| 1.16.5 | 36.2.34 | FAIL — `NoClassDefFoundError: net/minecraft/commands/CommandSourceStack`. Decisive for THIS jar: see below. The era is now covered by the mc116 jar. |
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

One jar spans Minecraft **1.17.1–1.20.4**: ten Minecraft versions across
nine Forge branches (37, 38, 39, 40, 42, 43, 47, 48, 49) — directly
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
A 1.16.x-and-below jar needs its own compile pass against
1.16.5-shaped official mappings (a genuinely separate source variant, the
same shape the Fabric mc114/mc1192/mc121 source-set split already uses for
exactly this reason), not just a different renamer mapping file on the
existing compiled output. Issue #30 built exactly that — the `mc116` target,
see its own section below — which is why 1.16.x stays out of THIS jar's
range: the fix is a fourth compile target, not a wider range on this one.

### Gate coverage

`.github/workflows/ci.yml`'s `e2e-forge-legacy-java17` job reads every
measured PASS version above from `tools/gen_matrix.go`'s
`forge_legacy_java17` output — unlike the modern jar's job, which lists
only the two edges plus 1.21.1, the whole point of this range was
proving SRG member-id stability *across* seven Forge major branches, so a
floor+ceiling-only gate would not exercise the thing being measured.
`scripts/e2e-run-one.sh` routes each Minecraft version to the mc116, legacy,
modern or eventbus7 jar by a single case statement (`FORGE_JAR_BAND`, probed
via `--print-forge-routing`); `scripts/test-jar-routing.sh` asserts that
routing offline, including that versions outside every range come back
refused rather than silently handed a jar.

## Forge EventBus-7 jar: one jar from 1.21.6 through 26.2, and 26.x collapses in

Issue #32 task 2. Forge 56 (Minecraft 1.21.6) ships EventBus 7: `Event` ->
`MutableEvent` + `Cancellable`, and the global `MinecraftForge.EVENT_BUS` is
replaced by a static per-event bus, so the modern entrypoint's registration
call cannot compile there — and an EventBus-7 registration cannot compile on
Forge 55 and below. The fix is a **sibling entrypoint source**, not a
mapping trick: `forge/src/eventbus7/java/.../CommandsSpyForge.java` is the
same class, package and hook body with one line changed
(`CommandEvent.BUS.addListener(...)` in the `@Mod` constructor), selected by
`-PforgeTarget=eventbus7`, which swaps only the entrypoint source dir and
keeps compiling the shared core from its one copy. Compile anchor Minecraft
1.21.8 / Forge 58.1.0 (recommended-promoted), Java 21 toolchain, official
mappings at runtime like the modern jar, no renamer. `make
build-forge-eventbus7` builds it; `MOD_JAR_FORGE_EB7` in the Makefile names
it (`commandsspy-<ver>+mc1.21.6-26.2-forge.jar`).

### Measured range

One jar, booted as a real Forge dedicated server with the full e2e
assertion set (mod banner, console `list`, RCON `save-all`, player `list`),
`minecraft_range` provisionally `[1.21.6,)` during measurement so mods.toml
metadata was never the limiting factor. Every version Forge publishes above
1.21.5 was booted — no sampling:

| Minecraft | Forge build | Result |
|---|---|---|
| 1.21.6 | 56.0.9 | PASS — the EventBus 6/7 seam is exactly the modern jar's `<1.21.6` ceiling |
| 1.21.7 | 57.0.3 | PASS |
| 1.21.8 | 58.1.0 | PASS (compile target) |
| 1.21.9 | 59.0.5 | PASS |
| 1.21.10 | 60.1.0 | PASS |
| 1.21.11 | 61.2.0 | PASS |
| 26.1 | 62.0.9 | PASS (java 25) |
| 26.1.1 | 63.0.2 | PASS (java 25) |
| 26.1.2 | 64.1.0 | PASS (java 25) |
| 26.2 | 65.1.0 | PASS (java 25) |

One jar spans Minecraft **1.21.6–26.2**: ten versions across ten
consecutive Forge major branches (56–65), every one Forge publishes above
1.21.5 — the range ends at 26.2 because Forge publishes nothing newer, not
because anything failed. Shipped `forge/gradle.properties` (`_eventbus7`
suffix): `minecraft_range = [1.21.6,26.3)`, `forge_range`/`loader_range =
[56,66)`.

### 26.x collapses into this jar — unlike Fabric's mc26 band

The open measurement question was whether 26.x (unobfuscated Minecraft)
needs its own jar the way Fabric's mc26 band does. Answer: **no**. Fabric's
mc26 split exists because the *Fabric* toolchain axis changed (intermediary
withdrawn in favor of official names); on Forge the modern/eventbus7 era
already compiles and runs official names, so 26.x removing obfuscation
changes nothing the jar can see — 26.1 through 26.2 booted the same
java-21-bytecode jar built against 1.21.8, on the default installer JDK,
with the full assertion set passing. The 26.1.1/26.1.2 patch releases are
included in the measured table (and CI) because each is its own Forge major
branch (63/64), which is the axis this band's measurement actually probes.

### Gate coverage

Same shape as the legacy band, for the same reason: the measurement's point
was one jar spanning ten EventBus-7 Forge majors, so
`.github/workflows/ci.yml` boots **every** measured version, split across
two generated jobs by era Java floor — `e2e-forge-eventbus7-java21`
(1.21.6–1.21.11) and `e2e-forge-eventbus7-java25` (26.1–26.2; 26.x servers
require Java 25) — reading `tools/gen_matrix.go`'s `forge_eventbus7_java21`
/`forge_eventbus7_java25` outputs, keyed on the `minecraft_range_eventbus7`
line in `forge/gradle.properties`.

## Forge mc116 jar: one jar from 1.14.4 through 1.16.5, and the 1.16.4 JDK wall

Issue #30. Two gated measurements: root-cause the 1.16.4 pre-mod-code
crash, and answer whether pre-1.17 SRG member ids are stable enough for one
compiled-once jar to span the 1.14–1.16 era. Both answered; the jar
shipped.

The target: `-PforgeTarget=mc116`, compile anchor Minecraft 1.16.5 / Forge
36.2.42, `--release 8` bytecode (1.16-era servers run Java 8). It needs BOTH
things the other Forge targets need only one of: its own entrypoint source
(`forge/src/mc116/java`) like eventbus7, **and** the SRG renamer like
legacy. The source variant exists because pre-1.17 dev-time class names are
the MCP ones — ForgeGradle 7's `official` channel for 1.16.5 materializes
`net.minecraft.command.CommandSource` and
`net.minecraft.entity.player.ServerPlayerEntity` (Mojang *member* names on
MCP *class* names), and Forge 36's own `CommandEvent.getParseResults()`
signature references them, so the 1.17-era entrypoint cannot compile there
(measured: `package net.minecraft.commands does not exist`). The renamer
then maps the members down to the runtime SRG shape — the class names
already match the runtime, so members are the only thing left — `javap` on
the shipped jar shows `net/minecraft/command/CommandSource.func_197022_f`
(`getEntity`), `ServerPlayerEntity.func_200200_C_` (`getName`),
`CommandSource.func_197037_c` (`getTextName`), the pre-1.17 `func_xxxxx_`
vocabulary. The jar also ships a `pack.mcmeta` (`forge/src/mc116/resources`):
Forge 32.x (1.16.1) throws an NPE in its own `ResourcePackLoader` on any mod
jar without one, measured live; later Forges only warn. `make
build-forge-mc116` builds it; `MOD_JAR_FORGE_MC116` in the Makefile names it
(`commandsspy-<ver>+mc1.16.x-forge.jar`).

### The same shape, at the other end: Forge modern on Java 24+

Forge's **modern** band (1.20.6-1.21.5) is **Java 21 only** — it has no
forward-JVM headroom at all, for a JVM-internals reason with no mod in it. `net.minecraftforge.bootstrap` 2.1.7
fails module resolution because `com.nimbusds.jose.jwt` requires
`jdk.crypto.ec`, a JDK module **removed in Java 24** (its EC support folded
into `java.base`):

```
java.lang.module.FindException: Module jdk.crypto.ec not found, required by com.nimbusds.jose.jwt
	at net.minecraftforge.bootstrap@2.1.7/net.minecraftforge.bootstrap.Bootstrap.moduleMain(Bootstrap.java:166)
```

| Minecraft / band | JVM | Result |
|---|---|---|
| 1.21.5, modern | Java 21 (the band's floor) | **PASS** — the `forge_java21` leg |
| 1.21.5, modern | Java 25 | crash: `FindException: Module jdk.crypto.ec not found`, before Minecraft starts |
| 1.21.5, modern | Java 26 | same crash, twice in CI |
| 26.2, eventbus7 | Java 26 | **PASS** — upstream fixed it in a later Forge generation |

So the modern band has **no forward-JVM headroom above its floor at all**, and
CI's forward-JVM probe (`forge_java26`) covers the eventbus7 band only. Unlike
the 1.16.4 case there is no install-time cure: you cannot `--add-modules` a
module the JDK no longer contains. Tracked as issue #66. **Not yet reported
upstream** — the fix belongs to MinecraftForge, in dropping or shading that
module requirement.

### Gate 1: the 1.16.4 crash is the JDK's `ManifestEntryVerifier` change

Reproduced, root-caused, and bounded — not fixable from this repo's jars,
but curable at server-install time (now supported in CI via the install-time
ModLauncher 8.1.3 drop-in — last row and verdict below):

| Forge build | JDK | Result |
|---|---|---|
| 35.1.4 (1.16.4 recommended) | Temurin 8 current (8u492) | crash: `java.lang.NoSuchMethodError: sun.security.util.ManifestEntryVerifier.<init>(Ljava/util/jar/Manifest;)V` at `cpw.mods.modlauncher.SecureJarHandler.createCodeSource(SecureJarHandler.java:66)`, before any mod is scanned |
| 35.1.37 (1.16.4 latest — newest build that will ever exist) | Temurin 8 current | same crash, byte-identical signature |
| 35.1.4 | Temurin **8u312** (pre-change) | **full e2e PASS** — mod loaded, console + RCON + player asserts all green |
| 35.1.4 / 35.1.37 + **ModLauncher 8.1.3 drop-in** | Temurin 8 current (8u492) | **full e2e PASS** — both builds, all asserts green (`ModLauncher 8.1.3+8.1.3+main-8.1.x.c94d18ec starting: java version 1.8.0_492`) |

JDK 8u321+ added a `Manifest` parameter to the internal
`sun.security.util.ManifestEntryVerifier` constructor; 2020-era ModLauncher
calls the old one reflectively-not-at-all — it just links against it
(upstream: McModLauncher/modlauncher#91). Forge shipped the fixed
ModLauncher only on the 1.16.5 branch (36.2.26+); every 1.16.1–1.16.4
branch is frozen before the fix, so **1.16.4 cannot boot a stock current
JDK 8 regardless of mods**. Empirically the older branches (32/33/34,
1.16.1–1.16.3) do not link the affected path and boot fine on current JDK
8 — only 35.x dies. Verdict: 1.16.4 stays inside the jar's declared range
AND in CI, on a current JDK: `scripts/e2e-run-one.sh` cures the install
the way a real 1.16.4 admin does — after `--installServer` it overwrites
the cached `libraries/cpw/mods/modlauncher/8.0.*/modlauncher-8.0.*.jar`
(8.0.6 on 35.1.4, 8.0.9 on 35.1.37; the filename is kept because the forge
jar manifest's `Class-Path` pins it) with ModLauncher **8.1.3** fetched
from Forge's own maven, sha256-pinned
(`4e0d846f75ffd0dd5042c9b1aa86b8fcc758acd27a004c259d26aebc100ffdf2` —
byte-identical to the jar every Forge 36.2.26+ install ships). With the
drop-in, both 35.1.4 and 35.1.37 pass the full assertion set on Temurin
8u492 (table above). The CI-side install cache is all that is patched; the
shipped mod jars are untouched.

### Gate 2 / measured range

One SRG-renamed jar, booted as a real Forge dedicated server with the full
e2e assertion set, `minecraft_range` `[1.14,1.17)` (the shipped range — wide
enough during measurement that mods.toml metadata was never the limiting
factor, and every measurement inside it passed, so it never needed
tightening):

| Minecraft | Forge build | Java | Result |
|---|---|---|---|
| 1.14.4 | 28.2.26 | 8 (current) | PASS |
| 1.15.2 | 31.2.57 | 8 (current) | PASS |
| 1.16.1 | 32.0.108 | 8 (current) | PASS — needed two era fixes, neither SRG-related: the jar's `pack.mcmeta` (Forge 32.x NPE, above) and the harness's flat-world `generator-settings` gaining the `structures` key its 1.16/1.16.1 codec requires (optional from 1.16.2, ignored-unknown from 1.19) |
| 1.16.2 | 33.0.61 | 8 (current) | PASS |
| 1.16.3 | 34.1.0 | 8 (current) | PASS |
| 1.16.4 | 35.1.4 / 35.1.37 | 8 (current) | PASS — via the install-time ModLauncher 8.1.3 drop-in (gate 1 above); stock 35.x still crashes pre-mod-code on 8u321+ |
| 1.16.5 | 36.2.34 | 8 (current) | PASS (36.2.26+ carries the ModLauncher fix) |

Answer to the gate-2 question: **pre-1.17 SRG member ids are frozen across
the whole band** — `func_197022_f`/`func_200200_C_`/`func_197037_c` resolve
and the `CommandEvent` hook fires identically across five consecutive Forge
major branches (28, 31, 32/33/34, 36) and three Minecraft minors, so one
jar covers 1.14.4–1.16.5 and the "one jar per version" fallback was never
needed. Confirming the 1.16.1–1.16.3 player-phase legs required a harness
fix first, same precedent as protocol 757 in the legacy band: the e2e bot's
protocol table was missing rows 736 (1.16/1.16.1), 751 (1.16.2) and 753
(1.16.3) — added to `tools/table.go` from minecraft-data dumps and confirmed
live (`keep_alive` `0x20/0x10` at 736, `0x1F/0x10` at 751/753, `chat`
`0x03` everywhere; see docs/protocol-table.md).

### Gate coverage

`.github/workflows/ci.yml`'s `e2e-forge-mc116-java8` job boots every
measured-PASS version (1.14.4, 1.15.2, 1.16.1, 1.16.2, 1.16.3, 1.16.4,
1.16.5) on java 8 — the era's real deployment JVM and the jar's own
bytecode floor; 1.16.4 rides via the gate-1 ModLauncher drop-in — reading
`tools/gen_matrix.go`'s `forge_mc116_java8` output, keyed on the
`minecraft_range_mc116` line in `forge/gradle.properties`. This replaces
the old `e2e-forge-legacy-guard-java8` leg: its 1.16.5 expected-REFUSED
probe flipped to an in-range PASS the moment a jar covered 1.16.5, and no
refusal guard below the new floor is possible — Forge's next line down
(1.13.2) is below the e2e harness's own 1.14 floor. The below-floor
metadata gate is still asserted offline: `scripts/test-jar-routing.sh`
checks the refusal flag for out-of-range versions.
