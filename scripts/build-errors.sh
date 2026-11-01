#!/usr/bin/env bash
# Gera out/docs/erros/index.html a partir de docs/erros.md com o pandoc (imagem
# fixada por digest em PANDOC_IMAGE). É o endereço do campo type de todo
# Problem: <Pages>/erros#slug. O GitHub Pages serve /erros/ a partir de
# erros/index.html e redireciona /erros para /erros/ (301); o navegador mantém
# o #fragmento no redirecionamento.
#
# A faixa "descrição não oficial" e o CSS dela vêm de docs/template.hbs, a
# mesma fonte da página do Redoc, para os dois avisos não divergirem.
set -euo pipefail

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$raiz"
: "${PANDOC_IMAGE:?defina PANDOC_IMAGE (digest da imagem do pandoc)}"

saida=out/docs/erros
tmp=out/erros-tmp
rm -rf "$tmp"
mkdir -p "$saida" "$tmp"

# Links relativos do repositório não existem no Pages: apontam para o GitHub.
repo="https://github.com/MarcoBuarque/abertura-mercado-energia/blob/main/docs"
sed -E "s#\]\((clientes\.md)#](${repo}/\1#g" docs/erros.md > "$tmp/erros.md"

# CSS e faixa extraídos do template do Redoc.
estilo="$(awk '/<style>/{d=1} d{print} /<\/style>/{d=0}' docs/template.hbs)"
faixa="$(awk '/<aside class="faixa-nao-oficial"/{d=1} d{print} /<\/aside>/{d=0}' docs/template.hbs)"
[ -n "$estilo" ] && [ -n "$faixa" ] || { echo "gera-erros: faixa ou estilo não achados em docs/template.hbs" >&2; exit 1; }

{
  echo '<meta name="viewport" content="width=device-width, initial-scale=1">'
  echo "$estilo"
  cat <<'CSS'
<style>
  html { scroll-padding-top: 6rem; }
  body { max-width: 52rem; margin: 0 auto; padding: 0 max(16px, env(safe-area-inset-right)) 3rem max(16px, env(safe-area-inset-left)); font: 16px/1.6 system-ui, -apple-system, "Segoe UI", Roboto, sans-serif; color: #1c1c1e; overflow-wrap: break-word; }
  .faixa-nao-oficial { margin-left: calc(-1 * max(16px, env(safe-area-inset-left))); margin-right: calc(-1 * max(16px, env(safe-area-inset-right))); width: auto; }
  h1 { font-size: clamp(1.5rem, 6vw, 2.2rem); line-height: 1.2; }
  h2 { font-size: clamp(1.2rem, 4.5vw, 1.5rem); line-height: 1.3; margin-top: 2rem; }
  a { color: #0b5cad; }
  code { font-size: .9em; background: #f2f2f4; padding: .1em .3em; border-radius: 4px; overflow-wrap: anywhere; }
  pre { overflow-x: auto; max-width: 100%; background: #f2f2f4; padding: 12px; border-radius: 6px; }
  pre code { background: none; padding: 0; overflow-wrap: normal; }
  table { display: block; max-width: 100%; overflow-x: auto; border-collapse: collapse; font-size: .9rem; }
  th, td { text-align: left; vertical-align: top; padding: 6px 10px; border-bottom: 1px solid #d8d8dc; min-width: 7rem; }
  th:first-child, td:first-child { position: sticky; left: 0; background: #fff; min-width: 9rem; }
  blockquote { margin-left: 0; padding-left: 12px; border-left: 4px solid #c98a1b; }
  hr { border: 0; border-top: 1px solid #d8d8dc; }
</style>
CSS
} > "$tmp/cabecalho.html"
echo "$faixa" > "$tmp/faixa.html"

docker run --rm -v "$raiz":/work -w /work "$PANDOC_IMAGE" \
  --from gfm --to html5 --standalone \
  --metadata lang=pt-BR --metadata pagetitle="Catálogo de erros (descrição não oficial)" \
  --include-in-header "$tmp/cabecalho.html" --include-before-body "$tmp/faixa.html" \
  --output "$saida/index.html" "$tmp/erros.md"

rm -rf "$tmp"
echo "gera-erros: $saida/index.html"
