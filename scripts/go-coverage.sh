#!/usr/bin/env bash
# Tests with -race and coverage; fails if the coverage of the SDK's root
# package (the `abertura` package, without internal/) falls below the minimum.
#   scripts/go-coverage.sh [minimum_percent]   (default 80)
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
min="${1:-80}"
profile="$root/out/go-coverage.out"
mkdir -p "$root/out"

cd "$root/abertura"
go test -race -covermode=atomic -coverprofile="$profile" ./...

module="$(sed -n 's/^module[[:space:]]*//p' go.mod | head -n1)"
awk -v module="$module" -v min="$min" '
  NR == 1 { next }                       # "mode: atomic" line
  {
    file = $1; sub(/:.*/, "", file)      # path/file.go
    rest = substr(file, length(module) + 2)
    if (index(file, module "/") != 1 || index(rest, "/") > 0) next   # root package only
    n = $2; total += n
    if ($3 > 0) covered += n
  }
  END {
    if (total == 0) { print "coverage: no statement of the root package in the profile" > "/dev/stderr"; exit 1 }
    pct = 100 * covered / total
    printf "root package coverage: %.1f%% (minimum %s%%)\n", pct, min
    if (pct < min) { print "coverage: below the minimum" > "/dev/stderr"; exit 1 }
  }' "$profile"
