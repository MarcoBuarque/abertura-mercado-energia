#!/usr/bin/env bash
# Prova scripts/quebra-spec.sh com CHANGELOGs sintéticos em diretório temporário.
set -euo pipefail

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/quebra-spec-teste.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
falhou=0

mkdir -p "$tmp/scripts"
cp "$raiz/scripts/quebra-spec.sh" "$tmp/scripts/"

CL_PREENCHIDA=$'# Changelog\n\n## [Não lançado]\n\n### Alterado\n\n- x\n\n### Quebras\n\nA propriedade `message` sai da resposta 400.\n\n## [0.1.0]\n\n### Quebras\n\nNenhuma.\n'
CL_NENHUMA=$'# Changelog\n\n## [Não lançado]\n\n### Quebras\n\nNenhuma.\n\n## [0.1.0]\n'
CL_NENHUMA_TEXTO=$'# Changelog\n\n## [Não lançado]\n\n### Quebras\n\nNENHUMA. A seção só existirá em versões 0.x futuras.\n\n## [0.1.0]\n'
CL_SEM_SECAO=$'# Changelog\n\n## [Não lançado]\n\n### Alterado\n\n- x\n\n## [0.1.0]\n'
CL_SO_LANCADA=$'# Changelog\n\n## [Não lançado]\n\n### Alterado\n\n- x\n\n## [0.1.0]\n\n### Quebras\n\nA propriedade `message` sai.\n'
CL_VAZIA=$'# Changelog\n\n## [Não lançado]\n\n### Quebras\n\n\n### Alterado\n\n- x\n'
CL_OUTRA_SUBSECAO=$'# Changelog\n\n## [Não lançado]\n\n### Quebras\n\n### Alterado\n\n- texto que não é quebra\n'

tests=(
  # nome|esperado (saída ou código)|flag|changelog
  "seção preenchida imprime 1|saida:1||CL_PREENCHIDA"
  "Nenhuma. imprime 0|saida:0||CL_NENHUMA"
  "Nenhuma. com texto depois imprime 0|saida:0||CL_NENHUMA_TEXTO"
  "sem seção imprime 0|saida:0||CL_SEM_SECAO"
  "seção só na versão lançada imprime 0|saida:0||CL_SO_LANCADA"
  "seção vazia imprime 0|saida:0||CL_VAZIA"
  "texto da subseção seguinte não conta imprime 0|saida:0||CL_OUTRA_SUBSECAO"
  "--exige com seção preenchida sai 0|codigo:0|--exige|CL_PREENCHIDA"
  "--exige com Nenhuma. sai 1|codigo:1|--exige|CL_NENHUMA"
  "--exige sem seção sai 1|codigo:1|--exige|CL_SEM_SECAO"
  "--exige com seção só na versão lançada sai 1|codigo:1|--exige|CL_SO_LANCADA"
)

for linha in "${tests[@]}"; do
  IFS='|' read -r nome esperado flag cl <<<"$linha"
  printf '%s' "${!cl}" >"$tmp/CHANGELOG.md"
  codigo=0
  args=(); [ -z "$flag" ] || args+=("$flag")
  saida="$("$tmp/scripts/quebra-spec.sh" ${args[@]+"${args[@]}"} "$tmp/CHANGELOG.md" 2>/dev/null)" || codigo=$?
  case "$esperado" in
    saida:*) [ "$codigo" -eq 0 ] && [ "$saida" = "${esperado#saida:}" ] && bom=1 || bom=0 ;;
    codigo:*) [ "$codigo" -eq "${esperado#codigo:}" ] && bom=1 || bom=0 ;;
  esac
  if [ "$bom" -eq 1 ]; then echo "ok: $nome"; else echo "FALHA: $nome (código $codigo, saída '$saida')" >&2; falhou=1; fi
done

codigo=0
"$tmp/scripts/quebra-spec.sh" "$tmp/nao-existe.md" >/dev/null 2>&1 || codigo=$?
if [ "$codigo" -eq 2 ]; then echo "ok: arquivo ausente sai 2"; else echo "FALHA: arquivo ausente devia sair 2 (código $codigo)" >&2; falhou=1; fi

codigo=0
"$tmp/scripts/quebra-spec.sh" --bobagem "$tmp/CHANGELOG.md" >/dev/null 2>&1 || codigo=$?
if [ "$codigo" -eq 2 ]; then echo "ok: opção desconhecida sai 2"; else echo "FALHA: opção desconhecida devia sair 2 (código $codigo)" >&2; falhou=1; fi

exit "$falhou"
