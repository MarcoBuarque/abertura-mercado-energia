#!/usr/bin/env bash
# Confere que a página do catálogo de erros publicada tem um id para cada slug
# da tabela do Go (go/problem.go): o campo type de todo Problem termina em
# #slug, e sem o id o link cai no topo da página. Também confere a faixa
# "descrição não oficial" e que o HTML foi gerado.
#
# Uso: scripts/confere-erros.sh [html] [problem.go]
set -euo pipefail

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
html="${1:-$raiz/out/docs/erros/index.html}"
fonte="${2:-$raiz/go/problem.go}"

[ -f "$html" ] || { echo "confere-erros: $html não existe (rode make docs)" >&2; exit 1; }
[ -f "$fonte" ] || { echo "confere-erros: $fonte não existe" >&2; exit 1; }

# Slugs: as chaves do mapa catalogo, entre "var catalogo" e o "}" da coluna 0.
slugs="$(awk '/^var catalogo = /{dentro=1; next} dentro && /^}/{exit} dentro && /^\t"[a-z0-9-]+": \{$/{gsub(/[\t":{ ]/,""); print}' "$fonte")"
[ -n "$slugs" ] || { echo "confere-erros: nenhum slug achado em $fonte" >&2; exit 1; }

falhou=0
total=0
for s in $slugs; do
  total=$((total + 1))
  if ! grep -Eq "id=\"$s\"" "$html"; then
    echo "confere-erros: o slug $s não existe como id em $html" >&2
    falhou=1
  fi
done
grep -q "descrição não oficial" "$html" || { echo "confere-erros: falta a faixa 'descrição não oficial' em $html" >&2; falhou=1; }
grep -q 'name="viewport"' "$html" || { echo "confere-erros: falta o meta viewport em $html" >&2; falhou=1; }

[ "$falhou" -eq 0 ] || exit 1
echo "confere-erros: ok ($total slugs com id em ${html#"$raiz"/})"
