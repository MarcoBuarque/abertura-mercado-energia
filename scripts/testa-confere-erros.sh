#!/usr/bin/env bash
# Prova que scripts/confere-erros.sh reprova: id ausente, faixa ausente e HTML
# inexistente; e aprova o caso completo. Não usa rede nem docker.
set -euo pipefail
raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cat > "$tmp/problem.go" <<'GO'
package x

var catalogo = map[string]entradaCatalogo{
	"um-slug": {
		titulo: "Um",
	},
	"outro-slug": {
		titulo: "Outro",
	},
}
GO
cabeca='<meta name="viewport" content="width=device-width">'
faixa='descrição não oficial'

caso() { # nome esperado html
  local nome="$1" esperado="$2" html="$3" rc=0
  printf '%s\n' "$html" > "$tmp/$nome.html"
  "$raiz/scripts/confere-erros.sh" "$tmp/$nome.html" "$tmp/problem.go" >/dev/null 2>&1 || rc=$?
  if [ "$esperado" = ok ] && [ "$rc" -ne 0 ]; then echo "testa-confere-erros: $nome deveria passar" >&2; exit 1; fi
  if [ "$esperado" = falha ] && [ "$rc" -eq 0 ]; then echo "testa-confere-erros: $nome deveria falhar" >&2; exit 1; fi
}

caso completo ok "$cabeca $faixa <a id=\"um-slug\"></a><a id=\"outro-slug\"></a>"
caso sem-id falha "$cabeca $faixa <a id=\"um-slug\"></a>"
caso sem-faixa falha "$cabeca <a id=\"um-slug\"></a><a id=\"outro-slug\"></a>"
caso sem-viewport falha "$faixa <a id=\"um-slug\"></a><a id=\"outro-slug\"></a>"
if "$raiz/scripts/confere-erros.sh" "$tmp/nao-existe.html" "$tmp/problem.go" >/dev/null 2>&1; then
  echo "testa-confere-erros: HTML inexistente deveria falhar" >&2; exit 1
fi
echo "testa-confere-erros: ok"
