#!/bin/bash
set -euo pipefail

MC_VERSION="${MC_VERSION:-1.21}"
BOOT_TIMEOUT="${BOOT_TIMEOUT:-180}"
LOADER_VERSION="${LOADER_VERSION:-0.19.3}"
INSTALLER_VERSION="${INSTALLER_VERSION:-1.1.2}"
RCON_PASSWORD="e2etest"
RCON_PORT="25575"
# PLAYER_PHASE=1 means this entrypoint runs the baked-in Go bot client
# (/usr/local/bin/tools bot) after boot: two fake players join over 127.0.0.1
# and e2e_player1 sends one command; we then assert on the resulting
# [Player: ...] log lines. 0 skips all of that (standalone `docker run`s of
# this image get a plain server with no fake players).
PLAYER_PHASE="${PLAYER_PHASE:-0}"

cd /fabric-server

# Jar cache: /jar-cache (bind-mounted by scripts/e2e-run-one.sh when
# E2E_JAR_CACHE is set; absent on standalone `docker run`s, which keep
# today's download-everything behaviour). It holds ONLY the immutable,
# version-keyed network artifacts — the 182KB launcher stub and the
# 36-61MB vanilla server jar the launcher would fetch from piston-data —
# because those are the whole download cost (measured 2026-08-18, see
# docs/superpowers/plans/2026-08-18-jar-download-cache.md). Mutable
# .fabric state (remappedJars, processedMods) is deliberately NOT cached:
# it interacts with the mod jar under mods/ and varies per era.
JAR_CACHE_KEY="/jar-cache/${MC_VERSION}-loader${LOADER_VERSION}-installer${INSTALLER_VERSION}"

# Download the Fabric server launcher jar (bundles the loader + installer logic;
# downloads/verifies the vanilla Minecraft server on first boot).
LAUNCHER_URL="https://meta.fabricmc.net/v2/versions/loader/${MC_VERSION}/${LOADER_VERSION}/${INSTALLER_VERSION}/server/jar"
if [ -f "${JAR_CACHE_KEY}/fabric-server-launch.jar" ] \
   && [ -f "${JAR_CACHE_KEY}/server/${MC_VERSION}-server.jar" ]; then
  echo "[e2e] Jar cache HIT for Minecraft $MC_VERSION (loader $LOADER_VERSION, installer $INSTALLER_VERSION) — skipping downloads"
  cp "${JAR_CACHE_KEY}/fabric-server-launch.jar" fabric-server-launch.jar
  # Pre-seed the launcher's own download target; it verifies the jar in
  # place and skips the piston-data fetch (verified on 1.14.4/1.21.11/26.2).
  mkdir -p .fabric/server
  cp -R "${JAR_CACHE_KEY}/server/." .fabric/server/
else
  echo "[e2e] Downloading Fabric server launcher for Minecraft $MC_VERSION (loader $LOADER_VERSION, installer $INSTALLER_VERSION)"
  curl -fsSL "$LAUNCHER_URL" -o fabric-server-launch.jar
fi

# Copy mod into place if it exists
if [ -f "/tmp/mod.jar" ]; then
  mkdir -p mods
  cp /tmp/mod.jar mods/
  echo "[e2e] Mod JAR copied to mods/"
fi

# NOTE: fabric-api is intentionally NOT installed — the mod's fabric-api
# dependency was removed, and running without it is part of what this
# test verifies.

# Ensure eula.txt exists
echo 'eula=true' > eula.txt

# Server properties: enable RCON so we can exercise the RCON command path,
# and keep the world small/fast to boot.
cat > server.properties <<EOF
# --- required by the test itself ---
enable-rcon=true
rcon.port=${RCON_PORT}
rcon.password=${RCON_PASSWORD}
online-mode=false
# --- minimal footprint: smallest world and least work to reach "Done (...)" ---
level-type=flat
# Void world: nothing to generate at all. Measured ~8s off Done() on the
# 1.19-1.20.2 era (worldgen is their whole boot cost) and noise elsewhere.
# 1.14/1.15 servers ignore generator-settings entirely (verified via region
# block palettes) — harmless there. Bots spawn mid-air; the command lands
# before falling matters (full player-phase e2e verified on void worlds).
# World CACHING was measured too and rejected: 3-5s actions/cache restore
# per job exceeds any saving, and the 1.19 era OOMs loading a saved world
# at our 512M heap while fresh void gen is faster than the disk load.
generator-settings={"layers":[],"biome":"minecraft:the_void"}
level-seed=e2e
spawn-protection=0
# 3 is the vanilla dedicated-server floor; values below are silently
# clamped up, so this is the true minimum (the previous 2 already ran as 3).
view-distance=3
simulation-distance=3
# 5, not 1: the player phase joins e2e_player1 AND e2e_player2 (a cap of 1
# would reject player2), with headroom for a manually attached debug client.
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
echo "[e2e] Starting Fabric server for Minecraft $MC_VERSION..."
# Minimal runtime footprint for a tiny, short-lived, flat-world server:
#   fixed 512M heap  -> no resize pauses during the ~30s process lifetime
#   SerialGC         -> no G1 region tables / concurrent GC threads at startup
#   TieredStopAtLevel=1 -> C1 only; the server exits before C2 ever pays back
# Intentionally NOT using -XX:+AlwaysPreTouch: it front-loads page faulting and
# would increase time-to-"Done", the metric being optimized.
JAVA_FLAGS="${JAVA_FLAGS:--Xms512M -Xmx512M -XX:+UseSerialGC -XX:TieredStopAtLevel=1}"
# The fifo is held open read-write on fd 3 and handed to java as stdin directly.
# A `tail -f console.in |` pipeline here is a trap: tail never exits, so a
# crashed java would block this script forever. With fd 3, $! is `timeout`'s
# PID (it forks java rather than exec-ing into it) — the orphaned java child
# this leaves behind on a hard kill is harmless: the whole container, and
# everything in it, is torn down the moment this script (its PID 1) exits.
exec 3<>console.in
# shellcheck disable=SC2086 # JAVA_FLAGS is a whitespace-separated flag list; word splitting is the point
timeout "$BOOT_TIMEOUT" java $JAVA_FLAGS -jar fabric-server-launch.jar nogui <&3 > server.log 2>&1 &
SERVER_PID=$!

# Wait (bounded) for the server to finish booting.
BOOTED=0
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
  echo "list" > console.in
  sleep 3

  echo "[e2e] Sending RCON command..."
  /usr/local/bin/tools rcon --port "$RCON_PORT" --password "$RCON_PASSWORD" save-all || echo "[e2e] ⚠ RCON client failed"

  sleep 1

  # Player phase: the baked-in Go bot client joins the two fake players and
  # sends the command. On any failure the player assertions below name it.
  # The bot's own global timeout is 150s; the explicit outer timeout is the
  # belt over it, so this line can never hold the server open indefinitely.
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

# Populate the jar cache from a SUCCESSFULLY BOOTED server only (a busted
# download must never get cached). Atomic via stage-dir + mv because
# parallel jobs of the same version could race; the loser's mv fails on
# the existing key and its stage dir is discarded.
if [ "$BOOTED" -eq 1 ] && [ -d /jar-cache ] && [ ! -d "$JAR_CACHE_KEY" ] \
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

# Prefer logs/latest.log; fall back to the captured boot/console output.
if [ -f logs/latest.log ]; then
  LOG_FILE="logs/latest.log"
else
  LOG_FILE="server.log"
fi

# The exact string vanilla uses to NAME the RCON command source changed at 1.16.
#
#   1.14.4 – 1.15.2 : the class holds TWO constants — "Recon" (the CommandSource name, i.e. what
#                     shows up in logs as `[CommandsSpy] [Recon] save-all`) and "Rcon" (used only
#                     for the internal Log4j logger name).
#   1.16 and later  : the constant pool collapses to a SINGLE "Rcon", reused for both.
#
# Verified by disassembling the server jars of 1.15, 1.15.1, 1.15.2, 1.16, 1.16.1 and 1.16.5 and
# reading the `ldc` feeding the source-name argument; 1.15.2 and 1.16 are adjacent stable releases
# and disagree, so the boundary is pinned exactly. This is asserted as an EXACT literal, never as
# a pattern that would match both spellings — an assertion that cannot fail proves nothing.
case "$MC_VERSION" in
  1.14|1.14.*|1.15|1.15.*) RCON_SOURCE_NAME="Recon" ;;
  *)                       RCON_SOURCE_NAME="Rcon" ;;
esac
echo "[e2e] Minecraft $MC_VERSION: expecting RCON command source named '$RCON_SOURCE_NAME'"

# What a PLAYER-issued /list looks like in the log also splits by era:
#   1.14 – 1.18.x : the command arrives as a chat message, slash included, so
#                   the mod logs `[Player: e2e_player1] /list`.
#   1.19 and later: 1.19 introduced the dedicated chat_command packet, whose
#                   payload has no slash, so the mod logs `... list`.
# Asserted as the exact era literal, never a pattern matching both — same
# rationale as the Recon/Rcon split above.
case "$MC_VERSION" in
  1.14|1.14.*|1.15|1.15.*|1.16|1.16.*|1.17|1.17.*|1.18|1.18.*) PLAYER_LIST_LITERAL="/list" ;;
  *)                                                           PLAYER_LIST_LITERAL="list" ;;
esac

FAILURES=""

if ! grep -q 'Loading CommandsSpy' "$LOG_FILE"; then
  FAILURES="${FAILURES}mod-not-loaded,"
fi

# Two phrasings, era-exact: older Mixin says "was not found", modern Mixin
# says "could not find any targets matching" (mutation-proven 2026-08-18 —
# the old single-literal grep was a dead assert on 1.21.x-era Mixin).
if grep -qE 'was not found|could not find any targets matching' "$LOG_FILE"; then
  FAILURES="${FAILURES}mixin-not-applied,"
fi

if ! grep -q '\[CommandsSpy\] \[Server\] list' "$LOG_FILE"; then
  FAILURES="${FAILURES}console-command-not-logged,"
fi

if ! grep -q "\[CommandsSpy\] \[${RCON_SOURCE_NAME}\] save-all" "$LOG_FILE"; then
  FAILURES="${FAILURES}rcon-command-not-logged,"
fi

if [ "$PLAYER_PHASE" = "1" ]; then
  # Positive: player1's /list must be attributed to player1, era-exact literal.
  if ! grep -q "\[CommandsSpy\] \[Player: e2e_player1\] ${PLAYER_LIST_LITERAL}" "$LOG_FILE"; then
    FAILURES="${FAILURES}player-command-not-logged,"
  fi
  # Negative CROSS-CHECK: player2 joined but sent NOTHING, so its name must
  # never appear as a command source. A count (not just -q) so the evidence
  # line below can show the exact number of offending lines.
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
if grep -q 'Loading CommandsSpy' "$LOG_FILE"; then echo "  [PASS] mod loaded (Loading CommandsSpy)"; else echo "  [FAIL] mod not loaded (Loading CommandsSpy)"; fi
if grep -qE 'was not found|could not find any targets matching' "$LOG_FILE"; then echo "  [FAIL] mixin not applied (injection target missing)"; else echo "  [PASS] mixin applied (no missing-target report)"; fi
if grep -q '\[CommandsSpy\] \[Server\] list' "$LOG_FILE"; then echo "  [PASS] console command logged"; else echo "  [FAIL] console command not logged"; fi
if grep -q "\[CommandsSpy\] \[${RCON_SOURCE_NAME}\] save-all" "$LOG_FILE"; then echo "  [PASS] rcon command logged as [${RCON_SOURCE_NAME}]"; else echo "  [FAIL] rcon command not logged as [${RCON_SOURCE_NAME}]"; fi
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
    # Console+RCON only. The note rides the verdict line so a summary reader
    # can tell a full pass from a player-less one at a glance. The harness
    # always runs the player phase now (the Go client speaks every supported
    # version, 26.2 included), so this is only reachable on standalone
    # PLAYER_PHASE=0 `docker run`s of this image.
    echo "E2E ${MC_VERSION} PASS players-skipped-unsupported-protocol"
  fi
  exit 0
else
  echo "E2E ${MC_VERSION} FAIL ${FAILURES%,}"
  exit 1
fi
