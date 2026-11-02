#!/usr/bin/env bash
# Proves that scripts/verify-errors.sh fails on: missing id, missing banner,
# missing viewport, undocumented spec code and missing HTML; and passes the
# complete case. Uses no network and no docker.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cat > "$tmp/errors.md" <<'MD'
# Catalog

<a id="one-slug"></a>
## One

Code `ERR_ONE`.

<a id="other-slug"></a>
## Other
MD
cat > "$tmp/spec.yaml" <<'YAML'
example: ERR_ONE
YAML
cat > "$tmp/spec-extra.yaml" <<'YAML'
example: ERR_ONE
other: ERR_UNDOCUMENTED
YAML
head='<meta name="viewport" content="width=device-width">'
banner='unofficial description'

case_() { # name expected html [spec]
  local name="$1" expected="$2" html="$3" spec="${4:-$tmp/spec.yaml}" rc=0
  printf '%s\n' "$html" > "$tmp/$name.html"
  "$root/scripts/verify-errors.sh" "$tmp/$name.html" "$tmp/errors.md" "$spec" >/dev/null 2>&1 || rc=$?
  if [ "$expected" = ok ] && [ "$rc" -ne 0 ]; then echo "test-verify-errors: $name should pass" >&2; exit 1; fi
  if [ "$expected" = fail ] && [ "$rc" -eq 0 ]; then echo "test-verify-errors: $name should fail" >&2; exit 1; fi
}

case_ complete ok "$head $banner <a id=\"one-slug\"></a><a id=\"other-slug\"></a>"
case_ missing-id fail "$head $banner <a id=\"one-slug\"></a>"
case_ missing-banner fail "$head <a id=\"one-slug\"></a><a id=\"other-slug\"></a>"
case_ missing-viewport fail "$banner <a id=\"one-slug\"></a><a id=\"other-slug\"></a>"
case_ undocumented-code fail "$head $banner <a id=\"one-slug\"></a><a id=\"other-slug\"></a>" "$tmp/spec-extra.yaml"
if "$root/scripts/verify-errors.sh" "$tmp/does-not-exist.html" "$tmp/errors.md" "$tmp/spec.yaml" >/dev/null 2>&1; then
  echo "test-verify-errors: a missing HTML should fail" >&2; exit 1
fi
echo "test-verify-errors: ok"
