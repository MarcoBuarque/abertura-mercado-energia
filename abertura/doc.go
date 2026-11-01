// Package abertura is a Go client for the retail APIs of the Brazilian energy
// market opening.
//
// This project is independent and is not affiliated with, sponsored or endorsed by the Câmara de Comercialização de Energia Elétrica (CCEE). The API description is derived from the public documentation and from tests in the sandbox environment.
//
// # Personal data in logs
//
// The types that carry a consumer unit, e-mail, document or consumption
// (Migration, Measurement and the request and filter types) print only
// non-personal data through fmt and slog. With the slog JSON handler, log the
// id rather than the whole list: a list goes through json.Marshal, which
// writes the fields in clear.
package abertura
