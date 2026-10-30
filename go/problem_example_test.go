package abertura_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"

	abertura "github.com/MarcoBuarque/abertura-mercado-energia/go"
)

// ProblemFrom aceita o erro direto ou embrulhado, e errors.Is continua
// funcionando com as sentinelas.
func ExampleProblemFrom() {
	err := fmt.Errorf("registrar migração: %w", &abertura.APIError{
		Operation:  "criarMigracao",
		StatusCode: http.StatusBadRequest,
		Code:       "ERR_MIGRACAO_SOLICITADA",
		Message:    "Inventado: já existe migração ativa",
	})

	fmt.Println(errors.Is(err, abertura.ErrMigracaoSolicitada))

	p := abertura.ProblemFrom(err)
	fmt.Println(p.Title)
	fmt.Println(p.Code, p.Status)
	fmt.Println(strings.TrimPrefix(p.Type, abertura.CatalogoErrosURL))
	// Output:
	// true
	// Migração já solicitada
	// ERR_MIGRACAO_SOLICITADA 400
	// #migracao-solicitada
}

// Um handler HTTP do usuário devolve o erro do SDK como
// application/problem+json. O Status do Problem é o da API; aqui o handler
// responde 409 e ajusta o campo para ele bater com a resposta.
func ExampleProblemFrom_handlerHTTP() {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		err := fmt.Errorf("registrar migração: %w", &abertura.APIError{
			Operation:  "criarMigracao",
			StatusCode: http.StatusBadRequest,
			Code:       "ERR_MIGRACAO_SOLICITADA",
			Message:    "Inventado: UC 000000000000001 já tem migração ativa",
			RequestID:  "req-inventado-0001",
		})

		p := abertura.ProblemFrom(err)
		p.Status = http.StatusConflict
		w.Header().Set("Content-Type", abertura.ProblemContentType)
		w.WriteHeader(p.Status)
		if err := json.NewEncoder(w).Encode(p); err != nil {
			log.Printf("escrever resposta: %v", err)
		}
	}

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodPost, "/migracoes", nil))

	fmt.Println(rec.Code, rec.Header().Get("Content-Type"))
	fmt.Print(rec.Body.String())
	// Output:
	// 409 application/problem+json
	// {"type":"https://marcobuarque.github.io/abertura-mercado-energia/erros#migracao-solicitada","title":"Migração já solicitada","status":409,"detail":"Operação criarMigracao: Já existe migração ativa para a mesma unidade consumidora e concessionária. Se a criação anterior ficou incerta, ela provavelmente valeu: confira.","code":"ERR_MIGRACAO_SOLICITADA","operation":"criarMigracao","requestId":"req-inventado-0001","retryable":false,"uncertain":false}
}
