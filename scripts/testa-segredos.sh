#!/usr/bin/env bash
# Prova que a guarda de segredos funciona: os três arquivos falsos, gerados
# numa pasta temporária fora do repositório, têm de ser barrados, e o
# repositório em si tem de passar limpo.
set -euo pipefail
GITLEAKS="${GITLEAKS:-gitleaks}"

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
config="$raiz/.gitleaks.toml"
[ -f "$config" ] || { echo "falha: $config não existe" >&2; exit 1; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# Tudo inventado na hora; nenhum valor real.
cabecalho="$(printf '{"alg":"none","typ":"JWT"}' | base64 | tr '+/' '-_' | tr -d '=\n')"
corpo="$(printf '{"sub":"inventado-para-teste"}' | base64 | tr '+/' '-_' | tr -d '=\n')"
printf 'token=%s.%s.assinatura-inventada\n' "$cabecalho" "$corpo" > "$tmp/jwt.txt"
# Os nomes são montados em pedaços para este script não casar com as próprias regras.
printf '%s = "%s"\n' "client_""secret" "valor-inventado-123456" > "$tmp/atribuicao.txt"
printf '{"info":{"%s":"00000000-inventado"}}\n' "_postman""_id" > "$tmp/colecao.json"

falhou=0
for arquivo in jwt.txt atribuicao.txt colecao.json; do
  # Código 42 = achou segredo; qualquer outro código de erro é falha da própria guarda.
  codigo=0
  $GITLEAKS dir --config "$config" --no-banner --redact --exit-code 42 "$tmp/$arquivo" >/dev/null 2>&1 || codigo=$?
  if [ "$codigo" -eq 42 ]; then
    echo "ok: $arquivo barrado"
  else
    echo "FALHA: a guarda não barrou $arquivo (código $codigo)" >&2
    falhou=1
  fi
done
[ "$falhou" -eq 0 ] || exit 1

if $GITLEAKS dir --config "$config" --no-banner --exit-code 42 "$raiz"; then
  echo "ok: repositório limpo"
else
  echo "FALHA: segredo no repositório ou erro da guarda" >&2
  exit 1
fi
