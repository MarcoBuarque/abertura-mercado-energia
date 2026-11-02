// Package abertura is a Go client for the retail APIs of the Brazilian energy
// market opening.
//
// This project is independent and is not affiliated with, sponsored or endorsed by the Câmara de Comercialização de Energia Elétrica (CCEE). The API description is derived from the public documentation and from tests in the sandbox environment.
//
// # Usage
//
// Create a [Client] with [NewClient] for [Sandbox] or [Production] and the
// credentials, then use [Client.Migrations] and [Client.Measurements]. Errors
// match the Err* sentinels with errors.Is; [ProblemFrom] turns any SDK error
// into an RFC 9457 [Problem] linked to the error catalog at [ErrorCatalogURL].
//
// # Personal data in logs
//
// The types that carry a consumer unit, e-mail, document or consumption
// (Migration, Measurement, the request, edit, filter and key types) print only
// non-personal data through fmt, whatever the verb, and through slog; a Page
// logs only its size. json.Marshal is the exception: it writes the fields in
// clear text, because the JSON is meant for storage. Log ids, not lists.
package abertura
