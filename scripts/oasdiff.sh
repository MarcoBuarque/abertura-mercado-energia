#!/usr/bin/env bash
# Compares openapi/abertura.yaml with the one at the latest openapi/v* tag and
# fails if the change breaks clients (removed operation or property, changed
# type, parameter that becomes required, closed enum that loses a value).
#
# With no previous tag, skips with a notice and exits 0.
# Accepted breaking change (0.x only): the "### Breaking" section of
# "## [Unreleased]" in openapi/CHANGELOG.md has content
# (scripts/breaking-spec.sh). With it, the break shows up as a warning and the
# script exits 0; without it, it fails.
# BREAKING_SPEC=1 forces acceptance (CI sets it when the PR carries the
# `breaking-spec` label, after checking the section); BREAKING_SPEC=0 forces
# strict mode; without the variable, what the CHANGELOG records applies (a
# missing CHANGELOG or scripts/breaking-spec.sh counts as 0).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

image="${OASDIFF_IMAGE:-$(sed -n 's/^OASDIFF_IMAGE[[:space:]]*:=[[:space:]]*\([^[:space:]]*\).*/\1/p' Makefile)}"
[ -n "$image" ] || { echo "oasdiff: OASDIFF_IMAGE not found in the Makefile" >&2; exit 1; }

spec=openapi/abertura.yaml

tag="$(git describe --tags --match 'openapi/v*' --abbrev=0 2>/dev/null || true)"
if [ -z "$tag" ]; then
  echo "oasdiff: no previous openapi/v* tag; skipping the comparison"
  exit 0
fi

tmp="$(mktemp -d "${TMPDIR:-/tmp}/oasdiff.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
git show "$tag:$spec" >"$tmp/base.yaml"
cp "$spec" "$tmp/revision.yaml"

echo "oasdiff: comparing $spec with $tag"
code=0
docker run --rm -v "$tmp":/work -w /work "$image" \
  breaking --fail-on ERR base.yaml revision.yaml || code=$?

breaking="${BREAKING_SPEC:-}"
if [ "$code" -ne 0 ] && [ -z "$breaking" ]; then
  breaking=0
  if [ -x scripts/breaking-spec.sh ] && [ -f openapi/CHANGELOG.md ]; then
    breaking="$(scripts/breaking-spec.sh openapi/CHANGELOG.md 2>/dev/null || echo 0)"
  fi
fi

if [ "$code" -ne 0 ] && [ "$breaking" = 1 ]; then
  echo "oasdiff: breaking change accepted in 0.x (BREAKING_SPEC=1 or filled 'Breaking' section under [Unreleased] in openapi/CHANGELOG.md)" >&2
  exit 0
fi
exit "$code"
