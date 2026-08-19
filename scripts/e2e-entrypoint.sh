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

cd /fabric-server

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

if [ -f "/tmp/mod.jar" ]; then
  mkdir -p mods
  cp /tmp/mod.jar mods/
  echo "[e2e] Mod JAR copied to mods/"
fi

# NOTE: fabric-api is intentionally NOT installed — the mod's fabric-api
# dependency was removed, and running without it is part of what this
# test verifies.

echo 'eula=true' > eula.txt

cat > server.properties <<EOF
enable-rcon=true
rcon.port=${RCON_PORT}
rcon.password=${RCON_PASSWORD}
online-mode=false
# --- minimal footprint: smallest world and least work to reach "Done (...)" ---
level-type=flat
# Void world: nothing to generate. Tuning rationale in docs/e2e-harness.md.
generator-settings={"layers":[],"biome":"minecraft:the_void"}
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
echo "[e2e] Starting Fabric server for Minecraft $MC_VERSION..."
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
# Atomic stage-dir+mv because parallel jobs race.
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

if [ -f logs/latest.log ]; then
  LOG_FILE="logs/latest.log"
else
  LOG_FILE="server.log"
fi

# RCON source name: 'Recon' <1.16, 'Rcon' >=1.16 — exact literal on purpose.
# See docs/e2e-harness.md.
case "$MC_VERSION" in
  1.14|1.14.*|1.15|1.15.*) RCON_SOURCE_NAME="Recon" ;;
  *)                       RCON_SOURCE_NAME="Rcon" ;;
esac
echo "[e2e] Minecraft $MC_VERSION: expecting RCON command source named '$RCON_SOURCE_NAME'"

# Player /list literal: slash included <1.19, bare 'list' on 1.19+.
# See docs/e2e-harness.md.
case "$MC_VERSION" in
  1.14|1.14.*|1.15|1.15.*|1.16|1.16.*|1.17|1.17.*|1.18|1.18.*) PLAYER_LIST_LITERAL="/list" ;;
  *)                                                           PLAYER_LIST_LITERAL="list" ;;
esac

FAILURES=""

if ! grep -q 'Loading CommandsSpy' "$LOG_FILE"; then
  FAILURES="${FAILURES}mod-not-loaded,"
fi

# Two phrasings, era-exact: older Mixin says "was not found", modern Mixin
# "could not find any targets matching".
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
    # Console+RCON-only pass; reachable only on standalone PLAYER_PHASE=0 runs.
    echo "E2E ${MC_VERSION} PASS players-skipped-unsupported-protocol"
  fi
  exit 0
else
  echo "E2E ${MC_VERSION} FAIL ${FAILURES%,}"
  exit 1
fi
