# Client guide

This guide shows how to start the spec's mock, how to generate a Python,
TypeScript or Java client from `openapi/abertura.yaml`, what those generated
clients do **not** do, and which retry rules anyone writing their own client
should follow.

> **Unofficial description.** This project is independent: it is not
> affiliated with, sponsored or endorsed by the CCEE. The spec describes what
> the API returned in sandbox tests and does not replace the operator's
> documentation. Everything here runs against the local mock, with invented
> data.

To read the spec in a browser, run `make docs` and open `out/docs/index.html`.

## What you need

- Docker (the mock and the generator run in containers; nothing is installed
  on your machine).
- For the Python client: Python 3.10 or newer.
- A terminal at the repository root.

## 1. Start the mock

```bash
docker compose -f mock/compose.yaml up --build
```

The mock is [Prism](https://stoplight.io/open-source/prism) serving the spec's
examples at `http://localhost:4010`. To stop it:
`docker compose -f mock/compose.yaml down`. Without Compose on the command
line, `make mock` starts it in the background and waits until the mock is
healthy.

The pinned Prism only exists for amd64. On a Mac with Apple Silicon it runs
under emulation (`platform: linux/amd64` in `compose.yaml`) and startup takes a
few seconds longer; on an amd64 computer it runs natively.

### What the mock answers

Each operation returns the **first example** of the 200 response. To pick
another response or another example, send the `Prefer` header:

| You want | Header |
|---|---|
| the second page of migrations (no cursor) | `Prefer: example=pagina-2` |
| a 403 `ERR_CREDENCIAL_INVALIDA` | `Prefer: code=403` |
| a 400 `ERR_MIGRACAO_SOLICITADA` when creating a migration | `Prefer: code=400, example=migracao-solicitada` |

Every example is invented: consumer unit `000000000000001`, codes in the
`900000` range, e-mails at `example.org`. The name of each example starts with
"Inventado:" in the spec.

### Where the mock differs from the real API

The mock validates the request against the spec, which is useful (it catches
client mistakes before touching the sandbox), but the error format **is not
the real API's**:

- **Invalid request** (missing `codigoAgente`, wrong enum, incomplete body):
  Prism answers `400` and returns, as the body, the *first 400 example of the
  operation*. That body is generic and **does not describe the problem**: on
  the migration listing, for instance, it always says
  `ERR_PERFIL_VAREJISTA_OBRIGATORIO`, even if the missing field is another
  one. The real reason comes in the `sl-violations` response header, a JSON
  list:

  ```text
  HTTP/1.1 400 Bad Request
  sl-violations: [{"location":["request","header"],"severity":"Error","code":"required","message":"Request header must have required property 'codigoagente'"}]
  Content-type: application/json

  {"timestamp":"2027-01-10T10:00:00.000-03:00","status":400,"error":"ERR_PERFIL_VAREJISTA_OBRIGATORIO","message":"Inventado: informe o perfil do varejista"}
  ```

  Header and field names appear in lower case in these messages
  (`codigoagente`). If your client "received a 400 that makes no sense", look
  at `sl-violations` first.

- **No token**: Prism answers `401` with `Content-Type: application/problem+json`
  and `WWW-Authenticate: OAuth2`. The real API returns `403` with
  `ERR_CREDENCIAL_INVALIDA` (never 401). Any `Authorization: Bearer ...` value
  works for the mock, but the header must exist.

- The mock does not rate limit, does not expire tokens and keeps no state:
  creating the same migration twice returns 200 twice. To see the duplicate
  400, ask for that example with `Prefer`.

## 2. Generate a client

The generator is [OpenAPI Generator](https://openapi-generator.tech) 7.26.0, in
a container pinned by digest. **Always generate outside the repository**
(generated clients are not versioned). From the root directory:

```bash
GENERATOR=docker.io/openapitools/openapi-generator-cli@sha256:a304ddf1e2e5f24f68fa3153568d6174cea4959d09aa8e3db6d526fd0782326d  # v7.26.0
OUT="$(mktemp -d)"
gen() { # generator, output folder, properties
  docker run --rm -u "$(id -u):$(id -g)" \
    -v "$PWD/openapi":/spec:ro -v "$OUT":/out "$GENERATOR" \
    generate -i /spec/abertura.yaml -g "$1" -o "/out/$2" --additional-properties="$3"
}
```

**Python:**

```bash
gen python python packageName=abertura_client,projectName=abertura-client
```

**TypeScript** (`fetch`, no runtime dependencies):

```bash
gen typescript-fetch ts npmName=abertura-client,supportsES6=true
```

**Java** (JDK 11+ native client, no dependencies besides Jackson):

```bash
gen java java library=native,groupId=example.abertura,artifactId=abertura-client,invokerPackage=example.abertura.client,apiPackage=example.abertura.client.api,modelPackage=example.abertura.client.model
```

On 30/10/2026 all three generated without errors, with 2 generator warnings
("Failed to get the schema name" and "Ignoring complex example on request
body") that do not affect the code. Only the Python client was installed and
run; the TypeScript and Java ones were only generated, **not compiled**.

Two details of the generated clients:

- The spec tags are `Authentication`, `Migrations`, `Measurements` and
  `Change requests`, so the classes come out as `AuthenticationApi`,
  `MigrationsApi`, `MeasurementsApi` and `ChangeRequestsApi`. Method names come
  from the `operationId` (`list_migrations` in Python, `listMigrations` in
  TypeScript and Java).
- Response dates have `format: date-time` in the spec, so the generated
  clients type them as `datetime` (see "What generated clients do not do").

## 3. Python: from token to migration list

With the mock running and the client generated in `$OUT/python`:

```bash
python3 -m venv "$OUT/venv"
"$OUT/venv/bin/pip" install "$OUT/python"
```

Save it as `list.py` and run it with `"$OUT/venv/bin/python" list.py`:

```python
import abertura_client as ac
from abertura_client.api import AuthenticationApi, MigrationsApi

HOST = "http://localhost:4010"  # the mock; for the sandbox, the host is in the spec's `servers`

# 1. Token. The mock accepts any pair; the credentials below are invented.
# With real credentials, read them from the environment: never write them in code.
ID = "invented-id"
SECRET = "invented-secret"
with ac.ApiClient(ac.Configuration(host=HOST)) as client:
    token = AuthenticationApi(client).get_token(client_id=ID, client_secret=SECRET)
print("token expires in", token.expires_in, "s")

# 2. One page of migrations. The profile is required in the header AND in the query.
cfg = ac.Configuration(host=HOST, access_token=token.access_token)
with ac.ApiClient(cfg) as client:
    page = MigrationsApi(client).list_migrations(
        codigo_agente=900002,
        codigo_perfil=900003,
        codigo_perfil_varejista=900003,
        mes_referencia_inicial="2027-01",
        mes_referencia_final="2027-03",
    )
for m in page.migracao:
    print(m.id_migracao, m.codigo_unidade_consumidora, m.status_migracao)
print("next page:", page.index_proxima_pagina)
```

Expected output:

```text
token expires in 300 s
00000000-0000-4000-8000-000000000001 000000000000001 CRIADA
next page: cursor-inventado-2
```

To see the last page and an error, pass `Prefer` through `_headers`:

```python
from abertura_client.exceptions import ApiException

api = MigrationsApi(client)
last = api.list_migrations(
    codigo_agente=900002, codigo_perfil=900003, codigo_perfil_varejista=900003,
    mes_referencia_inicial="2027-01", mes_referencia_final="2027-03",
    _headers={"Prefer": "example=pagina-2"},
)
print(last.index_proxima_pagina)  # None

try:
    api.list_migrations(
        codigo_agente=900002, codigo_perfil=900003, codigo_perfil_varejista=900003,
        mes_referencia_inicial="2027-01", mes_referencia_final="2027-03",
        _headers={"Prefer": "code=403"})
except ApiException as err:
    print(err.status, err.body)  # 403 {"...","error":"ERR_CREDENCIAL_INVALIDA",...}
```

Measured on 30/10/2026 on a Mac with Apple Silicon, with the images already
downloaded (mock starting under emulation): 6 s to build and start the mock,
1 s to generate the client, 2 s for the `venv` and `pip`, 10 s in total until
the list appears. The first time, add the time to download the Prism and
generator images.

## 4. What generated clients do not do

A generated client is only the HTTP and types layer. These are missing, and
are up to you:

1. **Renew the token.** The token lasts about 300 seconds and there is no
   refresh token (`refresh_expires_in` is 0). Ask for another before it
   expires; a 30-second margin works. The generated client keeps the
   `access_token` you give it and uses it until the API refuses it.
2. **Follow the cursor.** The listing returns `indexProximaPagina`; while it
   is filled, repeat the call with it in the `indexProximaPagina` parameter.
   The generated client reads one page per call. Guard against a repeated
   cursor (returning the same cursor twice is an infinite loop). No response
   observed in the sandbox carried a filled cursor, so the rule comes from the
   envelope's structure and has low confidence.
3. **Handle writes with an uncertain result.** On a `POST` or `PUT`, if the
   connection drops after the request was sent, or the time runs out, or the
   response body is unreadable, you **do not know** whether the write
   happened. Blindly retrying may duplicate it. Check first: list the
   consumer unit's migrations, or retry and treat `400 ERR_MIGRACAO_SOLICITADA`
   as a sign that the first attempt took effect (the sandbox refuses the
   duplicate migration). The generated client only throws an exception; it
   does not tell "did not leave" from "left and I do not know".
4. **Accept the three month formats.** The reference month appears as
   `YYYY-MM` (filters and request bodies), as a date and time with a `-03:00`
   offset always on day 1 (listing) and, in the detail, in `dataReferencia`,
   which is also RFC 3339 with an offset (D21). The spec declares the response
   dates as `date-time`; even so, the reference month in filters and request
   bodies is `YYYY-MM`, and interpreting each format is up to you.
5. **Codes as numbers or as text.** Agent, profile and distributor codes are
   integers in the sandbox and text in the public documentation; the
   `consumo` and `codigoAgenteConcessionaria` fields of a measurement come
   without a fixed type in the spec. Accept both forms.
6. **Rate limit and retry with backoff.** See the next section.

## 5. Retry rules for whoever writes the client

These are the rules of the project's Go SDK. They serve as a starting point
for your own client, in any language. The parts about 429 and 5xx are
**hypotheses**: none of those responses was seen in the sandbox. The 403 and
the duplicate are observed facts.

### When to retry

- **Failure before writing** (DNS, connection refused, TLS error, nothing
  left): retry, on any method. Nothing reached the server.
- **GET**: retry on transport error, timeout, 429 and any 5xx except 501.
  Reads are safe to repeat.
- **POST and PUT** after the body has left: transport error, timeout and any
  5xx except 501 are an **uncertain result** (section 4, item 3). Do not
  retry on your own.
- **429 on POST and PUT**: retry only if the response carries the
  `x-amzn-ErrorType` header (the refusal came from the gateway, before the
  backend). Without it, treat it as a rate limit and do not retry.
- **2xx**: success. On POST and PUT, an unreadable body in a 2xx response is
  uncertain.
- **400, 404 and other 4xx**: do not retry. Fix the request.
- **Caller cancellation or deadline**: stop and return the context error. On
  a POST or PUT that already wrote, it is uncertain.

### The 403 `ERR_CREDENCIAL_INVALIDA`

The real API returns **403, not 401**, for an invalid or expired token and
also for an agent or profile that does not belong to the credential. Only the
`message` tells the cases apart. Renew the token and retry **once**, on any
method, when both conditions hold:

1. the response came from the gateway authorizer (it carries
   `x-amzn-ErrorType`): the backend processed nothing, not even on a POST;
2. the rejected token **had already been accepted** before by a response
   without that mark. A token that worked and stopped has expired or been
   revoked.

Without condition 2, the error is about the agent, profile or credential:
retrying only spends requests. Without condition 1, the 403 is from the
backend and is not retried. The retry happens once per call.

### How many times, and waiting how long

- At most **2 retries** per call.
- Exponential backoff with full *jitter* (a draw between zero and the cap):
  base 500 ms and cap 8 s. On 429, base 2 s and cap 30 s.
- If the response carries `Retry-After`, it rules, capped at 30 s.
- Every wait must respect the caller's deadline.
- **Never send `Idempotency-Key`**: the API does not know it, and some HTTP
  clients resend POST on their own when it is present.

### Timeouts and limits

- 30 s per attempt; 10 s for the token. The gateway cuts off near 30 s.
- The API answers `Connection: close`: each request opens a new TLS
  connection. Expect that latency.
- Rate limit to respect on your side: 900 requests every 5 minutes in general
  (1,000 per IP at most) and 9,000 every 5 minutes on measurements. It is per
  process; several processes add up.
- Unverified hypothesis: the 5-minute per-IP window resembles the rate rule
  of a web application firewall, which blocks with 403. A 403 without an
  `ERR_*` code is **not** retried.

## 6. Problem Details in a Python client

The API returns errors with four fields (`timestamp`, `status`, `error`,
`message`) and the spec describes exactly that. The Go SDK delivers the same
error as RFC 9457 Problem Details; in a Python client, the conversion fits in
one function, backed by the [error catalogue](errors.md). Save it as
`problem.py`, start the mock (`make mock`) and run `python3 -I problem.py`:

```python
import json
import re
import urllib.error
import urllib.request

CATALOG = "https://marcobuarque.github.io/abertura-mercado-energia/errors"

# Slug (anchor in docs/errors.md) and title per API code. A code outside the
# table falls into "unknown-code"; with no code, the status decides, as in the SDK.
CODES = {
    "ERR_CREDENCIAL_INVALIDA": ("invalid-credential-authorizer", "Credential refused by the authorizer"),
    "ERR_VALIDACAO_CAMPOS": ("field-validation", "Invalid field in the request"),
    "ERR_PERFIL_VAREJISTA_OBRIGATORIO": ("retailer-profile-required", "Retailer profile required"),
    "ERR_MIGRACAO_SOLICITADA": ("migration-already-requested", "Migration already requested"),
    "ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO": ("month-range-required", "Month range required"),
    "ERR_MES_REFERENCIA_INICIAL_DIFERENCA": ("month-range-too-wide", "Month range too wide"),
}
NO_CODE = {
    404: ("not-found", "Resource not found"),
    429: ("rate-limited", "Rate limit exceeded"),
    502: ("gateway-failure", "API gateway failure"),
    504: ("gateway-failure", "API gateway failure"),
}


def to_problem(status, body, headers, operation):
    """Convert the API's four-field error into Problem Details (RFC 9457)."""
    try:
        code = json.loads(body).get("error", "")
    except (ValueError, AttributeError):
        code = ""
    if not re.fullmatch(r"ERR_[A-Z0-9_]{1,80}", str(code)):
        code = ""  # free text from the server does not go into the Problem
    if code in CODES:
        slug, title = CODES[code]
        if code == "ERR_CREDENCIAL_INVALIDA" and not headers.get("x-amzn-ErrorType"):
            slug, title = "invalid-credential-backend", "Credential refused by the backend"
    elif code:
        slug, title = "unknown-code", "Error code unknown to the SDK"
    elif status in NO_CODE:
        slug, title = NO_CODE[status]
    elif status >= 500:
        slug, title = "internal-error", "API internal error"
    else:
        slug, title = "refusal-without-code", "Refusal without an API code"
    problem = {
        "type": f"{CATALOG}#{slug}",
        "title": title,
        "status": status,
        # Never copy the API's "message": it may mention a consumer unit, document or e-mail.
        "detail": f"Operation {operation}: see the catalogue.",
        "operation": operation,
    }
    if code:
        problem["code"] = code
    return problem


request = urllib.request.Request(
    "http://localhost:4010/v1.1/varejista/migracoes",
    data=b"{}",
    method="POST",
    headers={
        "Authorization": "Bearer invented-token",
        "Content-Type": "application/json",
        "codigoAgente": "900002",
        "codigoPerfil": "900003",
        "Prefer": "code=400, example=migracao-solicitada",
    },
)
try:
    urllib.request.urlopen(request)
except urllib.error.HTTPError as err:
    problem = to_problem(err.code, err.read(), err.headers, "createMigration")
    print(json.dumps(problem, ensure_ascii=False, indent=2))
```

Expected output (run against the mock on 30/10/2026):

```json
{
  "type": "https://marcobuarque.github.io/abertura-mercado-energia/errors#migration-already-requested",
  "title": "Migration already requested",
  "status": 400,
  "detail": "Operation createMigration: see the catalogue.",
  "operation": "createMigration",
  "code": "ERR_MIGRACAO_SOLICITADA"
}
```

Three precautions, the same as the SDK's: the `detail` is your own text and
does **not** copy the API's `message` (it may mention a consumer unit,
document or e-mail); the `code` is only included if it has the `ERR_` form
and upper case; and the 403 `ERR_CREDENCIAL_INVALIDA` has two catalogue
entries, depending on whether it comes with the `x-amzn-ErrorType` header.
To hand the result to whoever calls your service, use the media type
`application/problem+json`.
