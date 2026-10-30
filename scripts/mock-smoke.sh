#!/usr/bin/env bash
# Sobe o mock (Prism) em contêiner e chama as 10 operações da spec com curl,
# conferindo a resposta com jq. Só fala com o mock local: nenhuma requisição
# sai para o operador. Ao fim, derruba o mock (MOCK_SMOKE_MANTER=1 mantém).
set -euo pipefail

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$raiz"

compose=(docker compose -f mock/compose.yaml)
base="${MOCK_URL:-http://localhost:4010}"
falhas=0

# Uma fonte só para o digest do Prism: o Makefile. O Dockerfile repete o valor
# (para `docker compose up` funcionar sem make) e este teste confere os dois.
prism_make="$(sed -n 's/^PRISM_IMAGE[[:space:]]*:=[[:space:]]*\([^[:space:]]*\).*/\1/p' Makefile)"
prism_docker="$(sed -n 's/^ARG PRISM_IMAGE=\(.*\)$/\1/p' mock/Dockerfile 2>/dev/null || true)"
versao_spec="$(sed -n 's/^  version:[[:space:]]*\([0-9][^[:space:]]*\).*/\1/p' openapi/abertura.yaml | head -1)"
versao_label="$(sed -n 's/.*spec-version="\([^"]*\)".*/\1/p' mock/Dockerfile 2>/dev/null || true)"

ok()    { echo "ok:    $1"; }
falha() { echo "FALHA: $1" >&2; falhas=$((falhas + 1)); }

confere() { # descrição, valor esperado, valor obtido
  if [ "$2" = "$3" ]; then ok "$1"; else falha "$1 (esperado '$2', veio '$3')"; fi
}

if [ -n "$prism_make" ] && [ "$prism_make" = "$prism_docker" ]; then
  ok "digest do Prism igual no Makefile e no Dockerfile"
else
  falha "digest do Prism difere: Makefile='$prism_make' Dockerfile='$prism_docker'"
fi
confere "label spec-version igual a info.version" "$versao_spec" "$versao_label"

if [ "${MOCK_SMOKE_SEM_COMPOSE:-0}" != 1 ]; then
  "${compose[@]}" up -d --build --wait --wait-timeout 180 >/dev/null \
    || { echo "FALHA: o mock não subiu" >&2; "${compose[@]}" logs --tail 40 >&2 || true; exit 1; }
  if [ "${MOCK_SMOKE_MANTER:-0}" != 1 ]; then
    trap '"${compose[@]}" down >/dev/null 2>&1 || true' EXIT
  fi
fi

for _ in $(seq 1 60); do
  curl -s -o /dev/null "$base/" && break
  sleep 1
done


# req <arquivo-de-saída> <curl args...>: devolve o status em $status e o corpo em $corpo.
req() {
  local saida
  saida="$(curl -s -w '\n%{http_code}' "$@")"
  status="${saida##*$'\n'}"
  corpo="${saida%$'\n'*}"
}

jqc() { jq -r "$1" <<<"$corpo"; }

# 1. obterToken
req -X POST "$base/sso/oauth/token" \
  -d client_id=id-inventado -d client_secret=segredo-inventado -d grant_type=client_credentials
confere "obterToken: status" 200 "$status"
confere "obterToken: expires_in" 300 "$(jqc '.expires_in')"
token="$(jqc '.access_token')"
# O Prism só confere que o cabeçalho existe; o valor vem do próprio mock.
cabecalho_auth="Authorization: Bearer ${token}"
agente=(-H "codigoAgente: 900002" -H "codigoPerfil: 900003" -H "$cabecalho_auth")

# 2. listarMigracoes
lista="$base/v1.1/varejista/migracoes?codigoPerfilVarejista=900003"
req "${agente[@]}" "$lista"
confere "listarMigracoes: status" 200 "$status"
confere "listarMigracoes: chave migracao" true "$(jqc 'has("migracao")')"
confere "listarMigracoes: cursor da página 1" cursor-inventado-2 "$(jqc '.indexProximaPagina')"

req "${agente[@]}" -H "Prefer: example=pagina-2" "$lista"
confere "listarMigracoes pagina-2: indexProximaPagina nulo" null "$(jqc '.indexProximaPagina')"

req "${agente[@]}" -H "Prefer: code=403" "$lista"
confere "listarMigracoes 403: status" 403 "$status"
confere "listarMigracoes 403: error" ERR_CREDENCIAL_INVALIDA "$(jqc '.error')"

# 3. criarMigracao
nova='{"codigoAgenteConcessionaria":900001,"codigoAgenteVarejista":900002,"codigoPerfilVarejista":900003,"codigoUnidadeConsumidora":"000000000000001","dataDenuncia":"2027-01-15","emailUnidadeConsumidora":"contato@example.org","mesReferencia":"2027-03","tipoDocumento":"CNPJ","numeroDocumento":"11111111111111"}'
req "${agente[@]}" -H "Content-Type: application/json" -X POST -d "$nova" "$base/v1.1/varejista/migracoes"
confere "criarMigracao: status" 200 "$status"
req "${agente[@]}" -H "Content-Type: application/json" -H "Prefer: code=400, example=migracao-solicitada" \
  -X POST -d "$nova" "$base/v1.1/varejista/migracoes"
confere "criarMigracao 400: status" 400 "$status"
confere "criarMigracao 400: error" ERR_MIGRACAO_SOLICITADA "$(jqc '.error')"

# 4. obterMigracao
idm=00000000-0000-4000-8000-000000000001
req "${agente[@]}" "$base/v1.1/varejista/migracoes/$idm"
confere "obterMigracao: status" 200 "$status"
confere "obterMigracao: é array" array "$(jqc 'type')"

# 5. editarMigracao
req "${agente[@]}" -H "Content-Type: application/json" -X PUT \
  -d '{"mesReferencia":"2027-04","codigoPerfilVarejista":900003,"emailUnidadeConsumidora":"novo@example.org"}' \
  "$base/v1/varejista/migracoes/$idm"
confere "editarMigracao: status" 200 "$status"

# 6. listarMedicoes
req "${agente[@]}" "$base/v1/varejista/consumo/medicoes?codigoUnidadeConsumidora=000000000000001&codigoAgenteConcessionaria=900001&dataInicio=2027-02-01T00:00:00-03:00&dataFim=2027-02-28T23:45:00-03:00"
confere "listarMedicoes: status" 200 "$status"
confere "listarMedicoes: chave medicoes" true "$(jqc 'has("medicoes")')"

# 7. listarSolicitacoes
req "${agente[@]}" "$base/v1/solicitacoes?tipoSolicitacao=SUSPENSAO_FORNECIMENTO_RESILICAO"
confere "listarSolicitacoes: status" 200 "$status"
confere "listarSolicitacoes: chave solicitacao" true "$(jqc 'has("solicitacao")')"

# 8. solicitarTrocaRepresentante
req "${agente[@]}" -H "Content-Type: application/json" -X POST \
  -d '{"codigoUnidadeConsumidora":"000000000000001","codigoConcessionaria":900001,"codigoVarejistaNovo":900004,"codigoPerfilVarejistaNovo":900005,"mesReferencia":"2027-05","tipoSolicitacao":"ALTERACAO_ALFA_VAREJISTA_RESILICAO"}' \
  "$base/v1/solicitacoes/representante"
confere "solicitarTrocaRepresentante: status" 200 "$status"

# 9. solicitarSuspensaoFornecimento
req "${agente[@]}" -H "Content-Type: application/json" -X POST \
  -d '{"codigoUnidadeConsumidora":"000000000000001","codigoConcessionaria":900001,"tipoSolicitacao":"SUSPENSAO_FORNECIMENTO_RESILICAO","dataNotificacao":"2027-02-01"}' \
  "$base/v1/solicitacoes/suspensao-fornecimento-varejista"
confere "solicitarSuspensaoFornecimento: status" 200 "$status"

# 10. decidirSolicitacao
req "${agente[@]}" -H "Content-Type: application/json" -X POST \
  -d '{"situacaoAlteracao":"CONCLUIDA","descricaoJustificativa":"Inventado: aprovada","tipoSolicitacao":"SUSPENSAO_FORNECIMENTO_RESILICAO"}' \
  "$base/v1/solicitacoes/00000000-0000-4000-8000-0000000000b1"
confere "decidirSolicitacao: status" 200 "$status"

# Validação do próprio Prism. A spec declara 400, então o Prism responde 400
# com o primeiro exemplo de 400 da operação (corpo genérico, que NÃO descreve o
# problema real) e põe as violações no cabeçalho `sl-violations`. Sem token,
# responde 401 application/problem+json com `WWW-Authenticate: OAuth2` (a API
# real devolveria 403). O guia (docs/clientes.md) explica os dois casos.
cab="$(curl -s -o /dev/null -D - -H "codigoPerfil: 900003" -H "$cabecalho_auth" "$lista")"
confere "sem codigoAgente: status do Prism" 400 "$(head -1 <<<"$cab" | awk '{print $2}')"
if grep -qi '^sl-violations:.*codigoagente' <<<"$cab"; then
  ok "sem codigoAgente: violação listada em sl-violations"
else
  falha "sem codigoAgente: sl-violations não cita codigoagente"
fi
cab="$(curl -s -o /dev/null -D - -H "codigoAgente: 900002" -H "codigoPerfil: 900003" "$lista")"
confere "sem token: status do Prism" 401 "$(head -1 <<<"$cab" | awk '{print $2}')"
grep -qi '^content-type: application/problem+json' <<<"$cab" \
  && ok "sem token: problem+json" || falha "sem token: esperava application/problem+json"

if [ "$falhas" -gt 0 ]; then
  echo "mock-smoke: $falhas falha(s)" >&2
  exit 1
fi
echo "mock-smoke: as 10 operações passaram"
