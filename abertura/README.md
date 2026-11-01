# abertura (Go SDK)

Go client for the retail APIs of the Brazilian energy market opening.

> This project is independent and is not affiliated with, sponsored or endorsed by the Câmara de Comercialização de Energia Elétrica (CCEE). The API description is derived from the public documentation and from tests in the sandbox environment.

Requires Go 1.26 or newer. Status 0.x: the interface may still change between minor versions.

```bash
go get github.com/MarcoBuarque/abertura-mercado-energia/abertura@v0.2.0
```

```go
import "github.com/MarcoBuarque/abertura-mercado-energia/abertura"

c, err := abertura.NewClient(abertura.Sandbox(),
	abertura.WithCredentials(clientID, clientSecret),
	abertura.WithAgent(agent, profile))
if err != nil { log.Fatal(err) }
for m, err := range c.Migrations.List(ctx, abertura.MigrationFilter{
	From: abertura.NewYearMonth(2027, 1), To: abertura.NewYearMonth(2027, 12)}) {
	if err != nil { log.Fatal(err) }
	fmt.Println(m.ID, m.Status, m.Month)
}
```

Methods: `Migrations` (`List`, `ListPage`, `Get`, `Create`, `Edit`, `CheckCreated`, `CheckEdited`), `Measurements` (`List`, `ListPage`) and `Client.Do` for routes that have no method yet. The runnable examples are in the `godoc`.

Creating a migration takes a `abertura.CPF(...)` or `abertura.CNPJ(...)` document in `MigrationRequest.Document`; the document is masked whenever it is printed.

Errors: `abertura.ProblemFrom(err)` turns any SDK error into a `Problem` in the RFC 9457 format (`application/problem+json`), with no personal data, whose `type` points to the [error catalog](https://marcobuarque.github.io/abertura-mercado-energia/errors/):

```go
p := abertura.ProblemFrom(err) // also works with an error wrapped by %w
w.Header().Set("Content-Type", abertura.ProblemContentType)
if err := json.NewEncoder(w).Encode(p); err != nil { // p.Retryable and p.Uncertain say whether retrying is safe
	log.Print(err)
}
```

For writes with an uncertain result, personal data, API coverage and the OpenAPI specification, see the [root README](../README.md) and [docs/clients.md](../docs/clients.md).
