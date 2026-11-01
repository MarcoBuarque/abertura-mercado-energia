#!/usr/bin/env bash
# Confere as regras de exemplo inventado da spec (arquitetura 2.7) que o vacuum
# não expressa: UC com zeros à frente, códigos na faixa 9xxxxx, sem o host de
# homologação e sem "CCEE" fora do aviso de não oficial.
set -euo pipefail

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$raiz"
spec=openapi/abertura.yaml
yq_versao="${YQ_VERSION:?YQ_VERSION não definida (rode pelo make)}"
yq() { go run "github.com/mikefarah/yq/v4@${yq_versao}" "$@"; }

falhou=0
confere() { # descrição, saída (vazia = ok); o status do comando já foi conferido
  if [ -n "$2" ]; then
    echo "FALHA: $1" >&2
    echo "$2" >&2
    falhou=1
  else
    echo "ok: $1"
  fi
}

# Com set -e, uma falha dentro de $(...) passada como argumento seria engolida;
# por isso cada consulta grava em variável antes, e o set -e para o script.
# Só valores escalares: os nomes de campo também aparecem como chaves de schema.
r1="$(yq '.. | select(has("codigoUnidadeConsumidora")) | .codigoUnidadeConsumidora | select(tag == "!!str" or tag == "!!int") | tostring | select(test("^00000") | not)' "$spec")"
confere "toda UC de exemplo começa com 00000" "$r1"

r2="$(yq '.. | select(tag == "!!map") | to_entries | .[] | select(.key | test("^(codigoAgente|codigoPerfil|codigoConcessionaria|codigoVarejista|codigoSupridora)")) | .value | select(tag == "!!str" or tag == "!!int") | tostring | select(test("^9") | not)' "$spec")"
confere "códigos de agente, perfil e concessionária de exemplo começam com 9" "$r2"

r3="$(yq '.components.parameters | to_entries | .[] | select(.value.name == "codigoAgente" or .value.name == "codigoPerfil" or .value.name == "codigoPerfilVarejista" or .value.name == "codigoAgenteConcessionaria") | .value.example | tostring | select(test("^9") | not)' "$spec")"
confere "exemplos dos parâmetros de agente e perfil começam com 9" "$r3"

r4="$(grep -n 'hom-migra' "$spec" || true)"
confere "sem host de homologação" "$r4"

# O aviso de não oficial (info.description) é o único lugar com "CCEE".
r5="$(yq 'del(.info.description)' "$spec" | grep -n 'CCEE' || true)"
confere "\"CCEE\" só em info.description" "$r5"

exit "$falhou"
