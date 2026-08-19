# E2E harness

Every supported Minecraft version boots as a real Fabric server in Docker,
receives commands over console, RCON, and from a protocol-level player bot,
and the log is asserted line by line. Entry points:

- `make e2e` — local dev: unbounded parallel fan-out over the default matrix.
- `make e2e-ci` — same flow, bounded (`PARALLEL=4` default).
- `scripts/e2e-run-one.sh <version>` — one version, one container. Always
  writes exactly one result file, even if killed; the Makefile reaps result
  files after the fan-out, so no failure can be lost.
- `scripts/e2e-entrypoint.sh` — runs inside the container: boots the server,
  drives the phases, prints the verdict.

The default version list and its sampling rationale live in
[version-matrix.md](version-matrix.md).

## Phases and assertions

1. **Boot** — Fabric launcher fetches the vanilla server; the entrypoint waits
   (bounded by `BOOT_TIMEOUT`, default 180s) for `Done`. A JVM that dies at
   mixin/loader bootstrap can deadlock in its shutdown hooks, so a fatal
   bootstrap crash fails fast instead of waiting out the timeout.
2. **Console + RCON** — `list` via the console fifo, `save-all` via RCON.
3. **Player phase** (`PLAYER_PHASE=1`, the default) — the baked-in Go bot
   (`tools/`) joins two protocol-level players. `e2e_player1` sends `/list`;
   `e2e_player2` joins and sends nothing. The bot's global timeout is 150s;
   the entrypoint wraps it in an outer 160s `timeout` as the belt to the
   bot's braces.

Verdict line grammar (the final line of container output is authoritative):

- `E2E <version> PASS`
- `E2E <version> PASS players-skipped-unsupported-protocol` — console+RCON
  only; reachable only on standalone `PLAYER_PHASE=0` runs.
- `E2E <version> FAIL <code>,<code>,...` with codes: `mod-not-loaded`,
  `mixin-not-applied`, `console-command-not-logged`, `rcon-command-not-logged`,
  `player-command-not-logged`, `player-misattributed`, `boot-failed`.
  `scripts/e2e-run-one.sh` adds `below-java-floor-<n>` for a JVM below the
  version's floor.

The player assertions are a positive/negative pair: player1's `/list` must be
attributed to player1, and player2 (who sent nothing) must appear in **zero**
`[CommandsSpy]` lines. The negative half catches misattribution that the
positive half alone would miss.

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
`QUILT_LOADER_VERSION=0.30.0`, `QUILT_INSTALLER_VERSION=0.15.1` (both
hardcoded in `scripts/e2e-run-one.sh`, mirroring how the Fabric harness's
own `LOADER_VERSION`/`INSTALLER_VERSION` defaults live in
`scripts/e2e-entrypoint.sh` rather than in `gradle.properties`).

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
