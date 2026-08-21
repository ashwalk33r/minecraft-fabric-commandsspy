#!/bin/bash
set -euo pipefail

MC_VERSION="${MC_VERSION:-1.21}"
BOOT_TIMEOUT="${BOOT_TIMEOUT:-180}"
LOADER_VERSION="${LOADER_VERSION:-0.19.3}"
INSTALLER_VERSION="${INSTALLER_VERSION:-1.1.2}"
RCON_PASSWORD="e2etest"
RCON_PORT="25575"
# PLAYER_PHASE=1 runs the baked-in Go bot phase; 0 = console+RCON only.
PLAYER_PHASE="${PLAYER_PHASE:-0}"
LOADER="${LOADER:-fabric}"
# 1 = this Minecraft version is OUTSIDE the Forge jar's declared range and Forge
# is expected to refuse the mod. Decided by scripts/e2e-run-one.sh.
FORGE_EXPECT_REFUSED="${FORGE_EXPECT_REFUSED:-0}"
# 1 = this Minecraft version is OUTSIDE every declared minecraft range in
# gradle.properties and the Fabric/Quilt loader is expected to refuse the mod.
# Decided by scripts/e2e-run-one.sh.
FABRIC_EXPECT_REFUSED="${FABRIC_EXPECT_REFUSED:-0}"
# Set by scripts/e2e-run-one.sh for LOADER=neoforge only.
NEOFORGE_VERSION="${NEOFORGE_VERSION:-}"
# 1 = the config-behaviors leg: seed config/commands-spy.json BEFORE boot and
# assert blacklist suppression + logArguments:true instead of the default-leg
# assertions. Its own boot exists because CommandsSpy.CONFIG is a static final
# read once at class-init, with no reload path. Set by scripts/e2e-run-one.sh.
E2E_CONFIG_VARIANT="${E2E_CONFIG_VARIANT:-0}"

cd /mc-server

if [ "$LOADER" = "fabric" ]; then
  # Mutable .fabric state is deliberately NOT cached — only immutable
  # downloads. See docs/e2e-harness.md.
  JAR_CACHE_KEY="/jar-cache/${MC_VERSION}-loader${LOADER_VERSION}-installer${INSTALLER_VERSION}"

  # Download the Fabric server launcher jar (bundles the loader + installer logic;
  # downloads/verifies the vanilla Minecraft server on first boot).
  LAUNCHER_URL="https://meta.fabricmc.net/v2/versions/loader/${MC_VERSION}/${LOADER_VERSION}/${INSTALLER_VERSION}/server/jar"
  if [ -f "${JAR_CACHE_KEY}/fabric-server-launch.jar" ] \
     && [ -f "${JAR_CACHE_KEY}/server/${MC_VERSION}-server.jar" ]; then
    echo "[e2e] Jar cache HIT for Minecraft $MC_VERSION (loader $LOADER_VERSION, installer $INSTALLER_VERSION) — skipping downloads"
    cp "${JAR_CACHE_KEY}/fabric-server-launch.jar" fabric-server-launch.jar
    # Pre-seed the launcher's own download target; it verifies the jar in
    # place and skips the piston-data fetch.
    mkdir -p .fabric/server
    cp -R "${JAR_CACHE_KEY}/server/." .fabric/server/
  else
    echo "[e2e] Downloading Fabric server launcher for Minecraft $MC_VERSION (loader $LOADER_VERSION, installer $INSTALLER_VERSION)"
    curl -fsSL "$LAUNCHER_URL" -o fabric-server-launch.jar
  fi
  SERVER_LAUNCH_ARGS="-jar fabric-server-launch.jar"
elif [ "$LOADER" = "quilt" ]; then
  # Quilt: the host (scripts/e2e-run-one.sh) already ran quilt-installer
  # (it needs Java 17+, which this container may not have — mc114 runs
  # Java 8) and bind-mounted the result read-only. Copy the whole tree in —
  # quilt-server-launch.jar is a thin jar whose manifest Class-Path points
  # at a relative libraries/ dir, not a fat jar.
  echo "[e2e] Copying pre-installed Quilt server for Minecraft $MC_VERSION..."
  cp -R /quilt-preinstalled/. .
  SERVER_LAUNCH_ARGS="-jar quilt-server-launch.jar"
elif [ "$LOADER" = "forge" ]; then
  # Forge: same host-side-install trick as Quilt (the installer wants a modern
  # JDK), but Forge is NEITHER a fat jar NOR a thin jar — 1.17+ installs a
  # `libraries/.../unix_args.txt` @argfile holding the module path and main
  # class, which is why the loader-varying thing here is the whole launch
  # ARGUMENT LIST and not a jar filename.
  echo "[e2e] Copying pre-installed Forge server for Minecraft $MC_VERSION..."
  cp -R /forge-preinstalled/. .
  FORGE_ARGS_FILE="$(find libraries/net/minecraftforge/forge -name unix_args.txt 2>/dev/null | head -1)"
  if [ -n "$FORGE_ARGS_FILE" ]; then
    SERVER_LAUNCH_ARGS="@${FORGE_ARGS_FILE}"
  else
    # <=1.16.5 layout: a single runnable forge-<mc>-<build>.jar, no argfile.
    SERVER_LAUNCH_ARGS="-jar $(find . -maxdepth 1 -name 'forge-*.jar' | head -1)"
  fi
else
  # NeoForge needs NO host-side install trick, unlike Quilt. Quilt needed one
  # because quilt-installer requires Java 17+ while the mc114 band boots Java 8;
  # NeoForge never targets a Minecraft version below 1.20.2 and so never runs
  # below Java 17 anyway. The installer is headless-safe, exits non-zero on
  # failure, downloads the vanilla server jar itself, and needs only a JRE — no
  # JDK, no javac — which is exactly what this image has.
  #
  # Deliberately NOT cached in /jar-cache: the install tree is ~250MB per
  # Minecraft version against GitHub's 10GB per-repo cache budget, and with only
  # two shipped lines the download is cheaper than the cache round-trip.
  echo "[e2e] Installing NeoForge $NEOFORGE_VERSION server for Minecraft $MC_VERSION..."
  curl -fsSL "https://maven.neoforged.net/releases/net/neoforged/neoforge/${NEOFORGE_VERSION}/neoforge-${NEOFORGE_VERSION}-installer.jar" \
    -o neoforge-installer.jar
  if ! java -jar neoforge-installer.jar --install-server . > neoforge-install.log 2>&1; then
    echo "[e2e] NeoForge install failed:"
    tail -40 neoforge-install.log
    echo "E2E ${MC_VERSION} FAIL neoforge-install-failed"
    exit 1
  fi
  rm -f neoforge-installer.jar
  # Every path inside unix_args.txt is relative and the argfile carries the main
  # class, so this must be launched from the server directory (we are: cd
  # /mc-server above) and needs no -jar and no version string of its own.
  SERVER_LAUNCH_ARGS="@libraries/net/neoforged/neoforge/${NEOFORGE_VERSION}/unix_args.txt"
fi

if [ -f "/tmp/mod.jar" ]; then
  mkdir -p mods
  cp /tmp/mod.jar mods/
  echo "[e2e] Mod JAR copied to mods/"
fi

# NOTE: fabric-api is intentionally NOT installed — the mod's fabric-api
# dependency was removed, and running without it is part of what this
# test verifies.

echo 'eula=true' > eula.txt

if [ "$E2E_CONFIG_VARIANT" = "1" ]; then
  echo "[e2e] Config-behaviors leg: seeding config/commands-spy.json before boot"
  mkdir -p config
  cat > config/commands-spy.json <<'CFGEOF'
{
  "blacklist": ["list"],
  "logArguments": true
}
CFGEOF
fi

cat > server.properties <<EOF
enable-rcon=true
rcon.port=${RCON_PORT}
rcon.password=${RCON_PASSWORD}
online-mode=false
# --- minimal footprint: smallest world and least work to reach "Done (...)" ---
level-type=flat
# Void world: nothing to generate. Tuning rationale in docs/e2e-harness.md.
# The empty "structures" object is REQUIRED by 1.16/1.16.1 (their flat codec
# has no default for it and the server dies at boot without it); 1.16.2+ made
# it optional and 1.19+ (which renamed it structure_overrides) ignores the
# unknown key, so one literal serves every version.
generator-settings={"layers":[],"biome":"minecraft:the_void","structures":{"structures":{}}}
level-seed=e2e
spawn-protection=0
# 3 is the vanilla floor; lower values are clamped up.
view-distance=3
simulation-distance=3
# 5: two bots plus headroom.
max-players=5
sync-chunk-writes=false
network-compression-threshold=-1
spawn-monsters=false
spawn-animals=false
spawn-npcs=false
generate-structures=false
allow-nether=false
difficulty=peaceful
enable-jmx-monitoring=false
EOF

# Boot the server with stdin attached to a fifo so we can send console commands
# after it finishes starting up.
mkfifo console.in
echo "[e2e] Starting $LOADER server for Minecraft $MC_VERSION..."
# Intentionally NOT using -XX:+AlwaysPreTouch: it front-loads page faulting and
# would increase time-to-"Done", the metric being optimized.
# 512M is tuned for vanilla+Fabric/Quilt; Forge's ModLauncher/transformer stack
# and NeoForge's own mod-loading pipeline on top of vanilla don't fit in it, so
# both get their own floor. Still overridable wholesale via JAVA_FLAGS.
DEFAULT_MAX_HEAP=512M
if [ "$LOADER" = "forge" ] || [ "$LOADER" = "neoforge" ]; then
  DEFAULT_MAX_HEAP=1G
fi
JAVA_FLAGS="${JAVA_FLAGS:--Xms512M -Xmx${DEFAULT_MAX_HEAP} -XX:+UseSerialGC -XX:TieredStopAtLevel=1}"
# The fifo is held open read-write on fd 3 and handed to java as stdin directly.
# A `tail -f console.in |` pipeline here is a trap: tail never exits, so a
# crashed java would block this script forever. With fd 3, $! is `timeout`'s
# PID (it forks java rather than exec-ing into it) — the orphaned java child
# this leaves behind on a hard kill is harmless: the whole container, and
# everything in it, is torn down the moment this script (its PID 1) exits.
exec 3<>console.in
# shellcheck disable=SC2086 # both are whitespace-separated argument lists; word splitting is the point
timeout "$BOOT_TIMEOUT" java $JAVA_FLAGS $SERVER_LAUNCH_ARGS nogui <&3 > server.log 2>&1 &
SERVER_PID=$!

BOOTED=0
CONFIG_FILE="config/commands-spy.json"
# MOD.md: "On startup, the config file will be created automatically." Sampled
# once, at boot, before any command runs — the only moment in this script where
# "on startup" is still measurable. The end-of-run check below tests the file's
# CONTENT and structurally cannot test its TIMING: by then four commands have
# executed, and on every loader the first one alone would have created it.
CONFIG_AT_BOOT=0
for _ in $(seq 1 "$BOOT_TIMEOUT"); do
  if grep -q 'Done (' server.log 2>/dev/null; then
    BOOTED=1
    break
  fi
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    break
  fi
  # Fail fast on a fatal bootstrap crash. A JVM that dies at mixin/loader
  # bootstrap can deadlock in its shutdown hooks instead of exiting, so the
  # kill -0 above never fires; catch the crash in the log instead of waiting
  # out the whole BOOT_TIMEOUT on a server that can never reach 'Done ('.
  if grep -qE 'Exception in thread "main"|FormattedException|Failed to launch|Minecraft has crashed|The requested compatibility level' server.log 2>/dev/null; then
    break
  fi
  sleep 1
done

if [ "$BOOTED" -eq 1 ]; then
  echo "[e2e] Server booted, sending console command..."
  if [ -f "$CONFIG_FILE" ]; then CONFIG_AT_BOOT=1; fi
  echo "list" > console.in
  sleep 3

  # Non-existing command (MOD.md's opening claim). Whether each loader's hook
  # fires for a name the dispatcher cannot resolve is per-loader behavior; this
  # send is what measures it. See UNKNOWN_COMMAND_GAP below.
  echo "[e2e] Sending non-existing console command..."
  echo "notacommand" > console.in
  sleep 2

  # logArguments probe: a command WITH arguments, so the default (false) can be
  # distinguished from true. `list` has no arguments, so every pre-existing
  # assertion in this file passes identically under either setting.
  echo "[e2e] Sending console command with arguments..."
  echo "say e2e-args-probe" > console.in
  sleep 2

  echo "[e2e] Sending RCON command..."
  /usr/local/bin/tools rcon --port "$RCON_PORT" --password "$RCON_PASSWORD" save-all || echo "[e2e] ⚠ RCON client failed"

  sleep 1

  # Bot's own timeout is 150s; the outer 160s timeout is the belt to its braces.
  if [ "$PLAYER_PHASE" = "1" ]; then
    echo "[e2e] Running player phase (bounded 160s)..."
    timeout 160 /usr/local/bin/tools bot --host 127.0.0.1 --port 25565 --command list || echo "[e2e] ⚠ Player phase failed"
    # Settle so the command's log line is flushed before the kill below.
    sleep 1
  fi
fi

# No graceful `stop`: the container and its world are discarded the instant
# the verdict below is computed, so a "Saving world..." shutdown is wasted
# wall-clock, not a safety measure. SIGKILL is immediate and, unlike SIGTERM,
# cannot hang in a stuck shutdown hook — no follow-up signal or poll needed.
echo "[e2e] Killing server..."
kill -9 "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true

# Populated only from a booted server (a busted download must never get cached).
# Atomic stage-dir+mv because parallel jobs race. Fabric only — Quilt's
# install is cached on the host, before this container ever started.
if [ "$LOADER" = "fabric" ] && [ "$BOOTED" -eq 1 ] && [ -d /jar-cache ] && [ ! -d "$JAR_CACHE_KEY" ] \
   && [ -f ".fabric/server/${MC_VERSION}-server.jar" ]; then
  STAGE="${JAR_CACHE_KEY}.tmp.$$"
  mkdir -p "${STAGE}/server"
  cp fabric-server-launch.jar "${STAGE}/fabric-server-launch.jar"
  cp -R .fabric/server/. "${STAGE}/server/"
  if mv "$STAGE" "$JAR_CACHE_KEY" 2>/dev/null; then
    echo "[e2e] Jar cache populated for Minecraft $MC_VERSION"
  else
    rm -rf "$STAGE"
  fi
fi

if [ -f logs/latest.log ]; then
  LOG_FILE="logs/latest.log"
else
  LOG_FILE="server.log"
fi

# Out-of-range guard leg: the assertions below all assume the mod RAN. Here the
# whole point is that it must not have, so this path has its own two and returns
# its own verdict. An unasserted guard is not a guard — this leg is what fails
# the day someone widens the modern jar's declared minecraft range.
#
# Scope, stated precisely, because it is narrower than it looks: what this
# asserts is mods.toml's `minecraft` DEPENDENCY range, and only the modern
# jar's ceiling. Every band declares a loader_range alongside its
# minecraft_range, and because a Forge major tracks its Minecraft version 1:1
# the two say the same thing — so on any other pairing FML rejects at the
# language-provider stage ("needs language provider javafml:N or above") and
# never evaluates the minecraft dependency at all. modern is the exception:
# its loader_range is [50,), unbounded above, so on 1.21.6 the javafml and
# forge gates both pass and minecraft_range_modern's <1.21.6 ceiling is the
# only thing left to refuse. The other three bands' minecraft ranges cannot be
# isolated by any bootable pairing; their loader_range covers them.
if [ "$FORGE_EXPECT_REFUSED" = "1" ]; then
  echo "[e2e] Assertion results (Forge out-of-range guard leg):"
  GUARD_FAILURES=""
  # Observed verbatim on Forge 56.0.9 / Minecraft 1.21.6 handed the modern jar:
  #   Mod §ecommandsspy§r requires §6minecraft§r §o1.20.6 or above, and below 1.21.6§r
  # The §-codes are Minecraft colour escapes, hence the wildcards. Deliberately
  # NOT matching the javafml language-provider line: that is the OTHER gate, and
  # accepting it here is what let this leg pass without testing anything.
  if grep -qE 'Mod .*commandsspy.* requires .*minecraft' "$LOG_FILE"; then
    echo "  [PASS] Forge refused the mod on $MC_VERSION via mods.toml's minecraft dependency range (modern jar, ceiling <1.21.6)"
  else
    echo "  [FAIL] Forge did NOT refuse the mod on $MC_VERSION via its minecraft range — the modern jar's [1.20.6,1.21.6) ceiling no longer guards"
    if grep -q 'needs language provider javafml' "$LOG_FILE"; then
      echo "  [INFO] the log shows 'needs language provider javafml': the loaderVersion gate fired first, so this leg no longer isolates the minecraft gate"
    fi
    GUARD_FAILURES="${GUARD_FAILURES}forge-out-of-range-not-refused,"
  fi
  if grep -q '\[CommandsSpy\] \[' "$LOG_FILE"; then
    echo "  [FAIL] the mod logged a command on $MC_VERSION, where its Minecraft calls do not resolve"
    GUARD_FAILURES="${GUARD_FAILURES}forge-out-of-range-executed,"
  else
    echo "  [PASS] no [CommandsSpy] line: the mod never ran"
  fi
  echo "[e2e] Full contents of $LOG_FILE:"
  cat "$LOG_FILE" || true
  if [ -z "$GUARD_FAILURES" ]; then
    echo "E2E ${MC_VERSION} PASS forge-out-of-range-refused-as-expected"
    exit 0
  fi
  echo "E2E ${MC_VERSION} FAIL ${GUARD_FAILURES%,}"
  exit 1
fi

# Fabric/Quilt out-of-range guard leg, the mirror of the Forge block above, and
# for the same reason: the assertions below all assume the mod RAN, and here the
# whole point is that it must not have. This is what fails the day one of
# gradle.properties' four minecraft_range_* values is widened over a version
# whose mixin cannot apply — commandsspy.mixins.json is "required": true with
# defaultRequire 1, so that widening is a hard crash at server start, not a
# no-op. Its own assertions, its own verdict, its own exit.
if [ "$FABRIC_EXPECT_REFUSED" = "1" ]; then
  echo "[e2e] Assertion results (Fabric/Quilt out-of-range guard leg):"
  GUARD_FAILURES=""
  if grep -q 'Loading CommandsSpy' "$LOG_FILE"; then
    echo "  [FAIL] the loader accepted the mod on $MC_VERSION, which is outside every declared minecraft range"
    GUARD_FAILURES="${GUARD_FAILURES}fabric-out-of-range-not-refused,"
  else
    echo "  [PASS] the loader refused the mod: $MC_VERSION is outside every declared minecraft range"
  fi
  # Informational only, never gating: the exact wording of a resolution failure
  # differs across Fabric and Quilt loader versions, and asserting on it would
  # make the guard brittle for no extra proof.
  if grep -qE 'Incompatible mod set|requires .*minecraft|unsupported|Mod resolution' "$LOG_FILE"; then
    echo "  [INFO] loader reported a dependency-resolution failure, as expected"
  fi
  echo "[e2e] Full contents of $LOG_FILE:"
  cat "$LOG_FILE" || true
  if [ -z "$GUARD_FAILURES" ]; then
    echo "E2E ${MC_VERSION} PASS fabric-out-of-range-refused-as-expected"
    exit 0
  fi
  echo "E2E ${MC_VERSION} FAIL ${GUARD_FAILURES%,}"
  exit 1
fi

# RCON source name: 'Recon' <1.16, 'Rcon' >=1.16 — exact literal on purpose.
# See docs/e2e-harness.md.
case "$MC_VERSION" in
  1.14|1.14.*|1.15|1.15.*) RCON_SOURCE_NAME="Recon" ;;
  *)                       RCON_SOURCE_NAME="Rcon" ;;
esac
echo "[e2e] Minecraft $MC_VERSION: expecting RCON command source named '$RCON_SOURCE_NAME'"

# Config-behaviors leg: its own assertions and its own verdict, exactly like the
# Forge out-of-range guard leg above. The default leg's assertions all assume the
# stock config; here the config is deliberately non-stock, so they would be wrong.
if [ "$E2E_CONFIG_VARIANT" = "1" ]; then
  echo "[e2e] Assertion results (config-behaviors leg: blacklist + logArguments:true):"
  CFG_FAILURES=""
  # No QUILT_ENTRYPOINT_GAP guard here (unlike the default leg below): this
  # check assumes an era where the entrypoint banner fires. Only safe while
  # every config-behaviors CI leg is pinned >=1.18 (see ci.yml).
  if grep -q 'Loading CommandsSpy' "$LOG_FILE"; then
    echo "  [PASS] mod loaded (Loading CommandsSpy)"
  else
    echo "  [FAIL] mod not loaded (Loading CommandsSpy)"
    CFG_FAILURES="${CFG_FAILURES}config-variant-mod-not-loaded,"
  fi
  if grep -q '\[CommandsSpy\] \[Server\] list' "$LOG_FILE"; then
    echo "  [FAIL] blacklisted command 'list' was logged"
    CFG_FAILURES="${CFG_FAILURES}blacklist-not-suppressed,"
  else
    echo "  [PASS] blacklisted command 'list' produced no [CommandsSpy] line"
  fi
  if grep -q "\[CommandsSpy\] \[${RCON_SOURCE_NAME}\] save-all" "$LOG_FILE"; then
    echo "  [PASS] non-blacklisted RCON command still logged (blacklist is not a global mute)"
  else
    echo "  [FAIL] non-blacklisted RCON command not logged"
    CFG_FAILURES="${CFG_FAILURES}rcon-command-not-logged,"
  fi
  if grep -q '\[CommandsSpy\] \[Server\] say e2e-args-probe' "$LOG_FILE"; then
    echo "  [PASS] logArguments=true: 'say e2e-args-probe' logged with arguments"
  else
    echo "  [FAIL] logArguments=true: arguments not logged"
    CFG_FAILURES="${CFG_FAILURES}logargs-true-not-logged,"
  fi
  if [ "$BOOTED" -ne 1 ]; then
    CFG_FAILURES="${CFG_FAILURES}boot-failed,"
  fi
  echo "[e2e] Full contents of $LOG_FILE:"
  cat "$LOG_FILE" || true
  if [ -z "$CFG_FAILURES" ]; then
    echo "E2E ${MC_VERSION} PASS config-behaviors"
    exit 0
  fi
  echo "E2E ${MC_VERSION} FAIL ${CFG_FAILURES%,}"
  exit 1
fi

# Player /list literal: slash included <1.19, bare 'list' on 1.19+.
# See docs/e2e-harness.md.
case "$MC_VERSION" in
  1.14|1.14.*|1.15|1.15.*|1.16|1.16.*|1.17|1.17.*|1.18|1.18.*) PLAYER_LIST_LITERAL="/list" ;;
  *)                                                           PLAYER_LIST_LITERAL="list" ;;
esac

# quilt-loader never invokes the ModInitializer "main" entrypoint on dedicated
# servers below 1.18 — silently, no crash. Mixins still apply, so every
# functional assertion below is unaffected; only the startup banner is missing.
# See docs/version-matrix.md. Asserted as EXPECTED-ABSENT, not skipped, so CI
# reports it the day upstream fixes this.
QUILT_ENTRYPOINT_GAP=0
if [ "$LOADER" = "quilt" ]; then
  case "$MC_VERSION" in
    1.14|1.14.*|1.15|1.15.*|1.16|1.16.*|1.17|1.17.*) QUILT_ENTRYPOINT_GAP=1 ;;
  esac
fi

FAILURES=""

if [ "$QUILT_ENTRYPOINT_GAP" = "1" ]; then
  if grep -q 'Loading CommandsSpy' "$LOG_FILE"; then
    FAILURES="${FAILURES}quilt-entrypoint-gap-closed-update-docs,"
  fi
elif ! grep -q 'Loading CommandsSpy' "$LOG_FILE"; then
  FAILURES="${FAILURES}mod-not-loaded,"
fi

# The gap above is specific to the "main" entrypoint's call site. preLaunch is a
# different one — Knot invokes it before the game's main class loads — and it fires
# on EVERY version, quilt 1.14-1.17 included, where "main" never does. Measured on
# quilt-loader 0.30.0: on 1.16.5 this line appears 25s before "Done (" while the
# "main" banner never appears at all, in the same boot. That is what puts config
# auto-creation back at boot time there. Deliberately its own gate and NOT
# QUILT_ENTRYPOINT_GAP: same version boundary today, but two independent upstream
# facts, and that flag's [FAIL] text tells whoever sees the "main" gap close to
# delete it — which would silently invert this assertion. Forge and NeoForge ship
# neither manifest and could never emit the string.
if [ "$LOADER" = "fabric" ] || [ "$LOADER" = "quilt" ]; then
  if ! grep -q 'CommandsSpy preLaunch: config loaded\.' "$LOG_FILE"; then
    FAILURES="${FAILURES}prelaunch-entrypoint-not-invoked,"
  fi
fi

# Two phrasings, era-exact: older Mixin says "was not found", modern Mixin
# "could not find any targets matching". Fabric/Quilt only: neither Forge nor
# NeoForge ships a mixin — both hook their platform's native CommandEvent — so
# this grep could not fail there and would prove nothing. What proves the
# Forge/NeoForge hook is the console/RCON/player assertions below.
if [ "$LOADER" != "forge" ] && [ "$LOADER" != "neoforge" ] && grep -qE 'was not found|could not find any targets matching' "$LOG_FILE"; then
  FAILURES="${FAILURES}mixin-not-applied,"
fi

if ! grep -q '\[CommandsSpy\] \[Server\] list' "$LOG_FILE"; then
  FAILURES="${FAILURES}console-command-not-logged,"
fi

# MOD.md's opening claim. Loader-specific: Fabric/Quilt hook
# CommandManager.execute, Forge/NeoForge their platform's CommandEvent, and
# whether either fires for a name the dispatcher cannot resolve is exactly what
# this asserts.
if ! grep -q '\[CommandsSpy\] \[Server\] notacommand' "$LOG_FILE"; then
  FAILURES="${FAILURES}unknown-command-not-logged,"
fi

if ! grep -q "\[CommandsSpy\] \[${RCON_SOURCE_NAME}\] save-all" "$LOG_FILE"; then
  FAILURES="${FAILURES}rcon-command-not-logged,"
fi

# MOD.md's "On startup" half, sampled at boot before any command ran.
# Unconditional on purpose: every loader's entrypoint touches CommandsSpy before
# the server is ready — Fabric/Quilt via the preLaunch entrypoint (measured, incl.
# quilt below 1.18 where "main" never fires), Forge/NeoForge via the @Mod
# constructor's CommandsSpy.init(). A red leg here is a finding to investigate,
# never a reason to narrow this check to a subset of loaders.
if [ "$CONFIG_AT_BOOT" -ne 1 ]; then
  FAILURES="${FAILURES}config-not-created-at-boot,"
fi

# The CONTENT half: the documented initial schema. Runs at end of run and says
# nothing about WHEN the file appeared — the CONFIG_AT_BOOT check above is what
# asserts MOD.md's "On startup".
if [ -f "$CONFIG_FILE" ] \
   && grep -q '"blacklist": \[\]' "$CONFIG_FILE" \
   && grep -q '"logArguments": false' "$CONFIG_FILE"; then
  :
else
  FAILURES="${FAILURES}config-not-autocreated,"
fi

# logArguments default (false): the bare name is logged and the arguments are
# NOT. Both halves are needed — the positive alone passes under either setting.
if ! grep -q '\[CommandsSpy\] \[Server\] say$' "$LOG_FILE"; then
  FAILURES="${FAILURES}logargs-default-bare-name-missing,"
fi
if grep -q '\[CommandsSpy\] \[Server\] say e2e-args-probe' "$LOG_FILE"; then
  FAILURES="${FAILURES}logargs-default-leaked-arguments,"
fi

if [ "$PLAYER_PHASE" = "1" ]; then
  if ! grep -q "\[CommandsSpy\] \[Player: e2e_player1\] ${PLAYER_LIST_LITERAL}" "$LOG_FILE"; then
    FAILURES="${FAILURES}player-command-not-logged,"
  fi
  # Negative CROSS-CHECK: player2 joined but sent NOTHING, so its name must
  # never appear as a command source; count so the evidence line can show the number.
  PLAYER2_LINES="$(grep -c 'Player: e2e_player2' "$LOG_FILE" || true)"
  PLAYER2_LINES="${PLAYER2_LINES:-0}"
  if [ "$PLAYER2_LINES" -ne 0 ]; then
    FAILURES="${FAILURES}player-misattributed,"
  fi
fi

if [ "$BOOTED" -ne 1 ]; then
  FAILURES="${FAILURES}boot-failed,"
fi

echo "[e2e] Assertion results:"
if [ "$QUILT_ENTRYPOINT_GAP" = "1" ]; then
  if grep -q 'Loading CommandsSpy' "$LOG_FILE"; then echo "  [FAIL] quilt pre-1.18 entrypoint gap has closed upstream — update docs/version-matrix.md and drop QUILT_ENTRYPOINT_GAP"; else echo "  [PASS] quilt pre-1.18: entrypoint banner absent as expected (mixins still asserted below)"; fi
elif grep -q 'Loading CommandsSpy' "$LOG_FILE"; then echo "  [PASS] mod loaded (Loading CommandsSpy)"; else echo "  [FAIL] mod not loaded (Loading CommandsSpy)"; fi
if [ "$LOADER" = "fabric" ] || [ "$LOADER" = "quilt" ]; then
  if grep -q 'CommandsSpy preLaunch: config loaded\.' "$LOG_FILE"; then echo "  [PASS] preLaunch entrypoint invoked (config pulled up to boot)"; else echo "  [FAIL] preLaunch entrypoint NOT invoked — the preLaunch call site regressed"; fi
fi
if [ "$LOADER" = "forge" ]; then echo "  [SKIP] mixin check: Forge uses CommandEvent, no Mixin to apply";
elif [ "$LOADER" = "neoforge" ]; then echo "  [SKIP] mixin assertion: the NeoForge jar has no mixin (it hooks CommandEvent)";
elif grep -qE 'was not found|could not find any targets matching' "$LOG_FILE"; then echo "  [FAIL] mixin not applied (injection target missing)"; else echo "  [PASS] mixin applied (no missing-target report)"; fi
if grep -q '\[CommandsSpy\] \[Server\] list' "$LOG_FILE"; then echo "  [PASS] console command logged"; else echo "  [FAIL] console command not logged"; fi
if grep -q '\[CommandsSpy\] \[Server\] notacommand' "$LOG_FILE"; then echo "  [PASS] non-existing command logged"; else echo "  [FAIL] non-existing command not logged"; fi
if grep -q "\[CommandsSpy\] \[${RCON_SOURCE_NAME}\] save-all" "$LOG_FILE"; then echo "  [PASS] rcon command logged as [${RCON_SOURCE_NAME}]"; else echo "  [FAIL] rcon command not logged as [${RCON_SOURCE_NAME}]"; fi
if [ "$CONFIG_AT_BOOT" -eq 1 ]; then echo "  [PASS] config/commands-spy.json existed at boot, before any command ran"; else echo "  [FAIL] config/commands-spy.json did NOT exist at boot — MOD.md's \"On startup\" promise unmet on this leg"; fi
if [ -f "$CONFIG_FILE" ] && grep -q '"blacklist": \[\]' "$CONFIG_FILE" && grep -q '"logArguments": false' "$CONFIG_FILE"; then echo "  [PASS] config/commands-spy.json auto-created with the documented initial schema"; else echo "  [FAIL] config/commands-spy.json missing or not the documented initial schema"; cat "$CONFIG_FILE" 2>/dev/null || true; fi
if grep -q '\[CommandsSpy\] \[Server\] say$' "$LOG_FILE" && ! grep -q '\[CommandsSpy\] \[Server\] say e2e-args-probe' "$LOG_FILE"; then echo "  [PASS] logArguments=false (default): 'say e2e-args-probe' logged as bare 'say'"; else echo "  [FAIL] logArguments=false (default) not honoured for 'say e2e-args-probe'"; fi
if [ "$PLAYER_PHASE" = "1" ]; then
  if grep -q "\[CommandsSpy\] \[Player: e2e_player1\] ${PLAYER_LIST_LITERAL}" "$LOG_FILE"; then echo "  [PASS] player command logged as [Player: e2e_player1] ${PLAYER_LIST_LITERAL}"; else echo "  [FAIL] player command not logged as [Player: e2e_player1] ${PLAYER_LIST_LITERAL}"; fi
  if [ "$PLAYER2_LINES" -eq 0 ]; then echo "  [PASS] cross-check: 0 'Player: e2e_player2' lines (silent player never attributed)"; else echo "  [FAIL] cross-check: ${PLAYER2_LINES} 'Player: e2e_player2' line(s) — silent player got attributed a command"; fi
fi

echo "[e2e] Full contents of $LOG_FILE:"
cat "$LOG_FILE" || true

if [ -z "$FAILURES" ]; then
  if [ "$PLAYER_PHASE" = "1" ]; then
    echo "E2E ${MC_VERSION} PASS"
  else
    # Console+RCON-only pass; reachable only on standalone PLAYER_PHASE=0 runs.
    echo "E2E ${MC_VERSION} PASS players-skipped-unsupported-protocol"
  fi
  exit 0
else
  echo "E2E ${MC_VERSION} FAIL ${FAILURES%,}"
  exit 1
fi
