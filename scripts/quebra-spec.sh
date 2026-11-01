#!/usr/bin/env bash
# Diz se a quebra da spec está registrada: a seção "## [Não lançado]" de
# openapi/CHANGELOG.md tem uma subseção "### Quebras" com conteúdo além de
# "Nenhuma." (qualquer caixa, aceita texto depois do ponto).
#
# Uso:
#   scripts/quebra-spec.sh [--exige] [CHANGELOG]
#     sem flag: imprime 1 (registrada) ou 0 (não registrada) e sai 0;
#     --exige:  sai 1 com mensagem se a seção não estiver preenchida
#               (usado quando o PR leva o rótulo `quebra-spec`).
# Sai 2 em uso errado ou arquivo ausente. Só lê arquivos.
set -euo pipefail

exige=0
arquivo=""
for arg in "$@"; do
  case "$arg" in
    --exige) exige=1 ;;
    -*) echo "quebra-spec: opção desconhecida: $arg" >&2; exit 2 ;;
    *)
      [ -z "$arquivo" ] || { echo "quebra-spec: argumentos demais" >&2; exit 2; }
      arquivo="$arg"
      ;;
  esac
done

if [ -z "$arquivo" ]; then
  arquivo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/openapi/CHANGELOG.md"
fi
[ -f "$arquivo" ] || { echo "quebra-spec: $arquivo não existe" >&2; exit 2; }

preenchida="$(awk '
  /^## / { naoLancado = ($0 ~ /^## \[Não lançado\]/); quebras = 0; next }
  /^### / { quebras = (naoLancado && $0 ~ /^### Quebras[[:space:]]*$/); next }
  quebras {
    linha = $0
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", linha)
    if (linha == "") next
    if (tolower(linha) ~ /^nenhuma\.?([[:space:]]|$)/) next
    achou = 1
  }
  END { print achou ? 1 : 0 }
' "$arquivo")"

if [ "$exige" -eq 1 ]; then
  if [ "$preenchida" -ne 1 ]; then
    echo "quebra-spec: rótulo quebra-spec sem seção Quebras preenchida em [Não lançado] ($arquivo)" >&2
    exit 1
  fi
  echo "quebra-spec: seção Quebras preenchida em [Não lançado]"
  exit 0
fi
echo "$preenchida"
