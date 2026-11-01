#!/usr/bin/env bash
# Starts the mock (Prism) in a container and calls the spec's 10 operations
# with curl, checking the response with jq. It only talks to the local mock: no
# request goes to the operator. At the end, it stops the mock
# (MOCK_SMOKE_KEEP=1 keeps it).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

compose=(docker compose -f mock/compose.yaml)
base="${MOCK_URL:-http://localhost:4010}"
failures=0

# A single source for the Prism digest: the Makefile. The Dockerfile repeats the
# value (so `docker compose up` works without make) and this test checks both.
prism_make="$(sed -n 's/^PRISM_IMAGE[[:space:]]*:=[[:space:]]*\([^[:space:]]*\).*/\1/p' Makefile)"
prism_docker="$(sed -n 's/^ARG PRISM_IMAGE=\(.*\)$/\1/p' mock/Dockerfile 2>/dev/null || true)"
spec_version="$(sed -n 's/^  version:[[:space:]]*\([0-9][^[:space:]]*\).*/\1/p' openapi/abertura.yaml | head -1)"

ok()    { echo "ok:    $1"; }
fail() { echo "FAIL: $1" >&2; failures=$((failures + 1)); }

verify() { # description, expected value, obtained value
  if [ "$2" = "$3" ]; then ok "$1"; else fail "$1 (expected '$2', got '$3')"; fi
}

if [ -n "$prism_make" ] && [ "$prism_make" = "$prism_docker" ]; then
  ok "Prism digest equal in the Makefile and the Dockerfile"
else
  fail "Prism digest differs: Makefile='$prism_make' Dockerfile='$prism_docker'"
fi
[ -n "$spec_version" ] || fail "info.version not found in openapi/abertura.yaml"
# The compose file reads SPEC_VERSION from the environment (the Makefile also exports it).
export SPEC_VERSION="$spec_version"

if [ "${MOCK_SMOKE_NO_COMPOSE:-0}" != 1 ]; then
  "${compose[@]}" up -d --build --wait --wait-timeout 180 >/dev/null \
    || { echo "FAIL: the mock did not start" >&2; "${compose[@]}" logs --tail 40 >&2 || true; exit 1; }
  if [ "${MOCK_SMOKE_KEEP:-0}" != 1 ]; then
    trap '"${compose[@]}" down >/dev/null 2>&1 || true' EXIT
  fi
fi

if [ "${MOCK_SMOKE_NO_COMPOSE:-0}" != 1 ]; then
  # The built image's label comes from the ARG and must match info.version.
  version_label="$(docker image inspect --format '{{ index .Config.Labels "io.github.abertura.spec-version" }}' abertura-mercado-energia-mock:local 2>/dev/null || true)"
  verify "image spec-version label equal to info.version" "$spec_version" "$version_label"
  source_label="$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.source" }}' abertura-mercado-energia-mock:local 2>/dev/null || true)"
  verify "image.source label of the image" "https://github.com/MarcoBuarque/abertura-mercado-energia" "$source_label"
fi

for _ in $(seq 1 60); do
  curl -s -o /dev/null "$base/" && break
  sleep 1
done


# req <curl args...>: returns the status in $status and the body in $body.
req() {
  local out
  out="$(curl -s -w '\n%{http_code}' "$@")"
  status="${out##*$'\n'}"
  body="${out%$'\n'*}"
}

jqc() { jq -r "$1" <<<"$body"; }

# 1. getToken
req -X POST "$base/sso/oauth/token" \
  -d client_id=id-inventado -d client_secret=segredo-inventado -d grant_type=client_credentials
verify "getToken: status" 200 "$status"
verify "getToken: expires_in" 300 "$(jqc '.expires_in')"
token="$(jqc '.access_token')"
# Prism only checks that the header exists; the value comes from the mock itself.
auth_header="Authorization: Bearer ${token}"
agent=(-H "codigoAgente: 900002" -H "codigoPerfil: 900003" -H "$auth_header")

# 2. listMigrations
list="$base/v1.1/varejista/migracoes?codigoPerfilVarejista=900003&mesReferenciaInicial=2027-01&mesReferenciaFinal=2027-03"
req "${agent[@]}" "$list"
verify "listMigrations: status" 200 "$status"
verify "listMigrations: key migracao" true "$(jqc 'has("migracao")')"
verify "listMigrations: cursor of page 1" cursor-inventado-2 "$(jqc '.indexProximaPagina')"

req "${agent[@]}" -H "Prefer: example=pagina-2" "$list"
verify "listMigrations pagina-2: indexProximaPagina null" null "$(jqc '.indexProximaPagina')"

req "${agent[@]}" -H "Prefer: code=403" "$list"
verify "listMigrations 403: status" 403 "$status"
verify "listMigrations 403: error" ERR_CREDENCIAL_INVALIDA "$(jqc '.error')"

# 3. createMigration
new_migration='{"codigoAgenteConcessionaria":900001,"codigoAgenteVarejista":900002,"codigoPerfilVarejista":900003,"codigoUnidadeConsumidora":"000000000000001","dataDenuncia":"2027-01-15","emailUnidadeConsumidora":"contato@example.org","mesReferencia":"2027-03","tipoDocumento":"CNPJ","numeroDocumento":"11111111111111"}'
req "${agent[@]}" -H "Content-Type: application/json" -X POST -d "$new_migration" "$base/v1.1/varejista/migracoes"
verify "createMigration: status" 200 "$status"
req "${agent[@]}" -H "Content-Type: application/json" -H "Prefer: code=400, example=migracao-solicitada" \
  -X POST -d "$new_migration" "$base/v1.1/varejista/migracoes"
verify "createMigration 400: status" 400 "$status"
verify "createMigration 400: error" ERR_MIGRACAO_SOLICITADA "$(jqc '.error')"

# 4. getMigration
migration_id=00000000-0000-4000-8000-000000000001
req "${agent[@]}" "$base/v1.1/varejista/migracoes/$migration_id"
verify "getMigration: status" 200 "$status"
verify "getMigration: is an array" array "$(jqc 'type')"

# 5. editMigration
req "${agent[@]}" -H "Content-Type: application/json" -X PUT \
  -d '{"mesReferencia":"2027-04","codigoPerfilVarejista":900003,"emailUnidadeConsumidora":"novo@example.org"}' \
  "$base/v1/varejista/migracoes/$migration_id"
verify "editMigration: status" 200 "$status"

# 6. listMeasurements
req "${agent[@]}" "$base/v1/varejista/consumo/medicoes?codigoUnidadeConsumidora=000000000000001&codigoAgenteConcessionaria=900001&dataInicio=2027-02-01T00:00:00-03:00&dataFim=2027-02-28T23:45:00-03:00"
verify "listMeasurements: status" 200 "$status"
verify "listMeasurements: key medicoes" true "$(jqc 'has("medicoes")')"

# 7. listChangeRequests
req "${agent[@]}" "$base/v1/solicitacoes?tipoSolicitacao=SUSPENSAO_FORNECIMENTO_RESILICAO"
verify "listChangeRequests: status" 200 "$status"
verify "listChangeRequests: key solicitacao" true "$(jqc 'has("solicitacao")')"

# 8. requestRepresentativeChange
req "${agent[@]}" -H "Content-Type: application/json" -X POST \
  -d '{"codigoUnidadeConsumidora":"000000000000001","codigoConcessionaria":900001,"codigoVarejistaNovo":900004,"codigoPerfilVarejistaNovo":900005,"mesReferencia":"2027-05","tipoSolicitacao":"ALTERACAO_ALFA_VAREJISTA_RESILICAO"}' \
  "$base/v1/solicitacoes/representante"
verify "requestRepresentativeChange: status" 200 "$status"

# 9. requestSupplySuspension
req "${agent[@]}" -H "Content-Type: application/json" -X POST \
  -d '{"codigoUnidadeConsumidora":"000000000000001","codigoConcessionaria":900001,"tipoSolicitacao":"SUSPENSAO_FORNECIMENTO_RESILICAO","dataNotificacao":"2027-02-01"}' \
  "$base/v1/solicitacoes/suspensao-fornecimento-varejista"
verify "requestSupplySuspension: status" 200 "$status"

# 10. decideChangeRequest
req "${agent[@]}" -H "Content-Type: application/json" -X POST \
  -d '{"situacaoAlteracao":"CONCLUIDA","descricaoJustificativa":"Inventado: aprovada","tipoSolicitacao":"SUSPENSAO_FORNECIMENTO_RESILICAO"}' \
  "$base/v1/solicitacoes/00000000-0000-4000-8000-0000000000b1"
verify "decideChangeRequest: status" 200 "$status"

# Prism's own validation. The spec declares 400, so Prism answers 400 with the
# operation's first 400 example (a generic body that does NOT describe the real
# problem) and puts the violations in the `sl-violations` header. Without a
# token, it answers 401 application/problem+json with `WWW-Authenticate: OAuth2`
# (the real API would return 403). The guide (docs/clients.md) explains both cases.
hdr="$(curl -s -o /dev/null -D - -H "codigoPerfil: 900003" -H "$auth_header" "$list")"
verify "without codigoAgente: Prism status" 400 "$(head -1 <<<"$hdr" | awk '{print $2}')"
if grep -qi '^sl-violations:.*codigoagente' <<<"$hdr"; then
  ok "without codigoAgente: violation listed in sl-violations"
else
  fail "without codigoAgente: sl-violations does not mention codigoagente"
fi
hdr="$(curl -s -o /dev/null -D - -H "codigoAgente: 900002" -H "codigoPerfil: 900003" "$list")"
verify "without token: Prism status" 401 "$(head -1 <<<"$hdr" | awk '{print $2}')"
grep -qi '^content-type: application/problem+json' <<<"$hdr" \
  && ok "without token: problem+json" || fail "without token: expected application/problem+json"

if [ "$failures" -gt 0 ]; then
  echo "mock-smoke: $failures failure(s)" >&2
  exit 1
fi
echo "mock-smoke: the 10 operations passed"
