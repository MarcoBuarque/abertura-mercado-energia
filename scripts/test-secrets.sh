#!/usr/bin/env bash
# Proves that the secrets guard works: the three fake files, generated in a
# temporary folder outside the repository, must be blocked, and the repository
# itself must pass clean.
set -euo pipefail
GITLEAKS="${GITLEAKS:-gitleaks}"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
config="$root/.gitleaks.toml"
[ -f "$config" ] || { echo "fail: $config does not exist" >&2; exit 1; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# Everything invented on the spot; no real value.
header="$(printf '{"alg":"none","typ":"JWT"}' | base64 | tr '+/' '-_' | tr -d '=\n')"
body="$(printf '{"sub":"invented-for-test"}' | base64 | tr '+/' '-_' | tr -d '=\n')"
printf 'token=%s.%s.invented-signature\n' "$header" "$body" > "$tmp/jwt.txt"
# The names are assembled in pieces so this script does not match its own rules.
printf '%s = "%s"\n' "client_""secret" "invented-value-123456" > "$tmp/assignment.txt"
printf '{"info":{"%s":"00000000-invented"}}\n' "_postman""_id" > "$tmp/collection.json"

failed=0
for file in jwt.txt assignment.txt collection.json; do
  # Code 42 = found a secret; any other error code is a failure of the guard itself.
  code=0
  $GITLEAKS dir --config "$config" --no-banner --redact --exit-code 42 "$tmp/$file" >/dev/null 2>&1 || code=$?
  if [ "$code" -eq 42 ]; then
    echo "ok: $file blocked"
  else
    echo "FAIL: the guard did not block $file (code $code)" >&2
    failed=1
  fi
done
[ "$failed" -eq 0 ] || exit 1

if $GITLEAKS dir --config "$config" --no-banner --exit-code 42 "$root"; then
  echo "ok: repository clean"
else
  echo "FAIL: secret in the repository or guard error" >&2
  exit 1
fi
