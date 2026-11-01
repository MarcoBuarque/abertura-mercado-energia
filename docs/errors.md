# Error catalogue

> **Unofficial description.** This project is independent: it is not affiliated with, sponsored or endorsed by the API operator. The catalogue describes what the API returned in sandbox tests and what the Go SDK does with each error. The examples are invented.

This is the teaching source for every error you may see when using the API, the Go SDK (`abertura/`) or a client generated from `openapi/abertura.yaml`. Each entry has a stable anchor, such as `#migration-already-requested`; the `type` field of the SDK's Problem Details points to it.

## Two formats

The API returns errors with **four fields**: `timestamp`, `status`, `error` (a textual code such as `ERR_MIGRACAO_SOLICITADA`) and `message` (text in Portuguese, for humans), with `Content-Type: application/json`. The spec describes that format as it is, and the mock repeats it; that is why the spec does not declare `application/problem+json` in any response.

The Go SDK delivers the same error as **Problem Details** from [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) (`application/problem+json`): `type` points to an entry of this catalogue, `title` and `detail` are our own text, and `code`, `operation`, `requestId`, `retryable` and `uncertain` are extensions. The `detail` never repeats the API's `message`, which may mention a consumer unit, document or e-mail; it stays in `APIError.Message` for whoever wants it.

As the API returns it (invented values):

```json
{"timestamp":"2027-01-10T10:00:00.000-03:00","status":400,"error":"ERR_MIGRACAO_SOLICITADA","message":"Inventado: já existe migração ativa para a unidade consumidora"}
```

As the SDK delivers it, with `abertura.ProblemFrom(err)`:

```json
{"type":"https://marcobuarque.github.io/abertura-mercado-energia/errors#migration-already-requested","title":"Migration already requested","status":400,"detail":"Operation createMigration: an active migration already exists for the same consumer unit and distributor. If the previous creation was uncertain, it probably took effect: check.","code":"ERR_MIGRACAO_SOLICITADA","operation":"createMigration","requestId":"req-inventado-0001","retryable":false,"uncertain":false}
```

This page is published on GitHub Pages at every final `openapi/vX.Y.Z` tag, only with the repository public (`make docs` generates `out/docs/errors/index.html`; the old `/erros/` address redirects to `/errors/`); until the first publication the `type` address does not respond, and this file is the source. For anyone who does not use Go, the "Problem Details in a Python client" section of [`clients.md`](clients.md) shows the conversion.

## Index

| Entry | API code | Status | Retry a GET | Retry a write |
|---|---|---|---|---|
| [Credential refused by the authorizer](#invalid-credential-authorizer) | `ERR_CREDENCIAL_INVALIDA` | 403 | no (the SDK renews the token on its own when it applies) | same |
| [Credential refused by the backend](#invalid-credential-backend) | `ERR_CREDENCIAL_INVALIDA` | 403 | no | no |
| [Invalid field in the request](#field-validation) | `ERR_VALIDACAO_CAMPOS` | 400 | no | no |
| [Retailer profile required](#retailer-profile-required) | `ERR_PERFIL_VAREJISTA_OBRIGATORIO` | 400 | no | no |
| [Migration already requested](#migration-already-requested) | `ERR_MIGRACAO_SOLICITADA` | 400 | no | no (check) |
| [Month range required](#month-range-required) | `ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO` | 400 | no | no |
| [Month range too wide](#month-range-too-wide) | `ERR_MES_REFERENCIA_INICIAL_DIFERENCA` | 400 | no | no |
| [Error code unknown to the SDK](#unknown-code) | new `ERR_*` | any | no | no |
| [Refusal without an API code](#refusal-without-code) | none | 4xx | no | no |
| [Resource not found](#not-found) | none | 404 | no | no |
| [Rate limit exceeded](#rate-limited) | none | 429 | yes, with backoff | only with the gateway mark |
| [API internal error](#internal-error) | none | 500, 503 and other 5xx, except 501 | yes | uncertain: check |
| [API gateway failure](#gateway-failure) | none | 502, 504 | yes | uncertain: check |
| [Network failure](#transport-error) | none | no response | yes | yes if nothing left; otherwise uncertain |
| [Timeout](#timeout) | none | no response | yes | yes if nothing left; otherwise uncertain |
| [Call canceled](#call-canceled) | none | no response | your decision | uncertain if it already left |
| [Uncertain write result](#uncertain-result) | depends on the cause | depends on the cause | not applicable | no: check |
| [Unreadable response](#unreadable-response) | none | 2xx | yes | uncertain: check |
| [Repeated pagination cursor](#repeated-cursor) | none | no HTTP error | no | not applicable |
| [Token request failed](#token-request-failed) | may come from the endpoint | from the endpoint | you may, on 429 and 5xx (the SDK does not retry) | same |
| [Invalid argument](#invalid-argument) | none | no request | no | no |
| [Error outside the catalogue](#not-in-catalog) | none | no response | depends | depends |

"Retry" follows the SDK's retry table (the same rule as [`clients.md`](clients.md), section 5): the SDK retries up to 2 times on its own what is marked safe, and shows the error only afterwards. `retryable` in the Problem says whether retrying **now** is safe; `uncertain` says the write may have been applied.

Confidence: **seen in the sandbox** means observed on 30/10/2026 (four codes) or on 01/11/2026 (the two month-range ones); **low** means not observed (a hypothesis from the gateway's structure or from the public collection); **SDK** means the SDK's own behaviour, covered by local tests. The public Postman collection mentions no `ERR_*` code besides the six seen in the sandbox (checked on 30/10/2026).

---

<a id="invalid-credential-authorizer"></a>
## Credential refused by the authorizer

- **API code:** `ERR_CREDENCIAL_INVALIDA`.
- **Status:** 403 (the API never returns 401), with the `x-amzn-ErrorType` header.
- **When it happens:** the API Gateway authorizer refused the token (for example, "not a valid JWT token") or the combination of `codigoAgente` and `codigoPerfil` that does not belong to the credential. Only the `message` tells the cases apart. An expired token was not observed: the sandbox accepted a token 12 s after `expires_in`.
- **What to do:** if the token worked and stopped, the SDK renews it and retries **once**, on its own, on any method (the gateway refused before the backend, so nothing was processed, not even on a POST). If the error reaches you, the problem is agent, profile or credential: check `WithAgent`, `WithCredentials` and the environment. Repeating the same request only spends requests.
- **Retry:** GET and write, no, after the automatic renewal. The renewal applies only when the token had already been accepted before by a response without the authorizer mark; without that, the error is a configuration one. With `WithTokenSource`, the SDK does not renew (the cache is yours).
- **Go sentinel:** `ErrInvalidCredential` (matches by code, in both 403s).
- **Confidence:** seen in the sandbox.

<a id="invalid-credential-backend"></a>
## Credential refused by the backend

- **API code:** `ERR_CREDENCIAL_INVALIDA`.
- **Status:** 403, **without** `x-amzn-ErrorType`.
- **When it happens:** the code comes from the backend and not from the authorizer. It was not observed; the SDK separates it from the previous case because the renewal rule only holds for the authorizer's refusal, which happens before the backend. Without that mark, the SDK neither renews the token nor retries.
- **What to do:** check credential, agent, profile and environment. Do not retry blindly.
- **Retry:** GET and write, no.
- **Go sentinel:** `ErrInvalidCredential`.
- **Confidence:** low.

<a id="field-validation"></a>
## Invalid field in the request

- **API code:** `ERR_VALIDACAO_CAMPOS`.
- **Status:** 400.
- **When it happens:** a body or query field was refused. In the sandbox, a consumer unit with letters, with fewer than 15 digits or with a wrong check digit gave this code; the other fields (month, document, e-mail) were not tested one by one. The API validates **one field at a time** and stops at the first failure: fixing one may reveal the next.
- **What to do:** read the `message` (in `APIError.Message`), fix the field and send again. In a generated client, send the month as `YYYY-MM`.
- **Retry:** GET and write, no, without fixing first.
- **Go sentinel:** `ErrFieldValidation`.
- **Confidence:** seen in the sandbox.

<a id="retailer-profile-required"></a>
## Retailer profile required

- **API code:** `ERR_PERFIL_VAREJISTA_OBRIGATORIO`.
- **Status:** 400.
- **When it happens:** the migration listing was requested without the profile in the query. The profile is required in the header **and** in the query.
- **What to do:** provide the profile. The SDK already sends it in both places; in your own client, add `codigoPerfilVarejista` to the query (the header stays `codigoPerfil`).
- **Retry:** GET and write, no, without fixing first.
- **Go sentinel:** `ErrRetailerProfileRequired`.
- **Confidence:** seen in the sandbox.

<a id="migration-already-requested"></a>
## Migration already requested

- **API code:** `ERR_MIGRACAO_SOLICITADA`.
- **Status:** 400.
- **When it happens:** a migration POST for a consumer unit and a distributor that already have an **active** migration. A closed or canceled migration was not tested.
- **What to do:** if you yourself retried after an [uncertain result](#uncertain-result), the first creation probably took effect: list the consumer unit with `CheckCreated` to see what exists. It is not a data error: it is the server's defence against duplicates.
- **Retry:** GET does not apply; write, no (check first).
- **Go sentinel:** `ErrMigrationAlreadyRequested`.
- **Confidence:** seen in the sandbox.

<a id="month-range-required"></a>
## Month range required

- **API code:** `ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO`.
- **Status:** 400.
- **When it happens:** the migration listing was requested without `mesReferenciaInicial` and `mesReferenciaFinal` (`YYYY-MM`). The API requires both; seen in the sandbox on 01/11/2026.
- **What to do:** provide the range. The SDK validates `From` and `To` before sending and returns `ErrInvalidArgument` without making a request; only `Client.Do` reaches this code. In your own client, add both months to the query.
- **Retry:** GET and write, no, without fixing first.
- **Go sentinel:** `ErrMonthRangeRequired`.
- **Confidence:** seen in the sandbox.

<a id="month-range-too-wide"></a>
## Month range too wide

- **API code:** `ERR_MES_REFERENCIA_INICIAL_DIFERENCA`.
- **Status:** 400.
- **When it happens:** the distance between the start month and the end month exceeded the API's limit. The limit was not published: in the sandbox, on 01/11/2026, 12-month ranges were accepted and 24, 48 and 96 were refused, so the cap lies between 12 and 24 months (probably 12).
- **What to do:** narrow the range or split the query into smaller parts, one at a time.
- **Retry:** GET and write, no, without narrowing the range first.
- **Go sentinel:** `ErrMonthRangeTooWide`.
- **Confidence:** seen in the sandbox (the exact cap is unknown: between 12 and 24 months).

<a id="unknown-code"></a>
## Error code unknown to the SDK

- **API code:** an `ERR_*` the SDK does not know yet.
- **Status:** whatever the API returned.
- **When it happens:** the API gained a new code. The SDK does not break: the error stays an `*APIError` with `Code`, `Message` and `StatusCode`, and the Problem uses this entry, with the code in `code`. Codes outside the form `ERR_` followed by upper-case letters, digits and underscores are dropped from the Problem and fall into [refusal without code](#refusal-without-code).
- **What to do:** handle it by `status` and `code`; the `message` is in `APIError.Message`. Open an issue so the code enters the catalogue.
- **Retry:** follows the status: 429 and 5xx as in their entries; the rest, no.
- **Go sentinel:** none; compare `APIError.Code`.
- **Confidence:** SDK.

<a id="refusal-without-code"></a>
## Refusal without an API code

- **API code:** none. The body is not JSON, or the `error` field does not have the form of a code.
- **Status:** a 4xx that is neither 404 nor 429 (for example, a 403 without `ERR_*`), or a 3xx, which the SDK does not follow.
- **When it happens:** a refusal from a component that does not speak the API's format. Unverified hypothesis: a per-IP rate limit applied by a web application firewall may answer 403.
- **What to do:** use the `status` and the `requestId` to investigate with the operator. Do not retry: a 403 without an `ERR_*` code is not retried by the SDK.
- **Retry:** GET and write, no.
- **Go sentinel:** none.
- **Confidence:** low.

<a id="not-found"></a>
## Resource not found

- **API code:** none.
- **Status:** 404. The SDK also returns this error, without an extra request, when a lookup by id comes back with an empty list (`Get`).
- **When it happens:** the migration id does not exist for this agent and profile.
- **What to do:** check the id and whether the agent and profile are the ones used at creation.
- **Retry:** GET and write, no.
- **Go sentinel:** `ErrNotFound`.
- **Confidence:** low for the API's 404 (not observed); SDK for the empty list.

<a id="rate-limited"></a>
## Rate limit exceeded

- **API code:** none.
- **Status:** 429, with or without the gateway's `x-amzn-ErrorType` mark.
- **When it happens:** the gateway limited the rate. The known limit is 1,000 requests per IP every 5 minutes and, in production, 10,000 per agent on measurements; the SDK's local limiter uses 900 and 9,000, with margin. Never observed.
- **What to do:** wait. The SDK retries reads on its own, with exponential backoff; `Retry-After`, if present, rules (cap of 30 s). If it reached you, slow down.
- **Retry:** GET, yes, with backoff. Write: only if the response carries the gateway mark (it limited before the backend); without the mark, no.
- **Go sentinel:** `ErrRateLimited`.
- **Confidence:** low.

<a id="internal-error"></a>
## API internal error

- **API code:** none known.
- **Status:** 500, 503 and any other 5xx, except 501. The 501 (not implemented) is final: the SDK does not retry and does not treat the write as uncertain.
- **When it happens:** the backend failed. Never observed in the sandbox.
- **What to do:** on a read, the SDK already tried again; try later. On a **write**, the result is uncertain: the backend may have applied the change before failing. Follow the step by step in [uncertain result](#uncertain-result).
- **Retry:** GET, yes. POST and PUT, no: check first. On 501, not even GET.
- **Go sentinel:** none.
- **Confidence:** low.

<a id="gateway-failure"></a>
## API gateway failure

- **API code:** none.
- **Status:** 502 and 504. Not observed; the body may not follow the four-field format (even plain text), and the SDK accepts any body.
- **When it happens:** the gateway could not talk to the backend, or the backend took longer than the gateway's cut-off (near 30 s).
- **What to do:** same as the [internal error](#internal-error): for a read, try later; for a write, check before retrying.
- **Retry:** GET, yes. POST and PUT, no.
- **Go sentinel:** none.
- **Confidence:** low.

<a id="transport-error"></a>
## Network failure

- **API code:** none.
- **Status:** none (there was no response).
- **When it happens:** DNS, connection refused, TLS (for example, an untrusted certificate) or a connection closed without a response (`EOF`). The error comes out **without the URL**, which would carry the consumer unit and the month in the query.
- **What to do:** check the network and the environment. The SDK already retried up to 2 times when it was safe.
- **Retry:** GET, yes. Write: yes if the failure was before writing (nothing left); if the failure was after, the result is [uncertain](#uncertain-result).
- **Go sentinel:** none; the cause (`x509.UnknownAuthorityError`, `io.EOF`, `net.Error`) stays reachable through `errors.As` and `errors.Is`.
- **Confidence:** SDK.

<a id="timeout"></a>
## Timeout

- **API code:** none.
- **Status:** none.
- **When it happens:** the attempt exceeded 30 s (or `WithTimeout`), the deadline of the `context` expired, or the local limiter did not release the turn in time. It matches `context.DeadlineExceeded` through `errors.Is`.
- **What to do:** raise the deadline if the operation is heavy; slow down if it was the local limiter.
- **Retry:** GET, yes, when it was the attempt's deadline; if it was the deadline of your `context`, the SDK stops and the decision is yours. Write already sent, no: [uncertain](#uncertain-result).
- **Go sentinel:** `context.DeadlineExceeded`.
- **Confidence:** SDK.

<a id="call-canceled"></a>
## Call canceled

- **API code:** none.
- **Status:** none.
- **When it happens:** the call's `context` was canceled. It matches `context.Canceled`.
- **What to do:** nothing: you gave up. On a write that already left, the error is [uncertain](#uncertain-result).
- **Retry:** the SDK never retries after a cancellation; the decision is yours.
- **Go sentinel:** `context.Canceled`.
- **Confidence:** SDK.

<a id="uncertain-result"></a>
## Uncertain write result

- **API code:** the cause's, if any.
- **Status:** the cause's (for example 502), or none.
- **When it happens:** on a `Create` or `Edit` (or on a write `Do`), the request left, but the response does not say whether it was applied: network error after sending, timeout, 5xx, or 2xx with an unreadable body. The error is a `*UncertainResultError`, with the cause in `Err` and the natural key in `Key`.
- **What to do, step by step:**
  1. Do not retry the write blindly.
  2. Get the error: `var unc *abertura.UncertainResultError; errors.As(err, &unc)`. The key is in `unc.Key`, of type `NaturalKey`; convert it to the concrete type with `k, ok := unc.Key.(abertura.MigrationKey)` (or `abertura.EditKey`).
  3. For a creation, call `CheckCreated(ctx, k)` with the `MigrationKey`; an empty list means "not created", and then you may retry `Create`. If it found any, check `Status` and `RequestDate`, because an earlier migration for the same consumer unit and month (canceled, for example) also comes back.
  4. For an edit, call `CheckEdited(ctx, k)` with the `EditKey`; the second return value says whether the migration already has the desired fields.
  5. Only retry the write if the check says it was not applied. If you retry `Create` with the migration active, you get [`ERR_MIGRACAO_SOLICITADA`](#migration-already-requested), which indicates the first one probably took effect: check again.
  6. On `Do`, `Key` comes empty: check on your own.
- **Retry:** no, until you check. In the Problem: `uncertain` true and `retryable` false.
- **Go sentinel:** none; use `errors.As` with `*UncertainResultError`. The `Key` never goes into the Problem, because it may contain the consumer unit.
- **Confidence:** SDK; the duplicate was seen in the sandbox.

<a id="unreadable-response"></a>
## Unreadable response

- **API code:** none.
- **Status:** 2xx (the API answered with success); the Problem does not repeat the status, because the error is in the body.
- **When it happens:** the connection dropped in the middle of the body, the body is not valid JSON, or a field has a value the SDK does not understand (a date or a number outside the format). The error message does not repeat the received value, which may be consumer unit data.
- **What to do:** on a read, the SDK already retried when the body was interrupted; if it persists, open an issue with the `requestId`. On a **write**, the API may have applied the change: the error comes as an [uncertain result](#uncertain-result), with this entry as the cause.
- **Retry:** GET, yes. POST and PUT, no: check first.
- **Go sentinel:** none.
- **Confidence:** SDK.

<a id="repeated-cursor"></a>
## Repeated pagination cursor

- **API code:** none.
- **Status:** no HTTP error; the API answered 200.
- **When it happens:** the API returned an already used cursor as the next page. Pagination was never seen working in the sandbox; without the guard, a fixed cursor would loop forever. The `List` iterator ends with this error: if the page points to the cursor it just used, its items are not delivered (they would repeat the previous ones); if it points to a cursor seen earlier, its items are delivered and the iterator stops without fetching again.
- **What to do:** use `ListPage` to control the cursor and open an issue with the `request_id` of the last page, which appears in the SDK log (`WithLogger`); the error does not carry it.
- **Retry:** no.
- **Go sentinel:** `ErrRepeatedCursor`.
- **Confidence:** SDK.

<a id="token-request-failed"></a>
## Token request failed

- **API code:** the token endpoint's, if one comes.
- **Status:** the token endpoint's (`operation` is `getToken`).
- **When it happens:** the token request was refused or failed: wrong `client_id` or `client_secret`, wrong environment, endpoint down. Network errors while fetching the token appear as [network failure](#transport-error) or [timeout](#timeout). This includes an endpoint response without `access_token` and an error from the `WithTokenSource` source. The error never carries the form or the body.
- **What to do:** check the credentials and the environment (`Sandbox()` or `Production()`).
- **Retry:** the SDK does not retry the token request on its own. Fetching a token has no side effect, so 429 and 5xx are safe to retry (`retryable` true); 4xx, no.
- **Go sentinel:** no specific one; `ErrInvalidCredential` matches if the endpoint returns that code.
- **Confidence:** low.

<a id="invalid-argument"></a>
## Invalid argument

- **API code:** none.
- **Status:** none: the SDK refused before sending.
- **When it happens:** something the API requires is missing (the measurement filter without consumer unit, distributor, start or end), the id is empty, the edit function is nil, the `Do` path does not start with `/`, the body does not become JSON, or the `NewClient` configuration is invalid (missing credential or agent, environment URL, timeout, limits). The error message does not repeat the consumer unit or the path.
- **What to do:** fix the call. Nothing was sent or spent from the rate limit.
- **Retry:** no, without fixing.
- **Go sentinel:** `ErrInvalidArgument`.
- **Confidence:** SDK.

<a id="not-in-catalog"></a>
## Error outside the catalogue

- **API code:** none.
- **Status:** none.
- **When it happens:** `ProblemFrom` received an error that does not come from the SDK, or that the SDK did not classify. The Problem uses this entry (`type` ends in `#not-in-catalog`, never `about:blank`) and does **not** copy the error text. The RFC 9457 `about:blank` is left for whoever builds a Problem by hand: the SDK's JSON, in that case, uses the status's standard phrase as `title`.
- **What to do:** handle the original error; it stays in your hands.
- **Retry:** depends; the Problem says `retryable` false out of prudence.
- **Go sentinel:** none.
- **Confidence:** SDK.
