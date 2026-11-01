#!/usr/bin/env bash
# Project rule (CCEE terms of use of 13/10/2025): the name "CCEE" appears only
# in the non-affiliation notice. Scans the tracked files and fails on every
# occurrence outside that rule, listing file:line.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

# Files that may contain the notice. Every line with "CCEE" must be inside a
# notice block (see below).
notices=(
  NOTICE                          # non-affiliation notice
  abertura/NOTICE                 # copy of NOTICE for the Go module zip
  README.md                       # notice at the top
  README.pt-BR.md                 # Portuguese translation of the README, same notice
  abertura/README.md              # notice at the top
  abertura/doc.go                 # notice in the package documentation
  openapi/abertura.yaml           # notice in info.description
  abertura/internal/spec/abertura.yaml  # copy of the spec (go generate), same notice
  docs/clients.md                 # notice at the top
  docs/template.hbs               # notice banner of the Redoc page
  openapi/CHANGELOG.md            # notice at the top
)

# Files that may mention the name without a notice, because they are the check itself.
tools=(
  openapi/vacuum.yaml             # rule that forbids the name in metadata
  scripts/verify-examples.sh      # checks that the name is only in info.description
  scripts/check-trademark.sh      # this script
  scripts/test-check-trademark.sh # this script's test cases
)

# Notice block: the line, or the previous one (the notice wraps lines), says
# "affiliated", "endorsed" or "unofficial" (or the Portuguese equivalents, for
# README.pt-BR.md).
notice_pattern='affiliated|endorsed|unofficial|afiliad|endoss|não oficial'

in_list() { # file list...
  local f="$1" x; shift
  for x in "$@"; do [ "$x" = "$f" ] && return 0; done
  return 1
}

failures=0
while IFS= read -r file; do
  [ -f "$file" ] || continue
  grep -q 'CCEE' "$file" 2>/dev/null || continue
  if in_list "$file" "${tools[@]}"; then continue; fi
  if ! in_list "$file" "${notices[@]}"; then
    grep -n 'CCEE' "$file" | while IFS=: read -r n _; do
      echo "name outside the list: $file:$n" >&2
    done
    failures=1
    continue
  fi
  while IFS= read -r n; do
    current="$(sed -n "${n}p" "$file")"
    previous=""; [ "$n" -gt 1 ] && previous="$(sed -n "$((n-1))p" "$file")"
    if ! printf '%s\n%s\n' "$previous" "$current" | grep -qiE "$notice_pattern"; then
      echo "name outside the non-affiliation notice: $file:$n" >&2
      failures=1
    fi
  done < <(grep -n 'CCEE' "$file" | cut -d: -f1)
done < <(git ls-files)

if [ "$failures" != 0 ]; then
  echo "check-trademark: \"CCEE\" may only appear in the non-affiliation notice" >&2
  exit 1
fi
echo "check-trademark: ok"
