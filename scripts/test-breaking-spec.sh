#!/usr/bin/env bash
# Proves scripts/breaking-spec.sh with synthetic CHANGELOGs in a temporary directory.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/breaking-spec-test.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
failed=0

mkdir -p "$tmp/scripts"
cp "$root/scripts/breaking-spec.sh" "$tmp/scripts/"

CL_FILLED=$'# Changelog\n\n## [Unreleased]\n\n### Changed\n\n- x\n\n### Breaking\n\nThe `message` property leaves the 400 response.\n\n## [0.1.0]\n\n### Breaking\n\nNone.\n'
CL_NONE=$'# Changelog\n\n## [Unreleased]\n\n### Breaking\n\nNone.\n\n## [0.1.0]\n'
CL_NONE_TEXT=$'# Changelog\n\n## [Unreleased]\n\n### Breaking\n\nNONE. The section will only exist in future 0.x versions.\n\n## [0.1.0]\n'
CL_NO_SECTION=$'# Changelog\n\n## [Unreleased]\n\n### Changed\n\n- x\n\n## [0.1.0]\n'
CL_RELEASED_ONLY=$'# Changelog\n\n## [Unreleased]\n\n### Changed\n\n- x\n\n## [0.1.0]\n\n### Breaking\n\nThe `message` property leaves.\n'
CL_EMPTY=$'# Changelog\n\n## [Unreleased]\n\n### Breaking\n\n\n### Changed\n\n- x\n'
CL_OTHER_SUBSECTION=$'# Changelog\n\n## [Unreleased]\n\n### Breaking\n\n### Changed\n\n- text that is not a break\n'

tests=(
  # name|expected (output or code)|flag|changelog
  "filled section prints 1|output:1||CL_FILLED"
  "None. prints 0|output:0||CL_NONE"
  "None. with text after prints 0|output:0||CL_NONE_TEXT"
  "no section prints 0|output:0||CL_NO_SECTION"
  "section only in the released version prints 0|output:0||CL_RELEASED_ONLY"
  "empty section prints 0|output:0||CL_EMPTY"
  "text of the next subsection does not count, prints 0|output:0||CL_OTHER_SUBSECTION"
  "--require with a filled section exits 0|code:0|--require|CL_FILLED"
  "--require with None. exits 1|code:1|--require|CL_NONE"
  "--require with no section exits 1|code:1|--require|CL_NO_SECTION"
  "--require with the section only in the released version exits 1|code:1|--require|CL_RELEASED_ONLY"
)

for line in "${tests[@]}"; do
  IFS='|' read -r name expected flag cl <<<"$line"
  printf '%s' "${!cl}" >"$tmp/CHANGELOG.md"
  code=0
  args=(); [ -z "$flag" ] || args+=("$flag")
  output="$("$tmp/scripts/breaking-spec.sh" ${args[@]+"${args[@]}"} "$tmp/CHANGELOG.md" 2>/dev/null)" || code=$?
  case "$expected" in
    output:*) [ "$code" -eq 0 ] && [ "$output" = "${expected#output:}" ] && good=1 || good=0 ;;
    code:*) [ "$code" -eq "${expected#code:}" ] && good=1 || good=0 ;;
  esac
  if [ "$good" -eq 1 ]; then echo "ok: $name"; else echo "FAIL: $name (code $code, output '$output')" >&2; failed=1; fi
done

code=0
"$tmp/scripts/breaking-spec.sh" "$tmp/does-not-exist.md" >/dev/null 2>&1 || code=$?
if [ "$code" -eq 2 ]; then echo "ok: missing file exits 2"; else echo "FAIL: missing file should exit 2 (code $code)" >&2; failed=1; fi

code=0
"$tmp/scripts/breaking-spec.sh" --nonsense "$tmp/CHANGELOG.md" >/dev/null 2>&1 || code=$?
if [ "$code" -eq 2 ]; then echo "ok: unknown option exits 2"; else echo "FAIL: unknown option should exit 2 (code $code)" >&2; failed=1; fi

exit "$failed"
