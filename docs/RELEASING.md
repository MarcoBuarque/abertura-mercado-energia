# Releasing

Two artifacts, two tag prefixes: `openapi/vX.Y.Z` (spec, Redoc page, mock image) and `abertura/vX.Y.Z` (Go module). Never a bare `vX.Y.Z` and never `go/v*`.

## Before any tag (also release candidates)

1. **Read the exported API end to end.** Run `make api-check`; then read `abertura/API.txt` as a user would: one language, no field repeating its type name (`YearMonth.Month` is fine, `Month.Month` is not), no package-level mutable state, every name a plain English word or an accepted acronym (CPF, CNPJ, DHC, MUSD, CER, SIGA). Anything odd is fixed before the tag, not listed for later.
2. `make ci` green locally and the `go` and `openapi` workflows green on GitHub for the commit you will tag.
3. CHANGELOG: the version section exists with the date (the `breaking-spec` PR label is checked against `[Unreleased]` only, so a PR that already dates the section drops the label, or the release commit comes after the merge); renamed or removed exported identifiers are under `### Breaking` (SDK) and schema/operationId renames under `### Breaking` (spec). `make check-tag` verifies the entry.
4. `README.md` and `README.pt-BR.md` show the same install line and the same example names.
5. Pages: catalog slugs in `docs/errors.md` match `ErrorCatalogURL` fragments (`make docs` (runs `scripts/verify-errors.sh`)).

## Order

`openapi/vX.Y.Z-rc.1` → release workflow green → `abertura/vX.Y.Z-rc.1` → release workflow green and `go get` of the rc works → final tags in the same order. Tag only with `git tag -s` or `-a` on a commit already on `main`, after the remote CI is green (`gh run watch`), never in the same shell line as the push of the commit.

## Guards that run in `make ci`

- `api-check`: exported surface equals `abertura/API.txt`. Also runs in the `go` workflow (Go 1.26 entry), as does `check-english`.
- A test on the `go.mod` module path: it must equal the import path the docs show.
- `naming_test.go` in the package: Portuguese words in identifiers, field/type stutter, non-ASCII in non-test sources, missing doc comments on exported names.
- `golangci-lint` with `gochecknoglobals`, `revive` (exported, var-naming, receiver-naming, package-comments, error-naming, redundant-import-alias) and `misspell` (US English).
- `check-english`: documents and the spec read as English (`README.pt-BR.md` exempt).
- `check-trademark`: the operator's name only in the non-affiliation notice.
