package abertura_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura"
)

// ProblemFrom accepts the error directly or wrapped, and errors.Is keeps
// working with the sentinels.
func ExampleProblemFrom() {
	err := fmt.Errorf("register migration: %w", &abertura.APIError{
		Operation:  "createMigration",
		StatusCode: http.StatusBadRequest,
		Code:       "ERR_MIGRACAO_SOLICITADA",
		Message:    "Invented: an active migration already exists",
	})

	fmt.Println(errors.Is(err, abertura.ErrMigrationAlreadyRequested))

	p := abertura.ProblemFrom(err)
	fmt.Println(p.Title)
	fmt.Println(p.Code, p.Status)
	fmt.Println(strings.TrimPrefix(p.Type, abertura.ErrorCatalogURL))
	// Output:
	// true
	// Migration already requested
	// ERR_MIGRACAO_SOLICITADA 400
	// #migration-already-requested
}

// A user HTTP handler returns the SDK error as application/problem+json. The
// Status of the Problem is the API one; here the handler responds 409 and
// adjusts the field so that it matches the response.
func ExampleProblemFrom_handlerHTTP() {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		err := fmt.Errorf("register migration: %w", &abertura.APIError{
			Operation:  "createMigration",
			StatusCode: http.StatusBadRequest,
			Code:       "ERR_MIGRACAO_SOLICITADA",
			Message:    "Invented: consumer unit 000000000000001 already has an active migration",
			RequestID:  "req-fake-0001",
		})

		p := abertura.ProblemFrom(err)
		p.Status = http.StatusConflict
		w.Header().Set("Content-Type", abertura.ProblemContentType)
		w.WriteHeader(p.Status)
		if err := json.NewEncoder(w).Encode(p); err != nil {
			log.Printf("write response: %v", err)
		}
	}

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodPost, "/migracoes", nil))

	fmt.Println(rec.Code, rec.Header().Get("Content-Type"))
	fmt.Print(rec.Body.String())
	// Output:
	// 409 application/problem+json
	// {"type":"https://marcobuarque.github.io/abertura-mercado-energia/errors#migration-already-requested","title":"Migration already requested","status":409,"detail":"Operation createMigration: An active migration already exists for the same consumer unit and distributor. If the previous creation was uncertain, it probably took effect: check.","code":"ERR_MIGRACAO_SOLICITADA","operation":"createMigration","requestId":"req-fake-0001","retryable":false,"uncertain":false}
}
