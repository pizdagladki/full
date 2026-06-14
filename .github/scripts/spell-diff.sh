#!/usr/bin/env bash
# Spell-check ONLY the lines this change adds, with cspell (English + Russian).
#
# Rationale: `typos` (the `spell` job) is a known-misspelling *corpus* — it can only
# flag words in its built-in table, so it never catches Russian typos or English
# misspellings outside that table. cspell is a real *dictionary* spell-checker and
# fills that gap, but run over the whole repo it would flag ~140 pre-existing domain
# terms. So we run it ONLY on newly added/changed lines: existing content is never
# checked, and only newly introduced misspellings fail CI.
#
# Allowlist legitimate-but-unknown terms in cspell.json ("words").
# Russian dictionary is provided by `cspell link add @cspell/dict-ru_ru` in CI.
set -uo pipefail

CSPELL="${CSPELL_BIN:-cspell}"
CONFIG="${CSPELL_CONFIG:-cspell.json}"

# Base commit to diff against: the PR base on pull_request, else the previous commit.
BASE="${BASE_SHA:-}"
if [ -z "$BASE" ] || ! git rev-parse -q --verify "${BASE}^{commit}" >/dev/null 2>&1; then
  BASE="$(git rev-parse -q --verify HEAD^ 2>/dev/null || true)"
fi
if [ -z "$BASE" ]; then
  echo "spell-diff: no base commit to diff against — nothing to check."
  exit 0
fi

# Text files added/copied/modified/renamed in the diff.
EXTS='go|md|mdx|ts|tsx|js|jsx|mjs|cjs|sql|ya?ml|toml|txt|json|sh'
mapfile -t FILES < <(git diff --name-only --diff-filter=ACMR "$BASE"...HEAD | grep -E "\.(${EXTS})$" || true)
if [ "${#FILES[@]}" -eq 0 ]; then
  echo "spell-diff: no changed text files."
  exit 0
fi

status=0
for f in "${FILES[@]}"; do
  [ -f "$f" ] || continue

  # Line numbers this diff ADDS to the new version of the file (--unified=0: no context).
  added="$(git diff --unified=0 "$BASE"...HEAD -- "$f" \
    | awk '/^@@ /{ h=$3; sub(/^\+/,"",h); split(h,a,","); s=a[1]; n=(a[2]==""?1:a[2]); for(i=0;i<n;i++) print s+i }')"
  [ -z "$added" ] && continue

  # cspell findings for the whole file: "<file>:<line>:<col> - Unknown word (..)".
  findings="$("$CSPELL" --no-progress --no-summary --no-must-find-files --config "$CONFIG" "$f" 2>/dev/null || true)"
  [ -z "$findings" ] && continue

  # Keep only findings that land on an added line.
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    ln="$(printf '%s' "$line" | sed -nE 's#^[^:]+:([0-9]+):[0-9]+.*#\1#p')"
    [ -z "$ln" ] && continue
    if grep -qxF "$ln" <<<"$added"; then
      echo "$line"
      status=1
    fi
  done <<<"$findings"
done

if [ "$status" -ne 0 ]; then
  echo ""
  echo "spell-diff: misspelling(s) on changed lines (above). Fix them, or add a"
  echo "legitimate term to the \"words\" list in cspell.json."
fi
exit "$status"
