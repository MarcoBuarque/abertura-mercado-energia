#!/usr/bin/env bash
# Proves scripts/oasdiff.sh in temporary repositories, outside this repository:
#   1. no previous openapi/v* tag: skips with a notice and exits 0;
#   2. with a tag and an identical spec: exits 0;
#   3. with a tag and a removed response property: exits non-zero;
#   4. the same break with BREAKING_SPEC=1: exits 0 and warns;
#   4b. without the variable, the break passes only if the Breaking section of
#       [Unreleased] in the CHANGELOG is filled; BREAKING_SPEC=0 forces strict mode;
#   5. a change that does not break (new description): exits 0.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/oasdiff-test.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
failed=0

ok()    { echo "ok: $1"; }
fail() { echo "FAIL: $1" >&2; failed=1; }

repo="$tmp/repo"
mkdir -p "$repo/scripts" "$repo/openapi"
cp "$root/scripts/oasdiff.sh" "$repo/scripts/" 2>/dev/null || true
cp "$root/Makefile" "$repo/Makefile"
cp "$root/openapi/abertura.yaml" "$repo/openapi/abertura.yaml"
git -C "$repo" init -q
git -C "$repo" add -A
git -C "$repo" -c user.name=test -c user.email=teste@example.org -c core.hooksPath=/dev/null commit -q -m base

run() { # name; output in $output, status in $code
  code=0
  output="$(cd "$repo" && env -u BREAKING_SPEC scripts/oasdiff.sh 2>&1)" || code=$?
}

# 1. no tag
run
if [ "$code" -eq 0 ] && grep -q "skipping" <<<"$output"; then ok "no tag: skips with a notice"; else fail "no tag: expected exit 0 with a notice (code $code): $output"; fi

git -C "$repo" -c user.name=test -c user.email=teste@example.org tag openapi/v0.0.1

# 2. no change
run
if [ "$code" -eq 0 ]; then ok "spec identical to the tag: passes"; else fail "identical spec should pass (code $code): $output"; fi

# 5. a change that does not break
perl -pi.bak -e 'if (!$done && /^\s{6,}description: [A-Za-z][^"\x27#|>]*$/) { chomp; $_ .= " (new text)\n"; $done = 1 }' "$repo/openapi/abertura.yaml"
if cmp -s "$root/openapi/abertura.yaml" "$repo/openapi/abertura.yaml"; then fail "the test could not change the description"; fi
rm -f "$repo/openapi/abertura.yaml.bak"
run
if [ "$code" -eq 0 ]; then ok "changed description: passes"; else fail "changed description should pass (code $code): $output"; fi
git -C "$repo" checkout -q -- openapi/abertura.yaml

# 3. breaking changes, each on a copy of the spec. The removed property is the
# required `message` of the Error schema (response); oasdiff only treats the
# removal of a required property as an error, and an optional one is a warning.
break_remove_message() {
  awk '
    /^      required: \[error, message\]$/ { print "      required: [error]"; next }
    /^        message:$/ && !done { skip = 1; done = 1; next }
    skip && /^          / { next }
    { skip = 0; print }
  ' "$root/openapi/abertura.yaml"
}
break_change_type() {
  awk '
    /^        expires_in:$/ { print; getline; sub(/integer/, "string"); print; next }
    { print }
  ' "$root/openapi/abertura.yaml"
}

for kase in remove_message:message change_type:expires_in; do
  fn="break_${kase%%:*}"; target="${kase##*:}"
  "$fn" >"$repo/openapi/abertura.yaml"
  if cmp -s "$root/openapi/abertura.yaml" "$repo/openapi/abertura.yaml"; then
    fail "$fn: the test could not change the copy"
  fi
  run
  if [ "$code" -ne 0 ] && [ "$code" -ne 127 ] && grep -q "$target" <<<"$output"; then
    ok "$fn: fails citing $target"
  else
    fail "$fn should fail citing $target (code $code): $output"
  fi
done

# 4. the 0.x exception
code=0
output="$(cd "$repo" && BREAKING_SPEC=1 scripts/oasdiff.sh 2>&1)" || code=$?
if [ "$code" -eq 0 ] && grep -qi "BREAKING_SPEC" <<<"$output"; then ok "BREAKING_SPEC=1: passes and warns"; else fail "BREAKING_SPEC=1 should pass with a notice (code $code): $output"; fi

# 4b. derivation from the CHANGELOG (the change_type break stays in the copy)
cp "$root/scripts/breaking-spec.sh" "$repo/scripts/"
printf '# Changelog\n\n## [Unreleased]\n\n### Breaking\n\nThe type of expires_in changes.\n' >"$repo/openapi/CHANGELOG.md"
run
if [ "$code" -eq 0 ]; then ok "break recorded in the CHANGELOG: passes without the variable"; else fail "a recorded break should pass (code $code): $output"; fi
code=0
output="$(cd "$repo" && BREAKING_SPEC=0 scripts/oasdiff.sh 2>&1)" || code=$?
if [ "$code" -ne 0 ]; then ok "BREAKING_SPEC=0 forces strict mode"; else fail "BREAKING_SPEC=0 should fail (code $code): $output"; fi
printf '# Changelog\n\n## [Unreleased]\n\n### Breaking\n\nNone.\n' >"$repo/openapi/CHANGELOG.md"
run
if [ "$code" -ne 0 ]; then ok "unrecorded break: fails"; else fail "an unrecorded break should fail (code $code): $output"; fi

exit "$failed"
