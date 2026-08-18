#!/usr/bin/env bash
# Offline trap for version->jar / version->Java drift. No GitHub, no network,
# no Docker.
#
# The version->jar-family and version->Java-floor mapping lives in THREE
# executable places — scripts/e2e-run-one.sh (the routing case, the table's
# single home), tools/gen_matrix.go (the CI grid's band lists) and the
# Makefile's default VERSIONS list — plus the two era-literal cases in
# scripts/e2e-entrypoint.sh and the two gate-canary rows in e2e.yml. They are
# held in sync by comments alone, and a drift boots a live server wrong before
# anyone notices. This file is the independent copy that argues back: ONE
# expected table below, and every source is probed by INVOKING its real code
# path (never by regex re-parsing) and asserted against it. A drift in any one
# file disagrees with the table and fails here, offline, in seconds.
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
# version -> "family floor rcon slash". Every supported version: the
# 1.14-1.18 samples, the 1.19.1-1.20.2 line, 1.20.3-1.21.11, and both 26.x
# minors. Boundaries are the point: 1.15.2|1.16 (Recon->Rcon), 1.18.2|1.19.1
# (mc114->mc1192 jar, /list->list), 1.20.2|1.20.3 (mc1192->mc121 jar, floor
# 17->21). 1.19 and 1.19.0 are UNSUPPORTED (the mc1192 jar's floor is 1.19.1:
# 1.19.0's execute() lacks the ParseResults overload the jar hooks) and must
# route to an error, never to a jar.
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

# ===========================================================================
# Probe 1 — scripts/e2e-run-one.sh, the code that picks the jar and JVM for a
# real container run. --print-routing invokes the actual routing case.
# ===========================================================================
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

# ===========================================================================
# Probe 2 — the Makefile. Its floor table is NOT a second copy (e2e-images
# queries `e2e-run-one.sh --print-java`, probed above), but its DEFAULT
# version list is part of the routing surface: a version dropped or added
# there silently changes what `make e2e` exercises. 1.14.4/1.15.2 are
# deliberately absent (explicit-run free-riders on the mc114 jar); everything
# else supported must be present exactly once.
# ===========================================================================
echo "== Makefile default VERSIONS"
default_versions="$(make -s -C "$repo_root" print-e2e-versions | tr ' ' '\n' | sort | tr '\n' ' ')"
# shellcheck disable=SC2086 # word splitting is the point: one version per line
expected_default="$(printf '%s\n' $ALL_VERSIONS | grep -vx -e 1.14.4 -e 1.15.2 | sort | tr '\n' ' ')"
check "default VERSIONS (sorted)" "$expected_default" "$default_versions"

# ===========================================================================
# Probe 3 — grid PROBE. The ONLY place this test reads the CI grid, which
# lives in tools/gen_matrix.go and is invoked here for real via
# `go run . gen-matrix` (full trigger).
#
#   grid_probe_init            runs the real grid once (full trigger)
#   grid_floor_of <version> -> "band floor" from the floor-row key holding the
#                              version, or "absent"
#   grid_has <key> <version> -> true/false membership in one output key
# ===========================================================================
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
# --- end of the grid probe -------------------------------------------------

# Which jar family each grid band boots. The band names are the grid's
# vocabulary; the families are the Makefile's. This pairing IS the contract.
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
# The unsupported versions must be absent from every floor row too.
for v in $UNSUPPORTED; do
  check "grid $v absent" "absent" "$(grid_floor_of "$v")"
done
# The canaries still get their newest-Java coverage rows.
check "grid 1.21.11 in mc121_java25 coverage" "true" "$(grid_has mc121_java25 1.21.11)"
check "grid 1.21.11 in mc121_java26 coverage" "true" "$(grid_has mc121_java26 1.21.11)"
check "grid 26.2 in mc26_java26 coverage" "true" "$(grid_has mc26_java26 26.2)"

# The gate itself lives in e2e.yml as two literal include rows; each canary
# must run on exactly the floor the other sources agree on.
echo "== e2e-gate canary pairs in e2e.yml"
gate_yml="$repo_root/.github/workflows/e2e.yml"
check "gate pair 1.21.11/java21" "1" \
      "$(grep -cF '{ mc: "1.21.11", java: "21" }' "$gate_yml")"
check "gate pair 26.2/java25" "1" \
      "$(grep -cF '{ mc: "26.2", java: "25" }' "$gate_yml")"

# ===========================================================================
# Probe 4 — the era-literal cases in scripts/e2e-entrypoint.sh (RCON source
# name flips Recon->Rcon at 1.16; the player /list literal drops its slash at
# 1.19). The entrypoint runs in-container and takes no flags, so the two
# `case "$MC_VERSION"` blocks are lifted VERBATIM and executed — the real
# code runs, only its location is text-addressed.
# ===========================================================================
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
