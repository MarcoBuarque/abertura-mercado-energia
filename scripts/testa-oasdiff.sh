#!/usr/bin/env bash
# Prova scripts/oasdiff.sh em repositórios temporários, fora deste repositório:
#   1. sem tag openapi/v* anterior: pula com aviso e sai 0;
#   2. com tag e spec igual: sai 0;
#   3. com tag e uma propriedade de resposta removida: sai diferente de 0;
#   4. a mesma quebra com QUEBRA_SPEC=1: sai 0 e avisa;
#   5. mudança que não quebra (descrição nova): sai 0.
set -euo pipefail

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/oasdiff-teste.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
falhou=0

ok()    { echo "ok: $1"; }
falha() { echo "FALHA: $1" >&2; falhou=1; }

repo="$tmp/repo"
mkdir -p "$repo/scripts" "$repo/openapi"
cp "$raiz/scripts/oasdiff.sh" "$repo/scripts/" 2>/dev/null || true
cp "$raiz/Makefile" "$repo/Makefile"
cp "$raiz/openapi/abertura.yaml" "$repo/openapi/abertura.yaml"
git -C "$repo" init -q
git -C "$repo" add -A
git -C "$repo" -c user.name=teste -c user.email=teste@example.org -c core.hooksPath=/dev/null commit -q -m base

roda() { # nome; saída em $saida, status em $codigo
  codigo=0
  saida="$(cd "$repo" && env -u QUEBRA_SPEC scripts/oasdiff.sh 2>&1)" || codigo=$?
}

# 1. sem tag
roda
if [ "$codigo" -eq 0 ] && grep -q "pulando" <<<"$saida"; then ok "sem tag: pula com aviso"; else falha "sem tag: esperava sair 0 com aviso (código $codigo): $saida"; fi

git -C "$repo" -c user.name=teste -c user.email=teste@example.org tag openapi/v0.0.1

# 2. sem mudança
roda
if [ "$codigo" -eq 0 ]; then ok "spec igual à tag: passa"; else falha "spec igual devia passar (código $codigo): $saida"; fi

# 5. mudança que não quebra
sed -i.bak 's/^  title: .*/&  /' "$repo/openapi/abertura.yaml"
sed -i.bak 's/Validade em segundos\./Validade em segundos (texto novo)./' "$repo/openapi/abertura.yaml"
rm -f "$repo/openapi/abertura.yaml.bak"
roda
if [ "$codigo" -eq 0 ]; then ok "descrição alterada: passa"; else falha "descrição alterada devia passar (código $codigo): $saida"; fi
git -C "$repo" checkout -q -- openapi/abertura.yaml

# 3. quebras, cada uma numa cópia da spec. A propriedade removida é a
# obrigatória `message` do schema Erro (resposta); o oasdiff só trata como
# erro a remoção de propriedade obrigatória, e a opcional vira aviso.
quebra_remove_message() {
  awk '
    /^      required: \[error, message\]$/ { print "      required: [error]"; next }
    /^        message:$/ && !feito { pula = 1; feito = 1; next }
    pula && /^          / { next }
    { pula = 0; print }
  ' "$raiz/openapi/abertura.yaml"
}
quebra_troca_tipo() {
  awk '
    /^        expires_in:$/ { print; getline; sub(/integer/, "string"); print; next }
    { print }
  ' "$raiz/openapi/abertura.yaml"
}

for caso in remove_message:message troca_tipo:expires_in; do
  fn="quebra_${caso%%:*}"; alvo="${caso##*:}"
  "$fn" >"$repo/openapi/abertura.yaml"
  if cmp -s "$raiz/openapi/abertura.yaml" "$repo/openapi/abertura.yaml"; then
    falha "$fn: o teste não conseguiu alterar a cópia"
  fi
  roda
  if [ "$codigo" -ne 0 ] && [ "$codigo" -ne 127 ] && grep -q "$alvo" <<<"$saida"; then
    ok "$fn: reprova citando $alvo"
  else
    falha "$fn devia reprovar citando $alvo (código $codigo): $saida"
  fi
done

# 4. exceção da 0.x
codigo=0
saida="$(cd "$repo" && QUEBRA_SPEC=1 scripts/oasdiff.sh 2>&1)" || codigo=$?
if [ "$codigo" -eq 0 ] && grep -qi "QUEBRA_SPEC" <<<"$saida"; then ok "QUEBRA_SPEC=1: passa e avisa"; else falha "QUEBRA_SPEC=1 devia passar com aviso (código $codigo): $saida"; fi

exit "$falhou"
