#!/bin/bash
# Prints the exported API of the abertura package in a stable text form:
# declarations and struct fields as `go doc -all` shows them, without comments.
# `make api-check` compares the output with abertura/API.txt; `make api-update`
# rewrites the file. Any change to the exported surface therefore shows up in
# the diff of the pull request and needs a CHANGELOG entry.
set -euo pipefail
cd "$(dirname "$0")/../abertura"
GOTOOLCHAIN="${GOTOOLCHAIN:-local}" go doc -all . | awk '
  /^(func|type|var|const) / { print; next }   # top-level declarations
  /^\t[A-Z]/                { print; next }   # exported fields and grouped consts/vars
  /^\t\/\//                 { next }          # comments inside blocks
  /^\}/ || /^\)/            { print; next }   # block ends
'
