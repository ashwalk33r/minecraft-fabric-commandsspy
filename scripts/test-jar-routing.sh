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
BOUNDARY_EXTRAS="1.16 1.19.1"

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
# 8.1.3 drop-in cures the Forge 35.x JDK 8u321+ crash. See docs/version-matrix.md.
FORGE_MC116_VERSIONS="1.14.4 1.15.2 1.16.1 1.16.2 1.16.3 1.16.4 1.16.5"
FORGE_LEGACY_VERSIONS="1.17.1 1.18 1.18.1 1.18.2 1.19.1 1.19.2 1.19.3 1.19.4 1.20 1.20.1 1.20.2 1.20.3 1.20.4"
FORGE_MODERN_VERSIONS="1.20.6 1.21 1.21.1 1.21.2 1.21.3 1.21.4 1.21.5"
FORGE_EB7_VERSIONS="1.21.6 1.21.7 1.21.8 1.21.9 1.21.10 1.21.11 26.1 26.1.1 26.1.2 26.2"
FORGE_OUT_OF_RANGE_VERSIONS="1.20.5"
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
done

# Probe 3c — the three generated NeoForge e2e legs in ci.yml. Every field is
# read out of the SAME job block as the job name, so a crossed-over pair (the
# java 25 leg wired to the java 17 band) fails here instead of booting a 26.2
# server on a JVM that cannot start it. The lists themselves are gen_matrix's
# job; what this pins is the wiring between a leg and the band it runs.
echo "== NeoForge e2e legs in ci.yml"
ci_job_block() {
  awk -v job="  $1:" '$0 == job { inb = 1; next } inb && /^  [^ ]/ { exit } inb' "$gate_yml"
}
ci_job_field() { ci_job_block "$1" | sed -n "s/^      $2: //p"; }
for spec in "e2e-neoforge-java17 neo_java17 17" \
            "e2e-neoforge-java21 neo_java21 21" \
            "e2e-neoforge-java25 neo_java25 25"; do
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

echo "== config-behaviors legs in ci.yml (#34)"
for loader in fabric quilt forge neoforge; do
  check "config-behaviors leg present ($loader)" "1" \
        "$(grep -cE "^  e2e-config-behaviors-${loader}:$" "$gate_yml")"
done
check "config-behaviors legs pass config-variant: 1" "4" \
      "$(grep -cE '^      config-variant: "1"$' "$gate_yml")"

# Probe 4 — the era-literal cases in scripts/e2e-entrypoint.sh: the two
# `case "$MC_VERSION"` blocks are lifted VERBATIM and executed via eval.
echo "== e2e-entrypoint.sh era literals (RCON source name, player /list form)"
entrypoint="$script_dir/e2e-entrypoint.sh"
# shellcheck disable=SC2016 # the $ is a sed-pattern literal, not an expansion
era_cases="$(sed -n '/^case "\$MC_VERSION" in/,/^esac/p' "$entrypoint")"
check "entrypoint has exactly two MC_VERSION case blocks" "2" \
      "$(printf '%s\n' "$era_cases" | grep -c '^case ')"
era_of() {
  # shellcheck disable=SC2034 # read by the eval'd case blocks below
  local MC_VERSION="$1" RCON_SOURCE_NAME="" PLAYER_LIST_LITERAL=""
  eval "$era_cases"
  echo "$RCON_SOURCE_NAME $PLAYER_LIST_LITERAL"
}
for v in $ALL_VERSIONS $BOUNDARY_EXTRAS; do
  read -r _family _floor rcon slash <<< "${EXPECTED[$v]}"
  check "era literals $v" "$rcon $slash" "$(era_of "$v")"
done

echo
if [[ "$failures" -eq 0 ]]; then
  echo "jar-routing: all assertions passed"
  exit 0
fi
echo "jar-routing: $failures assertion(s) failed"
exit 1
