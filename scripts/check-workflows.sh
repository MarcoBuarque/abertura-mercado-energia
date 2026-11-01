#!/usr/bin/env bash
# Checks the GitHub Actions workflows without GitHub:
#   - actionlint (Makefile version) clean;
#   - the 5 workflows and dependabot.yml exist;
#   - every external `uses:` pinned by a 40-character SHA, with the version in a comment;
#   - every image (`image:`, `docker://`, `docker run`) with a sha256 digest;
#   - `permissions:` and `concurrency:` at the top of each workflow;
#   - no secret besides GITHUB_TOKEN;
#   - every `make <target>` mentioned exists in the Makefile.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

failed=0
err() { echo "workflows: $1" >&2; failed=1; }

expected=(openapi go secrets release-openapi release-go)
for name in "${expected[@]}"; do
  [ -f ".github/workflows/$name.yml" ] || err "missing .github/workflows/$name.yml"
done
[ -f .github/dependabot.yml ] || err "missing .github/dependabot.yml"

shopt -s nullglob
files=(.github/workflows/*.yml .github/workflows/*.yaml)
[ "${#files[@]}" -gt 0 ] || { echo "workflows: no file in .github/workflows" >&2; exit 1; }

version="${ACTIONLINT_VERSION:-$(sed -n 's/^ACTIONLINT_VERSION[[:space:]]*:=[[:space:]]*\([^[:space:]]*\).*/\1/p' Makefile)}"
[ -n "$version" ] || { echo "workflows: ACTIONLINT_VERSION not found in the Makefile" >&2; exit 1; }
# shellcheck disable=SC2086
if ! go run "github.com/rhysd/actionlint/cmd/actionlint@$version" -color=false "${files[@]}"; then
  err "actionlint failed"
fi

for f in "${files[@]}"; do
  # external uses: (ignores ./local and docker://, handled below)
  while IFS= read -r line; do
    n="${line%%:*}"; text="${line#*:}"
    if ! grep -Eq '^[[:space:]]*-?[[:space:]]*uses:[[:space:]]+[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+@[0-9a-f]{40}[[:space:]]+#[[:space:]]*v[0-9]+(\.[0-9]+)*[[:space:]]*$' <<<"$text"; then
      err "$f:$n: uses without a 40-character SHA or without the version in a comment: $(echo "$text" | sed 's/^[[:space:]]*//')"
    fi
  done < <(grep -nE '^[[:space:]]*-?[[:space:]]*uses:[[:space:]]' "$f" | grep -vE 'uses:[[:space:]]+\./' || true)

  # images by digest
  while IFS= read -r line; do
    n="${line%%:*}"; text="${line#*:}"
    if ! grep -Eq '@sha256:[0-9a-f]{64}' <<<"$text"; then
      err "$f:$n: image without a sha256 digest: $(echo "$text" | sed 's/^[[:space:]]*//')"
    fi
  done < <(grep -nE '^[[:space:]]*(image:|.*docker://|.*docker run )' "$f" | grep -vE '^[0-9]+:[[:space:]]*#' || true)

  grep -Eq '^permissions:' "$f" || err "$f: missing permissions: at the top"
  grep -Eq '^concurrency:' "$f" || err "$f: missing concurrency: at the top"

  # secrets: only GITHUB_TOKEN
  while IFS= read -r s; do
    [ "$s" = "GITHUB_TOKEN" ] || err "$f: uses secrets.$s (only GITHUB_TOKEN is allowed)"
  done < <(grep -oE 'secrets\.[A-Za-z0-9_]+' "$f" | sed 's/^secrets\.//' | sort -u || true)

  # Makefile targets
  while IFS= read -r target; do
    grep -Eq "^${target}:" Makefile || err "$f: make $target does not exist in the Makefile"
  done < <(grep -oE '(^|[^A-Za-z0-9_./-])make[[:space:]]+[a-z][a-z0-9-]*' "$f" | awk '{print $NF}' | sort -u || true)
done

if [ "$failed" -eq 0 ]; then echo "workflows: ok (${#files[@]} files)"; fi
exit "$failed"
