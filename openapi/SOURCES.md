# Sources and divergences of the spec

Independent, unofficial project. This file says where each statement in `abertura.yaml` comes from and where the public documentation and the observed behavior diverge. Nothing here reproduces text from the operator's documentation: only field names, routes, `ERR_*` codes and descriptions written by us.

## Sources

| Source | What it gives | Date and time |
|---|---|---|
| Public Postman collection "Abertura de Mercado" (<https://documenter.getpostman.com/view/30322966/2sAXqqchhJ>), published on 2025-05-16 | Routes, field names, enums and short descriptions | extracted on 2026-10-30 at 10:52 (-03:00) |
| Manual test in the sandbox environment, with 38 requests | Real format of responses and errors, types, nulls, validations and authentication rules | 2026-10-30, from 11:56 (-03:00) |
| Five contract rounds in the sandbox, with 6, 7, 10, 7 and 7 requests (limit of 90 per round) | Token form, mandatory month range and its ceiling, migration detail, date formats | 2026-11-01 (see the round section) |

Precedence rule: **what was observed in the sandbox beats the collection**. What only the collection shows carries `x-confidence: low` (or `medium`, when the whole operation was not exercised).

The sandbox data is fictitious and shared. No observed value (UC, agent and profile codes, documents, identifiers) was copied into the examples: all of them are invented, with leading-zero UCs, codes in the 900000 range and e-mails at `example.org`.

## Operations

| `operationId` | Method and path | `x-confidence` | `x-verified-at` | Divergences |
|---|---|---|---|---|
| `getToken` | `POST /sso/oauth/token` | high | 2026-11-01 | D15, D17 |
| `listMigrations` | `GET /v1.1/varejista/migracoes` | high | 2026-11-01 | D2, D8, D10, D14, D18, D19, D21, D22 |
| `createMigration` | `POST /v1.1/varejista/migracoes` | high | 2026-10-30 | D2, D10, D12, D14 |
| `getMigration` | `GET /v1.1/varejista/migracoes/{idMigracao}` | high | 2026-11-01 | D2, D10, D14, D20, D21, D23 |
| `editMigration` | `PUT /v1/varejista/migracoes/{idMigracao}` | medium | n/a | D10, D14 |
| `listMeasurements` | `GET /v1/varejista/consumo/medicoes` | medium | 2026-11-01 | D12, D14, D16 |
| `listChangeRequests` | `GET /v1/solicitacoes` | high | 2026-10-30 | D3, D5, D11, D14 |
| `requestRepresentativeChange` | `POST /v1/solicitacoes/representante` | medium | n/a | D4, D5, D13, D14 |
| `requestSupplySuspension` | `POST /v1/solicitacoes/suspensao-fornecimento-varejista` | medium | n/a | D13, D14 |
| `decideChangeRequest` | `POST /v1/solicitacoes/{idSolicitacaoAlteracao}` | medium | n/a | D14 |

Every operation declares the 400, 403, 429 and 500 responses. The last two were never observed and carry `x-confidence: low`.

## Divergences

Each item says what the collection claims, what was observed and what the spec adopts.

**D1. Host in the examples.** Some examples in the collection point to an internal staging host of the operator, and not to the sandbox or to production. The spec ignores that host and declares only the sandbox, the local mock and production in `servers`. Source: collection, example of the distributor's PUT v2.

**D2. Three month formats.** The reference month appears as `YYYY-MM` (filters and request bodies), as a date and time with a -03:00 offset always on day 1 (v1.1 list) and, in the detail, as `dataReferencia`, whose exact format the test did not record. The spec validates `YYYY-MM` where the server accepts it (requests and filters) and declares the response dates as `date-time` (RFC 3339 with offset, observed on 2026-11-01, D21). On the request list, the collection text gives `MM-YYYY` for the month filters and `DD-MM-YYYY` for the request dates, while the example URL of the same request uses `YYYY-MM` and `YYYY-MM-DD`; the spec follows the URL. In the sandbox, the migration month filters in `YYYY-MM` answered 200, but the test record does not say whether the returned items respected the range, and the request filters were not tested. Confirmed on 2026-11-01: with `mesReferenciaInicial` and `mesReferenciaFinal` equal to the same `YYYY-MM` month, the migration list returned 7 items, all with the month inside the filter. The request filters remain untested. Source: collection and sandbox tests.

**D3. Envelope of the request list.** Fixed. The collection suggests a plain list; the sandbox returns an object with the list in `solicitacao` (singular) and `indexProximaPagina`. The spec adopts the object. Source: sandbox test.

**D4. No `id` in the receiver's message.** The structure of the notification message documented for the receiver carries no identifier, which makes deduplication harder. It does not affect the 10 operations of this version; left for v0.2. Source: receiver documentation.

**D5. No answer to the change of representative.** The collection says that a change requested more than 90 days ahead (termination by notice, "resilição") is approved automatically, and more than 15 days ahead in the case of "resolução". The Commercialization Procedure (Procedimento de Comercialização) treats a missing answer as a compulsory transfer in the intended month. The spec adopts the Procedure, which is the rule, and records the collection's rule only as a divergence. Source: collection and Commercialization Procedure, item 3.37.3.

**D6. Error format.** Fixed. The collection shows two formats, one with generically named keys and another with `timestamp`, `status`, `error`, `message` and `content`. In both cases (backend and authorizer) the sandbox returns only `timestamp`, `status`, `error` and `message`, with no `content`. The backend uses a timestamp with milliseconds; the authorizer, without milliseconds and with spaces after `:`. The spec has an `Error` schema with the four fields; `error` is a stable textual code. Source: sandbox test.

**D7. The `codigoAgente` header.** The collection describes it as the distributor's code, but the retailer sends its own agent code, validated against the token. The spec describes it as "code of the agent making the call". Source: collection and sandbox test.

**D8. Profile also required in the query.** The collection lists the `codigoPerfilVarejista` of the migration list without marking it as required. Without it, the sandbox answers 400 `ERR_PERFIL_VAREJISTA_OBRIGATORIO`, even with the `codigoPerfil` header. The spec marks the parameter as `required`. Confirmed on 2026-11-01: the same 400 response came back on a new list without the profile. Source: sandbox tests.

**D9. 403 instead of 401.** An invalid token, or an agent or profile that does not belong to the credential, returns 403 `ERR_CREDENCIAL_INVALIDA`, with the `x-amzn-ErrorType: AccessDeniedException` header. No 401 was seen and there is no `www-authenticate`. The cases can only be told apart by the `message`. The spec declares 403 on every operation and does not declare 401. Source: sandbox test.

**D10. List x detail.** The schemas differ: the list uses `mesReferencia`, `codigoAgenteSupridora` and `idCelebracaoCER`; the detail uses `dataReferencia` and `codigoSupridora`, adds `tipoConectado` and does not carry `idCelebracaoCER`. The GET by identifier returns a one-item list. The spec has two schemas, `MigrationSummary` and `MigrationDetail`. Source: sandbox test.

**D11. Required request filters.** `tipoSolicitacao` is in fact required (the collection shows it as optional) and `situacaoAlteracao`, marked as required in the collection, is optional. The contracts endpoint, outside this spec, also required `codigoAgenteConcessionaria` in the sandbox; for measurements the spec marks it as required per the collection, without a test with items. The spec follows what was observed. Source: sandbox test and collection.

**D12. 15-digit UC.** The v1.1 migrations require a numeric 15-digit UC with a check digit (algorithm not documented; an invented value was rejected). The v1 measurements accepted a 10-digit UC. The spec applies `pattern` only to the v1.1 body and leaves the measurements' UC without `pattern`. Source: sandbox test and collection.

**D13. Names in the request body and response of the change requests.** The bodies for change of representative and for suspension use `codigoConcessionaria` and `codigoVarejistaNovo`; the response uses `codigoAgenteConcessionaria` and `codigoAgenteVarejistaNovo`. The spec keeps each name where it appears. Source: collection; response confirmed in the sandbox.

**D14. Code as string or integer.** In the collection's examples, agent, profile and distributor codes appear as strings; in the sandbox, they are integers. The spec uses integer and leaves the two `Measurement` fields (`consumo` and `codigoAgenteConcessionaria`), never observed, without a fixed type. The examples `page-1` (string) and `page-2` (number) show both forms. Source: collection and sandbox test.

**D15. Token validity.** The collection shows `expires_in` 298 in one example; the sandbox returned 300, with `refresh_expires_in` 0 (no refresh token). A 312-second-old token was still accepted, so the expired-token code was not observed. The spec records 300 and recommends renewing before expiry. Source: sandbox test.

**D16. Unit of `consumo`.** Neither the collection nor the sandbox (which returned empty measurements) declares the unit. The spec assumes none and marks the `Measurement` schema with `x-confidence: low`. Source: collection and sandbox test.

**D17. Token form.** The collection and the default `client_credentials` client send `client_id`, `client_secret` and `grant_type=client_credentials`. On 2026-11-01, six requests with the three fields came back 400; with only `client_id` and `client_secret` the request came back 200 (fields `access_token`, `expires_in`, `not-before-policy`, `refresh_expires_in`, `scope` and `token_type`). The body of this route's 400 is an OAuth2-style error object (`error`, `error_description`), and not the `Error` with `status`, `timestamp` and `message`. The spec removes `grant_type` from the form and documents the 400 with the `OAuth2Error` schema, whose `error` carries `x-confidence: medium` because it was not recorded which of the six RFC 6749 codes came back. Source: sandbox tests.

**D18. Mandatory month range.** The collection does not mark `mesReferenciaInicial` or `mesReferenciaFinal` of the migration list as required. With the profile and without the range, the sandbox answered 400 `ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO` (body in the `Error` format). The spec marks both as `required` on this operation, with dedicated parameters, so as not to affect the request list. Source: sandbox test on 2026-11-01.

**D19. Maximum distance between months.** A one-month range was accepted; a 96-month one (2020-01 to 2027-12) came back 400 `ERR_MES_REFERENCIA_INICIAL_DIFERENCA`. The measurement of 2026-11-01 refined the limit in D22. Source: sandbox test on 2026-11-01.

**D20. `dataReferencia` of the detail.** In the second round, the response of the GET by identifier matched the schema, but the SDK failed to read the body, and `dataReferencia` was suspected. The third round ruled the suspicion out: the field is RFC 3339 with an offset, no fraction, and `ParseYearMonth` reads it (D21). The cause of the failure is in D23. Source: sandbox tests on 2026-11-01.

**D21. Date and time formats of the responses.** By mask (`9` digit, `A` letter), on 2026-11-01: `mesReferencia` of the list `9999-99-99A99:99:99-99:99` (RFC 3339 with offset, no fraction; it is a date and time and not `YYYY-MM`, for example day 1 at midnight at -03:00); `dataReferencia` of the detail, the same mask; `dataSolicitacao` `9999-99-99A99:99:99.999-99:99` (three fractional digits); `dataValidacao` `9999-99-99A99:99:99-99:99` in one response and `….999-99:99` in another (fraction sometimes present); `dataDenuncia` `9999-99-99A99:99:99A` (RFC 3339 in UTC, with `Z`); `Error.timestamp` `9999-99-99A99:99:99.999-99:99` in most and `….99-99:99` in one (fraction of 2 or 3 digits, trailing zeros trimmed). The spec marks these fields as `format: date-time`, except `Error.timestamp`, which stays without `format` because of the variation (and because the authorizer uses another form). The query filters remain `YYYY-MM`. Source: sandbox tests on 2026-11-01.

**D22. Range ceiling and page size.** Ranges of 96, 48 and 24 months came back 400 `ERR_MES_REFERENCIA_INICIAL_DIFERENCA`; 12 months were accepted. The ceiling is between 12 and 24 months, probably 12. The 12-month range returned 71 items in a single page, with `indexProximaPagina` present and no cursor: the page holds at least 71 items and the cursor is still never seen (updates D19). In the fourth round, the same 12-month range returned 92 items in one page, with no cursor: the shared sandbox grows, and the page holds at least 92 items. Source: sandbox tests on 2026-11-01.

**D23. Repeated entries in the detail.** The GET by id returned 200, valid against the spec, and `Migrations.Get` failed until the response was understood. Confirmed in the fifth round (2026-11-01): the response had 2 items at the root, both with the requested id, differing only in `dataReferencia` (on 2026-10-30 it was one item). The detail apparently carries one row per reference month of the same migration, like the list, which can also be filtered by month. Which month is the current one is not documented; the SDK returns the one with the most recent month (an SDK rule, not an API fact) and, if the entries differ in another field, fails with `#unreadable-response`. To read all the rows, use `Client.Do`. Source: sandbox tests on 2026-11-01.

## Contract round of 2026-11-01

Five rounds against the sandbox, with the shared fictitious credentials and the collection's fictitious profile. No credential, token, UC, document or e-mail value is stored here.

- **Round 1, writes allowed (6 of 90 requests).** Every `POST /sso/oauth/token` with `client_id`, `client_secret` and `grant_type` came back 400 with an OAuth2 body. Nothing else ran.
- **Round 2, read-only (7 of 90).** The SDK now sent only the two fields. Results: token 200 (D17); migration list with one `YYYY-MM` month in 200, with 7 items inside the filter and no next cursor (D2); without a profile, 400 `ERR_PERFIL_VAREJISTA_OBRIGATORIO` (D8); with a profile and no range, 400 `ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO` (D18); migration detail in 200, matching the schema, but unreadable for the SDK (D20); measurements of the previous month in 200 with an empty list, as on 2026-10-30; 96-month range in 400 `ERR_MES_REFERENCIA_INICIAL_DIFERENCA` (D19).
- **Round 3, read-only (10 of 90).** Token 200 with only `client_id` and `client_secret` (`expires_in` with three digits); one-month list with 7 items and the date masks of D21; without a profile, 400 `ERR_PERFIL_VAREJISTA_OBRIGATORIO`; without a range, 400 `ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO`; detail by id in 200 and valid, but the SDK still failed (D23); measurements in 200 with an empty list; ranges of 96, 48 and 24 months in 400 `ERR_MES_REFERENCIA_INICIAL_DIFERENCA` and the 12-month one in 200 with 71 items in one page, no cursor (D22).
- **Round 4, read-only (7 of 90).** Token, one-month list (7 items), profile and range validations as in round 3; 12-month range accepted again, with 92 items (71 an hour earlier) and no cursor (D22); detail by id in 200 with 2 items at the root, both with the requested id, and `Get` failed with "more than one migration for the id" (D23); empty measurements.
- **Round 5, read-only (7 of 90).** Token, one-month list (7 items), profile and range validations and empty measurements as before; 12-month range with 92 items and no cursor (D22); detail by id in 200 with 2 items at the root that differ only in `dataReferencia`, apparently one row per reference month (D23).
- **Confirmed:** D2 (`YYYY-MM` filter), D8, the `indexProximaPagina` and `migracao` envelope, the detail as a one-item list.
- **Changed in the spec:** token form without `grant_type`; token 400 with `OAuth2Error`; mandatory month range on the migration list; two new codes in `Error.error`.
- **Still unverified:** `editMigration` and the PUT semantics (partial or total), the unit of `consumo`, the pagination cursor, which month of the detail is the current one (D23) and the exact ceiling of the range (D22). No write occurred in rounds 2 to 5.

## Notes on the enums

- `statusMigracao` and the other lists of response values use `x-extensible-enum`: the list is the known one and may grow. In query parameters and request bodies, `enum` is closed.
- `tipoSolicitacao`: the values come from the collection and have the prefix `ALTERACAO_ALFA_VAREJISTA_`, `ALTERACAO_RETORNO_CATIVO_` or `SUSPENSAO_FORNECIMENTO_`. Of the ten, only `SUSPENSAO_FORNECIMENTO_*` appeared in the sandbox.
- `situacaoMedicao`: `CONSISTIDA` and `REJEITADA`, from the collection only.
