#!/usr/bin/env bash
# Regra do projeto (termos de uso da CCEE de 13/10/2025): o nome "CCEE" aparece
# só no aviso de não afiliação. Varre os arquivos versionados e reprova toda
# ocorrência fora dessa regra, listando arquivo:linha.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

# Arquivos que podem conter o aviso. Cada linha com "CCEE" precisa estar num
# bloco de aviso (ver abaixo).
avisos=(
  NOTICE                          # aviso de não afiliação
  go/NOTICE                       # cópia do NOTICE para o zip do módulo Go
  README.md                       # aviso no topo
  go/README.md                    # aviso no topo
  go/doc.go                       # aviso na documentação do pacote
  openapi/abertura.yaml           # aviso em info.description
  go/internal/spec/abertura.yaml  # cópia da spec (go generate), mesmo aviso
  docs/clientes.md                # aviso no topo
  docs/template.hbs               # faixa de aviso da página Redoc
  openapi/CHANGELOG.md            # aviso no topo
)

# Arquivos que podem citar a marca sem aviso, por serem a própria checagem.
ferramentas=(
  openapi/vacuum.yaml             # regra que proíbe a marca nos metadados
  scripts/confere-exemplos.sh     # confere que a marca só está em info.description
  scripts/checa-marca.sh          # este script
  scripts/testa-checa-marca.sh    # os casos de teste deste script
)

# Bloco de aviso: a linha, ou a anterior (o aviso quebra linhas), diz
# "afiliad", "endoss" ou "não oficial".
padrao_aviso='afiliad|endoss|não oficial'

em_lista() { # arquivo lista...
  local f="$1" x; shift
  for x in "$@"; do [ "$x" = "$f" ] && return 0; done
  return 1
}

falhas=0
while IFS= read -r arq; do
  [ -f "$arq" ] || continue
  grep -q 'CCEE' "$arq" 2>/dev/null || continue
  if em_lista "$arq" "${ferramentas[@]}"; then continue; fi
  if ! em_lista "$arq" "${avisos[@]}"; then
    grep -n 'CCEE' "$arq" | while IFS=: read -r n _; do
      echo "marca fora da lista: $arq:$n" >&2
    done
    falhas=1
    continue
  fi
  while IFS= read -r n; do
    atual="$(sed -n "${n}p" "$arq")"
    anterior=""; [ "$n" -gt 1 ] && anterior="$(sed -n "$((n-1))p" "$arq")"
    if ! printf '%s\n%s\n' "$anterior" "$atual" | grep -qiE "$padrao_aviso"; then
      echo "marca fora do aviso de não afiliação: $arq:$n" >&2
      falhas=1
    fi
  done < <(grep -n 'CCEE' "$arq" | cut -d: -f1)
done < <(git ls-files)

if [ "$falhas" != 0 ]; then
  echo "checa-marca: \"CCEE\" só pode aparecer no aviso de não afiliação" >&2
  exit 1
fi
echo "checa-marca: ok"
