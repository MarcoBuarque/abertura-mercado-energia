#!/usr/bin/env bash
# Prova scripts/checa-marca.sh em repositórios git temporários, fora deste
# repositório. Cada caso monta os arquivos, faz git add e roda o script.
set -euo pipefail

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/checa-marca-teste.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
falhou=0

aviso='Projeto independente e não oficial. Não é afiliado, patrocinado nem endossado pela CCEE.'

# caso nome esperado(0|1) trecho_da_saida arquivo=conteudo...
caso() {
  local nome="$1" esperado="$2" trecho="$3"; shift 3
  rm -rf "$tmp/r"; mkdir -p "$tmp/r/scripts"
  cp "$raiz/scripts/checa-marca.sh" "$tmp/r/scripts/" 2>/dev/null || true
  git -C "$tmp/r" init -q
  local par arq
  for par in "$@"; do
    arq="${par%%=*}"
    mkdir -p "$tmp/r/$(dirname "$arq")"
    printf '%s\n' "${par#*=}" > "$tmp/r/$arq"
  done
  git -C "$tmp/r" add -A
  local saida rc=0
  saida="$(cd "$tmp/r" && bash scripts/checa-marca.sh 2>&1)" || rc=$?
  if [ "$rc" != "$esperado" ] || { [ -n "$trecho" ] && ! grep -qF -- "$trecho" <<<"$saida"; }; then
    echo "FALHOU: $nome (saída $rc, esperado $esperado, trecho '$trecho')" >&2
    echo "$saida" >&2
    falhou=1
  else
    echo "ok: $nome"
  fi
}

caso "aviso válido no NOTICE passa" 0 "" "NOTICE=$aviso"
caso "sem nenhuma ocorrência passa" 0 "" "go/x.go=package x"
caso "aviso em duas linhas (marca na linha seguinte) passa" 0 "" \
  "README.md=nem endossado pela
(CCEE), e só."
caso "CCEE em .go fora da lista reprova" 1 "go/x.go:1" "go/x.go=// fala com a CCEE"
caso "CCEE em arquivo da lista fora do aviso reprova" 1 "README.md:3" \
  "README.md=$aviso

Use a API da CCEE para tudo."
caso "reprova todas as ocorrências e lista arquivo:linha" 1 "go/y.go:2" \
  "go/x.go=// CCEE" "go/y.go=package y
// CCEE de novo"
caso "regra do vacuum aceita a marca sem aviso" 0 "" \
  "openapi/vacuum.yaml=não usam o nome da CCEE."

exit "$falhou"
