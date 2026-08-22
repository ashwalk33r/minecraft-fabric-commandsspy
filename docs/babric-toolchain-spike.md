# Babric toolchain spike

Task 1 of the [Babric loader support plan](superpowers/plans/2026-08-22-babric-loader-support.md).
Nothing in this repository had ever built against Ornithe, so the plan opens with
a spike whose deliverable is knowledge rather than code. This page records what
was run on 2026-08-22 and what it printed. Where the result contradicts the plan
or the spec, the contradiction is stated here and the observed value wins.

Everything below was executed on this machine. Nothing here is read from
documentation. The scratch tree the commands ran in was thrown away; only this
page and `babric/gradle.properties` survive it.

## Summary

The whole chain works, on the plan's primary path. The documented fallback
(`babric-loom` against Babric's own intermediary, dropping the conversion step)
was **not** taken and is not needed: `com.periut.retroconvert` is publicly
resolvable, and a jar compiled against Ornithe named mappings was
reverse-converted, installed, and loaded on a real Babric server.

Four things the plan states differently are corrected below: the loader banner's
version string, the ploceus plugin id, the maven that hosts `retroconvert`, and
what the `class_N` intermediary names actually are.

## 1. The two command seams are named in biny-ornithe

`maven-metadata.xml` for `net.glasslauncher:biny-ornithe` lists nine builds; the
`<release>` is `b1.7.3+build.57cc158` (`lastUpdated` 20260324050228). That is the
build resolved and used throughout. RetroCommands 0.7.5 pins the older
`bdae1bc`; nothing required matching it, and the newer build was used instead.

```bash
curl -fsSL -o biny.jar \
  "https://maven.glass-launcher.net/releases/net/glasslauncher/biny-ornithe/b1.7.3+build.57cc158/biny-ornithe-b1.7.3+build.57cc158-mergedv2.jar"
unzip -p biny.jar mappings/mappings.tiny > mappings.tiny
```

The file's header line is:

```
tiny	2	0	intermediary	clientOfficial	serverOfficial	named
```

**The plan's grep finds nothing, and the reason matters.** Ornithe's intermediary
namespace is Calamus gen2, which names classes `C_<digits>`, methods `m_<digits>`
and fields `f_<digits>`. The `class_11` / `method_836` names the plan and spec
quote are **Babric's** intermediary namespace, not Ornithe's. Both are correct;
they describe different stages of the pipeline. Section 4 below shows one class
crossing from one to the other.

Searching the `named` column instead, every member the plan's Task 3 mixins need
is present and every name matches what the plan predicted:

| What Task 3 needs | Named | Calamus gen2 | Descriptor |
|---|---|---|---|
| Player seam class | `net/minecraft/server/network/ServerPlayNetworkHandler` | `C_99660737` | |
| Player seam method | `handleCommand` | `m_26878237` | `(Ljava/lang/String;)V` |
| Player field | `player` | `f_49078788` | `LC_49202226;` |
| Console seam class | `net/minecraft/server/command/ServerCommandHandler` | `C_79915627` | |
| Console seam method | `executeCommand` | `m_64252524` | `(LC_81097776;)V` |
| Command object | `net/minecraft/server/command/Command` | `C_81097776` | |
| Command text field | `commandAndArgs` | `f_88660840` | `Ljava/lang/String;` |
| Command source field | `output` | `f_29949921` | `LC_23078083;` |
| Source class | `net/minecraft/server/command/CommandOutput` | `C_23078083` | |
| Source name accessor | `getName` | `m_64276922` | `()Ljava/lang/String;` |
| Player entity | `net/minecraft/entity/player/ServerPlayerEntity` | `C_49202226` | |
| Player name field | `name` (on `PlayerEntity`, `C_40996800`) | `f_59008391` | `Ljava/lang/String;` |

**Task 3 needs no change on this account.** Every `@Mixin` target, every
`@Inject` method name, and every field the plan's two mixins read exists under
exactly the name the plan gives it. One detail the plan's import list gets right
and is worth not re-deriving: `name` is inherited from `PlayerEntity`, not
declared on `ServerPlayerEntity`.

## 2. retroconvert is publicly resolvable

RetroCommands was cloned at depth 1 and its build files read directly.
`settings.gradle`, verbatim and complete:

```groovy
pluginManagement {
    repositories {
        maven{ url = "https://maven.fabricmc.net/"}
        maven{ url = "https://maven.ornithemc.net/releases"}
        maven{ url = "https://maven.ornithemc.net/snapshots"}
        // RetroAPI repository - hosts the com.periut.retroconvert plugin.
        maven{ url = "https://matthewperiut.github.io/repository"}
        mavenCentral()
        mavenLocal()
        gradlePluginPortal()
    }
}
```

Its `plugins` block, verbatim:

```groovy
plugins {
	id 'maven-publish'
	id 'net.fabricmc.fabric-loom-remap' version '1.15.+'
	id 'ploceus' version '1.15-SNAPSHOT'
	// Emits retrocommands-<version>-babric.jar: reverse-converted to babric and bundled with OSL.
	id 'com.periut.retroconvert' version '1.0.0'
}
```

Its mapping coordinate, verbatim, and its intermediary generation:

```groovy
ploceus {
	setIntermediaryGeneration(2)
}

mappings(ploceus.mappings("net.glasslauncher:biny-ornithe:b1.7.3+build.${project.biny_mappings}:mergedv2"))
```

**The plugin id is `ploceus`, not `net.ornithemc.ploceus`.** The plan's Task 2
`build.gradle` uses the latter; it is wrong and would fail plugin resolution.

**`retroconvert` is hosted on the author's GitHub-Pages maven, not on
glass-launcher.** The plan's Step 2 check targets glass-launcher and returns 404,
which would read as "not publicly resolvable" and send Task 2 down the fallback
path unnecessarily. Against the maven that actually hosts it:

```bash
curl -fsS -o /dev/null -w '%{http_code}\n' \
  "https://matthewperiut.github.io/repository/com/periut/retroconvert/maven-metadata.xml"
# 200
```

and the Gradle plugin marker resolves and points at a real implementation
coordinate:

```xml
<groupId>com.periut.retroconvert</groupId>
<artifactId>com.periut.retroconvert.gradle.plugin</artifactId>
<version>1.0.0</version>
<dependencies>
  <dependency>
    <groupId>com.periut</groupId>
    <artifactId>retroconvert-gradle</artifactId>
    <version>1.0.0</version>
  </dependency>
</dependencies>
```

Section 3 confirms it resolves in an actual build, not only over HTTP. **The
fallback path is not taken.** Nothing was vendored.

The conversion registers the task **`babricJar`**, which emits
`<archivesName>-<version>-babric.jar` alongside Loom's `remapJar` output.

One property of the plugin worth recording because it can produce a false pass:
a jar that references no Minecraft class is reported as
`detected as NEUTRAL (not ornithe); copying through unchanged`. A probe that
never touches a game class therefore proves nothing about the conversion. This
was noticed during the spike and is why section 4 exists.

## 3. The toolchain builds

A throwaway mod — one `ModInitializer`, one log line, no mixins — was built with
Gradle 9.7.0 (this repository's wrapper) on OpenJDK 21.0.11, `options.release =
21`. There is no standalone `gradle` on this machine; the wrapper was copied into
the probe tree. **Task 2's `gradle -p babric test` will not run as written here**
— `./gradlew -p babric` does.

`BUILD SUCCESSFUL`. The plugins resolved to:

```
Fabric Loom: 1.15.5
Ploceus: 1.15.9
```

so `1.15.+` resolves to Loom 1.15.5 and `1.15-SNAPSHOT` to Ploceus 1.15.9 as of
this run. The exact coordinates that built green are pinned in
`babric/gradle.properties`; the repositories the probe declared were
`maven.glass-launcher.net/releases`, `maven.glass-launcher.net/babric`,
`maven.ornithemc.net/releases`, `matthewperiut.github.io/repository` and
`mavenCentral`, with `maven.ornithemc.net/snapshots` additionally in
`pluginManagement` (Ploceus is a snapshot, so that entry is required).

The probe was thrown away, as the plan specifies.

## 4. A Babric server boots it, and the conversion is real

Installed with the Babric installer, exactly the argv the spec gives:

```bash
java -jar installer.jar server -dir "$PWD/srv" -mcversion b1.7.3 -loader 0.19.3 -downloadMinecraft
sha1sum srv/server.jar
# 2f90dc1cb5ca7e9d71786801b307390a67fcf954  srv/server.jar
```

The sha1 matches the pin in the plan's Global Constraints exactly. The installer
printed `Done, start server by running fabric-server-launch.jar` and produced
`fabric-server-launch.jar`, `libraries/` and `server.jar`. It downloaded
`babric:intermediary-upstream:b1.7.3` and `net.fabricmc:fabric-loader:0.19.3`,
confirming the upstream loader rather than the frozen fork.

The server booted the probe and exited 0 with no `eula.txt` present and without
ever asking for one.

**Verbatim loader banner**, which Task 5 greps for:

```
[10:41:04] [main/INFO] (FabricLoader/GameProvider) Loading Minecraft Beta 1.7.3 with Fabric Loader 0.19.3
```

**This contradicts the plan.** The plan's Step 4 expects
`Loading Minecraft b1.7.3 with Fabric Loader 0.19.3`. The loader prints the
human-readable **`Beta 1.7.3`**, not the `b1.7.3` version id. An assertion
written against `b1.7.3` never fires.

**Verbatim ready line:**

```
2026-08-22 10:41:15 [INFO] Done (10563057748ns)! For help, type "help" or "?"
```

Nanoseconds, as the spec says; `Done \(\d+ns\)!` matches and a modern
`Done \([\d.]+s\)!` matcher does not. The figure is not stable between runs — a
second boot on an already-generated world printed `Done (401062869ns)!`, four
orders of magnitude smaller. Only the shape may be asserted, never the number.

The loader's mod list confirms the metadata the Babric build will template:

```
[10:41:04] [main/INFO] (FabricLoader) Loading 5 mods:
	- babricprobe 0.0.1
	- fabricloader 0.19.3
	   \-- mixinextras 0.5.4
	- java 21
	- minecraft 1.0.0-beta.7.3
```

`minecraft 1.0.0-beta.7.3` and `java 21` are the exact strings
`minecraft_range_babric` and `java_range_babric` must produce. Mixin is present
and initialises — `SpongePowered MIXIN Subsystem Version=0.8.7`,
`sponge-mixin-0.17.3+mixin.0.8.7`, `Env=SERVER` — and MixinExtras 0.5.4 arrives
bundled inside the loader, so Task 3 needs to add neither.

The vanilla log prefix takes over mid-log exactly as the spec describes: loader
lines are `[HH:MM:SS] [main/INFO]`, and from `Starting minecraft server version
Beta 1.7.3` onward they are `YYYY-MM-DD HH:MM:SS [INFO]`.

### The conversion, proven on a jar that actually references a game class

Because a Minecraft-free jar is passed through untouched (section 2), the probe
was rebuilt with a single reference to `ServerCommandHandler` and the two output
jars compared:

```bash
unzip -p babricprobe-0.0.1.jar        probe/BabricProbe.class | strings | grep net/minecraft
# net/minecraft/unmapped/C_79915627
unzip -p babricprobe-0.0.1-babric.jar probe/BabricProbe.class | strings | grep net/minecraft
# net/minecraft/class_426
```

The `NEUTRAL` message did not appear on this build. `C_79915627` is Calamus
gen2; `class_426` is Babric intermediary — and `class_426` is precisely the name
the spec independently verified for `ServerCommandHandler`. That agreement is the
strongest single piece of evidence in this spike: it confirms the mapping table
in section 1, the direction of the conversion, and the spec's intermediary names,
all at once.

Booting the converted jar resolved that class at runtime on the real server:

```
[10:41:53] [main/INFO] (FabricLoader/GameProvider) Loading Minecraft Beta 1.7.3 with Fabric Loader 0.19.3
BABRIC-PROBE-LOADED net.minecraft.class_426
2026-08-22 10:41:54 [INFO] Done (401062869ns)! For help, type "help" or "?"
```

Compile against named Ornithe mappings, remap to Calamus, reverse-convert to
Babric, load on a Babric server, resolve the class. The full chain, executed.

## What this spike did not establish

- **No mixin was ever built, converted, or applied.** The probe carries no
  mixin config by the plan's own instruction. Reverse-conversion of a
  `refmap`/mixin config is a different code path from the class-constant
  rewriting proven above, and it is unproven here. If Task 3 fails, this is the
  first place to look. The spike proves the toolchain and the namespace
  crossing; it does not pre-verify Task 3.
- **No command was ever executed on the booted server.** Neither seam has been
  observed firing. That is Task 3 Step 5's job and it is not front-run here.
- **Nothing was run in Docker**, on a JVM other than 21, or under CI. The
  `eclipse-temurin:21-jre-jammy` image, the vendored installer and the pinned
  server jar in the Dockerfile are Task 7's, and untested.
- **The mirror risk is unchanged.** `files.betacraft.uk` served the pinned bytes
  today. It is one community mirror and the spec already states the consequence
  of it disappearing.
- **Ploceus is pinned to a SNAPSHOT** (`1.15-SNAPSHOT`), because that is the only
  version line Ornithe publishes for the Loom 1.15 series and it is what
  RetroCommands itself uses. It resolved to build 1.15.9 today. A snapshot can
  change under a later build with no version bump; this is a real and
  unmitigated reproducibility gap, recorded rather than engineered around.
- **`matthewperiut.github.io/repository` is a single-maintainer GitHub-Pages
  maven.** It resolved today. It carries the same class of availability risk as
  the server-jar mirror, for the one plugin that has no substitute short of the
  fallback path.
