# E2E harness

Every supported Minecraft version boots as a real server in Docker — Fabric,
Quilt, Forge, NeoForge, Babric or BTA, per the `LOADER` axis — receives commands over
console, RCON, and from a protocol-level player bot, and the log is asserted
line by line.
Entry points:

- `make e2e` — local dev: unbounded parallel fan-out over the default matrix.
- `make e2e-ci` — same flow, bounded (`PARALLEL=4` default).
- `scripts/e2e-run-one.sh <version>` — one version, one container. Always
  writes exactly one result file, even if killed; the Makefile reaps result
  files after the fan-out, so no failure can be lost.
- `scripts/e2e-entrypoint.sh` — runs inside the container: boots the server,
  drives the phases, prints the verdict.

The default version list and its sampling rationale live in
the wiki's [Version boundaries and root causes](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/Version-Boundaries-And-Root-Causes).

## Phases and assertions

1. **Boot** — Fabric launcher fetches the vanilla server; the entrypoint waits
   (bounded by `BOOT_TIMEOUT`, default 180s) for `Done`. A JVM that dies at
   mixin/loader bootstrap can deadlock in its shutdown hooks, so a fatal
   bootstrap crash fails fast instead of waiting out the timeout.
2. **Console + RCON** — `list` via the console fifo, `save-all` via RCON. The
   RCON client retries a connection that breaks after being established, three
   attempts a second apart (issue #89); a refused dial is never retried, which
   is what keeps the Babric/BTA absence probe (below) instant.
3. **Player phase** (`PLAYER_PHASE=1`, the default) — the baked-in Go bot
   (`tools/`) joins two protocol-level players. `e2e_player1` sends `/list`;
   `e2e_player2` joins and sends nothing. The bot's global timeout is 150s;
   the entrypoint wraps it in an outer 160s `timeout` as the belt to the
   bot's braces.

The default leg also asserts, on the config as-shipped: a non-existing
command still produces a `[CommandsSpy]` line; with `logArguments: false`
(the default) an argument-bearing command is logged with the bare command
name and not its arguments; and `config/commands-spy.json` matches the
documented initial schema. The schema check reads the file from disk after
the run completes, so it proves content, not timing; the separate
config-at-boot assertion is what proves the file existed at server startup.

Verdict line grammar (the final line of container output is authoritative):

- `E2E <version> PASS`
- `E2E <version> PASS players-skipped-unsupported-protocol` — console+RCON
  only; reachable only on standalone `PLAYER_PHASE=0` runs.
- `E2E <version> FAIL <code>,<code>,...` with codes: `mod-not-loaded`,
  `mixin-not-applied`, `console-command-not-logged`, `rcon-command-not-logged`,
  `player-command-not-logged`, `player-misattributed`, `boot-failed`,
  `neoforge-install-failed`. `scripts/e2e-run-one.sh` adds
  `below-java-floor-<n>` for a JVM below the version's floor,
  `above-java-ceiling-<n>` for a JVM above a band's ceiling — only the Forge
  `modern` band has one (java 21; issue #66),
  `neoforge-unsupported-version` for a Minecraft version below 1.20.2, which
  is NeoForge's own floor, and `mod-jar-missing` when the jar under test does
  not exist
  (`docker run -v <missing path>` silently creates an empty directory and mounts
  that, so without this check a build or path bug arrives disguised as
  `mod-not-loaded` on a perfectly healthy server). The default leg also adds
  `unknown-command-not-logged`, `config-not-autocreated`,
  `logargs-default-bare-name-missing`, and
  `logargs-default-leaked-arguments` for the config-as-shipped assertions
  above.
- `E2E <version> PASS forge-out-of-range-refused-as-expected` — reachable
  only when `FORGE_EXPECT_REFUSED=1` (a Forge leg outside the jar's declared
  Minecraft range): Forge refused the mod on mods.toml's `minecraft`
  dependency range (`Mod commandsspy requires minecraft ...`) and no
  `[CommandsSpy] [` line was ever logged. See "Forge server install" below.
- `E2E <version> PASS fabric-out-of-range-refused-as-expected` — the
  Fabric/Quilt mirror, reachable only when `FABRIC_EXPECT_REFUSED=1` (today
  that is Minecraft 1.19.0, which falls in the crack between the mc114
  ceiling `<1.19` and the mc1192 floor `>=1.19.1`): the loader refused the
  mod, so no `Loading CommandsSpy` banner was ever logged. Its one `FAIL`
  code is `fabric-out-of-range-not-refused`. The leg is handed the mc1192
  jar deliberately — the one a real operator would install — so what it
  asserts is that jar's `fabric.mod.json`/`quilt.mod.json` range gate, not an
  absent file.
- `E2E <version> PASS config-behaviors` — the config-behaviors leg
  (`CONFIG_VARIANT=1`): a `config/commands-spy.json` is seeded before boot
  with a blacklisted command and `logArguments: true`, and the leg asserts
  the blacklist suppresses logging while `logArguments: true` logs the
  command's arguments. Its `FAIL <code>,<code>,...` codes are
  `config-variant-mod-not-loaded`, `blacklist-not-suppressed`, and
  `logargs-true-not-logged`.

The player assertions are a positive/negative pair: player1's `/list` must be
attributed to player1, and player2 (who sent nothing) must appear in **zero**
`[CommandsSpy]` lines. The negative half catches misattribution that the
positive half alone would miss.

Warnings are a separate channel and are not part of the verdict. A leg that
only reached PASS because something was retried prints, immediately above the
log dump:

- `[e2e] ⚠ warnings (not failures): rcon-retried` — the RCON client had to
  reconnect at least once. The leg still PASSes; `FAILURES` is untouched.
  Grepping a CI run for `rcon-retried` (or, in the raw job log, for
  `[rcon] attempt`) is how the frequency of issue #89's flake is counted
  without needing a red build to notice it.

## Era-exact literals

Assertions use the exact literal for the version under test, never a pattern
matching both eras — an assertion that cannot fail proves nothing.

- **RCON source name**: `Recon` on 1.14.4–1.15.2, `Rcon` on 1.16+. Vanilla
  1.14–1.15 holds two constants ("Recon" for the command-source name, "Rcon"
  for the internal logger); 1.16 collapsed them to a single "Rcon". Pinned by
  disassembling the adjacent stable releases 1.15.2 and 1.16.
- **Player `/list` literal**: 1.14–1.18.x deliver commands as chat messages,
  slash included, so the mod logs `/list`; 1.19+ use the dedicated
  `chat_command` packet, whose payload has no slash, so the mod logs `list`.
- **Mixin failure phrasing**: older Mixin reports "was not found", modern
  Mixin "could not find any targets matching"; the assert greps both.
- **Console source name**: `CONSOLE` on Beta 1.7.3, `Server` on 1.14+. Both come
  from the game's own `CommandOutput.getName()` rather than from this mod —
  b1.7.3 vanilla prints `CONSOLE: Stopping the server..` for the same reason.
  Measured on a booted b1.7.3 server, not disassembled.

## Boot-time tuning

The server runs a void world (superflat, `air` preset, generate-structures
off): nothing to generate cuts ~8s off `Done()`. World caching was measured
too and rejected — the 3–5s cache restore costs more than it saves on a void
world. `view-distance`/`simulation-distance` 3 is the vanilla floor (lower
values are silently clamped up). `max-players=5`: two bots plus headroom.
`-XX:+AlwaysPreTouch` is deliberately not used — it front-loads the cost it
is meant to hide.

## Jar cache

`E2E_JAR_CACHE` (default `~/.cache/commandsspy-e2e-jars`) bind-mounts a
host-side cache of the per-version download artifacts (Fabric launcher +
vanilla server jar) into every container, saving ~1.35GB of re-downloads per
full-matrix run. Mutable `.fabric` state is deliberately not cached — only
immutable downloads. The cache is populated only from a successfully booted
server, atomically via stage-dir + `mv`, because parallel jobs race.
Disposable: `rm -rf` it any time; `make e2e E2E_JAR_CACHE=` disables it.

## Quilt server install

`LOADER=quilt` (default `fabric`) swaps which server boots. Quilt's
install tool, `quilt-installer`, requires a Java 17+ JVM to run — but some
server containers in the matrix run Java 8 (the `mc114` band) or Java 11,
so the install cannot happen inside the container under test. Instead
`scripts/e2e-run-one.sh` runs it on the **host**, before starting the
server container, via a one-off `eclipse-temurin:17-jre-jammy` container
that downloads `quilt-installer` and runs
`install server <version> <loader-version> --download-server`, producing
`quilt-server-launch.jar` + `server.jar`. Those two files are cached under
`E2E_JAR_CACHE` (key `quilt-<version>-loader<N>-installer<N>`, alongside
the existing Fabric cache entries) and bind-mounted read-only into the
server container at `/quilt-preinstalled`; `scripts/e2e-entrypoint.sh`'s
`LOADER=quilt` branch just copies them into its working directory and
launches `quilt-server-launch.jar` exactly like the Fabric path launches
`fabric-server-launch.jar` — same console-fifo boot wait, same RCON/player
assertions, all of which are loader-agnostic. Default pins:
`QUILT_LOADER_VERSION=0.30.1`, `QUILT_INSTALLER_VERSION=0.15.1` (both
hardcoded in `scripts/e2e-run-one.sh`, mirroring how the Fabric harness's
own `LOADER_VERSION`/`INSTALLER_VERSION` defaults live in
`scripts/e2e-entrypoint.sh` rather than in `gradle.properties`).

The loader pin is not free-floating: `scripts/test-jar-routing.sh` asserts it
equals the `quilt_loader_range_*` floor declared for all four eras in
`gradle.properties`. That floor is `>=0.30.1` because every quilt-loader below
it silently never invoked the `main` entrypoint on dedicated servers under
Minecraft 1.18.2 (QuiltMC/quilt-loader#500), so the mod's startup promise only
holds on 0.30.1 and up. Bumping one number without the other now fails the
offline contract instead of leaving a declared requirement that no leg boots.

## Forge server install

`LOADER` accepts `fabric|quilt|forge|neoforge` (Makefile, `scripts/e2e-run-one.sh`,
`scripts/e2e-entrypoint.sh`). Forge build resolution reads
`promotions_slim.json` and prefers `<mc>-recommended`, falling back to
`<mc>-latest` when no recommended build exists for that Minecraft version
(true for 1.21, which ships only `-latest`). The resolved installer,
`maven.minecraftforge.net/.../forge-<mc>-<build>-installer.jar`, is run
host-side — `java -jar installer.jar --installServer` inside a one-off
`eclipse-temurin:21-jdk-jammy` container — the same host-side-install trick
the Quilt path above already uses. Output is cached under `E2E_JAR_CACHE`
as `forge-<mc>-<build>` and bind-mounted read-only into the server
container at `/forge-preinstalled`.

`SERVER_LAUNCH_JAR` is gone, replaced by `SERVER_LAUNCH_ARGS`. This is a
real refactor, not a new branch: Forge is neither a Fabric fat jar nor a
Quilt thin launch jar. 1.17+ installs
`libraries/net/minecraftforge/forge/<mc>-<build>/unix_args.txt`, launched as
`java <flags> @<argfile> nogui`; <=1.16.5 instead produces a runnable
`forge-<mc>-<build>.jar`. The loader-varying thing had to become the whole
launch argument list, not just a jar name.

Forge legs run with `-Xms1G -Xmx1G` instead of the 512M tuned for
vanilla+Fabric — Forge's ModLauncher/transformer stack does not fit in
512M. The `mixin-not-applied` assertion is skipped for Forge: there is no
Mixin on the Forge side (a `CommandEvent` listener — on
`MinecraftForge.EVENT_BUS`, or `CommandEvent.BUS` in the EventBus-7 band —
not a Mixin injection), so the command-logged assertions are what prove the
hook is live. Every other assertion carries over unchanged.

An out-of-range GUARD leg exercises the boundary directly:
`scripts/e2e-run-one.sh` sets `FORGE_EXPECT_REFUSED=1` for any Minecraft
version outside the jar's declared range, and `scripts/e2e-entrypoint.sh`
then asserts (a) Forge refused the mod on mods.toml's `minecraft` dependency
range (`Mod commandsspy requires minecraft 1.20.6 or above, and below
1.21.6`) and (b) no `[CommandsSpy] [` line was ever logged — verdict `PASS
forge-out-of-range-refused-as-expected`. A metadata string is the only thing
standing between a user on an uncovered version and a server that dies
mid-command, and an unasserted guard is not a guard.

Which version that leg runs on is not obvious, and the obvious choices do not
work. The two holes between the four declared Forge ranges are **1.17** and
**1.20.5**, and Forge published no server build for either — the holes exist
*because* nothing was published there — so a leg on them dies at
`no-forge-build-for-version` before a container starts. The bootable form of
the same assertion puts the mismatch on the other axis: `make e2e ...
FORGE_REFUSAL_PROBE=1` pre-sets `FORGE_JAR_BAND=modern`, handing a version the
wrong band's jar. **1.21.6 with the modern jar** is the one pairing that
isolates the minecraft range. Every band declares a `loader_range` alongside
its `minecraft_range`, and a Forge major tracks its Minecraft version 1:1, so
the two normally say the same thing and FML rejects at the language-provider
stage (`needs language provider javafml:N or above`) without ever reading the
minecraft dependency. `modern`'s `loader_range` is `[50,)` — unbounded above —
so on 1.21.6 the javafml and forge gates both pass and
`minecraft_range_modern`'s `<1.21.6` ceiling is the only thing left to refuse.
The other three bands' minecraft ranges cannot be isolated by any bootable
pairing; their `loader_range` covers them. The always-on offline half lives in
`scripts/test-jar-routing.sh`, which pins both holes and all four declared
ranges and fails in `contracts` before a jar is built.

Also fixed while adding this leg: a `set -e` trap where a failing `grep`
inside a command substitution silently killed `scripts/e2e-run-one.sh`
before it could write a verdict — hit on Minecraft 1.21, which has no
`-recommended` promotion.

## NeoForge server install

`LOADER=neoforge` boots a real NeoForge server, on any Minecraft version from
1.20.2 up — one band jar serves every leg, so the version axis is the loader
build and the JVM, not the jar. Unlike Quilt this needs **no host-side install
trick**: Quilt needed one because `quilt-installer` requires Java 17+ while the
`mc114` band boots Java 8, but NeoForge never targets a Minecraft version below
1.20.2 and so never runs below Java 17 anyway. The whole
install therefore happens inside the same container that runs the server, on the
shape of the Fabric code path.

`scripts/e2e-entrypoint.sh` downloads
`neoforge-<ver>-installer.jar` from `maven.neoforged.net` and runs
`--install-server .`. The installer is headless-safe, exits non-zero on failure,
downloads the vanilla server jar itself, and needs only a **JRE** — no JDK, no
`javac` — which is what the e2e images have. It produces a `libraries/` tree
plus `run.sh`/`user_jvm_args.txt`.

Launch is **not** `-jar`: it is the argfile the installer generates,
`java @libraries/net/neoforged/neoforge/<ver>/unix_args.txt nogui`, which
carries the module path and the main class. Every path inside it is relative, so
it only works from the server directory — launching from anywhere else fails
with `Could not find or load main class ...BootstrapLauncher`. The entrypoint's
launch line is generalised to a `SERVER_LAUNCH_ARGS` variable for this; the
Fabric and Quilt paths still pass `-jar <launcher>.jar` through it unchanged.

NeoForge servers also get a larger default max heap (1G vs 512M) — the modular
bootstrap plus NeoForge's own mod-loading sits on top of vanilla. `JAVA_FLAGS`
still overrides both wholesale.

Which NeoForge version goes with which Minecraft version is pinned in
`scripts/e2e-run-one.sh`, not resolved at run time: a 21-row table, one row per
Minecraft version NeoForge publishes for, mapping it to the loader build the
installer fetches and to NeoForge's **own** Java floor (17 up to line 20.4, 21
through 21.11, 25 on 26.x — which is not the jar's bytecode level, and not the
Fabric era table's floor either; the era table reports 21 for 1.20.4).
`--print-neo-routing <mcver>` prints the pair, `scripts/test-jar-routing.sh`
asserts it offline, and a version outside the table prints `unsupported 0`.
Rows whose line never published a stable build carry a `-beta` version and are
marked as such in the boot table in
the wiki's [Version boundaries and root causes](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/Version-Boundaries-And-Root-Causes) — 1.20.3, 1.20.5, 1.21.2, 1.21.6,
1.21.7, 1.21.9, 26.1 and 26.1.1. The band's compile anchor is never one of them.

The pins are explicit on purpose. The Maven
`latest/version?filter=<mcMinor>.<mcPatch>.` endpoint exists and works — note
the **trailing dot is load-bearing**, `filter=21.1` returns `21.11.45` — but an
e2e run that silently retargets itself when upstream publishes is not a
regression test. Two related traps: 26.x versions are four-component
(`26.2.0.<n>`), and Maven's `<release>` marker for `net.neoforged:neoforge`
resolves to `26.1.2.97`, which sorts *above* `26.2.0.64`.

**Deliberately not cached.** The Fabric path caches its launcher and server jar
under `E2E_JAR_CACHE`; the NeoForge install tree is ~250MB per Minecraft version
against GitHub's 10GB per-repo cache budget, and the CI legs boot six versions,
so the download still costs less than the cache round-trip. Revisit only with a
measurement.

### Assertion differences

Every existing assertion carries over **unchanged**. Source names come from
vanilla `CommandSourceStack`, so `[CommandsSpy] [Server] list`,
`[CommandsSpy] [Rcon] save-all` and `[CommandsSpy] [Player: e2e_player1] list`
are byte-identical to the Fabric path.

The one exception is the mixin assertion, which is reported as `[SKIP]` on
`LOADER=neoforge`: the jar contains no mixin at all (it listens to NeoForge's
own `CommandEvent`), so the "no missing-target report" grep could not fail there
and would prove nothing. What proves the NeoForge hook is the console, RCON and
player assertions themselves.

Two verdict codes are NeoForge-specific: `neoforge-install-failed` (the
installer exited non-zero; its log tail is printed) and
`neoforge-unsupported-version` — a Minecraft version NeoForge publishes nothing
for, i.e. below its 1.20.2 floor. The message names that floor
(`NeoForge publishes no line for it (its floor is Minecraft 1.20.2)`) rather
than a shipped-jar list, because the jar is not the limit any more: one band jar
covers 1.20.2-26.2, so the only way to miss is to ask for a version that has no
NeoForge at all. Explicit failure, never a silent fallback to a jar that cannot
load.

## Babric server install

`LOADER=babric` boots a real Babric server — Fabric for Minecraft Beta 1.7.3.
The version axis has exactly one value: `b1.7.3` is the only Minecraft version
Babric exists for, so `scripts/e2e-run-one.sh` exits non-zero on any other
version under `LOADER=babric`, and on `b1.7.3` under any other loader. The two
are inseparable and the check is asserted in both directions.

The install happens **host-side**, the same trick the Quilt and Forge paths use,
and for a sharper version of the same reason: nothing inside the server
container knows where a b1.7.3 server jar lives. The Babric installer,
`fabric-installer-1.0.0-babric.2.jar`, runs in a one-off
`eclipse-temurin:21-jre-jammy` container as
`java -jar installer.jar server -dir /out -mcversion b1.7.3 -loader 0.19.3
-downloadMinecraft`. Only jars ever enter the server image, so the Java-21
floor's alpine variant needed no change — Ornithe's installer, a glibc-linked
ELF binary, would have forced a jammy base, which is one reason the harness uses
the Babric one. The result is cached under `E2E_JAR_CACHE` with key
`babric-b1.7.3-loader<N>-installer<N>`, beside the Fabric and Quilt entries, and
bind-mounted read-only at `/babric-preinstalled`.

Pins: loader `0.19.3` — **upstream `net.fabricmc:fabric-loader`, not the frozen
`0.15.6-babric.2` babric fork** — and installer `1.0.0-babric.2`, both
overridable via `BABRIC_LOADER_VERSION` / `BABRIC_INSTALLER_VERSION`.

**The server jar is not Mojang's.** Mojang publishes no b1.7.3 server download
at all; the installer's polyfilled manifest points at `files.betacraft.uk`, a
single community mirror. This is the only leg in the harness that boots a server
jar Mojang did not publish. The harness pins it: size `503100`, sha256
`033a127e4a25a60b038f15369c89305a3d53752242a1cff11ae964954e79ba4d`, checked
after install, and the leg fails with `babric-server-jar-hash-mismatch` on any
other bytes. If that mirror disappears, this leg goes red — by design, because
an unpinned fetch from a single community mirror is the failure mode the hash is
there to make loud.

`fabric-server-launch.jar` is a **thin** jar: its manifest `Class-Path` points
into a relative `libraries/` tree, so the entrypoint copies the whole install
tree (`cp -R /babric-preinstalled/.`) rather than just the launch jar. Copying
the jar alone fails with `Could not find or load main class`, which reads like a
metadata bug and is not one.

Five ways a b1.7.3 server is not a modern one, each a specific branch in
`scripts/e2e-entrypoint.sh`:

1. **No `eula.txt`.** It postdates b1.7.3 (it landed in 1.7.10); the beta server
   never reads it and never asks for it. The harness does not write one.
2. **`server.properties` has fifteen keys and none of the modern ones.**
   `enable-rcon`, `level-type`, `generator-settings`, `spawn-protection`,
   `simulation-distance`, `sync-chunk-writes`,
   `network-compression-threshold` and `generate-structures` are all absent, so
   Babric writes its own small block instead of the modern one.
3. **No RCON.** There is no `enable-rcon` key and no listener, so stdin is the
   only control channel. The RCON assertion is replaced, not skipped — see
   below.
4. **A nanosecond ready line**: `Done (13105814836ns)!`, not
   `Done (12.345s)!`. The boot wait greps `Done (`, which matches both
   unchanged; only assertion text and docs need the `ns` form.
5. **Two log formats in one file.** Loader lines are log4j
   (`[HH:MM:SS] [Server thread/INFO]`), vanilla lines are
   `YYYY-MM-DD HH:MM:SS [INFO]`, and the vanilla format takes over mid-log. The
   mod's own lines come through log4j, so the `[CommandsSpy]` greps are
   unaffected — but no grep here may be tightened to a modern timestamp shape.

The player bot speaks **protocol 14** (`tools/beta.go`), a second and
structurally different client from the modern one in `tools/table.go`: pre-Netty,
so no VarInt length prefix, no compression, no encryption in offline mode and no
login state machine. Protocol 14 cannot be negotiated by the modern ping path,
so the bot takes an explicit `--protocol` flag (default `0` = negotiate, leaving
every other loader's behaviour unchanged). The bot also sends `/me` rather than
`/list`: on vanilla b1.7.3 `list` is a console command and never reaches the
player seam.

### Babric assertion differences

Babric asserts **ten of the eleven**, and the eleventh is *replaced*, not
skipped. Beta 1.7.3 predates RCON, so instead of the RCON logging assertion the
leg asserts RCON's **absence**: no `rcon.*` key in the properties file the server
rewrites at boot, and nothing answering on the RCON port. Either appearing fails
the leg as `babric-rcon-appeared-update-docs`. A skipped assertion is invisible
on the wiki; an asserted absence is a row.

The leg then adds a row of its own, pinning the loader version: the banner
`Loading Minecraft Beta 1.7.3 with Fabric Loader 0.19.3`, so toolchain drift onto
the frozen babric fork surfaces as a named failure
(`babric-loader-version-drift`) rather than as a passing test of something else.

Console-source assertions use `CONSOLE`, not `Server` — see "Era-exact
literals" above.

## BTA server install

`LOADER=bta` boots a real BTA server. BTA — "Better than Adventure!" — is a fork
of the Beta 1.7.3 *game*, not another loader for it: unobfuscated classes, its
own class layout, its own `fabric-loader` fork. Neither jar loads on the other's
server, so the inseparability check has a second, independent pair: every
`bta*` version is refused under any other loader, and `LOADER=bta` is refused
with any non-`bta*` version. `scripts/test-jar-routing.sh` asserts all four
pairings offline.

The version axis is BTA's own release line, tokenised with a `bta` prefix:
`bta7.3`, `bta7.3_01`, `bta7.3_02`, `bta7.3_03`, `bta7.3_04`, `bta8.0`,
`bta8.0.1`. The prefix is stripped to recover the upstream version. `bta7.3` is
a hard floor: the Brigadier `CommandManager` the jar's one mixin targets does not
exist in BTA 7.2 and older.

The install is **host-side** and needs no installer at all: BTA publishes a
ready-made modded server package per release —
`fabric-server-launch.jar` + `libraries/` + `server.jar` + `mods/` + `start.sh`,
which is exactly the tree Babric's installer produces. The harness downloads the
release zip, verifies it, unzips it into `E2E_JAR_CACHE`, and bind-mounts it
read-only at `/bta-preinstalled`; the entrypoint copies the whole tree for the
same thin-jar reason as Babric and drops the mod jar into `mods/` beside the
HalpLibe jar the package already ships.

**Every package is pinned by URL and sha256**, one literal row per booted
version, and the hash is re-checked on cache hits. A mismatch fails the leg as
`bta-package-hash-mismatch`; a failed download as `bta-install-failed`. The
asset name changes mid-history — `bta_babric_server_<v>.zip` through 7.3_03,
`bta_fabric_server_<v>.zip` from 7.3_04 — so the table carries whole URLs rather
than deriving them by rule, and "latest" is never resolved.

Four ways a BTA server is not a modern one, beyond what it shares with Babric:

1. **No `eula.txt`**, same as Babric — the Beta codebase never reads one.
2. **`server.properties` is BTA's own set**, with keys like `allow-drift` and
   `world-type=minecraft:overworld.extended`, and no `rcon.*` key at all.
3. **The ready line is nanoseconds**: `Done (13297414790ns)!`. A
   `Done \([\d.]+s\)!` matcher never fires.
4. **The console source name is `Server`**, like every modern loader and unlike
   Babric's `CONSOLE` — BTA's own `ConsoleCommandSource.getName()`.

### BTA assertion differences

Same shape as Babric: **ten of the eleven** standard assertions hold, and the
eleventh — RCON — is *replaced* by an asserted absence, failing as
`bta-rcon-appeared-update-docs`. It adds the same kind of extra row Babric does,
pinning the platform: the banner
`Loading Minecraft <ver> with Fabric Loader <BTA_LOADER_VERSION>` proves both the
game version and the loader fork in one line, failing as
`bta-loader-version-drift`.

The player-typed leg is booted, not asserted from prose. It needs a second bot:
BTA keeps Beta's framing but changes the string encoding, the protocol version
(`32769`), the login packet's fields and the chat packet's header, and it opens
with an unsolicited custom payload. `tools/bta.go` speaks it, and the bot is
invoked with `--protocol 32769`. Like Babric it sends `/me`, because BTA
inherited the Beta codebase's narrow player-reachable command set. A vanilla BTA
server never logs command lines itself, so the assertion reads CommandsSpy's own
output — which is the point of the leg.

## Log capture

Assertions grep whichever capture provably holds the **whole run**, decided by
content: `logs/latest.log` if it contains the boot banner (`Starting minecraft
server`), otherwise `server.log` — the raw stdout redirect of the server process,
which has one run in it by construction and is never rolled or reconfigured. If
neither has the banner the run fails as `log-capture-truncated`, printing the head
of both files: that verdict accuses the harness, not the mod, the same attribution
rule `scripts/test-wiki-links.sh` applies to its own environment errors.

Why the check exists: Minecraft's log4j config rolls `latest.log` on a date change,
so a run crossing midnight left a file beginning mid-run — and `grep -q` cannot
tell a missing line from a rolled-away one, so four legs reported `mod-not-loaded`
for a mod that worked (issue #78). The container therefore also boots with
`-Dlog4j2.configurationFile=/mc-server/e2e-log4j2.xml` (`scripts/e2e-log4j2.xml`,
baked into the image): Console plus a non-rolling File appender, nothing more, so
`latest.log` is one run by construction. The config is deliberately plain — the
mc114 band runs log4j 2.8.1 on Java 8 — and its pattern ends in `%msg%n` because
the entrypoint asserts `[CommandsSpy] [Server] say$` anchored at end of line.

The banner selection is what covers the cases the config cannot: a loader that
reconfigures log4j onto its own file mid-boot falls back to `server.log` with a
`NOTE:` line naming what happened.

## Config-behaviors leg

`CONFIG_VARIANT=1` (Makefile, `scripts/e2e-run-one.sh`,
`scripts/e2e-entrypoint.sh`) runs an opt-in extra leg that boots its own
server rather than reusing the default boot. It has to: `CommandsSpy.CONFIG`
is a `static final` read once at class-init, and the mod has no
config-reload path, so a non-default config can only be observed by seeding
`config/commands-spy.json` **before** that boot.

Before the server starts, the leg writes:

```json
{"blacklist": ["list"], "logArguments": true}
```

It then asserts: the blacklisted `list` command (sent over RCON) produces no
`[CommandsSpy]` line at all; a non-blacklisted RCON command (`save-all`)
still logs as usual; and an argument-bearing command is logged **with** its
arguments, proving `logArguments: true` is honored end to end.

Run it with:

```bash
make e2e-ci VERSIONS=1.21.11 JAVA=21 LOADER=fabric CONFIG_VARIANT=1
```

Its log and result files use the `-cfgvar` key suffix, keeping them
distinct from the same version's default-config leg in `E2E_JAR_CACHE` and
in the aggregated CI results.

## Routing drift protection

The version→jar/Java mapping lives in one executable home — the `case`
statement in `scripts/e2e-run-one.sh` (probe flags `--print-java`,
`--print-routing`) — but is consumed by the Makefile, the CI grid generator,
and the workflows. `scripts/test-jar-routing.sh` is the independent copy that
argues back: an offline contract table probing the real routing code paths,
run by `make test` and CI before anything is built.
`tools/floors_test.go` pins the same floors from the Go side.

Container detail worth knowing: no host port is ever published (parallel runs
would collide); RCON is exercised from inside the container network.
