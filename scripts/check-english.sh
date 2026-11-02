#!/bin/bash
# Fails when a document that must be in English reads like Portuguese.
# Heuristic: counts frequent Portuguese function words per file and fails on a high rate. Quoted API
# messages and domain terms do not trip it; a paragraph of Portuguese does.
# Exempt: README.pt-BR.md (the only Portuguese document) and generated output.
set -euo pipefail
cd "$(dirname "$0")/.."
MIN_HITS="${CHECK_ENGLISH_MIN_HITS:-8}"
MAX_PER_1000="${CHECK_ENGLISH_MAX_PER_1000:-20}"
WORDS='não|para|uma|são|também|então|porque|até|após|ainda|mês|meses|isso|isto|está|estão|pelo|pela|dos|das|aos|às|você|quando|sempre|nunca|apenas|cada|entre|sobre|que|com|em|um|por|mais|como|se|os'
status=0
while IFS= read -r f; do
  hits=$( (grep -o -i -w -E "$WORDS" "$f" || true) | wc -l | tr -d ' ')
  total=$(wc -w < "$f" | tr -d ' ')
  rate=$(( total > 0 ? hits * 1000 / total : 0 ))
  if [ "$hits" -ge "$MIN_HITS" ] && [ "$rate" -gt "$MAX_PER_1000" ]; then
    echo "FAIL $f: $hits Portuguese function words, $rate per 1000 words (fails at $MIN_HITS or more and above $MAX_PER_1000)"
    grep -n -i -w -E "$WORDS" "$f" | head -5 || true
    status=1
  fi
done < <(git ls-files '*.md' '*.yaml' '*.yml' '*.hbs' 'scripts/*' 'Makefile' | grep -v -E '^README\.pt-BR\.md$|^out/|^scripts/check-english\.sh$')
[ "$status" -eq 0 ] && echo "OK: documents read as English"
exit "$status"
