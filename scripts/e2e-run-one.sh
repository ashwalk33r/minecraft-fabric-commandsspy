#!/bin/bash
# Run the e2e suite for exactly one Minecraft version, in one container.
#
# Correctness contract: this script ALWAYS writes exactly one result file, even
# if it is killed. The Makefile reaps those files after the parallel fan-out; a
# missing file is treated as a failure. No failure can be lost.
set -euo pipefail

VERSION="${1:?usage: e2e-run-one.sh <minecraft-version> | --print-java|--print-routing <minecraft-version>}"

# Probe modes: query the routing table and exit, before any env validation.
#   --print-java     -> the era-correct Java floor        ("17")
#   --print-routing  -> jar family and Java floor         ("1192 17")
PROBE=""
case "$VERSION" in
  --print-java|--print-routing)
    PROBE="$VERSION"
    VERSION="${2:?usage: e2e-run-one.sh $VERSION <minecraft-version>}"
    ;;
esac

# Era-correct routing — this case statement is the floor/jar table's SINGLE
# HOME; consumers use the probe flags. 1.19.0 is unsupported (no ParseResults
# overload). Rationale: docs/version-matrix.md.
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

LOADER="${LOADER:-fabric}"
case "$LOADER" in
  fabric|quilt) ;;
  *) echo "[e2e] Unsupported LOADER=$LOADER. Supported: fabric quilt" >&2; exit 1 ;;
esac
QUILT_LOADER_VERSION="${QUILT_LOADER_VERSION:-0.30.0}"
QUILT_INSTALLER_VERSION="${QUILT_INSTALLER_VERSION:-0.15.1}"

: "${REPO_ROOT:?REPO_ROOT must be set}"
: "${MOD_JAR_121:?MOD_JAR_121 must be set}"
: "${MOD_JAR_1192:?MOD_JAR_1192 must be set}"
: "${MOD_JAR_114:?MOD_JAR_114 must be set}"
: "${MOD_JAR_26:?MOD_JAR_26 must be set}"

_mod_jar_var="MOD_JAR_${JAR_FAMILY}"
MOD_JAR="${!_mod_jar_var}"
: "${E2E_LOG_DIR:=build/e2e-logs}"
: "${E2E_RESULT_DIR:=build/e2e-results}"
: "${E2E_RUN_ID:=manual}"
: "${BOOT_TIMEOUT:=180}"

: "${JAVA_OVERRIDE:=}"

: "${E2E_JAR_CACHE:=}"

BASE_KEY="$VERSION"
if [ "$LOADER" = "quilt" ]; then
  BASE_KEY="${BASE_KEY}-quilt"
fi
if [ -n "$JAVA_OVERRIDE" ]; then
  JAVA_VERSION="$JAVA_OVERRIDE"
  KEY="${BASE_KEY}-java${JAVA_VERSION}"
else
  JAVA_VERSION="$FLOOR_JAVA"
  KEY="$BASE_KEY"
fi

IMAGE="commandsspy-e2e:java${JAVA_VERSION}"

# Player phase: two fake players join and one sends /list.
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

QUILT_TMP_DIR=""
# shellcheck disable=SC2329 # invoked via the trap below, not directly
cleanup() {
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  [ -n "$QUILT_TMP_DIR" ] && rm -rf "$QUILT_TMP_DIR"
  return 0
}
trap cleanup EXIT INT TERM

# Quilt path: quilt-installer needs Java 17+, but some server containers run
# Java 8 (mc114 band) — so the install happens HERE, on the host, via a
# one-off Java-17 container, never inside the per-Java-floor server
# container. Populates the same host-side cache the Fabric path already
# uses (or a per-run temp dir when caching is disabled), then bind-mounts
# the result read-only into the server container below.
QUILT_MOUNT_ARGS=""
if [ "$LOADER" = "quilt" ]; then
  QUILT_CACHE_KEY="quilt-${VERSION}-loader${QUILT_LOADER_VERSION}-installer${QUILT_INSTALLER_VERSION}"
  if [ -n "$E2E_JAR_CACHE" ]; then
    QUILT_INSTALL_DIR="${E2E_JAR_CACHE}/${QUILT_CACHE_KEY}"
  else
    QUILT_TMP_DIR="$(mktemp -d)"
    QUILT_INSTALL_DIR="$QUILT_TMP_DIR"
  fi
  mkdir -p "$QUILT_INSTALL_DIR"
  # quilt-server-launch.jar is a THIN jar (Main-Class + a relative
  # Class-Path: libraries/... manifest entry) — the whole libraries/ tree
  # the installer downloads alongside it must travel with it, not just the
  # two top-level jars.
  if [ -f "${QUILT_INSTALL_DIR}/quilt-server-launch.jar" ] && [ -f "${QUILT_INSTALL_DIR}/server.jar" ] && [ -d "${QUILT_INSTALL_DIR}/libraries" ]; then
    echo "[e2e] Quilt install cache HIT for Minecraft $VERSION (loader $QUILT_LOADER_VERSION, installer $QUILT_INSTALLER_VERSION)"
  else
    echo "[e2e] Installing Quilt server for Minecraft $VERSION (loader $QUILT_LOADER_VERSION, installer $QUILT_INSTALLER_VERSION)..."
    QUILT_STAGE_DIR="$(mktemp -d)"
    # --user maps the container process to the invoking host user, so files
    # it writes into the bind mount are host-owned and removable afterward —
    # without it, this runs as root and a later `rm -rf` of root-owned files
    # fails Permission denied under `set -e`, killing the script before the
    # server ever boots (macOS/Docker Desktop hides this; a real Linux CI
    # runner does not).
    if docker run --rm \
        --user "$(id -u):$(id -g)" \
        -v "${QUILT_STAGE_DIR}:/out" \
        eclipse-temurin:17-jre-jammy \
        sh -c "curl -fsSL https://maven.quiltmc.org/repository/release/org/quiltmc/quilt-installer/${QUILT_INSTALLER_VERSION}/quilt-installer-${QUILT_INSTALLER_VERSION}.jar -o /tmp/installer.jar && java -jar /tmp/installer.jar install server ${VERSION} ${QUILT_LOADER_VERSION} --download-server --install-dir=/out"; then
      cp -R "${QUILT_STAGE_DIR}/." "$QUILT_INSTALL_DIR/"
      rm -rf "$QUILT_STAGE_DIR"
    else
      rm -rf "$QUILT_STAGE_DIR"
      printf 'E2E %s java%s FAIL quilt-install-failed\n' "$VERSION" "$JAVA_VERSION" > "$RESULT_FILE"
      echo "[e2e] <- FAIL Minecraft $VERSION: Quilt install failed"
      exit 1
    fi
  fi
  QUILT_MOUNT_ARGS="-v ${QUILT_INSTALL_DIR}:/quilt-preinstalled:ro"
fi

echo "[e2e] -> starting Minecraft $VERSION on java $JAVA_VERSION (container $CONTAINER_NAME)"

# Optional jar-cache mount, built via `set --` (an empty "$@" expands to
# nothing under `set -u` on every bash, unlike an empty array on bash 3.2,
# which macOS still ships as /bin/bash). $1 (VERSION) was consumed above.
set --
if [ -n "$E2E_JAR_CACHE" ]; then
  mkdir -p "$E2E_JAR_CACHE"
  set -- -v "${E2E_JAR_CACHE}:/jar-cache"
fi
if [ -n "$QUILT_MOUNT_ARGS" ]; then
  # shellcheck disable=SC2086 # QUILT_MOUNT_ARGS is a "-v host:container:ro" pair; word splitting is the point
  set -- "$@" $QUILT_MOUNT_ARGS
fi

# NOTE: no -p/--publish. RCON is reached from inside the container over
# 127.0.0.1; publishing a host port would make parallel runs collide.
if docker run --rm \
    --name "$CONTAINER_NAME" \
    --label commandsspy-e2e=1 \
    -e MC_VERSION="$VERSION" \
    -e BOOT_TIMEOUT="$BOOT_TIMEOUT" \
    -e PLAYER_PHASE="$PLAYER_PHASE" \
    -e LOADER="$LOADER" \
    -v "${REPO_ROOT}/${MOD_JAR}:/tmp/mod.jar:ro" \
    "$@" \
    "$IMAGE" 2>&1 | tee "$LOG_FILE" | sed -u "s/^/[$KEY] /"; then
  STATUS=0
else
  STATUS=1
fi

# The entrypoint's final line is the authoritative verdict; it knows nothing
# about Java, so splice the JVM in here.
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
