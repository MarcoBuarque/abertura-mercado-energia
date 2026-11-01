#!/usr/bin/env bash
# O runtime do pacote abertura (go/) só pode depender da biblioteca padrão, de
# golang.org/x/oauth2 e de golang.org/x/time. A kin-openapi e o resto ficam em
# testes, no testserver e no contrato, que `go list -deps .` não alcança.
set -euo pipefail

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$raiz/go"

modulo="$(go list -m)"
permitidos=("$modulo" "golang.org/x/oauth2" "golang.org/x/time")

# Pacotes que não são da biblioteca padrão e entram no binário do usuário.
total=0
falhou=0
while IFS= read -r dep; do
  [ -n "$dep" ] || continue
  total=$((total + 1))
  ok=0
  for p in "${permitidos[@]}"; do
    if [ "$dep" = "$p" ] || [[ "$dep" == "$p"/* ]]; then ok=1; break; fi
  done
  if [ "$ok" -eq 0 ]; then
    echo "falha: dependência de runtime não permitida: $dep" >&2
    falhou=1
  fi
done < <(go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' . | sort)

if [ "$falhou" -ne 0 ]; then
  echo "O pacote abertura só pode importar a biblioteca padrão, golang.org/x/oauth2 e golang.org/x/time." >&2
  exit 1
fi
echo "ok: o runtime do pacote abertura usa só a biblioteca padrão, x/oauth2 e x/time ($total pacotes fora da stdlib, todos permitidos)"
