package abertura_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	abertura "github.com/MarcoBuarque/abertura-mercado-energia/abertura"
)

// exampleEnvironment starts a local server that mimics the API with the
// fixtures from testdata/, so the Examples run without network or credentials.
// It returns the environment and the function that shuts the server down.
//
// Routes: the token; the migration listing (pages 1 and 2 by cursor, and the
// check response when the query carries a consumer unit); the migration POST,
// which answers 502; the measurements; and the change requests.
func exampleEnvironment() (abertura.Environment, func()) {
	root, err := os.OpenRoot("testdata")
	if err != nil {
		log.Fatalf("example: open testdata: %v", err)
	}
	serve := func(w http.ResponseWriter, status int, fixture string) {
		body, err := root.ReadFile(fixture)
		if err != nil {
			log.Printf("example: read fixture: %v", err)
			http.Error(w, "fixture missing", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write(body); err != nil {
			log.Printf("example: write response: %v", err)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sso/oauth/token", func(w http.ResponseWriter, _ *http.Request) { serve(w, 200, "token.json") })
	mux.HandleFunc("GET /v1.1/varejista/migracoes", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("codigoUnidadeConsumidora") != "":
			serve(w, 200, "migrations_check.json")
		case q.Get("indexProximaPagina") != "":
			serve(w, 200, "migrations_page2.json")
		default:
			serve(w, 200, "migrations_page1.json")
		}
	})
	mux.HandleFunc("POST /v1.1/varejista/migracoes", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	})
	mux.HandleFunc("GET /v1/varejista/consumo/medicoes", func(w http.ResponseWriter, _ *http.Request) {
		serve(w, 200, "measurements_page1.json")
	})
	mux.HandleFunc("GET /v1/solicitacoes", func(w http.ResponseWriter, _ *http.Request) {
		serve(w, 200, "requests_page1.json")
	})
	srv := httptest.NewServer(mux)
	return abertura.Environment{
			Name: "example", APIURL: srv.URL, TokenURL: srv.URL + "/sso/oauth/token",
		}, func() {
			srv.Close()
			if err := root.Close(); err != nil {
				log.Printf("example: close testdata: %v", err)
			}
		}
}

// exampleClient builds the client for the Examples. In a real program the
// environment is abertura.Sandbox() or abertura.Production() and the
// credentials come from your secret store or the process environment.
func exampleClient(env abertura.Environment) *abertura.Client {
	c, err := abertura.NewClient(env,
		abertura.WithCredentials("fake-client-id", "fake-secret"),
		abertura.WithAgent(900002, 900003),
		abertura.WithRateLimits(abertura.RateLimits{}),
	)
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}
	return c
}

// Walk the profile's migrations over a month range, page by page. The range is
// required. The document is masked in any printout.
func Example_listMigrations() {
	env, closeServer := exampleEnvironment()
	defer closeServer()
	c := exampleClient(env)

	for m, err := range c.Migrations.List(context.Background(), abertura.MigrationFilter{
		From: abertura.NewYearMonth(2027, time.January), To: abertura.NewYearMonth(2027, time.March)}) {
		if err != nil {
			log.Fatalf("List: %v", err)
		}
		fmt.Println(m.ID, m.Status, m.Month, m.Document)
	}
	// Output:
	// 00000000-0000-4000-8000-000000000001 CRIADA 2027-03 CNPJ ************11
	// 00000000-0000-4000-8000-000000000002 CONCLUIDA 2027-03 CPF *********22
	// 00000000-0000-4000-8000-000000000003 CANCELADA 2027-03 CPF *********33
}

// Create does not retry the write. If the response is lost, the error is an
// *UncertainResultError with the natural key of the migration, and
// CheckCreated tells whether it was actually created.
func ExampleMigrations_Create_uncertainResult() {
	env, closeServer := exampleEnvironment()
	defer closeServer()
	c := exampleClient(env)
	ctx := context.Background()

	_, err := c.Migrations.Create(ctx, abertura.MigrationRequest{
		ConsumerUnit:          "000000000000001",
		Distributor:           900001,
		Document:              abertura.CNPJ("11.111.111/1111-11"),
		Email:                 "contato@example.org",
		Month:                 abertura.NewYearMonth(2027, time.March),
		TerminationNoticeDate: time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC),
	})
	var uncertain *abertura.UncertainResultError
	if !errors.As(err, &uncertain) {
		log.Fatalf("expected an uncertain result, got %v", err)
	}
	fmt.Println("uncertain result in", uncertain.Operation)

	key, ok := uncertain.Key.(abertura.MigrationKey)
	if !ok {
		log.Fatalf("unexpected key: %T", uncertain.Key)
	}
	found, err := c.Migrations.CheckCreated(ctx, key)
	if err != nil {
		log.Fatalf("CheckCreated: %v", err)
	}
	fmt.Println("migrations found:", len(found))
	// Output:
	// uncertain result in createMigration
	// migrations found: 2
}

// ListPage returns the cursor of the next page, so the read can be resumed
// later. The consumption is an exact Decimal; the API does not declare the unit.
func ExampleMeasurements_ListPage() {
	env, closeServer := exampleEnvironment()
	defer closeServer()
	c := exampleClient(env)

	page, err := c.Measurements.ListPage(context.Background(), abertura.MeasurementFilter{
		ConsumerUnit: "000000000000001",
		Distributor:  900001,
		From:         time.Date(2027, 2, 1, 0, 0, 0, 0, time.FixedZone("", -3*3600)),
		To:           time.Date(2027, 2, 28, 23, 45, 0, 0, time.FixedZone("", -3*3600)),
	}, "")
	if err != nil {
		log.Fatalf("ListPage: %v", err)
	}
	for _, m := range page.Items {
		fmt.Println(m.Day.Format("2006-01-02"), m.Consumption, m.Status)
	}
	fmt.Println("next page:", page.Next)
	// Output:
	// 2027-02-01 12.345 CONSISTIDA
	// next page: cursor-inventado-2
}

// A Document is never printed in the clear: fmt, slog and JSON show only the
// masked text. Number returns the number, by explicit call.
func ExampleDocument() {
	d := abertura.CNPJ("11.111.111/1111-11")

	fmt.Println(d)
	fmt.Printf("%+v\n", d)
	text, err := d.MarshalText()
	if err != nil {
		log.Fatalf("MarshalText: %v", err)
	}
	fmt.Println(string(text))
	fmt.Println(d.Number())
	// Output:
	// CNPJ ************11
	// CNPJ ************11
	// CNPJ ************11
	// 11111111111111
}

// Do calls a route the SDK does not cover yet, such as the change request
// listing. It goes through the same loop of token, limits, retries and errors.
func ExampleClient_Do() {
	env, closeServer := exampleEnvironment()
	defer closeServer()
	c := exampleClient(env)

	var list struct {
		ChangeRequests []struct {
			ID     string `json:"idSolicitacaoAlteracao"`
			Status string `json:"situacaoAlteracao"`
			Type   string `json:"tipoSolicitacao"`
		} `json:"solicitacao"`
	}
	err := c.Do(context.Background(), http.MethodGet,
		"/v1/solicitacoes?tipoSolicitacao=SUSPENSAO_FORNECIMENTO_RESILICAO", nil, &list)
	if err != nil {
		log.Fatalf("Do: %v", err)
	}
	for _, s := range list.ChangeRequests {
		fmt.Println(s.ID, s.Status, s.Type)
	}
	// Output:
	// 00000000-0000-4000-8000-0000000000b1 CRIADA SUSPENSAO_FORNECIMENTO_RESILICAO
}
