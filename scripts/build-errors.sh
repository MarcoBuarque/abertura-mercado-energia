#!/usr/bin/env bash
# Builds out/docs/errors/index.html from docs/errors.md with pandoc (image
# pinned by digest in PANDOC_IMAGE). It is the address of the type field of
# every Problem: <Pages>/errors#slug. GitHub Pages serves /errors/ from
# errors/index.html and redirects /errors to /errors/ (301); the browser keeps
# the #fragment across the redirect.
#
# It also writes out/docs/erros/index.html, a small redirect (meta refresh and
# link) to ../errors/, so the old Portuguese address keeps working.
#
# The "unofficial description" banner and its CSS come from docs/template.hbs,
# the same source as the Redoc page, so the two notices do not diverge.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
: "${PANDOC_IMAGE:?set PANDOC_IMAGE (digest of the pandoc image)}"

output=out/docs/errors
legacy=out/docs/erros
tmp=out/errors-tmp
rm -rf "$tmp" "$legacy"
mkdir -p "$output" "$legacy" "$tmp"

# Relative repository links do not exist on Pages: point them to GitHub.
repo="https://github.com/MarcoBuarque/abertura-mercado-energia/blob/main/docs"
sed -E "s#\]\((clients\.md)#](${repo}/\1#g" docs/errors.md > "$tmp/errors.md"

# CSS and banner extracted from the Redoc template (the first <style> block and
# the first <aside> element).
style="$(awk '/<style>/{d=1} d{print} /<\/style>/{d=0}' docs/template.hbs)"
banner="$(awk '/<aside/{d=1} d{print} /<\/aside>/{d=0; exit}' docs/template.hbs)"
[ -n "$style" ] && [ -n "$banner" ] || { echo "build-errors: banner or style not found in docs/template.hbs" >&2; exit 1; }

{
  echo '<meta name="viewport" content="width=device-width, initial-scale=1">'
  echo "$style"
  cat <<'CSS'
<style>
  html { scroll-padding-top: 6rem; }
  body { max-width: 52rem; margin: 0 auto; padding: 0 max(16px, env(safe-area-inset-right)) 3rem max(16px, env(safe-area-inset-left)); font: 16px/1.6 system-ui, -apple-system, "Segoe UI", Roboto, sans-serif; color: #1c1c1e; overflow-wrap: break-word; }
  aside[role="note"] { margin-left: calc(-1 * max(16px, env(safe-area-inset-left))); margin-right: calc(-1 * max(16px, env(safe-area-inset-right))); width: auto; }
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
} > "$tmp/header.html"
echo "$banner" > "$tmp/banner.html"

docker run --rm -v "$root":/work -w /work "$PANDOC_IMAGE" \
  --from gfm --to html5 --standalone \
  --metadata lang=en --metadata pagetitle="Error catalogue (unofficial description)" \
  --include-in-header "$tmp/header.html" --include-before-body "$tmp/banner.html" \
  --output "$output/index.html" "$tmp/errors.md"

cat > "$legacy/index.html" <<'HTML'
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="refresh" content="0; url=../errors/">
<link rel="canonical" href="../errors/">
<title>Moved to /errors/</title>
</head>
<body>
<p>This page moved to <a href="../errors/">the error catalogue</a>.</p>
<script>location.replace("../errors/" + location.hash);</script>
</body>
</html>
HTML

rm -rf "$tmp"
echo "build-errors: $output/index.html (and the $legacy/ redirect)"
