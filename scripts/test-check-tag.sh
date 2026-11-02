#!/usr/bin/env bash
# Proves scripts/check-tag.sh in temporary directories, outside this repository
# and without creating any tag: the script only reads files, the tag is an argument.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/check-tag-test.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
failed=0

# builds a fake repository (no git) with the files the script reads.
build() { # info_version Version changelog
  rm -rf "$tmp/r"
  mkdir -p "$tmp/r/scripts" "$tmp/r/openapi" "$tmp/r/abertura/internal/version"
  cp "$root/scripts/check-tag.sh" "$tmp/r/scripts/" 2>/dev/null || true
  cat >"$tmp/r/openapi/abertura.yaml" <<YAML
openapi: 3.0.3
info:
  title: Test
  version: $1
  description: x
paths: {}
components:
  schemas:
    Outro:
      version: 9.9.9
YAML
  cat >"$tmp/r/abertura/internal/version/version.go" <<GO
package version

// Version is the current SDK version.
const Version = "$2"
GO
  printf '%s\n' "$3" >"$tmp/r/abertura/CHANGELOG.md"
}

CL_OK=$'# Changelog\n\n## [Unreleased]\n\n## [0.1.0]\n\n- Generated from `openapi/v0.1.0`.\n'
CL_NO_SECTION=$'# Changelog\n\n## [Unreleased]\n\n- Generated from `openapi/v0.1.0`.\n'
CL_NO_CITATION=$'# Changelog\n\n## [Unreleased]\n\nSee `openapi/v0.1.0`.\n\n## [0.1.0]\n\n- First version.\n'
CL_RC=$'# Changelog\n\n## [0.1.0-rc.1]\n\n- Generated from `openapi/v0.1.0-rc.1`.\n\n## [0.1.0]\n\n- First version.\n'
CL_RC_CORE=$'# Changelog\n\n## [0.1.0]\n\n- Generated from `openapi/v0.1.0`.\n'

tests=(
  # name|expected|kind|tag|info.version|Version|changelog
  "openapi tag equal to info.version passes|0|openapi|openapi/v0.1.0|0.1.0|0.1.0-dev|OK"
  "openapi tag different from info.version fails|1|openapi|openapi/v0.1.0|0.1.1|0.1.0-dev|OK"
  "openapi rc with the core info.version passes|0|openapi|openapi/v0.1.0-rc.1|0.1.0|0.1.0-dev|OK"
  "openapi rc of another core fails|1|openapi|openapi/v0.1.1-rc.1|0.1.0|0.1.0-dev|OK"
  "openapi final with an rc info.version fails|1|openapi|openapi/v0.1.0|0.1.0-rc.1|0.1.0-dev|OK"
  "openapi reads the version of the info block, not another|0|openapi|openapi/v0.1.0|0.1.0|0.1.0-dev|OK"
  "openapi bare vX.Y.Z tag fails|1|openapi|v0.1.0|0.1.0|0.1.0-dev|OK"
  "openapi tag without the v fails|1|openapi|openapi/0.1.0|0.1.0|0.1.0-dev|OK"
  "openapi tag with two numbers fails|1|openapi|openapi/v0.1|0.1.0|0.1.0-dev|OK"
  "openapi with an abertura tag fails|1|openapi|abertura/v0.1.0|0.1.0|0.1.0|OK"
  "abertura tag, Version and section citing the spec passes|0|abertura|abertura/v0.1.0|0.1.0|0.1.0|OK"
  "abertura without a CHANGELOG section fails|1|abertura|abertura/v0.1.0|0.1.0|0.1.0|NO_SECTION"
  "abertura section without citing openapi/v fails|1|abertura|abertura/v0.1.0|0.1.0|0.1.0|NO_CITATION"
  "abertura bare vX.Y.Z tag fails|1|abertura|v0.1.0|0.1.0|0.1.0|OK"
  "abertura with a Version different from the tag fails|1|abertura|abertura/v0.1.0|0.1.0|0.1.0-dev|OK"
  "abertura rc with an rc Version and rc section passes|0|abertura|abertura/v0.1.0-rc.1|0.1.0|0.1.0-rc.1|RC"
  "abertura rc with an rc Version and core section passes|0|abertura|abertura/v0.1.0-rc.1|0.1.0|0.1.0-rc.1|RC_CORE"
  "abertura rc with a final Version fails|1|abertura|abertura/v0.1.0-rc.1|0.1.0|0.1.0|RC"
  "abertura with an openapi tag fails|1|abertura|openapi/v0.1.0|0.1.0|0.1.0|OK"
  "abertura with the old go/ prefix fails|1|abertura|go/v0.1.0|0.1.0|0.1.0|OK"
  "go kind is wrong usage (exits 2)|2|go|go/v0.1.0|0.1.0|0.1.0|OK"
  "unknown kind is wrong usage (exits 2)|2|npm|npm/v0.1.0|0.1.0|0.1.0|OK"
)

for linha in "${tests[@]}"; do
  IFS='|' read -r name expected kind tag info ver cl <<<"$linha"
  case "$cl" in
    OK) content="$CL_OK" ;; NO_SECTION) content="$CL_NO_SECTION" ;;
    NO_CITATION) content="$CL_NO_CITATION" ;; RC) content="$CL_RC" ;;
    RC_CORE) content="$CL_RC_CORE" ;;
  esac
  build "$info" "$ver" "$content"
  code=0
  output="$("$tmp/r/scripts/check-tag.sh" "$kind" "$tag" 2>&1)" || code=$?
  if [ "$code" -eq "$expected" ]; then
    echo "ok: $name"
  else
    echo "FAIL: $name (expected $expected, got $code): $output" >&2
    failed=1
  fi
done

# no arguments: usage, exits 2
build 0.1.0 0.1.0 "$CL_OK"
code=0; "$tmp/r/scripts/check-tag.sh" >/dev/null 2>&1 || code=$?
if [ "$code" -eq 2 ]; then echo "ok: no arguments shows the usage (exits 2)"; else echo "FAIL: no arguments should exit 2 (got $code)" >&2; failed=1; fi

exit "$failed"
