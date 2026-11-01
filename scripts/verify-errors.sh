#!/usr/bin/env bash
# Checks the published error catalogue page against its sources:
#   - every `ERR_*` code declared in openapi/abertura.yaml appears in
#     docs/errors.md;
#   - every entry of docs/errors.md (the `<a id="slug"></a>` anchors, which are
#     the fragments of Problem.Type) has that id in the generated HTML, since
#     without the id the link lands at the top of the page;
#   - the "unofficial description" banner and the viewport meta are present.
#
# Usage: scripts/verify-errors.sh [html] [errors.md] [spec]
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
html="${1:-$root/out/docs/errors/index.html}"
source_md="${2:-$root/docs/errors.md}"
spec="${3:-$root/openapi/abertura.yaml}"

[ -f "$html" ] || { echo "verify-errors: $html does not exist (run make docs)" >&2; exit 1; }
[ -f "$source_md" ] || { echo "verify-errors: $source_md does not exist" >&2; exit 1; }
[ -f "$spec" ] || { echo "verify-errors: $spec does not exist" >&2; exit 1; }

slugs="$(sed -n 's/^<a id="\([a-z0-9-]*\)"><\/a>$/\1/p' "$source_md")"
[ -n "$slugs" ] || { echo "verify-errors: no <a id> anchor found in $source_md" >&2; exit 1; }

failed=0
total=0

# 1. Spec error codes documented in the catalogue.
codes="$(grep -oE 'ERR_[A-Z0-9_]+' "$spec" | sort -u || true)"
for c in $codes; do
  if ! grep -q "$c" "$source_md"; then
    echo "verify-errors: code $c from the spec is not in $source_md" >&2
    failed=1
  fi
done

# 2. Every catalogue anchor exists as an id in the HTML.
for s in $slugs; do
  total=$((total + 1))
  if ! grep -Eq "id=\"$s\"" "$html"; then
    echo "verify-errors: slug $s does not exist as an id in $html" >&2
    failed=1
  fi
done

grep -q "unofficial description" "$html" || { echo "verify-errors: missing the 'unofficial description' banner in $html" >&2; failed=1; }
grep -q 'name="viewport"' "$html" || { echo "verify-errors: missing the viewport meta in $html" >&2; failed=1; }

[ "$failed" -eq 0 ] || exit 1
echo "verify-errors: ok ($total slugs with an id in ${html#"$root"/}, $(wc -w <<<"$codes" | tr -d ' ') spec codes documented)"
