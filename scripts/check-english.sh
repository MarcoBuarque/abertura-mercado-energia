#!/bin/bash
# Fails when a document that must be in English reads like Portuguese.
# Heuristic: counts frequent Portuguese function words per file. Quoted API
# messages and domain terms do not trip it; a paragraph of Portuguese does.
# Exempt: README.pt-BR.md (the only Portuguese document) and generated output.
set -euo pipefail
cd "$(dirname "$0")/.."
LIMIT="${CHECK_ENGLISH_LIMIT:-6}"
WORDS='não|para|uma|são|também|então|porque|até|após|ainda|mês|meses|isso|isto|está|estão|pelo|pela|dos|das|aos|às|você|quando|sempre|nunca|apenas|cada|entre|sobre'
status=0
while IFS= read -r f; do
  hits=$( (grep -o -i -w -E "$WORDS" "$f" || true) | wc -l | tr -d ' ')
  if [ "$hits" -gt "$LIMIT" ]; then
    echo "FAIL $f: $hits Portuguese function words (limit $LIMIT)"
    grep -n -i -w -E "$WORDS" "$f" | head -5 || true
    status=1
  fi
done < <(git ls-files '*.md' '*.yaml' '*.yml' '*.hbs' 'scripts/*' 'Makefile' | grep -v -E '^README\.pt-BR\.md$|^out/|^scripts/check-english\.sh$')
[ "$status" -eq 0 ] && echo "OK: documents read as English"
exit "$status"
