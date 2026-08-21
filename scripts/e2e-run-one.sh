#!/bin/bash
# Run the e2e suite for exactly one Minecraft version, in one container.
#
# Correctness contract: this script ALWAYS writes exactly one result file, even
# if it is killed. The Makefile reaps those files after the parallel fan-out; a
# missing file is treated as a failure. No failure can be lost.
set -euo pipefail

VERSION="${1:?usage: e2e-run-one.sh <minecraft-version> | --print-java|--print-routing|--print-fabric-routing|--print-forge-routing|--print-neo-routing <minecraft-version>}"

# Probe modes: query the routing table and exit, before any env validation.
#   --print-java           -> the era-correct Java floor            ("17")
#   --print-routing        -> jar family and Java floor             ("1192 17")
#   --print-forge-routing  -> Forge jar band and expect-refused flag ("legacy 0")
#   --print-fabric-routing -> Fabric jar family and expect-refused flag ("1192 1")
#   --print-neo-routing    -> NeoForge build and its Java floor  ("21.1.248 21")
PROBE=""
case "$VERSION" in
  --print-java|--print-routing|--print-fabric-routing|--print-forge-routing|--print-neo-routing)
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
    # Not merely skipped: with FABRIC_EXPECT_REFUSED=1 this version becomes the
    # out-of-range guard leg and routes to the mc1192 jar deliberately -- see the
    # refusal block below. --print-fabric-routing is exempt because reporting
    # "this must be refused" IS its job; --print-routing/--print-java keep
    # erroring here, which scripts/test-jar-routing.sh's UNSUPPORTED list pins.
    if [ "$PROBE" != "--print-fabric-routing" ] && [ "${FABRIC_EXPECT_REFUSED:-0}" != "1" ]; then
      echo "[e2e] Minecraft $VERSION is unsupported: the mc1192 jar's floor is 1.19.1 (1.19.0's execute() lacks the ParseResults overload the jar hooks). Set FABRIC_EXPECT_REFUSED=1 to run it as a refusal guard." >&2
      exit 2
    fi
    FLOOR_JAVA=17; JAR_FAMILY=1192
    ;;
  1.19*|1.20|1.20.1|1.20.2) FLOOR_JAVA=17; JAR_FAMILY=1192 ;;
  1.18*)               FLOOR_JAVA=17; JAR_FAMILY=114 ;;
  1.14*|1.15*|1.16*)   FLOOR_JAVA=8;  JAR_FAMILY=114 ;;
  *)                   FLOOR_JAVA=21; JAR_FAMILY=121 ;;
esac

# Out-of-range guard, mirroring FORGE_EXPECT_REFUSED below. The four declared
# ranges live in gradle.properties (minecraft_range_114/_1192/_121/_26); a
# version that falls in none of them must be REFUSED by the loader, not merely
# skipped by this harness. 1.19.0 is the live example: it sits between the mc114
# ceiling (<1.19) and the mc1192 floor (>=1.19.1). JAR_FAMILY=1192 above is
# deliberate — the guard hands the server the jar a real operator would install,
# so what is asserted is that jar's metadata gate, not an absent file. An
# unasserted guard is not a guard: this is what fails the day one of those
# ranges is widened by hand or by a processResources bug.
FABRIC_EXPECT_REFUSED="${FABRIC_EXPECT_REFUSED:-0}"
case "$VERSION" in
  1.19|1.19.0) FABRIC_EXPECT_REFUSED=1 ;;
esac

# Forge routing — version-only, this case statement is the single home for
# which of the four Forge jars (mc116/legacy/modern/eventbus7) a version maps
# to, computed unconditionally (cheap, LOADER-independent) so both the probe
# below and the LOADER=forge runtime path further down read the same values.
# Ranges mirror forge/gradle.properties' minecraft_range_mc116/_legacy/
# _modern/_eventbus7; keep them in step. FORGE_KNOWN_GOOD_MC116/LEGACY/MODERN/
# EB7 are overridable for ad hoc probing (e.g. widening one jar's declared
# range to measure how far the underlying code actually stretches, independent
# of the mods.toml metadata gate a real Forge run enforces separately).
# 1.14/1.15 route to mc116 (the nearest jar, whose declared range they sit
# just below) so their refusal guard probes the jar a user would actually try.
case "$VERSION" in
  1.14*|1.15*|1.16*)                                   FORGE_JAR_BAND=mc116 ;;
  1.17*|1.18*|1.19*|1.20|1.20.1|1.20.2|1.20.3|1.20.4)  FORGE_JAR_BAND=legacy ;;
  1.21.6|1.21.7|1.21.8|1.21.9|1.21.10|1.21.11|26*)     FORGE_JAR_BAND=eventbus7 ;;
  *)                                                   FORGE_JAR_BAND=modern ;;
esac
# 1.16.4's whole Forge 35.x line predates the ModLauncher fix for the JDK
# 8u321+ ManifestEntryVerifier change and cannot boot a STOCK current JDK 8
# (mod-independent) — the harness makes it known-good by dropping the fixed
# ModLauncher 8.1.3 into the server install at install time; see the
# LOADER=forge install block below and docs/version-matrix.md.
FORGE_KNOWN_GOOD_MC116="${FORGE_KNOWN_GOOD_MC116:-1.14.4 1.15.2 1.16.1 1.16.2 1.16.3 1.16.4 1.16.5}"
FORGE_KNOWN_GOOD_LEGACY="${FORGE_KNOWN_GOOD_LEGACY:-1.17.1 1.18 1.18.1 1.18.2 1.19.1 1.19.2 1.19.3 1.19.4 1.20 1.20.1 1.20.2 1.20.3 1.20.4}"
FORGE_KNOWN_GOOD_MODERN="${FORGE_KNOWN_GOOD_MODERN:-1.20.6 1.21 1.21.1 1.21.2 1.21.3 1.21.4 1.21.5}"
FORGE_KNOWN_GOOD_EB7="${FORGE_KNOWN_GOOD_EB7:-1.21.6 1.21.7 1.21.8 1.21.9 1.21.10 1.21.11 26.1 26.1.1 26.1.2 26.2}"
case "$FORGE_JAR_BAND" in
  mc116)     FORGE_KNOWN_GOOD="$FORGE_KNOWN_GOOD_MC116" ;;
  legacy)    FORGE_KNOWN_GOOD="$FORGE_KNOWN_GOOD_LEGACY" ;;
  eventbus7) FORGE_KNOWN_GOOD="$FORGE_KNOWN_GOOD_EB7" ;;
  *)         FORGE_KNOWN_GOOD="$FORGE_KNOWN_GOOD_MODERN" ;;
esac
FORGE_EXPECT_REFUSED=0
case " $FORGE_KNOWN_GOOD " in
  *" $VERSION "*) ;;
  *) FORGE_EXPECT_REFUSED=1 ;;
esac

# NeoForge routing -- version-only, computed unconditionally like the Forge
# table above so the probe and the LOADER=neoforge runtime path read the same
# values. Two things come from here, and neither can be derived from the era
# table: WHICH loader build the installer fetches (NeoForge publishes one line
# per Minecraft version -- that part of the old two-jar story was always true,
# it just never constrained the JAR), and NeoForge's OWN Java floor (17 up to
# line 20.4, 21 through 21.11, 25 on 26.x), which is not the Fabric jar's
# bytecode level. One band jar serves every row; see docs/version-matrix.md.
#
# Rows marked (beta) are lines that never published a stable build; the band's
# compile anchor is never one of them. Keep this table in step with the boot
# table in docs/version-matrix.md.
NEOFORGE_VERSION=""
NEO_FLOOR_JAVA=0
case "$VERSION" in
  1.20.2)  NEOFORGE_VERSION="20.2.93";        NEO_FLOOR_JAVA=17 ;;
  1.20.3)  NEOFORGE_VERSION="20.3.8-beta";    NEO_FLOOR_JAVA=17 ;;
  1.20.4)  NEOFORGE_VERSION="20.4.251";       NEO_FLOOR_JAVA=17 ;;
  1.20.5)  NEOFORGE_VERSION="20.5.21-beta";   NEO_FLOOR_JAVA=21 ;;
  1.20.6)  NEOFORGE_VERSION="20.6.139";       NEO_FLOOR_JAVA=21 ;;
  1.21)    NEOFORGE_VERSION="21.0.167";       NEO_FLOOR_JAVA=21 ;;
  1.21.1)  NEOFORGE_VERSION="21.1.248";       NEO_FLOOR_JAVA=21 ;;
  1.21.2)  NEOFORGE_VERSION="21.2.1-beta";    NEO_FLOOR_JAVA=21 ;;
  1.21.3)  NEOFORGE_VERSION="21.3.97";        NEO_FLOOR_JAVA=21 ;;
  1.21.4)  NEOFORGE_VERSION="21.4.157";       NEO_FLOOR_JAVA=21 ;;
  1.21.5)  NEOFORGE_VERSION="21.5.98";        NEO_FLOOR_JAVA=21 ;;
  1.21.6)  NEOFORGE_VERSION="21.6.20-beta";   NEO_FLOOR_JAVA=21 ;;
  1.21.7)  NEOFORGE_VERSION="21.7.25-beta";   NEO_FLOOR_JAVA=21 ;;
  1.21.8)  NEOFORGE_VERSION="21.8.54";        NEO_FLOOR_JAVA=21 ;;
  1.21.9)  NEOFORGE_VERSION="21.9.16-beta";   NEO_FLOOR_JAVA=21 ;;
  1.21.10) NEOFORGE_VERSION="21.10.64";       NEO_FLOOR_JAVA=21 ;;
  1.21.11) NEOFORGE_VERSION="21.11.45";       NEO_FLOOR_JAVA=21 ;;
  26.1)    NEOFORGE_VERSION="26.1.0.19-beta"; NEO_FLOOR_JAVA=25 ;;
  26.1.1)  NEOFORGE_VERSION="26.1.1.15-beta"; NEO_FLOOR_JAVA=25 ;;
  26.1.2)  NEOFORGE_VERSION="26.1.2.97";      NEO_FLOOR_JAVA=25 ;;
  26.2)    NEOFORGE_VERSION="26.2.0.64";      NEO_FLOOR_JAVA=25 ;;
esac

case "$PROBE" in
  --print-java)          echo "$FLOOR_JAVA"; exit 0 ;;
  --print-routing)       echo "$JAR_FAMILY $FLOOR_JAVA"; exit 0 ;;
  --print-forge-routing) echo "$FORGE_JAR_BAND $FORGE_EXPECT_REFUSED"; exit 0 ;;
  --print-fabric-routing) echo "$JAR_FAMILY $FABRIC_EXPECT_REFUSED"; exit 0 ;;
  --print-neo-routing)
    if [ -n "$NEOFORGE_VERSION" ]; then
      echo "$NEOFORGE_VERSION $NEO_FLOOR_JAVA"
    else
      echo "unsupported 0"
    fi
    exit 0 ;;
esac

LOADER="${LOADER:-fabric}"
case "$LOADER" in
  fabric|quilt|forge|neoforge) ;;
  *) echo "[e2e] Unsupported LOADER=$LOADER. Supported: fabric quilt forge neoforge" >&2; exit 1 ;;
esac
# FORGE_EXPECT_REFUSED/FORGE_JAR_BAND above are computed loader-independently
# (cheap, and --print-forge-routing wants them regardless of LOADER), but the
# out-of-range guard leg they drive only makes sense for a real Forge run —
# every other loader has its own mods.toml/fabric.mod.json range gate, unrelated
# to Forge's. Zero it here, once, rather than gating every consumer downstream.
if [ "$LOADER" != "forge" ]; then
  FORGE_EXPECT_REFUSED=0
fi
# Same reasoning in reverse: the Fabric/Quilt guard asserts fabric.mod.json's
# (and quilt.mod.json's) minecraft range, which Forge and NeoForge never read.
if [ "$LOADER" != "fabric" ] && [ "$LOADER" != "quilt" ]; then
  FABRIC_EXPECT_REFUSED=0
fi
# In-range Forge legs: the jar's OWN bytecode floor is what matters here, not
# the generic per-MC-version table above (that table reflects the FABRIC
# jar's bytecode requirement at 1.20.3+, e.g. 21 -- irrelevant to the Forge
# legacy jar, which is Java-17 bytecode uniformly across 1.17.1-1.20.4).
# eventbus7 keeps the generic per-version floor: 21 for 1.21.x and 25 for
# 26.x are both >= the jar's java-21 bytecode, and 26.x servers themselves
# require 25. Guard-leg (out-of-range) probes are deliberately left alone
# here -- they get their own override further down, once JAVA_VERSION is
# resolved.
if [ "$LOADER" = "forge" ] && [ "$FORGE_EXPECT_REFUSED" != "1" ]; then
  case "$FORGE_JAR_BAND" in
    mc116)  FLOOR_JAVA=8 ;;
    legacy) FLOOR_JAVA=17 ;;
    modern) FLOOR_JAVA=21 ;;
  esac
fi
QUILT_LOADER_VERSION="${QUILT_LOADER_VERSION:-0.30.0}"
QUILT_INSTALLER_VERSION="${QUILT_INSTALLER_VERSION:-0.15.1}"
# Forge's analogue of Fabric's meta API. FORGE_BUILD pins a build explicitly;
# empty means "ask the feed for <mc>-recommended, else <mc>-latest".
FORGE_PROMOTIONS_URL="${FORGE_PROMOTIONS_URL:-https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json}"
FORGE_BUILD="${FORGE_BUILD:-}"
# The installer is a desktop-JDK tool, run on the HOST side (see below), so it
# is independent of the server container's era Java floor.
FORGE_INSTALL_JDK="${FORGE_INSTALL_JDK:-21}"

: "${REPO_ROOT:?REPO_ROOT must be set}"
: "${MOD_JAR_121:?MOD_JAR_121 must be set}"
: "${MOD_JAR_1192:?MOD_JAR_1192 must be set}"
: "${MOD_JAR_114:?MOD_JAR_114 must be set}"
: "${MOD_JAR_26:?MOD_JAR_26 must be set}"
if [ "$LOADER" = "neoforge" ]; then
  : "${MOD_JAR_NEO:?MOD_JAR_NEO must be set for LOADER=neoforge}"
fi

if [ "$LOADER" = "forge" ]; then
  # FORGE_JAR_BAND was computed above, in the single-home routing table.
  : "${MOD_JAR_FORGE:?MOD_JAR_FORGE must be set when LOADER=forge}"
  : "${MOD_JAR_FORGE_LEGACY:?MOD_JAR_FORGE_LEGACY must be set when LOADER=forge}"
  : "${MOD_JAR_FORGE_EB7:?MOD_JAR_FORGE_EB7 must be set when LOADER=forge}"
  : "${MOD_JAR_FORGE_MC116:?MOD_JAR_FORGE_MC116 must be set when LOADER=forge}"
  case "$FORGE_JAR_BAND" in
    mc116)     MOD_JAR="$MOD_JAR_FORGE_MC116" ;;
    legacy)    MOD_JAR="$MOD_JAR_FORGE_LEGACY" ;;
    eventbus7) MOD_JAR="$MOD_JAR_FORGE_EB7" ;;
    *)         MOD_JAR="$MOD_JAR_FORGE" ;;
  esac
else
  _mod_jar_var="MOD_JAR_${JAR_FAMILY}"
  MOD_JAR="${!_mod_jar_var}"
fi

# NeoForge OVERRIDES the era table above on both axes it owns: the band jar
# instead of the era jar, and NeoForge's own Java floor instead of the Fabric
# jar's bytecode level. Both come from the routing table near the top of this
# script. A Minecraft version with no NeoForge line at all keeps an empty
# NEOFORGE_VERSION and fails explicitly below, never with a jar that cannot
# load it.
# FLOOR_JAVA only decides the JVM when no JAVA override is given (see the KEY
# block below), so overriding it unconditionally here is safe and needs no
# ordering dance with JAVA_OVERRIDE's own default.
if [ "$LOADER" = "neoforge" ] && [ -n "$NEOFORGE_VERSION" ]; then
  MOD_JAR="$MOD_JAR_NEO"
  FLOOR_JAVA="$NEO_FLOOR_JAVA"
fi
: "${E2E_LOG_DIR:=build/e2e-logs}"
: "${E2E_RESULT_DIR:=build/e2e-results}"
: "${E2E_RUN_ID:=manual}"
: "${BOOT_TIMEOUT:=180}"

: "${JAVA_OVERRIDE:=}"

: "${E2E_JAR_CACHE:=}"

# 1 = run the config-behaviors leg (pre-seeded blacklist + logArguments:true)
# instead of the default assertions. Its own KEY suffix keeps its logs, results
# and container name from colliding with the default leg for the same version.
: "${CONFIG_VARIANT:=0}"

# fabric contributes no suffix, so its keys -- and therefore its log files,
# result files and container names -- are byte-for-byte what they were before
# the loader axis existed. Mirrored by the Makefile's _loader_suffix.
BASE_KEY="$VERSION"
if [ "$LOADER" != "fabric" ]; then
  BASE_KEY="${BASE_KEY}-${LOADER}"
fi
if [ -n "$JAVA_OVERRIDE" ]; then
  JAVA_VERSION="$JAVA_OVERRIDE"
  KEY="${BASE_KEY}-java${JAVA_VERSION}"
else
  JAVA_VERSION="$FLOOR_JAVA"
  KEY="$BASE_KEY"
fi
if [ "$CONFIG_VARIANT" = "1" ]; then
  KEY="${KEY}-cfgvar"
fi

IMAGE="commandsspy-e2e:java${JAVA_VERSION}"

# Player phase: two fake players join and one sends /list.
PLAYER_PHASE=1
if [ "$CONFIG_VARIANT" = "1" ]; then
  # The config-behaviors leg's verdict block returns before any player
  # assertion runs, so the Go bot phase would be pure wasted boot time.
  PLAYER_PHASE=0
fi

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

# Both checks below are reported here, after the result file is armed, so the
# failure is recorded rather than lost to an early exit.

# A missing jar must fail HERE. `docker run -v <missing path>:/tmp/mod.jar`
# silently creates an empty DIRECTORY at the source path and mounts that, so the
# container sees no mod, boots perfectly, and reports mod-not-loaded — a build
# or path bug wearing a mod bug's clothes.
if [ ! -f "${REPO_ROOT}/${MOD_JAR}" ]; then
  printf 'E2E %s java%s FAIL mod-jar-missing\n' "$VERSION" "$JAVA_VERSION" > "$RESULT_FILE"
  echo "[e2e] <- FAIL Minecraft $VERSION: no mod jar at ${REPO_ROOT}/${MOD_JAR} (build it first)"
  exit 1
fi

if [ "$LOADER" = "neoforge" ] && [ -z "$NEOFORGE_VERSION" ]; then
  printf 'E2E %s java%s FAIL neoforge-unsupported-version\n' "$VERSION" "$JAVA_VERSION" > "$RESULT_FILE"
  echo "[e2e] <- FAIL Minecraft $VERSION on neoforge: NeoForge publishes no line for it (its floor is Minecraft 1.20.2)"
  exit 1
fi

PREINSTALL_TMP_DIR=""
# shellcheck disable=SC2329 # invoked via the trap below, not directly
cleanup() {
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  [ -n "$PREINSTALL_TMP_DIR" ] && rm -rf "$PREINSTALL_TMP_DIR"
  return 0
}
trap cleanup EXIT INT TERM

# Quilt path: quilt-installer needs Java 17+, but some server containers run
# Java 8 (mc114 band) — so the install happens HERE, on the host, via a
# one-off Java-17 container, never inside the per-Java-floor server
# container. Populates the same host-side cache the Fabric path already
# uses (or a per-run temp dir when caching is disabled), then bind-mounts
# the result read-only into the server container below.
PREINSTALL_MOUNT_ARGS=""
if [ "$LOADER" = "quilt" ]; then
  QUILT_CACHE_KEY="quilt-${VERSION}-loader${QUILT_LOADER_VERSION}-installer${QUILT_INSTALLER_VERSION}"
  if [ -n "$E2E_JAR_CACHE" ]; then
    QUILT_INSTALL_DIR="${E2E_JAR_CACHE}/${QUILT_CACHE_KEY}"
  else
    PREINSTALL_TMP_DIR="$(mktemp -d)"
    QUILT_INSTALL_DIR="$PREINSTALL_TMP_DIR"
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
  PREINSTALL_MOUNT_ARGS="-v ${QUILT_INSTALL_DIR}:/quilt-preinstalled:ro"
fi

# Forge path: same host-side-install trick, different installer. Forge has no
# launcher jar to download — `--installServer` materialises a whole server tree
# (libraries/, the vanilla jar, and a `unix_args.txt` @argfile), which is then
# cached and bind-mounted read-only exactly like Quilt's.
if [ "$LOADER" = "forge" ]; then
  if [ -z "$FORGE_BUILD" ]; then
    PROMOS="$(curl -fsSL "$FORGE_PROMOTIONS_URL" || true)"
    for _channel in recommended latest; do
      # `|| true`: not every Minecraft version has a -recommended promotion (1.21
      # has only -latest), and under `set -e` a failing grep in a command
      # substitution kills this script outright — silently, before it can write a
      # verdict. Found the hard way; the fallback loop only works if it can fail.
      FORGE_BUILD="$(printf '%s' "$PROMOS" \
        | grep -oE "\"${VERSION}-${_channel}\" *: *\"[^\"]+\"" \
        | grep -oE '[^"]+"$' | tr -d '"' | head -1 || true)"
      [ -n "$FORGE_BUILD" ] && break
    done
  fi
  if [ -z "$FORGE_BUILD" ]; then
    printf 'E2E %s java%s FAIL no-forge-build-for-version\n' "$VERSION" "$JAVA_VERSION" > "$RESULT_FILE"
    echo "[e2e] <- FAIL Minecraft $VERSION: Forge publishes no build for this version"
    exit 1
  fi
  echo "[e2e] Forge build for Minecraft $VERSION: $FORGE_BUILD"

  # Each jar's mods.toml declares its own minecraft range -- mc116
  # [1.14,1.17), legacy [1.17.1,1.20.5), modern [1.20.6,1.21.6),
  # eventbus7 [1.21.6,26.3).
  # Outside its own jar's range
  # Forge MUST refuse to load the mod: the selected jar's official-name
  # (modern) or SRG-name (legacy) calls would resolve to nothing on a
  # runtime whose mapping shape does not match, and the server would die on
  # the FIRST command executed. A metadata string is the only thing
  # preventing that, so the harness asserts the refusal (FORGE_EXPECT_REFUSED,
  # computed above in the single-home routing table) rather than trusting it.

  FORGE_CACHE_KEY="forge-${VERSION}-${FORGE_BUILD}"
  if [ -n "$E2E_JAR_CACHE" ]; then
    FORGE_INSTALL_DIR="${E2E_JAR_CACHE}/${FORGE_CACHE_KEY}"
  else
    PREINSTALL_TMP_DIR="$(mktemp -d)"
    FORGE_INSTALL_DIR="$PREINSTALL_TMP_DIR"
  fi
  mkdir -p "$FORGE_INSTALL_DIR"
  if [ -d "${FORGE_INSTALL_DIR}/libraries" ]; then
    echo "[e2e] Forge install cache HIT for Minecraft $VERSION (build $FORGE_BUILD)"
  else
    echo "[e2e] Installing Forge server for Minecraft $VERSION (build $FORGE_BUILD)..."
    FORGE_STAGE_DIR="$(mktemp -d)"
    FORGE_INSTALLER_URL="https://maven.minecraftforge.net/net/minecraftforge/forge/${VERSION}-${FORGE_BUILD}/forge-${VERSION}-${FORGE_BUILD}-installer.jar"
    # --user: same host-ownership reason as the Quilt block above.
    if docker run --rm \
        --user "$(id -u):$(id -g)" \
        -v "${FORGE_STAGE_DIR}:/out" \
        "eclipse-temurin:${FORGE_INSTALL_JDK}-jdk-jammy" \
        sh -c "cd /out && curl -fsSL '${FORGE_INSTALLER_URL}' -o /tmp/installer.jar && java -jar /tmp/installer.jar --installServer /out"; then
      cp -R "${FORGE_STAGE_DIR}/." "$FORGE_INSTALL_DIR/"
      rm -rf "$FORGE_STAGE_DIR"
    else
      rm -rf "$FORGE_STAGE_DIR"
      printf 'E2E %s java%s FAIL forge-install-failed\n' "$VERSION" "$JAVA_VERSION" > "$RESULT_FILE"
      echo "[e2e] <- FAIL Minecraft $VERSION: Forge install failed"
      exit 1
    fi
  fi
  # Forge 35.x (1.16.4) ships ModLauncher 8.0.x, which cannot boot JDK 8u321+
  # (NoSuchMethodError on sun.security.util.ManifestEntryVerifier.<init> —
  # upstream McModLauncher/modlauncher#91; Forge shipped the fix only on the
  # 1.16.5 branch, and 35.x is frozen forever). Apply the same cure a real
  # 1.16.4 admin does: overwrite the cached 8.0.x jar with ModLauncher 8.1.3
  # (byte-identical to the jar every Forge 36.2.26+ install ships), KEEPING
  # the 8.0.x filename because the forge jar manifest's Class-Path pins it
  # (8.0.6 on 35.1.4, 8.0.9 on 35.1.37 — hence the glob, not a hardcode).
  # sha256-pinned; idempotent, so pre-patch caches heal on their next run.
  if [ "${FORGE_BUILD%%.*}" = "35" ]; then
    ML_FIXED_SHA=4e0d846f75ffd0dd5042c9b1aa86b8fcc758acd27a004c259d26aebc100ffdf2
    for _ml_jar in "${FORGE_INSTALL_DIR}"/libraries/cpw/mods/modlauncher/8.0.*/modlauncher-8.0.*.jar; do
      [ -f "$_ml_jar" ] || continue
      if printf '%s  %s\n' "$ML_FIXED_SHA" "$_ml_jar" | sha256sum -c --status; then
        continue # already patched (cache hit)
      fi
      echo "[e2e] Patching ModLauncher 8.0.x -> 8.1.3 in the Forge $FORGE_BUILD install (JDK 8u321+ ManifestEntryVerifier fix)"
      curl -fsSL "https://maven.minecraftforge.net/cpw/mods/modlauncher/8.1.3/modlauncher-8.1.3.jar" -o "${_ml_jar}.new"
      printf '%s  %s\n' "$ML_FIXED_SHA" "${_ml_jar}.new" | sha256sum -c --status
      mv "${_ml_jar}.new" "$_ml_jar"
    done
  fi
  PREINSTALL_MOUNT_ARGS="-v ${FORGE_INSTALL_DIR}:/forge-preinstalled:ro"
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
if [ -n "$PREINSTALL_MOUNT_ARGS" ]; then
  # shellcheck disable=SC2086 # PREINSTALL_MOUNT_ARGS is a "-v host:container:ro" pair; word splitting is the point
  set -- "$@" $PREINSTALL_MOUNT_ARGS
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
    -e FORGE_EXPECT_REFUSED="$FORGE_EXPECT_REFUSED" \
    -e FABRIC_EXPECT_REFUSED="$FABRIC_EXPECT_REFUSED" \
    -e E2E_CONFIG_VARIANT="$CONFIG_VARIANT" \
    -e NEOFORGE_VERSION="$NEOFORGE_VERSION" \
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
