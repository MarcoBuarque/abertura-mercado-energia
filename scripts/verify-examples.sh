#!/usr/bin/env bash
# Checks the spec's invented-example rules that vacuum cannot express: consumer
# unit with leading zeros, codes in the 9xxxxx range, no staging host and no
# "CCEE" outside the unofficial notice.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
spec=openapi/abertura.yaml
yq_version="${YQ_VERSION:?YQ_VERSION not set (run through make)}"
yq() { go run "github.com/mikefarah/yq/v4@${yq_version}" "$@"; }

failed=0
verify() { # description, output (empty = ok); the command status was already checked
  if [ -n "$2" ]; then
    echo "FAIL: $1" >&2
    echo "$2" >&2
    failed=1
  else
    echo "ok: $1"
  fi
}

# With set -e, a failure inside $(...) passed as an argument would be swallowed;
# so each query is stored in a variable first, and set -e stops the script.
# Scalar values only: field names also appear as schema keys.
r1="$(yq '.. | select(has("codigoUnidadeConsumidora")) | .codigoUnidadeConsumidora | select(tag == "!!str" or tag == "!!int") | tostring | select(test("^00000") | not)' "$spec")"
verify "every example consumer unit starts with 00000" "$r1"

r2="$(yq '.. | select(tag == "!!map") | to_entries | .[] | select(.key | test("^(codigoAgente|codigoPerfil|codigoConcessionaria|codigoVarejista|codigoSupridora)")) | .value | select(tag == "!!str" or tag == "!!int") | tostring | select(test("^9") | not)' "$spec")"
verify "example agent, profile and distributor codes start with 9" "$r2"

r3="$(yq '.components.parameters | to_entries | .[] | select(.value.name == "codigoAgente" or .value.name == "codigoPerfil" or .value.name == "codigoPerfilVarejista" or .value.name == "codigoAgenteConcessionaria") | .value.example | tostring | select(test("^9") | not)' "$spec")"
verify "agent and profile parameter examples start with 9" "$r3"

r4="$(grep -n 'hom-migra' "$spec" || true)"
verify "no staging host" "$r4"

# The unofficial notice (info.description) is the only place with "CCEE".
r5="$(yq 'del(.info.description)' "$spec" | grep -n 'CCEE' || true)"
verify "\"CCEE\" only in info.description" "$r5"

exit "$failed"
