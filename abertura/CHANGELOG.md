# Changelog of the Go SDK

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). SDK versions use the tags `abertura/vX.Y.Z` (up to 0.1.0 the tags were `go/vX.Y.Z`).

## [Unreleased]

### Breaking

The module moved from `go/` to `abertura/`, the whole public API is now in English, and the error catalogue anchors changed. There are no known consumers of 0.1.0. Update the import path and apply the tables below.

Module, package and tags

| Old | New |
|---|---|
| `github.com/MarcoBuarque/abertura-mercado-energia/go` | `github.com/MarcoBuarque/abertura-mercado-energia/abertura` |
| tags `go/vX.Y.Z` | tags `abertura/vX.Y.Z` |
| package `contrato` (`go/contrato`) | package `contract` (`abertura/contract`) |
| files `orcamento.go`, `escolha.go` (contract package) | `budget.go`, `choice.go` |

Package-level identifiers

| Old | New |
|---|---|
| `CatalogoErrosURL` (ends in `/erros`) | `ErrorCatalogURL` (ends in `/errors`) |
| `var Sandbox`, `var Production` | `func Sandbox() Environment`, `func Production() Environment` (each call returns a fresh copy) |
| `RateLimits{Default, Medicoes}` | `RateLimits{Default, Measurements}` |
| `ErrCredencialInvalida` | `ErrInvalidCredential` |
| `ErrValidacaoCampos` | `ErrFieldValidation` |
| `ErrPerfilVarejistaObrigatorio` | `ErrRetailerProfileRequired` |
| `ErrMigracaoSolicitada` | `ErrMigrationAlreadyRequested` |
| `ErrFaixaDeMesesObrigatoria` | `ErrMonthRangeRequired` |
| `ErrFaixaDeMesesLargaDemais` | `ErrMonthRangeTooWide` |
| `ErrArgumentoInvalido` | `ErrInvalidArgument` |
| `ErrCursorRepetido` | `ErrRepeatedCursor` |
| `ParseMes` | `ParseYearMonth` |
| `MesDe(ano, mes)` | `NewYearMonth(year, month)` |
| `WithAgent(agente, perfil)` | `WithAgent(agent, profile)` |
| `DocumentoCPF(numero)` | `CPF(number)` |
| `DocumentoCNPJ(numero)` | `CNPJ(number)` |

Types, fields and methods

| Old | New |
|---|---|
| `ChaveNatural` | `NaturalKey` |
| `ChaveMigracao{UC, Concessionaria, Mes, Perfil}` | `MigrationKey{ConsumerUnit, Distributor, Month, Profile}` |
| `ChaveEdicao{ID, Desejado}` | `EditKey{ID, Desired}` |
| `Client{Migracoes, Medicoes}` | `Client{Migrations, Measurements}` |
| `Documento`, `Numero()`, `Tipo()` | `Document`, `Number()`, `Type()` |
| `TipoDocumento`, `TipoCPF`, `TipoCNPJ` | `DocumentType`, `DocumentTypeCPF`, `DocumentTypeCNPJ` |
| `EdicaoMigracao{Mes, Perfil, Documento, Email, Status}` | `MigrationEdit{Month, Profile, Document, Email, Status}` |
| `FiltroMedicoes{UC, Concessionaria, Inicio, Fim, Situacao}` | `MeasurementFilter{ConsumerUnit, Distributor, From, To, Status}` |
| `FiltroMigracoes{De, Ate, UC, Status, Perfil}` | `MigrationFilter{From, To, ConsumerUnit, Status, Profile}` |
| `Medicao{ID, UC, Concessionaria, Dia, Referencia, AtualizadaEm, Consumo, TipoConsumo, Situacao}` | `Measurement{ID, ConsumerUnit, Distributor, Day, Reference, UpdatedAt, Consumption, ConsumptionType, Status}` |
| `Medicoes`; `Listar`, `ListarPagina` | `Measurements`; `List`, `ListPage` |
| `Mes{Ano, Mes}` | `YearMonth{Year, Month}` |
| `Migracao` | `Migration` |
| `Migracao.UC`, `.Concessionaria`, `.Supridora`, `.Varejista`, `.Perfil`, `.Documento`, `.Mes` | `.ConsumerUnit`, `.Distributor`, `.Supplier`, `.Retailer`, `.Profile`, `.Document`, `.Month` |
| `Migracao.Submercado`, `.ValorDHC`, `.ValorMUSD`, `.PagamentoMulta` | `.Submarket`, `.DHCAmount`, `.MUSDAmount`, `.PenaltyPayment` |
| `Migracao.DataDenuncia`, `.DataSolicitacao`, `.DataValidacao` | `.TerminationNoticeDate`, `.RequestDate`, `.ValidationDate` |
| `Migracao.Justificativa`, `.Comentario`, `.TipoConectado` | `.Justification`, `.Comment`, `.ConnectionType` |
| `Migracao.IDCelebracaoCER`, `.IndicadorProcessoSiga` | `.CERSigningID`, `.SIGAProcessIndicator` |
| `Migracoes`; `Listar`, `ListarPagina`, `Obter`, `Criar` | `Migrations`; `List`, `ListPage`, `Get`, `Create` |
| `Migracoes.Editar(ctx, id, mudar func(*EdicaoMigracao))` | `Migrations.Edit(ctx, id, change func(*MigrationEdit))` |
| `ConferirCriacao`, `ConferirEdicao` | `CheckCreated`, `CheckEdited` |
| `NovaMigracao{UC, Concessionaria, Documento, Email, Mes, DataDenuncia, Comentario}` | `MigrationRequest{ConsumerUnit, Distributor, Document, Email, Month, TerminationNoticeDate, Comment}` |
| `Pagina[T]{Itens, Proxima}` | `Page[T]{Items, Next}` |
| `SimNao`, `Sim`, `Nao` | `YesNo`, `Yes`, `No` (JSON values `SIM` and `NAO` unchanged) |
| `SituacaoMedicao`, `SituacaoConsistida`, `SituacaoRejeitada`, `Conhecido()` | `MeasurementStatus`, `MeasurementStatusConsistent`, `MeasurementStatusRejected`, `Known()` |
| `StatusMigracao`, `Conhecido()` | `MigrationStatus`, `Known()` |
| `StatusCriada`, `StatusConcluida`, `StatusReprovada` | `MigrationStatusCreated`, `MigrationStatusCompleted`, `MigrationStatusRejected` |
| `StatusEncerrada`, `StatusCancelada`, `StatusExpirada` | `MigrationStatusClosed`, `MigrationStatusCanceled`, `MigrationStatusExpired` |
| `StatusAprovadaComPendencia` | `MigrationStatusApprovedWithPendingIssues` |

Unchanged: `NewClient`, `Client.Do`, `Decimal` and its methods, `APIError`, `Problem`, `ProblemFrom`, `ProblemContentType`, `UncertainResultError`, `ErrNotFound`, `ErrRateLimited`, `Environment`, `RateLimit`, `Option`, `CallOption`, `AgentOption`, `TimeoutOption` and the other `With*` functions.

Error texts and `Problem` anchors

- Every error text, `Problem` title and detail, catalogue text and log message is now in English. The sentinel texts keep the API code in parentheses, for example `abertura: invalid credential (ERR_CREDENCIAL_INVALIDA)`. The API codes (`ERR_*`), JSON field names, paths and enum values are unchanged.
- `Problem.Type` fragments (the anchors of the error catalogue, now served at `/errors/`):

| Old | New |
|---|---|
| `#argumento-invalido` | `#invalid-argument` |
| `#chamada-cancelada` | `#call-canceled` |
| `#codigo-desconhecido` | `#unknown-code` |
| `#credencial-invalida-autorizador` | `#invalid-credential-authorizer` |
| `#credencial-invalida-backend` | `#invalid-credential-backend` |
| `#cursor-repetido` | `#repeated-cursor` |
| `#erro-de-transporte` | `#transport-error` |
| `#erro-interno` | `#internal-error` |
| `#faixa-de-meses-larga-demais` | `#month-range-too-wide` |
| `#faixa-de-meses-obrigatoria` | `#month-range-required` |
| `#falha-ao-obter-token` | `#token-request-failed` |
| `#falha-do-gateway` | `#gateway-failure` |
| `#fora-do-catalogo` | `#not-in-catalog` |
| `#limite-de-requisicoes` | `#rate-limited` |
| `#migracao-solicitada` | `#migration-already-requested` |
| `#nao-encontrado` | `#not-found` |
| `#perfil-varejista-obrigatorio` | `#retailer-profile-required` |
| `#recusa-sem-codigo` | `#refusal-without-code` |
| `#resposta-ilegivel` | `#unreadable-response` |
| `#resultado-incerto` | `#uncertain-result` |
| `#tempo-esgotado` | `#timeout` |
| `#validacao-campos` | `#field-validation` |

### Changed

- The Go module path is now `github.com/MarcoBuarque/abertura-mercado-energia/abertura`, and the package is imported without an alias.
- All documentation, comments, godoc, tests and diagnostics are in English; the only Portuguese document is the root `README.pt-BR.md`.
- The User-Agent product token is now `abertura-mercado-energia/<version>` (it was `abertura-mercado-energia-go/<version>`).
- The error catalogue moved to `/errors/` (`ErrorCatalogURL`); `/erros/` only redirects.
- `Sandbox` and `Production` are functions, so no mutable package-level state remains.

## [0.1.0] - 2026-11-01

First release. It follows the spec `openapi/v0.1.0`. Pre-releases: `go/v0.1.0-rc.1` followed `openapi/v0.1.0-rc.2`; `go/v0.1.0-rc.2` (no published release: the tag CI failed on the old govulncheck) and `go/v0.1.0-rc.3` followed `openapi/v0.1.0-rc.3`. Requires Go 1.26 or newer.

Names in this section are those of 0.1.0; see the tables under Unreleased for the current ones.

### Added

- `NewClient` with the environment as the first argument (`Sandbox`, `Production` or a custom `Environment`, such as the mock's) and the options `WithCredentials`, `WithTokenSource`, `WithAgent`, `WithTimeout`, `WithHTTPClient`, `WithRetries`, `WithRateLimits`, `WithLogger` and `WithUserAgent`.
- Client credentials token with a cache, renewed 30 s before it expires; on a 403 `ERR_CREDENCIAL_INVALIDA` from the authorizer with a token that was already accepted, it renews and retries once.
- Local per-process rate limiter: 900 requests every 5 minutes in the sandbox; in production, also 9,000 for measurements.
- `Migracoes`: `Listar`, `ListarPagina`, `Obter`, `Criar`, `Editar`, `ConferirCriacao` and `ConferirEdicao`. `Medicoes`: `Listar` and `ListarPagina`. `Client.Do` for routes without a method, such as the change request ones.
- Retries only on reads, except when nothing was sent or the authorizer refused before the backend. A write with no known result returns `*UncertainResultError` with the natural key.
- `*APIError` errors with `Code`, `Message`, `RequestID` and `GatewayErrorType`, and the sentinels `ErrCredencialInvalida`, `ErrValidacaoCampos`, `ErrPerfilVarejistaObrigatorio`, `ErrMigracaoSolicitada`, `ErrNotFound`, `ErrRateLimited` and `ErrCursorRepetido`.
- Types `Migracao`, `Medicao`, `Mes` (three month formats), `Documento` (masked in `fmt`, `slog` and JSON) and `Decimal` (exact, accepts a number or text).
- Optional `slog` logging, with no query, body, token, credentials or personal data.
- `Problem`, `ProblemFrom`, `APIError.Problem`, `UncertainResultError.Problem`, `ProblemContentType` and `CatalogoErrosURL`: the SDK errors in the RFC 9457 format, linked to the `docs/erros.md` catalogue.
- Transport failures (untrusted TLS, connection closed without a response, attempt deadline) and unreadable 2xx responses (interrupted body, invalid JSON, unreadable field) enter the catalogue: `ProblemFrom` classifies them as `#erro-de-transporte`, `#tempo-esgotado` or `#resposta-ilegivel` (new entry) instead of `about:blank`. The error texts and `errors.As`/`errors.Is` on the cause do not change.
- `ErrArgumentoInvalido`: matches the local validation done before sending, which includes a body that does not become JSON, an invalid path and the `NewClient` validation errors. The error texts do not change.
- `ProblemFrom` no longer returns `about:blank`: anything without its own entry uses `#fora-do-catalogo`. Token responses without `access_token` and errors from the `WithTokenSource` source use `#falha-ao-obter-token`. The JSON of a hand-built `Problem` omits an empty `type` and, with `about:blank`, uses the standard phrase of the status as `title` (RFC 9457, 4.2.1).
- Keys and errors do not leak personal data through `fmt` or `slog`: `ChaveMigracao` and `ChaveEdicao` gain `LogValue` and `GoString`, without the consumer unit or e-mail; `APIError` and `UncertainResultError` gain `GoString`, without the body or key. The API `Message` remains in `Error()`. The `json.Marshal` of the keys carries the data in clear, so that the check can be stored.
- `Migracao` and `Medicao` gain `String` and `GoString`, and `NovaMigracao`, `EdicaoMigracao`, `FiltroMigracoes` and `FiltroMedicoes` gain `String`, `GoString` and `LogValue`, all without the consumer unit, e-mail, document, comment or consumption: `%v`, `%+v`, `%#v`, `%s` and `slog` (including a `[]Migracao` in the text handler) show only id, status, month, distributor and the like. The fields remain exported and `json.Marshal` does not change, so it carries the data in clear; with the `slog` JSON handler, log the id and not the whole list.
- The token request never follows redirects (a 307 would resend the `client_secret`), also with `WithHTTPClient`: the SDK uses a copy of your client with its own `CheckRedirect`. The `CheckRedirect` of your client applies only to API calls.
- The token request carries only `client_id` and `client_secret` in the form, without `grant_type`: the endpoint refused `grant_type` with a 400 on 01/11/2026 and accepted the reduced form. `expires_in` is read as a number or text, and the token request does not follow redirects.
- The month range of `Migracoes.Listar` and `ListarPagina` is mandatory: the API answers 400 `ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO` without `mesReferenciaInicial` and `mesReferenciaFinal`. The SDK validates `FiltroMigracoes.De` and `Ate` before sending: missing, or `Ate` before `De`, gives `ErrArgumentoInvalido`, with no request. `FiltroMedicoes` already required `Inicio` and `Fim`.
- `ErrFaixaDeMesesObrigatoria` (`ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO`, reachable only through `Client.Do`) and `ErrFaixaDeMesesLargaDemais` (`ERR_MES_REFERENCIA_INICIAL_DIFERENCA`), with the entries `#faixa-de-meses-obrigatoria` and `#faixa-de-meses-larga-demais` in the catalogue. Both are final 400s: neither reads nor writes are retried.
- `ParseMes` also accepts the date `2026-12-01` and the date-time without offset `2026-12-01T00:00:00` (with or without fractional seconds). Both formats are tolerance: the sandbox returned `dataReferencia` as RFC 3339 with an offset (D21), and the `Obter` failure has another cause (D23); they stay because they are harmless. For the same reason, the day or date-time fields (`dataSolicitacao`, `dataValidacao`, `dataDenuncia`) also accept a date-time without offset (`2026-12-01T00:00:00`, with or without fractional seconds), read at -03:00 like the day-only date.
- `Migracoes.Obter` picks the item by `IdMigracao` when the API returns more than one (one matches: returns it; none: `ErrNotFound`). If several match and are all equal after decoding, it returns the first. If they differ only in `dataReferencia` (one entry per reference month, observed on 01/11/2026, D23), it returns the one with the most recent month; if they differ in any other field, or a month cannot be read, it fails with an unreadable response (`#resposta-ilegivel`). To read all the rows, use `Client.Do`.
- The contract test (`go/contrato`) starts the range search at 12 months and prints the item count of the root (`itens na raiz: N`) and, for the detail, whether the entries are equal, the names of the fields that differ and the fixed text of the SDK's own errors, without values.
- Any 5xx, except 501, follows the 500 line: reads retry, writes become uncertain (`*UncertainResultError`). The 501 is a final error.
- `SituacaoRejeitada` (`REJEITADA`) joins the known measurement statuses.
- A page that points to the cursor it just used (what the mock does) ends with `ErrCursorRepetido` without delivering its items, which would repeat the previous page. A page that points to a cursor seen before (cycle A, B, A) delivers its items and only then returns `ErrCursorRepetido`, without fetching again.

### Known limits

- The maximum distance between `De` and `Ate` is between 12 and 24 months (probably 12; the exact cap was not measured): 12 were accepted and 24, 48 and 96 refused (`ERR_MES_REFERENCIA_INICIAL_DIFERENCA`) on 01/11/2026. Whoever lists a wide range must be ready for `ErrFaixaDeMesesLargaDemais` and split the query.
- The GET by id returns one entry per reference month of the migration (01/11/2026: two, differing only in `dataReferencia`, D23). The API does not say which month is current; the SDK returns the most recent, and `Migracao` has a single `Mes`. Entries that differ in another field make `Obter` fail with `#resposta-ilegivel`.
- Pagination by `indexProximaPagina` has never been seen working: a 12-month range returned 71 and then 92 items in a single page (page size of at least 92) and no cursor; the cursor was never observed. A repeated cursor ends the iteration with `ErrCursorRepetido`.
- The SDK write methods (`Criar`, `Editar`) never ran against the sandbox: the 30/10/2026 writes were made by the investigation scripts, not by the SDK, and the 01/11/2026 rounds were read-only (the first stopped at the token). What the writes really do in the API has not been confirmed by this SDK.
- When the migration detail has one row per reference month, `Obter` picks the most recent month and `Editar` sends it back in the full PUT body (PUT semantics not verified). If the migration spans more than one month, set `EdicaoMigracao.Mes` explicitly.
- It is not known whether the edit PUT is partial or total: `Editar` reads the migration and sends the whole object, with no protection against another write in between.
- The unit of `Consumo` is not declared by the API.
- The limiter is per process; several processes on the same IP or agent add up requests without it knowing.

[0.1.0]: https://github.com/MarcoBuarque/abertura-mercado-energia/releases/tag/go/v0.1.0
[Unreleased]: https://github.com/MarcoBuarque/abertura-mercado-energia/compare/go/v0.1.0...HEAD
