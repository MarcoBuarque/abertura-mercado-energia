#!/usr/bin/env bash
# Proves scripts/check-trademark.sh in temporary git repositories, outside this
# repository. Each case builds the files, runs git add and runs the script.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/check-trademark-test.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
failed=0

notice='Independent, unofficial project. Not affiliated with, sponsored or endorsed by the CCEE.'
notice_pt='Projeto independente e não oficial. Não é afiliado, patrocinado nem endossado pela CCEE.'

# case name expected(0|1) output_fragment file=content...
case_() {
  local name="$1" expected="$2" fragment="$3"; shift 3
  rm -rf "$tmp/r"; mkdir -p "$tmp/r/scripts"
  cp "$root/scripts/check-trademark.sh" "$tmp/r/scripts/" 2>/dev/null || true
  git -C "$tmp/r" init -q
  local pair file
  for pair in "$@"; do
    file="${pair%%=*}"
    mkdir -p "$tmp/r/$(dirname "$file")"
    printf '%s\n' "${pair#*=}" > "$tmp/r/$file"
  done
  git -C "$tmp/r" add -A
  local output rc=0
  output="$(cd "$tmp/r" && bash scripts/check-trademark.sh 2>&1)" || rc=$?
  if [ "$rc" != "$expected" ] || { [ -n "$fragment" ] && ! grep -qF -- "$fragment" <<<"$output"; }; then
    echo "FAILED: $name (exit $rc, expected $expected, fragment '$fragment')" >&2
    echo "$output" >&2
    failed=1
  else
    echo "ok: $name"
  fi
}

case_ "valid notice in NOTICE passes" 0 "" "NOTICE=$notice"
case_ "valid Portuguese notice in README.pt-BR.md passes" 0 "" "README.pt-BR.md=$notice_pt"
case_ "notice in abertura/NOTICE passes" 0 "" "abertura/NOTICE=$notice"
case_ "no occurrence at all passes" 0 "" "abertura/x.go=package x"
case_ "notice on two lines (name on the next line) passes" 0 "" \
  "README.md=nor endorsed by
(CCEE), and only that."
case_ "CCEE in a .go outside the list fails" 1 "abertura/x.go:1" "abertura/x.go=// talks to the CCEE"
case_ "the old go/ path is no longer allowed" 1 "go/NOTICE:1" "go/NOTICE=$notice"
case_ "CCEE in a listed file outside the notice fails" 1 "README.md:3" \
  "README.md=$notice

Use the CCEE API for everything."
case_ "fails on every occurrence and lists file:line" 1 "abertura/y.go:2" \
  "abertura/x.go=// CCEE" "abertura/y.go=package y
// CCEE again"
case_ "the vacuum rule accepts the name without a notice" 0 "" \
  "openapi/vacuum.yaml=do not use the name of the CCEE."

exit "$failed"
