#!/usr/bin/env bash
# Asserts every link into or inside the GitHub wiki still resolves: the target
# page exists and, where the link carries one, the #anchor exists among that
# page's headings. Also re-checks the shas the Supported-Versions provenance
# line cites. Replaces hand-verification; see docs/ci.md.
#
# WIKI_DIR=<path>  use an existing wiki clone instead of cloning (offline).
# Otherwise the public wiki is cloned into a temp dir (needs network, no auth).
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
failures=0
checked=0
links=0

check() {
  local label="$1" expected="$2" actual="$3"
  checked=$((checked + 1))
  if [[ "$expected" == "$actual" ]]; then
    echo "  ok   $label"
  else
    echo "  FAIL $label: $actual"
    failures=$((failures + 1))
  fi
}

WIKI_URL="https://github.com/ashwalk33r/minecraft-fabric-commandsspy.wiki.git"
# The prefix every repo->wiki link carries. Matched literally, never by
# extension: PR #71 filtered a grep to *.go *.sh *.md *.yml and missed 30 live
# references in .java/.properties/.gradle/.toml/Makefile. `git ls-files` is the
# whole tracked tree, and -a keeps a tracked binary from aborting the scan.
WIKI_PREFIX="https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/"

anchor_cache="$(mktemp -d)"
clone_dir=""
trap 'rm -rf "$anchor_cache" ${clone_dir:+"$clone_dir"}' EXIT
if [[ -n "${WIKI_DIR:-}" ]]; then
  wiki_dir="$WIKI_DIR"
else
  clone_dir="$(mktemp -d)"
  wiki_dir="$clone_dir"
  # Retried once, then fatal with its OWN exit code and message. A failed clone
  # must never reach the link loop: zero links checked would otherwise print as
  # "all assertions passed". Same attribution rule as the base-ref branch below
  # — accuse the network, not the citations.
  cloned=""
  for attempt in 1 2; do
    rm -rf "$clone_dir"
    mkdir -p "$clone_dir"
    if git clone --depth 1 --quiet "$WIKI_URL" "$clone_dir"; then cloned="yes"; break; fi
    if [[ "$attempt" -eq 1 ]]; then
      echo "wiki-links: clone attempt 1 failed, retrying" >&2
    fi
  done
  if [[ -z "$cloned" ]]; then
    echo "wiki-links: CANNOT REACH THE WIKI at $WIKI_URL after 2 attempts." >&2
    echo "wiki-links: this is a network failure, NOT a broken citation. Nothing was checked." >&2
    exit 2
  fi
fi

# --- GitHub's heading -> anchor slug ---------------------------------------
# Lowercase; delete everything that is not a letter, digit, space, hyphen or
# underscore (backticks, colons, apostrophes, dots, em dashes, parentheses all
# go); spaces become hyphens. An em dash surrounded by spaces therefore leaves
# a DOUBLE hyphen — both its spaces survive, only the dash dies. Duplicate
# slugs on one page get -1, -2, ... in document order (applied by anchors_of).
slug() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr -cd 'a-z0-9 _-' | tr ' ' '-'
}

# Every anchor a page defines, in document order, one per line. Headings inside
# fenced code blocks are not headings — without the fence toggle a shell comment
# in an example block becomes a phantom anchor that makes a dangling link pass.
anchors_of() {
  local page="$1" heading s n
  local cache="$anchor_cache/$page"
  if [[ -f "$cache" ]]; then cat "$cache"; return 0; fi
  declare -A seen=()
  while IFS= read -r heading; do
    s="$(slug "$heading")"
    n="${seen[$s]:-0}"
    seen[$s]=$((n + 1))
    if [[ "$n" -eq 0 ]]; then echo "$s"; else echo "$s-$n"; fi
  done < <(awk '
      /^[ \t]*(```|~~~)/ { fence = !fence; next }
      !fence && /^#{1,6}[ \t]/ { sub(/^#+[ \t]*/, ""); sub(/[ \t]+$/, ""); print }
    ' "$wiki_dir/$page.md") > "$cache"
  cat "$cache"
}

# "ok", or the diagnosis of why the link dangles.
resolve() {
  local page="$1" anchor="$2"
  if [[ ! -f "$wiki_dir/$page.md" ]]; then
    echo "no such wiki page: $page.md"
    return 0
  fi
  if [[ -z "$anchor" ]]; then
    echo "ok"
    return 0
  fi
  # Process substitution, not a pipe: `grep -q` exits on the first match, and
  # under `set -o pipefail` the SIGPIPE that gives the producer would turn a
  # link that DOES resolve into a failure.
  if grep -qxF -- "$anchor" <(anchors_of "$page"); then
    echo "ok"
  else
    echo "no such anchor in $page.md: #$anchor"
  fi
}

# --- A. repo -> wiki -------------------------------------------------------
echo "== repo -> wiki links (every tracked file, no extension filter)"
while IFS= read -r hit; do
  [[ -n "$hit" ]] || continue
  loc="${hit%%:*}"; rest="${hit#*:}"          # file
  loc="$loc:${rest%%:*}"; url="${rest#*:}"    # file:line
  url="${url%%[.,;:]}"
  target="${url#"$WIKI_PREFIX"}"
  page="${target%%#*}"
  anchor=""
  if [[ "$target" == *#* ]]; then anchor="${target#*#}"; fi
  links=$((links + 1))
  check "$loc -> $target" "ok" "$(resolve "$page" "$anchor")"
done < <(cd "$repo_root" && git ls-files -z \
  | xargs -0 grep -oaHnE "${WIKI_PREFIX//./\\.}[A-Za-z0-9._%#-]+" || true)

# --- A2. repo -> wiki prose citations --------------------------------------
# The unlinked half of the same corpus: a page slug, an arrow, a quoted
# heading. Both halves are asserted — the page exists AND the heading exists on
# it — reusing the same resolver and the same fenced-code-block exclusion.
#
# The page-slug shape is what makes this safe. Ordinary prose does not produce
# Capitalized-Hyphenated-Token followed by an arrow and a quoted string;
# matching the arrow alone would drag in this repo's own
# `-> "family floor rcon slash"` comments. The optional backslash admits Go
# string literals, which escape their quotes.
#
# DELIBERATE SKIP, and it is not small: anything outside that grammar is not
# checked — a bare page name with no quoted heading, and a citation wrapped
# across two comment lines (grep is line-based). The wrapped ones are LISTED at
# run time rather than swallowed, because a skip you cannot see is the defect
# this script exists to retire. Widening the grammar to catch them is a
# separate change; do not widen it by loosening the page-slug requirement.
CITATION_RE='[A-Z][A-Za-z0-9]*(-[A-Za-z0-9]+)+ *-> *\\?"[^"]+\\?"'
echo "== repo -> wiki prose citations (page-slug -> \"Heading\")"
while IFS= read -r hit; do
  [[ -n "$hit" ]] || continue
  loc="${hit%%:*}"; rest="${hit#*:}"
  loc="$loc:${rest%%:*}"; cite="${rest#*:}"
  page="${cite%% *}"
  heading="${cite#*\"}"; heading="${heading%\"}"; heading="${heading%\\}"
  links=$((links + 1))
  check "$loc -> $page \"$heading\"" "ok" "$(resolve "$page" "$(slug "$heading")")"
done < <(cd "$repo_root" && git ls-files -z | xargs -0 grep -oaHnE "$CITATION_RE" || true)
# Visible skips: a citation that opens its quote and never closes it on that
# line. Reported, not asserted — the grammar cannot read the rest of it.
while IFS= read -r hit; do
  [[ -n "$hit" ]] || continue
  echo "  SKIP $hit"
done < <(cd "$repo_root" && git ls-files -z \
  | xargs -0 grep -oaHnE '[A-Z][A-Za-z0-9]*(-[A-Za-z0-9]+)+ *-> *\\?"[^"]*$' || true)

# --- B. wiki -> wiki -------------------------------------------------------
# ](Page), ](Page#anchor) and same-page ](#anchor). External links are somebody
# else's uptime problem and are not checked.
echo "== wiki -> wiki links (page + anchor)"
while IFS= read -r hit; do
  [[ -n "$hit" ]] || continue
  file="${hit%%:*}"; rest="${hit#*:}"
  line="${rest%%:*}"; link="${rest#*:}"
  link="${link#](}"; link="${link%)}"
  case "$link" in *://*|mailto:*|"") continue ;; esac
  self="$(basename "$file" .md)"
  page="${link%%#*}"
  [[ -n "$page" ]] || page="$self"            # ](#anchor) targets its own page
  anchor=""
  if [[ "$link" == *#* ]]; then anchor="${link#*#}"; fi
  links=$((links + 1))
  check "$(basename "$file"):$line -> $link" "ok" "$(resolve "$page" "$anchor")"
done < <(grep -oHnE '\]\([^)]+\)' "$wiki_dir"/*.md || true)

# --- C. the slug algorithm itself ------------------------------------------
# The anchors above are only as trustworthy as the slug function; these are the
# five real headings whose real anchors pin every rule it implements.
echo "== slug algorithm (headings whose anchors are in live use)"
# The backticks below are markdown inside the heading text, not substitutions
# (shellcheck SC2016) — they are exactly the characters rule 3 has to delete.
# shellcheck disable=SC2016
check "backticks deleted" "forge-modern-is-java-21-only" \
      "$(slug 'Forge `modern` is Java 21 only')"
check "em dash leaves a double hyphen" "tier-3--declared-but-never-booted" \
      "$(slug 'Tier 3 — declared but never booted')"
check "colon, dots and apostrophe deleted" \
      "gate-1-the-1164-crash-is-the-jdks-manifestentryverifier-change" \
      "$(slug "Gate 1: the 1.16.4 crash is the JDK's \`ManifestEntryVerifier\` change")"
check "apostrophe inside backticks deleted" "the-makefiles-default-version-list" \
      "$(slug "The \`Makefile\`'s default version list")"
check "colon deleted, dots closed up" "quilt-the-pre-118-entrypoint-gap" \
      "$(slug 'Quilt: the pre-1.18 entrypoint gap')"

# --- D. the provenance line ------------------------------------------------
# Supported-Versions opens with a hand-maintained "Applies to: the grid as it
# stood at `<sha>`" line. Each sha it cites must resolve AND be an ancestor of
# origin/main. That is the strongest claim about the citation that is actually
# falsifiable: it catches a typo, a fabricated sha, and a citation that pointed
# into history someone has since rewritten.
#
# The stronger reading — "the page is current with main's tip" — was considered
# and REJECTED, so do not "improve" this into a tip-equality check. Currency is
# not mechanically checkable: nothing here can tell whether a commit since the
# cited sha changed anything the page describes. A check demanding the citation
# equal main's tip would go red on every merge, for a wiki page no PR touched,
# by design rather than by defect. Swapping an unfalsifiable claim for a check
# that fires on non-defects is the worse trade: the first is merely useless, the
# second trains everyone to ignore a red gate. Ancestry is what is left once
# both are excluded, and it is worth having.
echo "== Supported-Versions provenance shas"
provenance="$(grep -m1 '^| Applies to' "$wiki_dir/Supported-Versions.md" || true)"
if [[ -z "$provenance" ]]; then
  check "provenance line present" "present" "absent from Supported-Versions.md"
else
  # A sha is cited in backticks; 7+ hex chars and nothing else, so `1.21.2`,
  # `gen-matrix` and `proven.csv` in the same sentence are not mistaken for one.
  # shellcheck disable=SC2016 # the backticks are markdown being matched
  shas="$(printf '%s' "$provenance" | grep -oE '`[0-9a-f]{7,40}`' | tr -d '`' | sort -u)"
  check "provenance line cites at least one sha" "yes" \
        "$([[ -n "$shas" ]] && echo yes || echo 'no sha in the Applies-to line')"
  # The base ref is RESOLVED, never assumed. `actions/checkout` does not always
  # leave refs/remotes/origin/main behind — on a pull_request run it checks out
  # the merge ref, and fetch-depth: 0 does not promise the remote-tracking
  # branch. Hardcoding origin/main made an absent ref look like a fabricated
  # sha: a red gate on a PR whose base is fine and a wiki page nobody touched,
  # which is the exact trade the comment above argues against. It cannot
  # reproduce locally, where origin/main always exists.
  base=""
  for ref in origin/main "${GITHUB_BASE_REF:+origin/$GITHUB_BASE_REF}" main; do
    if [[ -n "$ref" ]] && git -C "$repo_root" rev-parse --verify --quiet "$ref" > /dev/null; then
      base="$ref"
      break
    fi
  done
  if [[ -z "$base" ]]; then
    git -C "$repo_root" fetch --quiet origin main 2>/dev/null || true
    if git -C "$repo_root" rev-parse --verify --quiet FETCH_HEAD > /dev/null; then
      base="FETCH_HEAD"
    fi
  fi
  shallow="$(git -C "$repo_root" rev-parse --is-shallow-repository)"
  if [[ -z "$base" ]]; then
    # FAILS, deliberately — do not "improve" this into a skip. A check that
    # skips silently is unfalsifiable exactly where you most need to know it
    # did not run: a green from a skipped check reads identically to a green
    # from a passed one, which is the extension-filtered completeness grep
    # above in a new costume. The friendly objection ("be kind to shallow or
    # offline checkouts") does not apply: with the fetch rung above, no base
    # means no refs AND no network, i.e. a broken runner, not a normal state.
    # See the rejected tip-equality argument above for the same trade.
    #
    # The wording is not the point; the ATTRIBUTION is. This accuses the
    # checkout, never the citation. A gate that misattributes its own failure
    # is worse than one that stays quiet — it sends someone to fix a file that
    # was never wrong. Reword freely, keep that property.
    check "provenance base ref" "ok" \
          "ENVIRONMENT ERROR: cannot resolve a base ref to check ancestry against (tried origin/main, \$GITHUB_BASE_REF, main, and a fetch)"
  else
    for sha in $shas; do
      if ! git -C "$repo_root" cat-file -e "$sha^{commit}" 2>/dev/null; then
        if [[ "$shallow" == "true" ]]; then
          got="ENVIRONMENT ERROR: $sha is absent from a SHALLOW checkout; deepen it before believing this"
        else
          got="$sha does not resolve to a commit"
        fi
      elif git -C "$repo_root" merge-base --is-ancestor "$sha" "$base"; then
        got="ok"
      else
        got="$sha is not an ancestor of $base"
      fi
      check "provenance sha $sha (vs $base)" "ok" "$got"
    done
  fi
fi

echo
echo "wiki-links: $links link(s) checked, $checked assertion(s), over $wiki_dir"
if [[ "$failures" -eq 0 ]]; then
  echo "wiki-links: all assertions passed"
  exit 0
fi
echo "wiki-links: $failures assertion(s) failed"
echo "wiki-links: the fix is EITHER the wiki page OR the citation in this repo — check which is wrong before editing."
echo "wiki-links: reproduce offline against a local clone: WIKI_DIR=../wiki scripts/test-wiki-links.sh"
exit 1
