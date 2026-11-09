# abertura-mercado-energia

[Versão em português](README.pt-BR.md)

OpenAPI specification and Go SDK for the retail APIs of Brazil's energy market opening ("Abertura de Mercado").

> This project is independent and is not affiliated with, sponsored or endorsed by the Câmara de Comercialização de Energia Elétrica (CCEE). The API description derives from public documentation and from tests in the sandbox environment.

Status: **0.x**. The spec and the SDK may still change in incompatible ways between minor versions. Every claim in the spec has a source and a confidence level in [`openapi/SOURCES.md`](openapi/SOURCES.md).

## Spec

[`openapi/abertura.yaml`](openapi/abertura.yaml) describes 10 operations: token, migrations, measurements and change requests. It records what the API actually returns, including where that differs from the public documentation (the divergences are numbered in `SOURCES.md`).

- **Read it in the browser:** the [Redoc page](https://marcobuarque.github.io/abertura-mercado-energia/) is published with every final `openapi/v*` tag; `make docs` builds it locally at `out/docs/index.html`.
- **Try it without credentials:** `make mock` starts a mock at `http://localhost:4010` with the spec's examples, all invented. To stop it: `docker compose -f mock/compose.yaml down`.
- **Generate a client** in Python, TypeScript or Java, and what it does not do on its own (retry rules, uncertain results): [`docs/clients.md`](docs/clients.md).
- **Errors:** the teaching [error catalog](https://marcobuarque.github.io/abertura-mercado-energia/errors/), with what to do and when to retry, lives in [`docs/errors.md`](docs/errors.md). The Go SDK also delivers every error as Problem Details (RFC 9457).

## Go SDK

Requires Go 1.26 or newer.

```bash
go get github.com/MarcoBuarque/abertura-mercado-energia/abertura@v0.2.0
```

```go
import "github.com/MarcoBuarque/abertura-mercado-energia/abertura"

client, err := abertura.NewClient(abertura.Sandbox(),
	abertura.WithCredentials(clientID, clientSecret),
	abertura.WithAgent(agent, profile))
if err != nil { log.Fatal(err) }
for m, err := range client.Migrations.List(ctx, abertura.MigrationFilter{
	From: abertura.NewYearMonth(2027, time.January),
	To:   abertura.NewYearMonth(2027, time.December)}) {
	if err != nil { log.Fatal(err) }
	fmt.Println(m.ID, m.Status, m.Month)
}
```

The environment is always explicit: nothing points to production unless you ask. The SDK does not read environment variables on its own.

Against the mock (`make mock`), the environment is your own `Environment`, and any credential works:

```go
mock := abertura.Environment{Name: "mock", APIURL: "http://localhost:4010",
	TokenURL: "http://localhost:4010/sso/oauth/token"}
client, err := abertura.NewClient(mock,
	abertura.WithCredentials("invented-id", "invented-secret"),
	abertura.WithAgent(900002, 900003))
if err != nil { log.Fatal(err) }
page, err := client.Migrations.ListPage(ctx, abertura.MigrationFilter{
	From: abertura.NewYearMonth(2027, time.January),
	To:   abertura.NewYearMonth(2027, time.March)}, "")
if err != nil { log.Fatal(err) }
for _, m := range page.Items {
	fmt.Println(m.ID, m.Status, m.Month) // 00000000-0000-4000-8000-000000000001 CRIADA 2027-03
}
```

The mock always returns the same page, with the same cursor. That is why, against it, you should use `ListPage`: `List` delivers the first page once and stops with `ErrRepeatedCursor` when the second one points to the same cursor, without repeating the items; that is the guard against a cursor that does not advance.

What it does for you:

- **Token and limits:** fetches and renews the token, and rate limits requests per process.
- **Pagination:** `List` walks the pages; `ListPage` returns the cursor so you can resume later.
- **Tolerated formats:** three month formats, detail as a list or an object, consumption as text or number, new fields and statuses.
- **Writes without retries:** `Create` and `Edit` never retry on their own. If the result is uncertain, the error is a `*UncertainResultError` carrying the natural key, and `FindCreated` or `CheckEdited` tell you what happened.
- **Personal data:** `Document` is masked in `fmt`, `slog` and JSON; the SDK log never carries a consumer unit, document, e-mail or token.
- **Routes without a method yet:** `Client.Do` uses the same token, limits and error loop.

All examples run without a network (`go test -run Example ./...`, inside `abertura/`).

### Coverage

v0.2 covers v1.1 of the retail APIs: migrations (list, get, create, edit), measurements (list) and, through `Do`, change requests. Consumer units of groups B1 to B4 depend on v2 of the API, which the spec does not describe yet.

## Documentation

- [Redoc page](https://marcobuarque.github.io/abertura-mercado-energia/) and [error catalog](https://marcobuarque.github.io/abertura-mercado-energia/errors/) (the old `/erros/` address redirects there).
- [`docs/clients.md`](docs/clients.md): client generation and retry rules.
- [`openapi/SOURCES.md`](openapi/SOURCES.md): sources, confidence and divergences.
- Changelogs: [`openapi/CHANGELOG.md`](openapi/CHANGELOG.md) and [`abertura/CHANGELOG.md`](abertura/CHANGELOG.md).
- [`SECURITY.md`](SECURITY.md): how to report a vulnerability. Never send credentials.

## Releases

Two independent artifacts, each with its own tag prefix:

- `openapi/vX.Y.Z` (and `openapi/vX.Y.Z-rc.N`): the spec; publishes the GitHub release with the spec file, the Pages site and the mock image.
- `abertura/vX.Y.Z` (and `abertura/vX.Y.Z-rc.N`): the Go module `github.com/MarcoBuarque/abertura-mercado-energia/abertura`.

Bare `vX.Y.Z` tags are not used. The tag checks live in `scripts/check-tag.sh`.

## Development

`make ci` runs the same steps as continuous integration: secrets guard, spec lint, mock, generation, tests, Go lint, `govulncheck` and the dependency check.

The contract test (`go test -tags sandbox ./contract/...`, inside `abertura/`) is manual: it uses sandbox credentials, limits itself to 90 requests and only writes with `ABERTURA_CONTRATO_ESCRITA=1`, `ABERTURA_UC` and `ABERTURA_DOCUMENTO` (a valid CPF or CNPJ, from the environment only). Details are in the header of `abertura/contract/contract_test.go`.

## Contact

Questions and usage problems: open an issue in this repository. Anything else: contato@quantogasta.com.br.

Maintained by Marco Oliveira. The project is presented, in Portuguese, on the [QuantoGasta developer page](https://quantogasta.com.br/desenvolvedores), next to an open API of residential electricity tariffs.

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
