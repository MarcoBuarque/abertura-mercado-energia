#!/usr/bin/env bash
# Compara openapi/abertura.yaml com a da última tag openapi/v* e falha se a
# mudança quebra clientes (operação ou propriedade removida, tipo trocado,
# parâmetro que vira obrigatório, enum fechado que perde valor).
#
# Sem tag anterior, pula com aviso e sai 0.
# QUEBRA_SPEC=1: mostra a quebra, mas sai 0. Só para a 0.x, com a seção
# "Quebras" no CHANGELOG e o rótulo `quebra-spec` no PR.
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

if [ "$codigo" -ne 0 ] && [ "${QUEBRA_SPEC:-0}" = 1 ]; then
  echo "oasdiff: QUEBRA_SPEC=1, quebra aceita na 0.x. Registre-a na seção 'Quebras' do openapi/CHANGELOG.md" >&2
  exit 0
fi
exit "$codigo"
