#!/bin/bash
# Run the e2e suite for exactly one Minecraft version, in one container.
#
# Correctness contract: this script ALWAYS writes exactly one result file, even
# if it is killed. The Makefile reaps those files after the parallel fan-out; a
# missing file is treated as a failure. No failure can be lost.
set -euo pipefail

VERSION="${1:?usage: e2e-run-one.sh <minecraft-version> | --print-java|--print-routing <minecraft-version>}"

# Probe modes: query the routing table for a version and exit, before any env
# validation, so the Makefile's e2e-images target, tools/floors_test.go and
# scripts/test-jar-routing.sh can read it without a full run setup.
#   --print-java     -> the era-correct Java floor        ("17")
#   --print-routing  -> jar family and Java floor         ("1192 17")
PROBE=""
case "$VERSION" in
  --print-java|--print-routing)
    PROBE="$VERSION"
    VERSION="${2:?usage: e2e-run-one.sh $VERSION <minecraft-version>}"
    ;;
esac

# Era-correct routing — this case statement is the table's single home; every
# other consumer queries it via the probe flags above. Floors:
#   1.14-1.16.x -> 8, 1.17.x -> 17 (historical floor 16 has no Temurin jre
#   image, so 1.17.x is CI-booted on 17), 1.18-1.20.2 -> 17,
#   1.20.3-1.21.x -> 21, 26.x -> 25.
# 1.19.1-1.20.2 map to the mc1192 jar (>=1.19.1 <1.20.3, java >=17). 1.19.0
# itself is UNSUPPORTED: its execute() lacks the ParseResults overload the
# mc1192 jar hooks (e2e-disproven: InvalidInjectionException), so it is
# rejected here rather than left to fail as a confusing boot error.
# 1.20.3/1.20.4 map to the mc121 jar: 1.20.3 flipped execute() to void, that
# jar's hook shape — and although their vanilla floor is Java 17, the mc121
# jar targets release 21, so their supported floor here is 21. 1.14-1.18 map
# to the mc114 jar (>=1.14 <1.19, java >=8, Java 8 bytecode): 1.19.1 is where
# execute() gained its ParseResults parameter, so the boundary is exact —
# 1.18.x is the last version of the direct-source descriptor.
case "$VERSION" in
  26*)                 FLOOR_JAVA=25; JAR_FAMILY=26 ;;
  1.20.3|1.20.4|1.20.5|1.20.6|1.21*) FLOOR_JAVA=21; JAR_FAMILY=121 ;;
  1.17*)               FLOOR_JAVA=17; JAR_FAMILY=114 ;;
  1.19|1.19.0)
    echo "[e2e] Minecraft $VERSION is unsupported: the mc1192 jar's floor is 1.19.1 (1.19.0's execute() lacks the ParseResults overload the jar hooks)" >&2
    exit 2
    ;;
  1.19*|1.20|1.20.1|1.20.2) FLOOR_JAVA=17; JAR_FAMILY=1192 ;;
  1.18*)               FLOOR_JAVA=17; JAR_FAMILY=114 ;;
  1.14*|1.15*|1.16*)   FLOOR_JAVA=8;  JAR_FAMILY=114 ;;
  *)                   FLOOR_JAVA=21; JAR_FAMILY=121 ;;
esac

case "$PROBE" in
  --print-java)    echo "$FLOOR_JAVA"; exit 0 ;;
  --print-routing) echo "$JAR_FAMILY $FLOOR_JAVA"; exit 0 ;;
esac

: "${REPO_ROOT:?REPO_ROOT must be set}"
: "${MOD_JAR_121:?MOD_JAR_121 must be set}"
: "${MOD_JAR_1192:?MOD_JAR_1192 must be set}"
: "${MOD_JAR_114:?MOD_JAR_114 must be set}"
: "${MOD_JAR_26:?MOD_JAR_26 must be set}"

# Resolve the routed family to its jar path (env validated just above).
_mod_jar_var="MOD_JAR_${JAR_FAMILY}"
MOD_JAR="${!_mod_jar_var}"
: "${E2E_LOG_DIR:=build/e2e-logs}"
: "${E2E_RESULT_DIR:=build/e2e-results}"
: "${E2E_RUN_ID:=manual}"
: "${BOOT_TIMEOUT:=180}"

# A Minecraft line has a Java FLOOR, not a Java pin. JAVA_OVERRIDE (set by
# `make e2e ... JAVA=<n>`) runs the same version on a newer JVM so that
# compatibility with 25/26 is asserted by a real boot instead of assumed.
: "${JAVA_OVERRIDE:=}"

# Host dir cached across runs (launcher + vanilla server jars, keyed per
# version by the entrypoint). Empty disables the mount and restores the
# download-every-time behaviour. Set by the Makefile; see the jar-cache
# section of scripts/e2e-entrypoint.sh for what lives inside.
: "${E2E_JAR_CACHE:=}"

if [ -n "$JAVA_OVERRIDE" ]; then
  JAVA_VERSION="$JAVA_OVERRIDE"
  # Only override runs get a suffixed key, so default runs keep their existing
  # filenames while grid runs of the same version on different JVMs never
  # overwrite each other's log or result.
  KEY="${VERSION}-java${JAVA_VERSION}"
else
  JAVA_VERSION="$FLOOR_JAVA"
  KEY="$VERSION"
fi

IMAGE="commandsspy-e2e:java${JAVA_VERSION}"

# Player phase: two fake players join and one sends /list. The Go client
# speaks every supported version including 26.2, so every version runs it.
PLAYER_PHASE=1

LOG_FILE="${REPO_ROOT}/${E2E_LOG_DIR}/${KEY}.log"
RESULT_FILE="${REPO_ROOT}/${E2E_RESULT_DIR}/${KEY}.result"

# Docker container names allow [a-zA-Z0-9][a-zA-Z0-9_.-]*, so dots are legal,
# but normalise anyway to keep names easy to read and to match on.
SAFE_KEY="$(printf '%s' "$KEY" | tr -c 'a-zA-Z0-9' '-')"
CONTAINER_NAME="commandsspy-e2e-${SAFE_KEY}-${E2E_RUN_ID}"

mkdir -p "$(dirname "$RESULT_FILE")" "$(dirname "$LOG_FILE")"

# Below the floor the server cannot start at all; report that as an explicit,
# named failure rather than letting it surface as a confusing boot timeout.
if [ "$JAVA_VERSION" -lt "$FLOOR_JAVA" ]; then
  printf 'E2E %s java%s FAIL below-java-floor-%s\n' "$VERSION" "$JAVA_VERSION" "$FLOOR_JAVA" > "$RESULT_FILE"
  echo "[e2e] <- FAIL Minecraft $VERSION on java $JAVA_VERSION (floor is java $FLOOR_JAVA)"
  exit 1
fi

# Default result is failure. Any exit path that does not explicitly overwrite
# this leaves a FAIL on disk, which is exactly what we want.
printf 'E2E %s java%s FAIL runner-died\n' "$VERSION" "$JAVA_VERSION" > "$RESULT_FILE"

# shellcheck disable=SC2329 # invoked via the trap below, not directly
cleanup() {
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

echo "[e2e] -> starting Minecraft $VERSION on java $JAVA_VERSION (container $CONTAINER_NAME)"

# Optional jar-cache mount, built via `set --` (an empty "$@" expands to
# nothing under `set -u` on every bash, unlike an empty array on bash 3.2,
# which macOS still ships as /bin/bash). $1 (VERSION) was consumed above.
set --
if [ -n "$E2E_JAR_CACHE" ]; then
  mkdir -p "$E2E_JAR_CACHE"
  set -- -v "${E2E_JAR_CACHE}:/jar-cache"
fi

# NOTE: no -p/--publish. RCON is reached from inside the container over
# 127.0.0.1; publishing a host port would make parallel runs collide.
if docker run --rm \
    --name "$CONTAINER_NAME" \
    --label commandsspy-e2e=1 \
    -e MC_VERSION="$VERSION" \
    -e BOOT_TIMEOUT="$BOOT_TIMEOUT" \
    -e PLAYER_PHASE="$PLAYER_PHASE" \
    -v "${REPO_ROOT}/${MOD_JAR}:/tmp/mod.jar:ro" \
    "$@" \
    "$IMAGE" > "$LOG_FILE" 2>&1; then
  STATUS=0
else
  STATUS=1
fi

# The entrypoint's final line is the authoritative verdict; it knows nothing
# about Java, so splice the JVM in here. The entrypoint is NOT modified.
RAW_VERDICT="$(grep -E '^E2E ' "$LOG_FILE" | tail -1 || true)"
if [ -z "$RAW_VERDICT" ]; then
  VERDICT="E2E ${VERSION} java${JAVA_VERSION} FAIL no-result"
  STATUS=1
else
  VERDICT="E2E ${VERSION} java${JAVA_VERSION} ${RAW_VERDICT#E2E "${VERSION}" }"
fi
printf '%s\n' "$VERDICT" > "$RESULT_FILE"

if [ "$STATUS" -eq 0 ]; then
  echo "[e2e] <- PASS Minecraft $VERSION on java $JAVA_VERSION"
else
  echo "[e2e] <- FAIL Minecraft $VERSION on java $JAVA_VERSION (log: ${E2E_LOG_DIR}/${KEY}.log)"
fi
exit "$STATUS"
