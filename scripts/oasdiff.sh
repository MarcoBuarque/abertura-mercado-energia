#!/usr/bin/env bash
# Compara openapi/abertura.yaml com a da última tag openapi/v* e falha se a
# mudança quebra clientes (operação ou propriedade removida, tipo trocado,
# parâmetro que vira obrigatório, enum fechado que perde valor).
#
# Sem tag anterior, pula com aviso e sai 0.
# Quebra aceita (só na 0.x): a seção "### Quebras" de "## [Não lançado]" em
# openapi/CHANGELOG.md tem conteúdo (scripts/quebra-spec.sh). Com ela, a quebra
# aparece como aviso e o script sai 0; sem ela, falha.
# QUEBRA_SPEC=1 força a aceitação (o CI põe quando o PR leva o rótulo
# `quebra-spec`, depois de conferir a seção); QUEBRA_SPEC=0 força o modo
# estrito; sem a variável, vale o que o CHANGELOG registra (CHANGELOG ou
# scripts/quebra-spec.sh ausentes contam como 0).
set -euo pipefail

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$raiz"

imagem="${OASDIFF_IMAGE:-$(sed -n 's/^OASDIFF_IMAGE[[:space:]]*:=[[:space:]]*\([^[:space:]]*\).*/\1/p' Makefile)}"
[ -n "$imagem" ] || { echo "oasdiff: OASDIFF_IMAGE não encontrada no Makefile" >&2; exit 1; }

spec=openapi/abertura.yaml

tag="$(git describe --tags --match 'openapi/v*' --abbrev=0 2>/dev/null || true)"
if [ -z "$tag" ]; then
  echo "oasdiff: sem tag openapi/v* anterior; pulando a comparação"
  exit 0
fi

tmp="$(mktemp -d "${TMPDIR:-/tmp}/oasdiff.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
git show "$tag:$spec" >"$tmp/base.yaml"
cp "$spec" "$tmp/revisao.yaml"

echo "oasdiff: comparando $spec com $tag"
codigo=0
docker run --rm -v "$tmp":/work -w /work "$imagem" \
  breaking --fail-on ERR base.yaml revisao.yaml || codigo=$?

quebra="${QUEBRA_SPEC:-}"
if [ "$codigo" -ne 0 ] && [ -z "$quebra" ]; then
  quebra=0
  if [ -x scripts/quebra-spec.sh ] && [ -f openapi/CHANGELOG.md ]; then
    quebra="$(scripts/quebra-spec.sh openapi/CHANGELOG.md 2>/dev/null || echo 0)"
  fi
fi

if [ "$codigo" -ne 0 ] && [ "$quebra" = 1 ]; then
  echo "oasdiff: quebra aceita na 0.x (QUEBRA_SPEC=1 ou seção 'Quebras' de [Não lançado] preenchida em openapi/CHANGELOG.md)" >&2
  exit 0
fi
exit "$codigo"
