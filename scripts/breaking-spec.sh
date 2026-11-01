#!/usr/bin/env bash
# Tells whether the spec breaking change is recorded: the "## [Unreleased]"
# section of openapi/CHANGELOG.md has a "### Breaking" subsection with content
# other than "None." (any case, text after the full stop is accepted).
#
# Usage:
#   scripts/breaking-spec.sh [--require] [CHANGELOG]
#     no flag:   prints 1 (recorded) or 0 (not recorded) and exits 0;
#     --require: exits 1 with a message if the section is not filled in
#                (used when the PR carries the `breaking-spec` label).
# Exits 2 on wrong usage or a missing file. Only reads files.
set -euo pipefail

require=0
file=""
for arg in "$@"; do
  case "$arg" in
    --require) require=1 ;;
    -*) echo "breaking-spec: unknown option: $arg" >&2; exit 2 ;;
    *)
      [ -z "$file" ] || { echo "breaking-spec: too many arguments" >&2; exit 2; }
      file="$arg"
      ;;
  esac
done

if [ -z "$file" ]; then
  file="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/openapi/CHANGELOG.md"
fi
[ -f "$file" ] || { echo "breaking-spec: $file does not exist" >&2; exit 2; }

filled="$(awk '
  /^## / { unreleased = ($0 ~ /^## \[Unreleased\]/); breaking = 0; next }
  /^### / { breaking = (unreleased && $0 ~ /^### Breaking[[:space:]]*$/); next }
  breaking {
    line = $0
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", line)
    if (line == "") next
    if (tolower(line) ~ /^none\.?([[:space:]]|$)/) next
    found = 1
  }
  END { print found ? 1 : 0 }
' "$file")"

if [ "$require" -eq 1 ]; then
  if [ "$filled" -ne 1 ]; then
    echo "breaking-spec: breaking-spec label without a filled Breaking section under [Unreleased] ($file)" >&2
    exit 1
  fi
  echo "breaking-spec: Breaking section filled under [Unreleased]"
  exit 0
fi
echo "$filled"
