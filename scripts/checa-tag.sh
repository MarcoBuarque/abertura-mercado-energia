#!/usr/bin/env bash
# Checagens de tag de release, as mesmas que os dois workflows de release rodam:
#
#   scripts/checa-tag.sh openapi openapi/vX.Y.Z[-rc.N]
#   scripts/checa-tag.sh go      go/vX.Y.Z[-rc.N]
#
# O script só lê arquivos; a tag é um argumento e não precisa existir.
#
# openapi: a tag é openapi/vX.Y.Z e info.version (openapi/abertura.yaml) é X.Y.Z.
#          Numa pré-versão (-rc.N), info.version continua sendo o núcleo X.Y.Z:
#          a spec do rc é a mesma da versão final.
# go:      a tag é go/vX.Y.Z[-rc.N]; Version em go/internal/version/version.go é
#          igual à versão da tag (com o sufixo -rc.N, se houver); o go/CHANGELOG.md
#          tem a seção "## [versão]" (num rc, vale também a do núcleo) e ela cita
#          uma tag openapi/vX.Y.Z de origem.
# Tag pura vX.Y.Z (sem prefixo) é sempre rejeitada: o prefixo escolhe o release.
#
# Saída: 0 passa, 1 reprova, 2 uso errado.
set -euo pipefail

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

uso() { echo "uso: scripts/checa-tag.sh openapi|go <tag>" >&2; exit 2; }
falha() { echo "checa-tag: $1" >&2; exit 1; }

[ "$#" -eq 2 ] || uso
tipo="$1"; tag="$2"
case "$tipo" in openapi|go) ;; *) uso ;; esac

semver='[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?'
if ! [[ "$tag" =~ ^${tipo}/v(${semver})$ ]]; then
  falha "tag '$tag' fora do formato ${tipo}/vX.Y.Z ou ${tipo}/vX.Y.Z-rc.N (tag pura vX.Y.Z não vale)"
fi
versao="${BASH_REMATCH[1]}"
nucleo="${versao%%-*}"

case "$tipo" in
  openapi)
    arq="$raiz/openapi/abertura.yaml"
    [ -f "$arq" ] || falha "$arq não existe"
    # primeiro `version:` filho direto do bloco info (dois espaços de recuo)
    info="$(awk '/^info:/ {dentro=1; next} dentro && /^[^ #]/ {exit} dentro && /^  version:/ {sub(/^  version:[ ]*/, ""); gsub(/["'"'"']/, ""); sub(/[ ]+#.*$/, ""); print; exit}' "$arq")"
    [ -n "$info" ] || falha "info.version não encontrada em openapi/abertura.yaml"
    if [ "$info" != "$nucleo" ]; then
      falha "info.version é '$info', mas a tag $tag pede '$nucleo'"
    fi
    echo "checa-tag: ok, $tag × info.version $info"
    ;;
  go)
    arq="$raiz/go/internal/version/version.go"
    [ -f "$arq" ] || falha "$arq não existe"
    atual="$(sed -n 's/^.*Version[[:space:]]*=[[:space:]]*"\([^"]*\)".*$/\1/p' "$arq" | head -n1)"
    [ -n "$atual" ] || falha "Version não encontrada em go/internal/version/version.go"
    if [ "$atual" != "$versao" ]; then
      falha "Version é '$atual', mas a tag $tag pede '$versao'"
    fi

    cl="$raiz/go/CHANGELOG.md"
    [ -f "$cl" ] || falha "$cl não existe"
    secao=""
    for v in "$versao" "$nucleo"; do
      secao="$(awk -v alvo="## [$v]" 'index($0, alvo) == 1 {dentro=1; print; next} dentro && /^## / {exit} dentro {print}' "$cl")"
      [ -n "$secao" ] && break
    done
    [ -n "$secao" ] || falha "go/CHANGELOG.md não tem a seção '## [$versao]'"
    if ! grep -Eq "openapi/v${semver}" <<<"$secao"; then
      falha "a seção de $versao em go/CHANGELOG.md não cita a tag openapi/vX.Y.Z de origem"
    fi
    echo "checa-tag: ok, $tag × Version $atual × CHANGELOG"
    ;;
esac
