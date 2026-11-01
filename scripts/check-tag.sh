#!/usr/bin/env bash
# Release tag checks, the same ones the two release workflows run:
#
#   scripts/check-tag.sh openapi  openapi/vX.Y.Z[-rc.N]
#   scripts/check-tag.sh abertura abertura/vX.Y.Z[-rc.N]
#
# The script only reads files; the tag is an argument and does not need to exist.
#
# openapi:  the tag is openapi/vX.Y.Z and info.version (openapi/abertura.yaml)
#           is X.Y.Z. On a pre-release (-rc.N), info.version stays the core
#           X.Y.Z: the rc's spec is the same as the final version's.
# abertura: the tag is abertura/vX.Y.Z[-rc.N]; Version in
#           abertura/internal/version/version.go equals the tag's version (with
#           the -rc.N suffix, if any); abertura/CHANGELOG.md has the section
#           "## [version]" (on an rc, the core's section also counts) and it
#           cites an originating openapi/vX.Y.Z tag.
# A bare vX.Y.Z tag and the old go/vX.Y.Z prefix are always rejected: the
# prefix chooses the release.
#
# Exit: 0 passes, 1 fails, 2 wrong usage.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

usage() { echo "usage: scripts/check-tag.sh openapi|abertura <tag>" >&2; exit 2; }
fail() { echo "check-tag: $1" >&2; exit 1; }

[ "$#" -eq 2 ] || usage
kind="$1"; tag="$2"
case "$kind" in openapi|abertura) ;; *) usage ;; esac

semver='[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?'
if ! [[ "$tag" =~ ^${kind}/v(${semver})$ ]]; then
  fail "tag '$tag' is not in the ${kind}/vX.Y.Z or ${kind}/vX.Y.Z-rc.N format (a bare vX.Y.Z or go/v* tag is not valid)"
fi
version="${BASH_REMATCH[1]}"
core="${version%%-*}"

case "$kind" in
  openapi)
    file="$root/openapi/abertura.yaml"
    [ -f "$file" ] || fail "$file does not exist"
    # first `version:` that is a direct child of the info block (two-space indent)
    info="$(awk '/^info:/ {inside=1; next} inside && /^[^ #]/ {exit} inside && /^  version:/ {sub(/^  version:[ ]*/, ""); gsub(/["'"'"']/, ""); sub(/[ ]+#.*$/, ""); print; exit}' "$file")"
    [ -n "$info" ] || fail "info.version not found in openapi/abertura.yaml"
    if [ "$info" != "$core" ]; then
      fail "info.version is '$info', but tag $tag requires '$core'"
    fi
    echo "check-tag: ok, $tag x info.version $info"
    ;;
  abertura)
    file="$root/abertura/internal/version/version.go"
    [ -f "$file" ] || fail "$file does not exist"
    current="$(sed -n 's/^.*Version[[:space:]]*=[[:space:]]*"\([^"]*\)".*$/\1/p' "$file" | head -n1)"
    [ -n "$current" ] || fail "Version not found in abertura/internal/version/version.go"
    if [ "$current" != "$version" ]; then
      fail "Version is '$current', but tag $tag requires '$version'"
    fi

    cl="$root/abertura/CHANGELOG.md"
    [ -f "$cl" ] || fail "$cl does not exist"
    section=""
    for v in "$version" "$core"; do
      section="$(awk -v target="## [$v]" 'index($0, target) == 1 {inside=1; print; next} inside && /^## / {exit} inside {print}' "$cl")"
      [ -n "$section" ] && break
    done
    [ -n "$section" ] || fail "abertura/CHANGELOG.md has no '## [$version]' section"
    if ! grep -Eq "openapi/v${semver}" <<<"$section"; then
      fail "the $version section of abertura/CHANGELOG.md does not cite the originating openapi/vX.Y.Z tag"
    fi
    echo "check-tag: ok, $tag x Version $current x CHANGELOG"
    ;;
esac
