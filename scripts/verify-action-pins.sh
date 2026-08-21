#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$script_dir/.." && pwd)"
files=(
  "$root/.github/workflows/ci.yml"
  "$root/.github/workflows/e2e-stage.yml"
)
actions=(
  "actions/cache"
  "actions/cache/restore"
  "actions/checkout"
  "actions/upload-artifact"
  "actions/download-artifact"
  "gradle/actions/wrapper-validation"
)

declare -A expected
expected["actions/cache"]="6.1.0"
expected["actions/cache/restore"]="6.1.0"
expected["actions/checkout"]="7.0.1"
expected["actions/upload-artifact"]="7.0.1"
expected["actions/download-artifact"]="8.0.1"
expected["gradle/actions/wrapper-validation"]="6.3.0"

for file in "${files[@]}"; do
  [[ -f "$file" ]] || { echo "ERROR: missing target file $file" >&2; exit 1; }
done

for name in "${actions[@]}"; do
  pin="${expected[$name]:-}"
  [[ -n "$pin" ]] || { echo "ERROR: missing expected pin for $name" >&2; exit 1; }
  version="v$pin"

  found_latest=0
  stale=()

  for file in "${files[@]}"; do
    while IFS= read -r match; do
      [[ -n "$match" ]] || continue
      if [[ "$match" == *"${name}@${version}"* ]]; then
        found_latest=1
      elif [[ "$match" == *"${name}@v"* ]]; then
        stale+=("$match")
      fi
    done < <(grep -nE "^[[:space:]]*(-[[:space:]]*)?uses:[[:space:]]*${name}@v[0-9]" "$file" || true)
  done

  if (( ! found_latest )); then
    echo "ERROR: expected latest pin missing for $name (${version})" >&2
    exit 1
  fi

  if (( ${#stale[@]} > 0 )); then
    echo "ERROR: stale pins remain for $name" >&2
    printf '%s\n' "${stale[@]}" >&2
    exit 1
  fi
done

echo "OK: verified latest pins in ${files[*]}"
