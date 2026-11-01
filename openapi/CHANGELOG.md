# Spec changelog

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Spec versions use the tags `openapi/vX.Y.Z`.

This is an **unofficial**, hand-written description. The project is independent and is not affiliated with, sponsored or endorsed by the CCEE. The spec describes what the API returned in sandbox tests, and does not replace the operator's documentation.

## [Unreleased]

### Breaking

Everything in the spec is now named in English. Wire names do not change: JSON field names, enum values, `ERR_*` error codes, paths, HTTP methods, parameter `name:` values (query, path and header names) and header names are exactly as the API has them. A client generated from the spec sees renamed types, methods and tags, not a different API on the wire.

Schemas (`components.schemas`):

| Old | New |
|---|---|
| `Erro` | `Error` |
| `ErroOAuth2` | `OAuth2Error` |
| `TokenRequisicao` | `TokenRequest` |
| `Token` | `Token` (unchanged) |
| `StatusMigracao` | `MigrationStatus` |
| `MigracaoResumo` | `MigrationSummary` |
| `ListaMigracoes` | `MigrationList` |
| `MigracaoDetalhe` | `MigrationDetail` |
| `NovaMigracao` | `MigrationRequest` |
| `EdicaoMigracao` | `MigrationEdit` |
| `Medicao` | `Measurement` |
| `ListaMedicoes` | `MeasurementList` |
| `Solicitacao` | `ChangeRequest` |
| `ListaSolicitacoes` | `ChangeRequestList` |
| `SolicitacaoRepresentante` | `RepresentativeChangeRequest` |
| `SolicitacaoSuspensao` | `SupplySuspensionRequest` |
| `DecisaoSolicitacao` | `ChangeRequestDecision` |

Parameters (`components.parameters`; the `name:` of each one is unchanged):

| Old | New |
|---|---|
| `CodigoAgente` | `AgentCode` |
| `CodigoPerfil` | `ProfileCode` |
| `IdMigracao` | `MigrationID` |
| `IdSolicitacaoAlteracao` | `ChangeRequestID` |
| `CodigoPerfilVarejistaQuery` | `RetailerProfileCodeQuery` |
| `MesReferenciaInicial` | `ReferenceMonthStart` |
| `MesReferenciaFinal` | `ReferenceMonthEnd` |
| `MesReferenciaInicialObrigatorio` | `ReferenceMonthStartRequired` |
| `MesReferenciaFinalObrigatorio` | `ReferenceMonthEndRequired` |
| `CodigoUnidadeConsumidoraQuery` | `ConsumerUnitCodeQuery` |
| `CodigoUnidadeConsumidoraMedicaoQuery` | `ConsumerUnitCodeMeasurementQuery` |
| `CodigoAgenteConcessionariaQuery` | `DistributorAgentCodeQuery` |
| `StatusMigracaoQuery` | `MigrationStatusQuery` |
| `DataInicio` | `StartDate` |
| `DataFim` | `EndDate` |
| `SituacaoMedicaoQuery` | `MeasurementStatusQuery` |
| `TipoSolicitacaoQuery` | `ChangeRequestTypeQuery` |
| `SituacaoAlteracaoQuery` | `ChangeRequestStatusQuery` |
| `DataSolicitacaoInicial` | `RequestDateStart` |
| `DataSolicitacaoFinal` | `RequestDateEnd` |
| `IndexProximaPagina` | `NextPageIndex` |

Responses (`components.responses`; the `RequestId` header is unchanged):

| Old | New |
|---|---|
| `Requisicao400` | `BadRequest400` |
| `Acesso403` | `Forbidden403` |
| `Limite429` | `TooManyRequests429` |
| `Interno500` | `InternalError500` |
| `SolicitacaoCriada200` | `ChangeRequestCreated200` |

Component examples (`components.examples`) and inline example keys, renamed for the same reason (the example values are unchanged apart from the prose):

| Old | New |
|---|---|
| `ErroValidacaoCampos` | `FieldValidationError` |
| `ErroPerfilObrigatorio` | `RetailerProfileRequiredError` |
| `ErroUcInvalida` | `InvalidConsumerUnitError` |
| `ErroMigracaoSolicitada` | `MigrationAlreadyRequestedError` |
| `ErroFaixaObrigatoria` | `MonthRangeRequiredError` |
| `ErroFaixaLargaDemais` | `MonthRangeTooWideError` |
| `ErroCredencialInvalida` | `InvalidCredentialError` |
| `ErroLimite` | `RateLimitError` |
| `ErroInterno` | `InternalFailureError` |
| `MigracaoDetalheCriada` | `MigrationDetailCreated` |
| `SolicitacaoCriada` | `ChangeRequestCreated` |
| `pagina-1`, `pagina-2` | `page-1`, `page-2` |
| `perfil-obrigatorio`, `validacao-campos`, `faixa-obrigatoria`, `faixa-larga-demais` | `profile-required`, `field-validation`, `range-required`, `range-too-wide` |
| `uc-invalida`, `migracao-solicitada`, `credencial-invalida` | `invalid-consumer-unit`, `migration-already-requested`, `invalid-credential` |
| `criada`, `migracao`, `editada`, `edicao`, `lista`, `nova` | `created`, `migration`, `edited`, `edit`, `list`, `new` |
| `troca`, `suspensao`, `decisao`, `solicitacao`, `formularioRecusado`, `limite`, `interno` | `change`, `suspension`, `decision`, `change-request`, `rejectedForm`, `rate-limit`, `internal` |

`operationId`:

| Old | New |
|---|---|
| `obterToken` | `getToken` |
| `listarMigracoes` | `listMigrations` |
| `criarMigracao` | `createMigration` |
| `obterMigracao` | `getMigration` |
| `editarMigracao` | `editMigration` |
| `listarMedicoes` | `listMeasurements` |
| `listarSolicitacoes` | `listChangeRequests` |
| `solicitarTrocaRepresentante` | `requestRepresentativeChange` |
| `solicitarSuspensaoFornecimento` | `requestSupplySuspension` |
| `decidirSolicitacao` | `decideChangeRequest` |

Tags:

| Old | New |
|---|---|
| `Autenticacao` | `Authentication` |
| `Migracoes` | `Migrations` |
| `Medicoes` | `Measurements` |
| `Solicitacoes` | `Change requests` |

Extensions:

| Old | New |
|---|---|
| `x-confianca` (values `alta`, `media`, `baixa`) | `x-confidence` (values `high`, `medium`, `low`) |
| `x-verificado-em` | `x-verified-at` |
| `x-divergencias` | `x-divergences` |
| `x-fonte` | `x-source` |
| `x-fonte` keys `colecao`, `extraida-em`, `teste-no-sandbox-em` (in `info`) | `collection`, `extracted-at`, `sandbox-tested-at` |
| `x-extensible-enum`, `x-display`, `x-amzn-*` | unchanged |

The file `FONTES.md` is now `SOURCES.md`; the `x-divergences` references and the divergence numbers (D1 to D23) are unchanged. The error catalog moved from `/erros` to `/errors`, and its anchors are in English (for example `#faixa-de-meses-obrigatoria` is now `#month-range-required`). The Redoc banner now reads "unofficial description".

### Changed

- All prose is in English: `info`, summaries, descriptions, titles, tag and server descriptions, and the prose inside examples and extensions (the invented examples now start with "Invented:"). Invented data examples (UC with leading zeros, codes in the 900000 range, repeated-digit documents, `example.org` e-mails), enum values, `ERR_*` codes and the `message` field (which the API returns in Portuguese) are unchanged.
- `info.version` is `0.2.0`.
- `info.contact.url` points to the project repository and a top-level `externalDocs` points to the documentation site.
- The lint rules (`openapi/vacuum.yaml`) follow the new extension names and have English messages; the rule ids are unchanged.

## [0.1.0] - 2026-11-01

First version. Ten operations, in four groups. The confidence column is the operation's `x-confianca`: `alta` means the behavior was checked in the sandbox (2026-10-30 and five rounds on 2026-11-01); `media`, that the operation came only from the public Postman collection and was not exercised (or was, but with no items in the response). Pre-releases: `openapi/v0.1.0-rc.1`, `-rc.2` and `-rc.3`; the changes between them are under "Changed". Names in this section are the 0.1.0 names, in Portuguese; see "Breaking" under Unreleased for the English ones.

| `operationId` | Method and path | Confidence |
|---|---|---|
| `obterToken` | `POST /sso/oauth/token` | alta (form with only `client_id` and `client_secret`) |
| `listarMigracoes` | `GET /v1.1/varejista/migracoes` | alta (the page cursor, baixa) |
| `criarMigracao` | `POST /v1.1/varejista/migracoes` | alta |
| `obterMigracao` | `GET /v1.1/varejista/migracoes/{idMigracao}` | alta (one entry per reference month) |
| `editarMigracao` | `PUT /v1/varejista/migracoes/{idMigracao}` | media |
| `listarMedicoes` | `GET /v1/varejista/consumo/medicoes` | media (empty list in the sandbox; the list item, baixa) |
| `listarSolicitacoes` | `GET /v1/solicitacoes` | alta |
| `solicitarTrocaRepresentante` | `POST /v1/solicitacoes/representante` | media |
| `solicitarSuspensaoFornecimento` | `POST /v1/solicitacoes/suspensao-fornecimento-varejista` | media |
| `decidirSolicitacao` | `POST /v1/solicitacoes/{idSolicitacaoAlteracao}` | media |

### Added

- The ten operations above, with invented examples (UC with leading zeros, codes in the 900000 range, `example.org` e-mails).
- 400, 403, 429 and 500 responses on every operation. Only the 400 and 403 were seen in the sandbox; 429 and 500 have `x-confianca: baixa`.
- The extensions `x-confianca`, `x-verificado-em`, `x-fonte`, `x-divergencias` and `x-extensible-enum`.
- Twenty-three divergences between the public documentation and what was observed (D1 to D23), numbered in `FONTES.md`, and the section "Contract round of 2026-11-01" with the five rounds against the sandbox.
- Schema `ErroOAuth2` (`error`, `error_description`) for the 400 of `obterToken`, which does not use `Erro` (D17).
- Codes `ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO` and `ERR_MES_REFERENCIA_INICIAL_DIFERENCA` in `Erro.error`, with descriptions and links to the catalog (D18, D19).

### Changed

Changes between the pre-releases (rc.1 to rc.3), recorded here because pre-releases do not count as a published version.

- `TokenRequisicao` without `grant_type`: the form with it was rejected with 400 on 2026-11-01 (D17). The 400 of `obterToken` now uses `ErroOAuth2`.
- `mesReferenciaInicial` and `mesReferenciaFinal` of `listarMigracoes` become `required` (D18); the range ceiling is between 12 and 24 months, probably 12 (D19, D22). The 12-month range returned 71 and then 92 items in a single page, with no cursor. The request list keeps the filters optional.
- `obterMigracao`: the response is a list with one entry per reference month of the migration; on 2026-11-01 two came back, differing only in `dataReferencia` (D23). The API does not say which month is the current one.
- `MigracaoResumo.mesReferencia`, `MigracaoDetalhe.dataReferencia`, `dataSolicitacao`, `dataValidacao` and `dataDenuncia` (list and detail) gain `format: date-time` and the observed format (D21); `mesReferencia` and `dataReferencia` with `x-confianca: alta`. `Erro.timestamp` describes the variable-length fraction; `Token.expires_in` confirmed with three digits.
- `x-verificado-em` moves to 2026-11-01 in `obterToken`, `listarMigracoes`, `obterMigracao` and `listarMedicoes` (the last still has an empty list and `media` confidence).
- Descriptions of the `Erro` schema (one code per line) and of the 400, 403, 429 and 500 responses point to the error catalog (`docs/erros.md`). Text only: format and types do not change.

### Fixed

- The spec said `grant_type` was optional on the token and that the migration month filters were optional; the sandbox showed the opposite (D17, D18).

### Known limits

- The page cursor (`indexProximaPagina`) has never been seen with a value; the page size is at least 92 items (D22).
- The exact ceiling of the month range between 12 and 24 was not measured (D22).
- Which month of a migration's detail is the current one is not documented (D23).
- Writes were only verified on 2026-10-30; the rounds of 2026-11-01 were read-only.
- Response enums are open (`x-extensible-enum`); parameter and request-body enums are closed.
- `consumo` and `codigoAgenteConcessionaria` of `Medicao` have no fixed type, and the unit of `consumo` is unknown.
- The error responses for rate limiting (429) and internal failure (500) are hypotheses.

### Breaking

None. The breaks between the pre-releases (rc.1 to rc.2: the token 400 without `message`, and the mandatory month filters of `listarMigracoes`) are under "Changed" and do not count against a published version. From here on, a break in 0.x is accepted when recorded under `### Breaking` of `[Unreleased]`; the PR label `breaking-spec` declares it and CI checks the declaration against that section (`scripts/breaking-spec.sh`).

[0.1.0]: https://github.com/MarcoBuarque/abertura-mercado-energia/releases/tag/openapi/v0.1.0
[Unreleased]: https://github.com/MarcoBuarque/abertura-mercado-energia/compare/openapi/v0.1.0...HEAD
