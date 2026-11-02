#!/usr/bin/env bash
# The runtime of the abertura package (abertura/) may only depend on the
# standard library, golang.org/x/oauth2 and golang.org/x/time. kin-openapi and
# the rest live in tests, in the testserver and in the contract tests, which
# `go list -deps .` does not reach.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root/abertura"

module="$(go list -m)"
allowed=("$module" "golang.org/x/oauth2" "golang.org/x/time")

# Packages outside the standard library that end up in the user's binary.
total=0
failed=0
while IFS= read -r dep; do
  [ -n "$dep" ] || continue
  total=$((total + 1))
  ok=0
  for p in "${allowed[@]}"; do
    if [ "$dep" = "$p" ] || [[ "$dep" == "$p"/* ]]; then ok=1; break; fi
  done
  if [ "$ok" -eq 0 ]; then
    echo "fail: runtime dependency not allowed: $dep" >&2
    failed=1
  fi
done < <(go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' . | sort)

if [ "$failed" -ne 0 ]; then
  echo "The abertura package may only import the standard library, golang.org/x/oauth2 and golang.org/x/time." >&2
  exit 1
fi
echo "ok: the abertura package runtime uses only the standard library, x/oauth2 and x/time ($total packages outside the stdlib, all allowed)"
