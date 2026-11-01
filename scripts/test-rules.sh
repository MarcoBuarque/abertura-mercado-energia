#!/usr/bin/env bash
# Prova as regras próprias do vacuum (openapi/vacuum.yaml): a spec mínima
# válida tem de passar, e cada spec em openapi/testes/regras/<regra>.yaml tem
# de falhar citando o id da regra. Uma regra que nunca dispara é regra morta.
set -euo pipefail

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$raiz"

# A mesma imagem do Makefile, lida de lá para haver uma fonte só.
vacuum_image="${VACUUM_IMAGE:-$(sed -n 's/^VACUUM_IMAGE[[:space:]]*:=[[:space:]]*\([^[:space:]]*\).*/\1/p' Makefile)}"
[ -n "$vacuum_image" ] || { echo "falha: VACUUM_IMAGE não encontrada no Makefile" >&2; exit 1; }

dir=openapi/testes/regras
regras=(
  abertura-confianca
  abertura-verificado-em
  abertura-operation-id
  abertura-cabecalhos-agente
  abertura-nao-oficial
  abertura-email-exemplo
)

lint() {
  docker run --rm -v "$raiz":/work -w /work "$vacuum_image" \
    lint -r openapi/vacuum.yaml -d -b -q -a --no-clip "$1" 2>&1
}

falhou=0

codigo=0
saida="$(lint "$dir/valida.yaml")" || codigo=$?
if [ "$codigo" -eq 0 ]; then
  echo "ok: valida.yaml passa"
else
  echo "FALHA: valida.yaml devia passar (código $codigo)" >&2
  echo "$saida" >&2
  falhou=1
fi

for regra in "${regras[@]}"; do
  arquivo="$dir/$regra.yaml"
  if [ ! -f "$arquivo" ]; then
    echo "FALHA: $arquivo não existe" >&2
    falhou=1
    continue
  fi
  codigo=0
  saida="$(lint "$arquivo")" || codigo=$?
  if [ "$codigo" -eq 0 ]; then
    echo "FALHA: $regra devia reprovar $arquivo, mas passou" >&2
    falhou=1
  elif ! grep -q -- "$regra" <<<"$saida"; then
    echo "FALHA: $arquivo reprovou, mas sem citar $regra" >&2
    echo "$saida" >&2
    falhou=1
  else
    echo "ok: $regra reprova $arquivo"
  fi
done

exit "$falhou"
