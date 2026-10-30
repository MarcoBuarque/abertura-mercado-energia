#!/usr/bin/env bash
# Confere os workflows do GitHub Actions sem GitHub:
#   - actionlint (versão do Makefile) limpo;
#   - os 5 workflows e o dependabot.yml existem;
#   - todo `uses:` externo fixado por SHA de 40 caracteres, com a versão em comentário;
#   - toda imagem (`image:`, `docker://`, `docker run`) com digest sha256;
#   - `permissions:` e `concurrency:` no topo de cada workflow;
#   - nenhum segredo além de GITHUB_TOKEN;
#   - todo `make <alvo>` citado existe no Makefile.
set -euo pipefail

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$raiz"

falhou=0
erro() { echo "workflows: $1" >&2; falhou=1; }

esperados=(openapi go segredos release-openapi release-go)
for nome in "${esperados[@]}"; do
  [ -f ".github/workflows/$nome.yml" ] || erro "falta .github/workflows/$nome.yml"
done
[ -f .github/dependabot.yml ] || erro "falta .github/dependabot.yml"

shopt -s nullglob
arquivos=(.github/workflows/*.yml .github/workflows/*.yaml)
[ "${#arquivos[@]}" -gt 0 ] || { echo "workflows: nenhum arquivo em .github/workflows" >&2; exit 1; }

versao="${ACTIONLINT_VERSION:-$(sed -n 's/^ACTIONLINT_VERSION[[:space:]]*:=[[:space:]]*\([^[:space:]]*\).*/\1/p' Makefile)}"
[ -n "$versao" ] || { echo "workflows: ACTIONLINT_VERSION não encontrada no Makefile" >&2; exit 1; }
# shellcheck disable=SC2086
if ! go run "github.com/rhysd/actionlint/cmd/actionlint@$versao" -color=false "${arquivos[@]}"; then
  erro "actionlint reprovou"
fi

for f in "${arquivos[@]}"; do
  # uses: externo (ignora ./local e docker://, tratado abaixo)
  while IFS= read -r linha; do
    n="${linha%%:*}"; texto="${linha#*:}"
    if ! grep -Eq '^[[:space:]]*-?[[:space:]]*uses:[[:space:]]+[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+@[0-9a-f]{40}[[:space:]]+#[[:space:]]*v[0-9]+(\.[0-9]+)*[[:space:]]*$' <<<"$texto"; then
      erro "$f:$n: uses sem SHA de 40 caracteres ou sem a versão em comentário: $(echo "$texto" | sed 's/^[[:space:]]*//')"
    fi
  done < <(grep -nE '^[[:space:]]*-?[[:space:]]*uses:[[:space:]]' "$f" | grep -vE 'uses:[[:space:]]+\./' || true)

  # imagens por digest
  while IFS= read -r linha; do
    n="${linha%%:*}"; texto="${linha#*:}"
    if ! grep -Eq '@sha256:[0-9a-f]{64}' <<<"$texto"; then
      erro "$f:$n: imagem sem digest sha256: $(echo "$texto" | sed 's/^[[:space:]]*//')"
    fi
  done < <(grep -nE '^[[:space:]]*(image:|.*docker://|.*docker run )' "$f" | grep -vE '^[0-9]+:[[:space:]]*#' || true)

  grep -Eq '^permissions:' "$f" || erro "$f: falta permissions: no topo"
  grep -Eq '^concurrency:' "$f" || erro "$f: falta concurrency: no topo"

  # segredos: só GITHUB_TOKEN
  while IFS= read -r s; do
    [ "$s" = "GITHUB_TOKEN" ] || erro "$f: usa secrets.$s (só GITHUB_TOKEN é permitido)"
  done < <(grep -oE 'secrets\.[A-Za-z0-9_]+' "$f" | sed 's/^secrets\.//' | sort -u || true)

  # alvos do Makefile
  while IFS= read -r alvo; do
    grep -Eq "^${alvo}:" Makefile || erro "$f: make $alvo não existe no Makefile"
  done < <(grep -oE '(^|[^A-Za-z0-9_./-])make[[:space:]]+[a-z][a-z0-9-]*' "$f" | awk '{print $NF}' | sort -u || true)
done

if [ "$falhou" -eq 0 ]; then echo "workflows: ok (${#arquivos[@]} arquivos)"; fi
exit "$falhou"
