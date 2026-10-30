#!/usr/bin/env bash
# Prova scripts/checa-tag.sh em diretórios temporários, fora deste repositório
# e sem criar tag nenhuma: o script só lê arquivos, a tag é um argumento.
set -euo pipefail

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/checa-tag-teste.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
falhou=0

# monta um repositório de mentira (sem git) com os arquivos que o script lê.
monta() { # info_version Version changelog
  rm -rf "$tmp/r"
  mkdir -p "$tmp/r/scripts" "$tmp/r/openapi" "$tmp/r/go/internal/version"
  cp "$raiz/scripts/checa-tag.sh" "$tmp/r/scripts/" 2>/dev/null || true
  cat >"$tmp/r/openapi/abertura.yaml" <<YAML
openapi: 3.0.3
info:
  title: Teste
  version: $1
  description: x
paths: {}
components:
  schemas:
    Outro:
      version: 9.9.9
YAML
  cat >"$tmp/r/go/internal/version/version.go" <<GO
package version

// Version é a versão corrente do SDK.
const Version = "$2"
GO
  printf '%s\n' "$3" >"$tmp/r/go/CHANGELOG.md"
}

CL_OK=$'# Changelog\n\n## [Não lançado]\n\n## [0.1.0]\n\n- Gerado de `openapi/v0.1.0`.\n'
CL_SEM_SECAO=$'# Changelog\n\n## [Não lançado]\n\n- Gerado de `openapi/v0.1.0`.\n'
CL_SEM_CITACAO=$'# Changelog\n\n## [Não lançado]\n\nVeja `openapi/v0.1.0`.\n\n## [0.1.0]\n\n- Primeira versão.\n'
CL_RC=$'# Changelog\n\n## [0.1.0-rc.1]\n\n- Gerado de `openapi/v0.1.0-rc.1`.\n\n## [0.1.0]\n\n- Primeira versão.\n'
CL_RC_NUCLEO=$'# Changelog\n\n## [0.1.0]\n\n- Gerado de `openapi/v0.1.0`.\n'

tests=(
  # nome|esperado|tipo|tag|info.version|Version|changelog
  "openapi tag igual a info.version passa|0|openapi|openapi/v0.1.0|0.1.0|0.1.0-dev|OK"
  "openapi tag diferente de info.version falha|1|openapi|openapi/v0.1.0|0.1.1|0.1.0-dev|OK"
  "openapi rc com info.version do núcleo passa|0|openapi|openapi/v0.1.0-rc.1|0.1.0|0.1.0-dev|OK"
  "openapi rc de outro núcleo falha|1|openapi|openapi/v0.1.1-rc.1|0.1.0|0.1.0-dev|OK"
  "openapi final com info.version de rc falha|1|openapi|openapi/v0.1.0|0.1.0-rc.1|0.1.0-dev|OK"
  "openapi lê o version do bloco info, não de outro|0|openapi|openapi/v0.1.0|0.1.0|0.1.0-dev|OK"
  "openapi tag pura vX.Y.Z falha|1|openapi|v0.1.0|0.1.0|0.1.0-dev|OK"
  "openapi tag sem o v falha|1|openapi|openapi/0.1.0|0.1.0|0.1.0-dev|OK"
  "openapi tag com dois números falha|1|openapi|openapi/v0.1|0.1.0|0.1.0-dev|OK"
  "openapi com tag go falha|1|openapi|go/v0.1.0|0.1.0|0.1.0|OK"
  "go tag, Version e seção que cita a spec passa|0|go|go/v0.1.0|0.1.0|0.1.0|OK"
  "go sem seção no CHANGELOG falha|1|go|go/v0.1.0|0.1.0|0.1.0|SEM_SECAO"
  "go seção sem citar openapi/v falha|1|go|go/v0.1.0|0.1.0|0.1.0|SEM_CITACAO"
  "go tag pura vX.Y.Z falha|1|go|v0.1.0|0.1.0|0.1.0|OK"
  "go com Version diferente da tag falha|1|go|go/v0.1.0|0.1.0|0.1.0-dev|OK"
  "go rc com Version rc e seção rc passa|0|go|go/v0.1.0-rc.1|0.1.0|0.1.0-rc.1|RC"
  "go rc com Version rc e seção do núcleo passa|0|go|go/v0.1.0-rc.1|0.1.0|0.1.0-rc.1|RC_NUCLEO"
  "go rc com Version final falha|1|go|go/v0.1.0-rc.1|0.1.0|0.1.0|RC"
  "go com tag openapi falha|1|go|openapi/v0.1.0|0.1.0|0.1.0|OK"
  "tipo desconhecido falha|1|npm|npm/v0.1.0|0.1.0|0.1.0|OK"
)

for linha in "${tests[@]}"; do
  IFS='|' read -r nome esperado tipo tag info ver cl <<<"$linha"
  case "$cl" in
    OK) conteudo="$CL_OK" ;; SEM_SECAO) conteudo="$CL_SEM_SECAO" ;;
    SEM_CITACAO) conteudo="$CL_SEM_CITACAO" ;; RC) conteudo="$CL_RC" ;;
    RC_NUCLEO) conteudo="$CL_RC_NUCLEO" ;;
  esac
  monta "$info" "$ver" "$conteudo"
  codigo=0
  saida="$("$tmp/r/scripts/checa-tag.sh" "$tipo" "$tag" 2>&1)" || codigo=$?
  if [ "$esperado" = 0 ] && [ "$codigo" -eq 0 ]; then
    echo "ok: $nome"
  elif [ "$esperado" = 1 ] && [ "$codigo" -eq 1 ]; then
    echo "ok: $nome"
  else
    echo "FALHA: $nome (esperado $esperado, veio $codigo): $saida" >&2
    falhou=1
  fi
done

# sem argumentos: uso, sai 2
monta 0.1.0 0.1.0 "$CL_OK"
codigo=0; "$tmp/r/scripts/checa-tag.sh" >/dev/null 2>&1 || codigo=$?
if [ "$codigo" -eq 2 ]; then echo "ok: sem argumentos mostra o uso (sai 2)"; else echo "FALHA: sem argumentos devia sair 2 (veio $codigo)" >&2; falhou=1; fi

exit "$falhou"
