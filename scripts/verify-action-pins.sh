#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$script_dir/.." && pwd)"
latest="$root/.superpowers/tooling/latest-pins.tsv"
files=(
  "$root/.github/workflows/gradle.yml"
  "$root/.github/actions/gradle/action.yml"
)
actions=(
  "actions/checkout"
  "actions/setup-java"
  "actions/upload-artifact"
  "gradle/actions/wrapper-validation"
  "gradle/actions/setup-gradle"
)

declare -A expected
while IFS=$'\t' read -r name pin; do
  [[ -n "${name:-}" && -n "${pin:-}" ]] || continue
  expected["$name"]="$pin"
done < "$latest"

for file in "${files[@]}"; do
  [[ -f "$file" ]] || { echo "ERROR: missing target file $file" >&2; exit 1; }
done

for name in "${actions[@]}"; do
  pin="${expected[$name]:-}"
  [[ -n "$pin" ]] || { echo "ERROR: missing latest pin for $name in $latest" >&2; exit 1; }
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
