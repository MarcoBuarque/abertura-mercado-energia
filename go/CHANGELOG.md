# Changelog do SDK Go

Formato baseado em [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/). Versões do SDK usam as tags `go/vX.Y.Z`.

## [Não lançado]

### Adicionado

- `Problem`, `ProblemFrom`, `APIError.Problem`, `UncertainResultError.Problem`, `ProblemContentType` e `CatalogoErrosURL`: os erros do SDK no formato da RFC 9457, ligados ao catálogo `docs/erros.md`.
- `ErrArgumentoInvalido`: casa com a validação local feita antes de enviar. O texto dos erros não muda.
