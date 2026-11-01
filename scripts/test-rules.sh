#!/usr/bin/env bash
# Proves the project's own vacuum rules (openapi/vacuum.yaml): the minimal
# valid spec must pass, and each spec in openapi/testes/regras/<rule>.yaml must
# fail citing the rule id. A rule that never fires is a dead rule.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

# The same image as the Makefile, read from there so there is a single source.
vacuum_image="${VACUUM_IMAGE:-$(sed -n 's/^VACUUM_IMAGE[[:space:]]*:=[[:space:]]*\([^[:space:]]*\).*/\1/p' Makefile)}"
[ -n "$vacuum_image" ] || { echo "fail: VACUUM_IMAGE not found in the Makefile" >&2; exit 1; }

dir=openapi/testes/regras
rules=(
  abertura-confianca
  abertura-verificado-em
  abertura-operation-id
  abertura-cabecalhos-agente
  abertura-nao-oficial
  abertura-email-exemplo
)

lint() {
  docker run --rm -v "$root":/work -w /work "$vacuum_image" \
    lint -r openapi/vacuum.yaml -d -b -q -a --no-clip "$1" 2>&1
}

failed=0

code=0
output="$(lint "$dir/valida.yaml")" || code=$?
if [ "$code" -eq 0 ]; then
  echo "ok: valida.yaml passes"
else
  echo "FAIL: valida.yaml should pass (code $code)" >&2
  echo "$output" >&2
  failed=1
fi

for rule in "${rules[@]}"; do
  file="$dir/$rule.yaml"
  if [ ! -f "$file" ]; then
    echo "FAIL: $file does not exist" >&2
    failed=1
    continue
  fi
  code=0
  output="$(lint "$file")" || code=$?
  if [ "$code" -eq 0 ]; then
    echo "FAIL: $rule should reject $file, but it passed" >&2
    failed=1
  elif ! grep -q -- "$rule" <<<"$output"; then
    echo "FAIL: $file was rejected, but without citing $rule" >&2
    echo "$output" >&2
    failed=1
  else
    echo "ok: $rule rejects $file"
  fi
done

exit "$failed"
