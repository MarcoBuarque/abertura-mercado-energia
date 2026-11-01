#!/usr/bin/env bash
# Testes com -race e cobertura; falha se a cobertura do pacote raiz do SDK
# (o pacote `abertura`, sem internal/) ficar abaixo do mínimo.
#   scripts/go-cobertura.sh [minimo_percentual]   (padrão 80)
set -euo pipefail

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
min="${1:-80}"
perfil="$raiz/out/go-cobertura.out"
mkdir -p "$raiz/out"

cd "$raiz/go"
go test -race -covermode=atomic -coverprofile="$perfil" ./...

modulo="$(sed -n 's/^module[[:space:]]*//p' go.mod | head -n1)"
awk -v modulo="$modulo" -v min="$min" '
  NR == 1 { next }                       # linha "mode: atomic"
  {
    arq = $1; sub(/:.*/, "", arq)        # caminho/arquivo.go
    resto = substr(arq, length(modulo) + 2)
    if (index(arq, modulo "/") != 1 || index(resto, "/") > 0) next   # só o pacote raiz
    n = $2; total += n
    if ($3 > 0) coberto += n
  }
  END {
    if (total == 0) { print "cobertura: nenhuma instrução do pacote raiz no perfil" > "/dev/stderr"; exit 1 }
    pct = 100 * coberto / total
    printf "cobertura do pacote raiz: %.1f%% (mínimo %s%%)\n", pct, min
    if (pct < min) { print "cobertura: abaixo do mínimo" > "/dev/stderr"; exit 1 }
  }' "$perfil"
