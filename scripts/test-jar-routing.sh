#!/usr/bin/env bash
# Asserts the version->jar/Java contract, independently restated in the
# EXPECTED table below, against every executable source of the mapping.
# Offline, no network, no Docker. See docs/e2e-harness.md.
set -uo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
failures=0

check() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    echo "  ok   $label = $actual"
  else
    echo "  FAIL $label: expected $expected, got $actual"
    failures=$((failures + 1))
  fi
}

# --- THE CONTRACT ----------------------------------------------------------
# version -> "family floor rcon slash". Boundaries are the point: 1.15.2|1.16
# (Recon->Rcon), 1.18.2|1.19.1 (jar + /list->list), 1.20.2|1.20.3 (jar + floor).
# 1.19 and 1.19.0 are UNSUPPORTED and must route to an error, never to a jar.
declare -A EXPECTED=(
  [1.14.4]="114 8 Recon /list"
  [1.15.2]="114 8 Recon /list"
  [1.16.5]="114 8 Rcon /list"
  [1.17.1]="114 17 Rcon /list"
  [1.18.2]="114 17 Rcon /list"
  [1.19.2]="1192 17 Rcon list"
  [1.19.4]="1192 17 Rcon list"
  [1.20.1]="1192 17 Rcon list"
  [1.20.2]="1192 17 Rcon list"
  [1.20.3]="121 21 Rcon list"
  [1.20.4]="121 21 Rcon list"
  [1.20.5]="121 21 Rcon list"
  [1.20.6]="121 21 Rcon list"
  [1.21]="121 21 Rcon list"
  [1.21.1]="121 21 Rcon list"
  [1.21.2]="121 21 Rcon list"
  [1.21.3]="121 21 Rcon list"
  [1.21.4]="121 21 Rcon list"
  [1.21.5]="121 21 Rcon list"
  [1.21.6]="121 21 Rcon list"
  [1.21.7]="121 21 Rcon list"
  [1.21.8]="121 21 Rcon list"
  [1.21.9]="121 21 Rcon list"
  [1.21.10]="121 21 Rcon list"
  [1.21.11]="121 21 Rcon list"
  [26.1]="26 25 Rcon list"
  [26.2]="26 25 Rcon list"
)
# Deterministic iteration order (associative-array order is arbitrary).
ALL_VERSIONS="1.14.4 1.15.2 1.16.5 1.17.1 1.18.2 \
1.19.2 1.19.4 1.20.1 1.20.2 \
1.20.3 1.20.4 1.20.5 1.20.6 \
1.21 1.21.1 1.21.2 1.21.3 1.21.4 1.21.5 1.21.6 1.21.7 1.21.8 1.21.9 1.21.10 1.21.11 \
26.1 26.2"

# Boundary probes outside the default matrix: 1.16 pins the exact edge its
# predecessor sits against (Recon->Rcon flip), 1.19.1 pins the mc1192 jar's
# raised floor — the first version the jar supports, the jar AND slash flip.
EXPECTED[1.16]="114 8 Rcon /list"
EXPECTED[1.19.1]="1192 17 Rcon list"
# The five releases inside >=1.14 <1.19 that this repo named nowhere until
# issue #84 enumerated the declared range. They are published on Modrinth and
# booted by the deep sweep, so their routing is asserted here too — as boundary
# extras rather than in ALL_VERSIONS, which would also grow `make e2e`.
EXPECTED[1.14.1]="114 8 Recon /list"
EXPECTED[1.14.2]="114 8 Recon /list"
EXPECTED[1.14.3]="114 8 Recon /list"
EXPECTED[1.15]="114 8 Recon /list"
EXPECTED[1.15.1]="114 8 Recon /list"
BOUNDARY_EXTRAS="1.16 1.19.1 1.14.1 1.14.2 1.14.3 1.15 1.15.1"

# Versions that must route NOWHERE: the mc1192 jar's floor is 1.19.1.
UNSUPPORTED="1.19 1.19.0"

echo "== e2e-run-one.sh routing (jar family, java floor)"
for v in $ALL_VERSIONS $BOUNDARY_EXTRAS; do
  read -r family floor _rcon _slash <<< "${EXPECTED[$v]}"
  check "run-one $v" "$family $floor" \
        "$("$script_dir/e2e-run-one.sh" --print-routing "$v")"
done
for v in $UNSUPPORTED; do
  if out="$("$script_dir/e2e-run-one.sh" --print-routing "$v" 2>/dev/null)"; then
    check "run-one $v unsupported" "error" "mapped to: $out"
  else
    check "run-one $v unsupported" "error" "error"
  fi
done

# Probe 2 — the Makefile's default VERSIONS list. 1.14.4/1.15.2 are
# deliberately absent from the default list.
echo "== Makefile default VERSIONS"
default_versions="$(make -s -C "$repo_root" print-e2e-versions | tr ' ' '\n' | sort | tr '\n' ' ')"
# shellcheck disable=SC2086 # word splitting is the point: one version per line
expected_default="$(printf '%s\n' $ALL_VERSIONS | grep -vx -e 1.14.4 -e 1.15.2 | sort | tr '\n' ' ')"
check "default VERSIONS (sorted)" "$expected_default" "$default_versions"

# Probe 3 — the CI grid (tools/gen_matrix.go), run for real via `go run . gen-matrix`.
#   grid_probe_init            runs the real grid once (full trigger)
#   grid_floor_of <version> -> "band floor" from the floor-row key holding the
#                              version, or "absent"
#   grid_has <key> <version> -> true/false membership in one output key
GRID_FLOOR_KEYS="mc121_java21 mc26_java25 t0_java21 mc1192_java17 mc114_java8 mc114_java17"
grid_output=""
grid_probe_init() {
  local out; out="$(mktemp)"
  (cd "$repo_root/tools" \
     && EVENT_NAME=workflow_dispatch GITHUB_OUTPUT="$out" REPO_ROOT="$repo_root" \
        go run . gen-matrix > /dev/null)
  grid_output="$(cat "$out")"
  rm -f "$out"
}
grid_floor_of() {
  local v="$1" key json
  for key in $GRID_FLOOR_KEYS; do
    json="$(printf '%s\n' "$grid_output" | grep -E "^${key}=" | cut -d= -f2-)"
    if printf '%s' "$json" | jq -e --arg v "$v" 'any(. == $v)' > /dev/null 2>&1; then
      echo "${key%_java*} ${key##*_java}"
      return 0
    fi
  done
  echo "absent"
}
grid_has() {
  local key="$1" v="$2"
  printf '%s\n' "$grid_output" | grep -E "^${key}=" | cut -d= -f2- \
    | jq --arg v "$2" 'any(. == $v)'
}
declare -A BAND_FAMILY=(
  [mc121]="121" [t0]="121" [mc1192]="1192" [mc114]="114" [mc26]="26"
)

echo "== gen-matrix band membership (floor rows)"
grid_probe_init
for v in $ALL_VERSIONS; do
  read -r family floor _rest <<< "${EXPECTED[$v]}"
  case "$v" in
    # The two gate canaries are MOVED to e2e-gate, never in a floor row.
    1.21.11|26.2) check "grid canary $v not in any floor row" "absent" "$(grid_floor_of "$v")" ;;
    *)
      got="$(grid_floor_of "$v")"
      band="${got%% *}"
      check "grid floor row $v" "$family $floor" \
            "${BAND_FAMILY[$band]:-UNKNOWN-BAND-$band} ${got##* }"
      ;;
  esac
done
for v in $UNSUPPORTED; do
  check "grid $v absent" "absent" "$(grid_floor_of "$v")"
done
# The canaries still get their newest-Java coverage rows.
check "grid 1.21.11 in mc121_java25 coverage" "true" "$(grid_has mc121_java25 1.21.11)"
check "grid 1.21.11 in mc121_java26 coverage" "true" "$(grid_has mc121_java26 1.21.11)"
check "grid 26.2 in mc26_java26 coverage" "true" "$(grid_has mc26_java26 26.2)"

# The gate itself lives in ci.yml as four literal include rows (two
# canaries x two loaders); each canary must run on exactly the floor the
# other sources agree on, on both loaders.
echo "== e2e-gate canary pairs in ci.yml"
gate_yml="$repo_root/.github/workflows/ci.yml"

# The deep sweep's concurrency group (#73, fixed in #75). Keyed on the EVENT as
# well as the ref so a workflow_dispatch sweep and a push run on main do not
# share a lane -- cancel-in-progress belongs to the ARRIVING run, so before this
# every merge to main evicted any sweep already running, silently, as
# `cancelled` rather than `failure`.
#
# Asserted here because the fix is a STRING with no behaviour a test can see:
# reverting it breaks nothing any other check would notice, and the whole
# matrix stays green. That is exactly how it nearly went: a link-checker branch
# restored ci.yml wholesale from a copy predating #75 and silently reverted this
# line, invisible in --stat, caught only by reading the diff. An unasserted
# one-line fix in a file many branches touch is a fix waiting to be undone.
# SC2016: the ${{ }} here is GitHub Actions template syntax being matched
# LITERALLY by grep -F, not a shell expansion that was forgotten. Single quotes
# are required; double quotes would let the shell eat it.
# shellcheck disable=SC2016
check "sweep concurrency group is event-keyed (#73)" "1" \
      "$(grep -cF 'group: ci-${{ github.ref }}-${{ github.event_name }}' "$gate_yml")"
check "no ref-only concurrency group survives" "0" \
      "$(grep -cE '^  group: ci-\$\{\{ github\.ref \}\}$' "$gate_yml")"
check "gate pair 1.21.11/java21/fabric" "1" \
      "$(grep -cF '{ mc: "1.21.11", java: "21", loader: "fabric" }' "$gate_yml")"
check "gate pair 1.21.11/java21/quilt" "1" \
      "$(grep -cF '{ mc: "1.21.11", java: "21", loader: "quilt" }' "$gate_yml")"
check "gate pair 26.2/java25/fabric" "1" \
      "$(grep -cF '{ mc: "26.2", java: "25", loader: "fabric" }' "$gate_yml")"
check "gate pair 26.2/java25/quilt" "1" \
      "$(grep -cF '{ mc: "26.2", java: "25", loader: "quilt" }' "$gate_yml")"

# Probe 3a — LOADER=forge routing (legacy vs modern jar, and the
# out-of-range refusal flag), via the --print-forge-routing probe. Offline:
# the probe exits before any env validation, network call or Docker run.
echo "== LOADER=forge routing (mc116/legacy/modern/eventbus7, refusal flag)"
# 1.19 is excluded (like the Fabric EXPECTED table): the era case statement
# treats it as globally unsupported before any probe runs, forge included.
# 1.16.4 is in-range and known-good: e2e-run-one.sh's install-time ModLauncher
# 8.1.3 drop-in cures the Forge 35.x JDK 8u321+ crash. See the wiki,
# Version-Boundaries-And-Root-Causes -> "Gate 1: the 1.16.4 crash is the JDK's `ManifestEntryVerifier` change".
FORGE_MC116_VERSIONS="1.14.4 1.15 1.15.1 1.15.2 1.16.1 1.16.2 1.16.3 1.16.4 1.16.5"
FORGE_LEGACY_VERSIONS="1.17.1 1.18 1.18.1 1.18.2 1.19.1 1.19.2 1.19.3 1.19.4 1.20 1.20.1 1.20.2 1.20.3 1.20.4"
FORGE_MODERN_VERSIONS="1.20.6 1.21 1.21.1 1.21.2 1.21.3 1.21.4 1.21.5"
FORGE_EB7_VERSIONS="1.21.6 1.21.7 1.21.8 1.21.9 1.21.10 1.21.11 26.1 26.1.1 26.1.2 26.2"
# The two interior holes between the four declared Forge ranges: 1.17 sits
# between the mc116 ceiling (<1.17) and the legacy floor (>=1.17.1), 1.20.5
# between the legacy ceiling (<1.20.5) and the modern floor (>=1.20.6). Both
# route to the nearest jar, which must refuse them. Offline only — Forge
# published no server build for either version, so neither can ever be booted;
# see the guard block in scripts/e2e-entrypoint.sh for what does boot instead.
FORGE_OUT_OF_RANGE_VERSIONS="1.17 1.20.5"
for v in $FORGE_MC116_VERSIONS; do
  check "forge-routing $v -> mc116, not refused" "mc116 0"           "$("$script_dir/e2e-run-one.sh" --print-forge-routing "$v")"
done
for v in $FORGE_LEGACY_VERSIONS; do
  check "forge-routing $v -> legacy, not refused" "legacy 0"         "$("$script_dir/e2e-run-one.sh" --print-forge-routing "$v")"
done
for v in $FORGE_MODERN_VERSIONS; do
  check "forge-routing $v -> modern, not refused" "modern 0"         "$("$script_dir/e2e-run-one.sh" --print-forge-routing "$v")"
done
for v in $FORGE_EB7_VERSIONS; do
  check "forge-routing $v -> eventbus7, not refused" "eventbus7 0"   "$("$script_dir/e2e-run-one.sh" --print-forge-routing "$v")"
done
for v in $FORGE_OUT_OF_RANGE_VERSIONS; do
  got="$("$script_dir/e2e-run-one.sh" --print-forge-routing "$v")"
  check "forge-routing $v refused" "1" "${got##* }"
done

# Probe 3d — the Forge modern band's Java CEILING (issue #66), asserted by
# RUNNING the harness, not by reading its table. Forge's modern bootstrap
# cannot resolve modules on java 24+ (nimbus-jose-jwt requires jdk.crypto.ec,
# removed from the JDK in 24), so the band is java 21 only. The guard exits
# before Docker is touched, which is what makes this cheap enough to pin here.
echo "== Forge modern java ceiling (issue #66)"
ceiling_probe() {
  local java="$1" version="$2" ceiling_override="${3:-}" tmp
  tmp="$(mktemp -d)"
  REPO_ROOT="$tmp" \
  E2E_LOG_DIR="logs" E2E_RESULT_DIR="results" \
  MOD_JAR_121="x.jar" MOD_JAR_1192="x.jar" MOD_JAR_114="x.jar" MOD_JAR_26="x.jar" \
  MOD_JAR_FORGE="x.jar" MOD_JAR_FORGE_LEGACY="x.jar" \
  MOD_JAR_FORGE_EB7="x.jar" MOD_JAR_FORGE_MC116="x.jar" \
  LOADER=forge JAVA_OVERRIDE="$java" FORGE_MODERN_JAVA_CEILING="$ceiling_override" \
    bash "$repo_root/scripts/e2e-run-one.sh" "$version" >/dev/null 2>&1 || true
  cat "$tmp"/results/*.result 2>/dev/null | tr -d '\n'
  rm -rf "$tmp"
}
check "modern 1.21.5 on java 25 is refused with a named verdict" \
      "E2E 1.21.5 java25 FAIL above-java-ceiling-21" "$(ceiling_probe 25 1.21.5)"
check "modern 1.20.6 on java 26 is refused with a named verdict" \
      "E2E 1.20.6 java26 FAIL above-java-ceiling-21" "$(ceiling_probe 26 1.20.6)"
# The ceiling must not swallow the band's own floor leg, nor any other band:
# these get past the guard and fail later, on the absent jar.
check "modern 1.21.5 on java 21 is not refused by the ceiling" \
      "E2E 1.21.5 java21 FAIL mod-jar-missing" "$(ceiling_probe 21 1.21.5)"
check "eventbus7 26.2 on java 26 is not refused by the ceiling" \
      "E2E 26.2 java26 FAIL mod-jar-missing" "$(ceiling_probe 26 26.2)"
# FORGE_MODERN_JAVA_CEILING keeps the java-21-only claim falsifiable: raise it
# and the same java-25 run that was refused above now gets past the guard
# (proving the override reaches the guard, not that java 25 actually boots --
# it fails later, on the absent jar, same as the unset-case checks above).
check "FORGE_MODERN_JAVA_CEILING=26 lets modern 1.21.5 on java 25 past the guard" \
      "E2E 1.21.5 java25 FAIL mod-jar-missing" "$(ceiling_probe 25 1.21.5 26)"

# ...and the four declared ranges the band table above mirrors, exactly as the
# Fabric block near the bottom of this file pins gradle.properties. The band
# case statement in e2e-run-one.sh is a hand-maintained copy of these, so the
# copy proves nothing on its own: without this block, widening a Forge range
# would silently make the two diverge and every check above would still pass.
# Pinning the literal strings is deliberately conservative — ANY move fails
# here, forcing whoever moves one to revisit whether 1.17 and 1.20.5 are still
# the versions no Forge jar claims.
echo "== declared Forge minecraft ranges (forge/gradle.properties)"
forge_gp="$repo_root/forge/gradle.properties"
forge_declared_range() { sed -n "s/^minecraft_range_$1=//p" "$forge_gp"; }
check "minecraft_range_mc116"     "[1.14.4,1.17)"    "$(forge_declared_range mc116)"
check "minecraft_range_legacy"    "[1.17.1,1.20.5)"  "$(forge_declared_range legacy)"
check "minecraft_range_modern"    "[1.20.6,1.21.6)"  "$(forge_declared_range modern)"
check "minecraft_range_eventbus7" "[1.21.6,26.3)"    "$(forge_declared_range eventbus7)"

# Probe 3a-bis — LOADER=fabric/quilt routing and the out-of-range refusal flag.
# This is the always-on half of the Fabric guard: it fails in `contracts`,
# before a single jar is built, whereas the boot leg in ci.yml proves the
# refusal once on one version.
echo "== LOADER=fabric routing (band + refusal flag)"
FABRIC_114_VERSIONS="1.14.4 1.16.5 1.18.2"
FABRIC_1192_VERSIONS="1.19.1 1.19.2 1.20.2"
FABRIC_121_VERSIONS="1.20.3 1.21 1.21.11"
FABRIC_26_VERSIONS="26.1 26.2"
# The crack between the mc114 ceiling (<1.19) and the mc1192 floor (>=1.19.1).
FABRIC_OUT_OF_RANGE_VERSIONS="1.19 1.19.0"
for v in $FABRIC_114_VERSIONS; do
  check "fabric-routing $v -> 114, not refused" "114 0"   "$("$script_dir/e2e-run-one.sh" --print-fabric-routing "$v")"
done
for v in $FABRIC_1192_VERSIONS; do
  check "fabric-routing $v -> 1192, not refused" "1192 0" "$("$script_dir/e2e-run-one.sh" --print-fabric-routing "$v")"
done
for v in $FABRIC_121_VERSIONS; do
  check "fabric-routing $v -> 121, not refused" "121 0"   "$("$script_dir/e2e-run-one.sh" --print-fabric-routing "$v")"
done
for v in $FABRIC_26_VERSIONS; do
  check "fabric-routing $v -> 26, not refused" "26 0"     "$("$script_dir/e2e-run-one.sh" --print-fabric-routing "$v")"
done
for v in $FABRIC_OUT_OF_RANGE_VERSIONS; do
  got="$("$script_dir/e2e-run-one.sh" --print-fabric-routing "$v")"
  check "fabric-routing $v refused" "1" "${got##* }"
done

# ...and the four declared ranges the table above mirrors. e2e-run-one.sh's
# routing case is a hand-maintained copy of these (exactly as the Forge bands
# copy forge/gradle.properties), so the copy proves nothing on its own: without
# this block, widening a range in gradle.properties would silently make the two
# diverge and every check above would still pass. Pinning the literal strings is
# deliberately conservative — ANY move fails here, forcing whoever moves one to
# revisit whether 1.19.0 is still the version no jar claims.
echo "== declared minecraft ranges (gradle.properties, the guard's source of truth)"
gp="$repo_root/gradle.properties"
declared_range() { sed -n "s/^minecraft_range_$1=//p" "$gp"; }
check "minecraft_range_114"  ">=1.14 <1.19"     "$(declared_range 114)"
check "minecraft_range_1192" ">=1.19.1 <1.20.3" "$(declared_range 1192)"
check "minecraft_range_121"  ">=1.20.3 <1.22"   "$(declared_range 121)"
check "minecraft_range_26"   ">=26.1 <26.3"     "$(declared_range 26)"

# Probe 3b — LOADER=neoforge. ONE band jar spans every Minecraft version
# NeoForge publishes for (1.20.2 up); what still varies per version is the
# loader BUILD the installer fetches and NeoForge's own Java floor, both of
# which live in e2e-run-one.sh's routing table. Below NeoForge's own floor a
# version must be REFUSED by name, never handed a jar that cannot load it.
#
# Queried through the probe flag, so this stays offline and needs no built jars.
echo "== LOADER=neoforge routing (via --print-neo-routing)"
neo_route() { "$script_dir/e2e-run-one.sh" --print-neo-routing "$1"; }
check "neoforge 1.20.2 routes"  "20.2.93 17"       "$(neo_route 1.20.2)"
check "neoforge 1.20.4 routes"  "20.4.251 17"      "$(neo_route 1.20.4)"
check "neoforge 1.20.6 routes"  "20.6.139 21"      "$(neo_route 1.20.6)"
check "neoforge 1.21.1 routes"  "21.1.248 21"      "$(neo_route 1.21.1)"
check "neoforge 1.21.11 routes" "21.11.45 21"      "$(neo_route 1.21.11)"
check "neoforge 26.1 routes"    "26.1.0.19-beta 25" "$(neo_route 26.1)"
check "neoforge 26.2 routes"    "26.2.0.64 25"     "$(neo_route 26.2)"
check "neoforge 1.20.1 refused" "unsupported 0"    "$(neo_route 1.20.1)"
check "neoforge 1.16.5 refused" "unsupported 0"    "$(neo_route 1.16.5)"
check "neoforge 1.14.4 refused" "unsupported 0"    "$(neo_route 1.14.4)"

# The refusal has to survive a REAL run too, not just the probe: e2e-run-one.sh
# must die on a named verdict before Docker. The four family jars point at a
# real (empty) file so an unsupported version sails past the jar-exists check
# and reaches the neoforge verdict; the band jar points at a path that does not
# exist, so a SUPPORTED version stops at mod-jar-missing instead — proving it
# routed to the band jar rather than being rejected.
echo "== LOADER=neoforge refusal verdict (via e2e-run-one.sh)"
neo_root="$(mktemp -d)"
trap 'rm -rf "$neo_root"' EXIT
: > "$neo_root/family.jar"
neo_verdict() {
  # e2e-run-one.sh's contract: exactly one result file, verdict as its last field.
  REPO_ROOT="$neo_root" LOADER=neoforge \
  E2E_RESULT_DIR=results E2E_LOG_DIR=logs \
  MOD_JAR_121=family.jar MOD_JAR_1192=family.jar \
  MOD_JAR_114=family.jar MOD_JAR_26=family.jar \
  MOD_JAR_NEO=absent-neoforge.jar \
    "$script_dir/e2e-run-one.sh" "$1" > /dev/null 2>&1
  awk '{print $NF}' "$neo_root/results/$1-neoforge.result" 2>/dev/null \
    || echo "no-result-file"
}
check "neoforge 1.21.1 reaches the band jar" \
      "mod-jar-missing" "$(neo_verdict 1.21.1)"
check "neoforge 1.20.2 reaches the band jar" \
      "mod-jar-missing" "$(neo_verdict 1.20.2)"
check "neoforge 1.16.5 refused by name" \
      "neoforge-unsupported-version" "$(neo_verdict 1.16.5)"

# The band jar must carry BOTH metadata files: FML 1.x/2.x read
# META-INF/mods.toml and require `mandatory`, FML 3.x+ read
# META-INF/neoforge.mods.toml and require `type`. One jar spans that seam only
# because each major reads the filename it knows and ignores the other.
echo "== neoforge band jar metadata"
# find, not ls: shellcheck SC2012, and the glob may legitimately match nothing.
neo_jar="$(find build/libs -maxdepth 1 -name 'commandsspy-*+mc1.20.2-26.2-neoforge.jar' 2>/dev/null | head -1)"
if [ -n "$neo_jar" ]; then
  check "band jar has META-INF/mods.toml" "present" \
        "$(unzip -l "$neo_jar" | grep -q 'META-INF/mods.toml' && echo present || echo absent)"
  check "band jar has META-INF/neoforge.mods.toml" "present" \
        "$(unzip -l "$neo_jar" | grep -q 'META-INF/neoforge.mods.toml' && echo present || echo absent)"
  check "legacy toml uses mandatory=true" "present" \
        "$(unzip -p "$neo_jar" META-INF/mods.toml | grep -q 'mandatory = true' && echo present || echo absent)"
  check "modern toml uses type=required" "present" \
        "$(unzip -p "$neo_jar" META-INF/neoforge.mods.toml | grep -q 'type = "required"' && echo present || echo absent)"
  check "both tomls declare the same range" "same" \
        "$([ "$(unzip -p "$neo_jar" META-INF/mods.toml | grep -c 'versionRange')" = \
             "$(unzip -p "$neo_jar" META-INF/neoforge.mods.toml | grep -c 'versionRange')" ] && echo same || echo differ)"
else
  echo "  [SKIP] band jar not built (run: make build-neo)"
fi

# Both metadata files must ship in every Fabric/Quilt era jar. Quilt Loader
# reads quilt.mod.json and does not fall back to fabric.mod.json for a jar that
# has one, so the Quilt platform badge rests on this file being present — and
# no e2e leg can catch its absence, because Quilt's fabric-compat layer would
# silently load a fabric.mod.json-only jar and every assertion would still pass.
echo "== fabric/quilt jar metadata"
# The entrypoint check compares the actual class lists, not counts: both files
# now declare main AND preLaunch, and CommandsSpyFabricPreLaunch contains the
# substring CommandsSpyFabric, so any count-based proxy is either blind to
# preLaunch or trivially equal. Empty extraction is a failure, not a pass —
# hence the distinct :- fallbacks, which can never compare equal.
for era in mc1.14.x mc1.19-1.20.2 mc1.21.x mc26.x; do
  era_jar="$(find build/libs -maxdepth 1 -name "commandsspy-*+${era}.jar" 2>/dev/null | head -1)"
  if [ -z "$era_jar" ]; then
    echo "  [SKIP] $era jar not built (run: make build)"
    continue
  fi
  check "$era has fabric.mod.json" "present" \
        "$(unzip -l "$era_jar" | grep -q 'fabric\.mod\.json' && echo present || echo absent)"
  check "$era has quilt.mod.json" "present" \
        "$(unzip -l "$era_jar" | grep -q 'quilt\.mod\.json' && echo present || echo absent)"
  fabric_eps="$(unzip -p "$era_jar" fabric.mod.json 2>/dev/null | grep -o 'pl\.m2x\.commandsspy\.[A-Za-z0-9_$]*' | sort -u | tr '\n' ' ')"
  quilt_eps="$(unzip -p "$era_jar" quilt.mod.json 2>/dev/null | grep -o 'pl\.m2x\.commandsspy\.[A-Za-z0-9_$]*' | sort -u | tr '\n' ' ')"
  check "$era names the same entrypoints in both" \
        "${fabric_eps:-<no-fabric-entrypoints>}" "${quilt_eps:-<no-quilt-entrypoints>}"
  # Both files must declare the SAME Minecraft range, or the jar advertises one
  # supported range to Fabric and another to Quilt — silently, since the e2e
  # matrix only boots versions that are supposed to work. They express it in
  # different shapes on purpose (Fabric a space-separated string, Quilt a JSON
  # array), so nothing textual about them can be compared; parse both and
  # normalise to a sorted list of constraint terms. Splitting the Fabric string
  # on whitespace runs mirrors build.gradle's own reshape, `.trim().split(/\s+/)`
  # — the trim half is already guaranteed by the declared-ranges block above,
  # which pins each property literally. Distinct :- fallbacks again: two failed
  # extractions must read FAIL, not an empty-equals-empty PASS, so both jq
  # expressions must yield NOTHING (not "null") when the key is missing.
  fabric_mc="$(unzip -p "$era_jar" fabric.mod.json 2>/dev/null \
    | jq -r '.depends.minecraft // empty' | tr -s '[:space:]' '\n' | sort | tr '\n' ' ')"
  quilt_mc="$(unzip -p "$era_jar" quilt.mod.json 2>/dev/null \
    | jq -r '.quilt_loader.depends[] | select(.id == "minecraft") | .versions.all[]' \
    | sort | tr '\n' ' ')"
  check "$era declares the same minecraft range in both" \
        "${fabric_mc:-<no-fabric-range>}" "${quilt_mc:-<no-quilt-range>}"
done

# Probe 3c — the four generated NeoForge e2e legs in ci.yml. Every field is
# read out of the SAME job block as the job name, so a crossed-over pair (the
# java 25 leg wired to the java 17 band) fails here instead of booting a 26.2
# server on a JVM that cannot start it. The lists themselves are gen_matrix's
# job; what this pins is the wiring between a leg and the band it runs.
echo "== NeoForge e2e legs in ci.yml"
ci_job_block() {
  awk -v job="  $1:" '$0 == job { inb = 1; next } inb && /^  [^ ]/ { exit } inb' "$gate_yml"
}
ci_job_field() { ci_job_block "$1" | sed -n "s/^      $2: //p"; }
# The fwd leg (#58) is the forward-JVM one: same java 25, but its band is the
# separate neo_fwd_java25 row, so a crossed-over wiring here would silently
# turn forward coverage back into a duplicate of the java-25 floor leg.
for spec in "e2e-neoforge-java17 neo_java17 17" \
            "e2e-neoforge-java21 neo_java21 21" \
            "e2e-neoforge-java25 neo_java25 25" \
            "e2e-neoforge-fwd-java25 neo_fwd_java25 25"; do
  read -r job key java <<< "$spec"
  check "$job versions" "\${{ needs.contracts.outputs.$key }}" "$(ci_job_field "$job" versions)"
  check "$job java"     "\"$java\""  "$(ci_job_field "$job" java)"
  check "$job loader"   '"neoforge"' "$(ci_job_field "$job" loader)"
  check "$job skips an empty band" "1" \
        "$(ci_job_block "$job" | grep -cF "needs.contracts.outputs.$key != '[]'")"
  check "$job needs build-neo" "1" \
        "$(ci_job_block "$job" | grep -cF 'build-neo,')"
done
# The band jar replaced two single-version jars: the old legs, the old build
# jobs and the old artifact names must all be gone, or a stale needs: entry
# fails the whole workflow parse and a stale download pattern fetches nothing.
check "no literal NeoForge e2e legs left" "0" "$(grep -cE '^  e2e-neoforge-mc' "$gate_yml")"
check "no split NeoForge build jobs left" "0" "$(grep -c 'build-neo121\|build-neo26' "$gate_yml")"
check "one NeoForge build job" "1" "$(grep -cE '^  build-neo:$' "$gate_yml")"
# The ${{ }} is workflow syntax being matched literally, not a shell
# expansion, so single quotes are deliberate (shellcheck SC2016).
# shellcheck disable=SC2016
check "band jar uploaded under one artifact name" "1" \
      "$(grep -cF 'name: commandsspy-jar-neoforge-${{ github.sha }}' "$gate_yml")"

# Probe 3e — the six generated Forge e2e legs in ci.yml. Same wiring check as
# the NeoForge legs above, and the one that keeps issue #66 honest: nothing
# else asserts that no MODERN-band leg is scheduled above java 21. The
# forward-JVM leg (forge_java26) is eventbus7-only for that reason, and
# floors_test.go pins its band membership; what is pinned here is its JVM.
echo "== Forge e2e legs in ci.yml"
for spec in "e2e-forge-java21 forge_java21 21" \
            "e2e-forge-legacy-java17 forge_legacy_java17 17" \
            "e2e-forge-mc116-java8 forge_mc116_java8 8" \
            "e2e-forge-eventbus7-java21 forge_eventbus7_java21 21" \
            "e2e-forge-eventbus7-java25 forge_eventbus7_java25 25" \
            "e2e-forge-java26 forge_java26 26"; do
  read -r job key java <<< "$spec"
  check "$job versions" "\${{ needs.contracts.outputs.$key }}" "$(ci_job_field "$job" versions)"
  check "$job java"     "\"$java\""  "$(ci_job_field "$job" java)"
  check "$job loader"   '"forge"'    "$(ci_job_field "$job" loader)"
  check "$job skips an empty band" "1" \
        "$(ci_job_block "$job" | grep -cF "needs.contracts.outputs.$key != '[]'")"
done
# This pins the six GENERATED Forge legs only -- there are two other, static
# Forge legs in ci.yml (e2e-config-behaviors-forge, e2e-forge-refusal-guard)
# that are not derived from the band tables above, so out of scope here; both
# are pinned separately below (the config-behaviors block, and the
# out-of-range refusal guard block). Among the six, forge_java21 is the only
# key touching the modern band, and it references its own band key twice (the
# if-guard and versions:), so 2 is the correct baseline. Any future GENERATED
# leg pointing forge_java21's band at a newer JVM adds a third reference and
# has to delete this line.
check "forge_java21 referenced exactly twice in ci.yml (if-guard + versions)" "2" \
      "$(grep -c 'needs.contracts.outputs.forge_java21' "$gate_yml")"

echo "== config-behaviors legs in ci.yml (#34)"
for loader in fabric quilt forge neoforge; do
  check "config-behaviors leg present ($loader)" "1" \
        "$(grep -cE "^  e2e-config-behaviors-${loader}:$" "$gate_yml")"
done
check "config-behaviors legs pass config-variant: 1" "4" \
      "$(grep -cE '^      config-variant: "1"$' "$gate_yml")"
# e2e-config-behaviors-forge is a static leg, not generated from the band
# tables above, and it boots 1.21.1 -- the MODERN band. Nothing pinned its
# java: before this; unpinned, a bump above 21 here would silently defeat
# issue #66's ceiling (the run would still be refused at runtime, but nothing
# would catch the regression before ci.yml ever ran).
check "e2e-config-behaviors-forge versions" "'[\"1.21.1\"]'" \
      "$(ci_job_field "e2e-config-behaviors-forge" versions)"
check "e2e-config-behaviors-forge java"     '"21"' \
      "$(ci_job_field "e2e-config-behaviors-forge" java)"
check "e2e-config-behaviors-forge loader"   '"forge"' \
      "$(ci_job_field "e2e-config-behaviors-forge" loader)"

# The boot half of the out-of-range guard. Pinned here so deleting a leg from
# ci.yml fails `contracts` loudly instead of quietly removing the only place
# that proves the loader actually refuses the mod.
echo "== out-of-range refusal guard legs in ci.yml"
for loader in fabric quilt; do
  check "refusal guard leg present ($loader)" "1" \
        "$(grep -cE "^  e2e-${loader}-refusal-guard:$" "$gate_yml")"
done
check "refusal guard legs pass fabric-expect-refused: 1" "2" \
      "$(grep -cE '^      fabric-expect-refused: "1"$' "$gate_yml")"
check "refusal guard legs run the uncovered version" "2" \
      "$(grep -cF "versions: '[\"1.19\"]'" "$gate_yml")"
# The Forge half of the same guard is shaped differently (forge-refusal-probe,
# not fabric-expect-refused; 1.21.6, not 1.19) because Forge's own holes
# (1.17, 1.20.5) have no published server build at all -- see the comment
# above e2e-forge-refusal-guard in ci.yml for why. Pinned here for
# completeness, not for issue #66: this leg forces FORGE_JAR_BAND=modern, but
# 1.21.6 is already outside FORGE_KNOWN_GOOD_MODERN, so FORGE_EXPECT_REFUSED
# is 1 before the ceiling arm ever runs (it's gated on FORGE_EXPECT_REFUSED !=
# "1" in e2e-run-one.sh) -- this leg can never exercise the ceiling.
check "e2e-forge-refusal-guard versions" "'[\"1.21.6\"]'" \
      "$(ci_job_field "e2e-forge-refusal-guard" versions)"
check "e2e-forge-refusal-guard java"     '"21"' \
      "$(ci_job_field "e2e-forge-refusal-guard" java)"
check "e2e-forge-refusal-guard loader"   '"forge"' \
      "$(ci_job_field "e2e-forge-refusal-guard" loader)"

# Probe 5 — THE SAMPLING RULE (issue #59). Two halves.
#
# Half one: the coverage table in tools/gen_matrix.go says which declared
# versions CI deliberately never boots, and gives a reason for each. Most of
# those reasons are claims about the world that go stale on their own -- "this
# NeoForge line only has a beta build", "Forge refuses this version". They are
# re-probed here, read out of the table itself rather than restated, so the day
# NeoForge promotes 21.9.16-beta the exclusion fails instead of quietly
# outliving its reason.
echo "== sampling rule: every exclusion still deserves its reason"
coverage_tsv="$(cd "$repo_root/tools" && go run . gen-matrix --coverage)"
excluded_of() {
  printf '%s\n' "$coverage_tsv" | awk -F'\t' -v b="$1" '$1 == b && $2 == "excluded" { print $3 }'
}
reason_of() {
  printf '%s\n' "$coverage_tsv" \
    | awk -F'\t' -v b="$1" -v v="$2" '$1 == b && $2 == "excluded" && $3 == v { print $4 }'
}
# NeoForge: excluded because the line's newest build is a prerelease. Field 1 of
# --print-neo-routing is that build.
for v in $(excluded_of neo); do
  build="$(neo_route "$v" | cut -d' ' -f1)"
  case "$build" in
    *-beta) got="beta" ;;
    *)      got="STABLE ($build) -- promote it into the band's deep list" ;;
  esac
  check "neo exclusion $v is still beta-only" "beta" "$got"
done
# Forge: three different situations reach `excluded`, and only two of them can
# be probed. Where Forge never PUBLISHED a build there is nothing to install and
# nothing to ask -- `e2e-run-one.sh` discovers that from the promotions feed at
# run time, which is a network call, and `contracts` makes none -- so those are
# skipped by their reason prefix, exactly like mc26's two below. What is still
# probed is that CI cannot boot the version: either the shared era gate rejects
# it outright (exit != 0) or the routing probe raises FORGE_EXPECT_REFUSED.
# Note the flag itself conflates unproven-by-CI with out-of-declared-range -- it
# is raised by absence from the hand-maintained FORGE_KNOWN_GOOD_* lists -- so
# the check below deliberately claims only "CI cannot boot this", which is what
# the flag actually witnesses. The coverage table's reason says which of the
# three it is.
forge_refusal_state() {
  local out
  if ! out="$("$script_dir/e2e-run-one.sh" --print-forge-routing "$1" 2>/dev/null)"; then
    echo "refused"
  elif [ "${out##* }" = "1" ]; then
    echo "refused"
  else
    echo "bootable ($out) -- promote it into the band's deep list, or correct its reason"
  fi
}
for band in forge forge_legacy forge_mc116 forge_eventbus7; do
  for v in $(excluded_of "$band"); do
    case "$(reason_of "$band" "$v")" in
      "no Forge build published:"*) continue ;;
    esac
    check "$band exclusion $v is still not bootable by CI (gated or refused)" \
          "refused" "$(forge_refusal_state "$v")"
  done
done
# mc26's two exclusions (26.1.1/26.1.2) are the one case with no executable
# probe: they route fine and are skipped for a mappings reason that only a human
# can retire (the wiki, Version-Boundaries-And-Root-Causes).
# tools/gen_matrix_test.go asserts the
# reason is present; nothing here can assert it is still true.

# Half two: the DENOMINATOR. Every Minecraft release inside a declared range --
# which IS enumerable offline since issue #84: mojangAxis in tools/gen_matrix.go
# holds the release list, releasesIn() cuts it to a range, and every band's
# `declared` is that cut. It is also the list published as Modrinth
# game_versions, so a version that is advertised and booted by nothing now fails
# here instead of shipping. See "The denominator, settled" in the wiki'"'"'s
# Supported-Versions page. The standing invariant: every version named anywhere
# in this repo is booted by some CI leg on some event, or carries a written
# waiver here.
echo "== sampling rule: every named version is booted or waived"
# Waived, with reasons. These are the versions no minecraft_range_* covers: the
# crack between the mc114 ceiling (<1.19) and the mc1192 floor (>=1.19.1). The
# two refusal-guard legs in ci.yml do boot 1.19, but they assert the loader
# REFUSES the mod, which is the absence of coverage rather than coverage.
WAIVED="1.19 1.19.0"
# shellcheck disable=SC2086 # word splitting is the point: one version per line
named="$(printf '%s\n' "${!EXPECTED[@]}" \
  $ALL_VERSIONS $BOUNDARY_EXTRAS $UNSUPPORTED \
  $FORGE_MC116_VERSIONS $FORGE_LEGACY_VERSIONS $FORGE_MODERN_VERSIONS \
  $FORGE_EB7_VERSIONS $FORGE_OUT_OF_RANGE_VERSIONS \
  $FABRIC_114_VERSIONS $FABRIC_1192_VERSIONS $FABRIC_121_VERSIONS \
  $FABRIC_26_VERSIONS $FABRIC_OUT_OF_RANGE_VERSIONS \
  "$(make -s -C "$repo_root" print-e2e-versions)" | tr ' ' '\n' | grep -v '^$' | sort -u)"
# Booted = every version in the deep (workflow_dispatch) grid, plus the literal
# legs ci.yml pins by hand: the four gate canary rows and every `versions:` list
# a caller job passes.
booted="$( { printf '%s\n' "$grid_output" | cut -d= -f2- | grep -oE '"[^"]+"' | tr -d '"'
            grep -oE 'mc: "[^"]+"' "$gate_yml" | cut -d'"' -f2
            grep -oE "^      versions: '\[[^]]*\]'" "$gate_yml" | grep -oE '"[^"]+"' | tr -d '"'
          } | sort -u)"
# shellcheck disable=SC2086 # ditto for $WAIVED
unaccounted="$(printf '%s\n' "$named" \
  | grep -vxF -f <(printf '%s\n' "$booted"; printf '%s\n' $WAIVED) | tr '\n' ' ')"
check "named versions with no CI leg and no waiver" "" "${unaccounted% }"

# Probe 4 — the era-literal cases in scripts/e2e-entrypoint.sh: the three
# `case "$MC_VERSION"` blocks are lifted VERBATIM and executed via eval.
echo "== e2e-entrypoint.sh era literals (RCON source name, console source name, player /list form)"
entrypoint="$script_dir/e2e-entrypoint.sh"
# shellcheck disable=SC2016 # the $ is a sed-pattern literal, not an expansion
era_cases="$(sed -n '/^case "\$MC_VERSION" in/,/^esac/p' "$entrypoint")"
check "entrypoint has exactly three MC_VERSION case blocks" "3" \
      "$(printf '%s\n' "$era_cases" | grep -c '^case ')"
era_of() {
  # shellcheck disable=SC2034 # read by the eval'd case blocks below
  local MC_VERSION="$1" RCON_SOURCE_NAME="" PLAYER_LIST_LITERAL=""
  eval "$era_cases"
  echo "$RCON_SOURCE_NAME $PLAYER_LIST_LITERAL"
}
console_of() {
  # shellcheck disable=SC2034 # read by the eval'd case blocks below
  local MC_VERSION="$1" CONSOLE_SOURCE_NAME=""
  eval "$era_cases"
  echo "$CONSOLE_SOURCE_NAME"
}
for v in $ALL_VERSIONS $BOUNDARY_EXTRAS; do
  read -r _family _floor rcon slash <<< "${EXPECTED[$v]}"
  check "era literals $v" "$rcon $slash" "$(era_of "$v")"
done

# The console source name is its own axis, kept out of EXPECTED because it has
# exactly one non-default row: CONSOLE on Beta 1.7.3 (the game's own
# CommandOutput.getName(); b1.7.3 vanilla prints "CONSOLE: Stopping the
# server.." for the same reason), Server on every modern version. Both halves
# are checked so the literal cannot be flattened into a pattern matching both —
# an assertion that cannot fail proves nothing.
check "console source name b1.7.3" "CONSOLE" "$(console_of b1.7.3)"
check "console source name 1.21"   "Server"  "$(console_of 1.21)"

# b1.7.3's routing row: it predates every era band, and it is reachable on the
# babric loader only. BABRIC is the JAR_FAMILY, 21 the loader stack's Java floor
# (not the game's era-contemporary 8).
check "b1.7.3 routes to the Babric jar on java 21" "BABRIC 21" \
      "$(MOD_JAR_BABRIC=x "$script_dir/e2e-run-one.sh" --print-routing b1.7.3)"

# The other half of that row: babric and b1.7.3 are inseparable, so BOTH
# mismatched pairings must be refused before anything is downloaded. Asserted
# on the exit status AND on the message, because a script that dies for an
# unrelated reason also exits non-zero -- and asserted in both directions,
# since one alone would let the other pairing hand a loader a jar whose
# declared Minecraft version its server can never satisfy.
check_refused() {
  local label="$1" loader="$2" version="$3" out rc
  out="$(LOADER="$loader" MOD_JAR_BABRIC=x MOD_JAR_121=x \
         "$script_dir/e2e-run-one.sh" "$version" 2>&1)" && rc=0 || rc=$?
  if [[ "$rc" -ne 0 && "$out" == *"LOADER=babric and VERSION=b1.7.3 are inseparable"* ]]; then
    echo "  ok   $label = refused (rc=$rc)"
  else
    echo "  FAIL $label: expected the inseparable-pair refusal, got rc=$rc: $out"
    failures=$((failures + 1))
  fi
}
check_refused "LOADER=babric on 1.21 is refused"   babric 1.21
check_refused "LOADER=fabric on b1.7.3 is refused" fabric b1.7.3

echo
if [[ "$failures" -eq 0 ]]; then
  echo "jar-routing: all assertions passed"
  exit 0
fi
echo "jar-routing: $failures assertion(s) failed"
exit 1
