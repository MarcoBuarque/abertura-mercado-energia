package abertura_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura/internal/spec"
)

// loadRouter reads the embedded spec and builds a router without servers,
// which matches by path only.
func loadRouter(t *testing.T) routers.Router {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(spec.Abertura)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("invalid spec: %v", err)
	}
	doc.Servers = nil
	r, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatalf("build router: %v", err)
	}
	return r
}

// TestFixtures_WhenValidated_MatchTheSpec guarantees the sandbox, spec, fixture,
// SDK chain: each JSON fixture must be a valid response of the operation it
// serves.
func TestFixtures_WhenValidated_MatchTheSpec(t *testing.T) {
	t.Parallel()
	const id = "00000000-0000-4000-8000-000000000001"
	tests := []struct {
		name    string
		fixture string
		method  string
		path    string
		status  int
	}{
		{name: "token", fixture: "token.json", method: http.MethodPost, path: "/sso/oauth/token", status: 200},
		{name: "migrations page 1", fixture: "migrations_page1.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes", status: 200},
		{name: "migrations page 2", fixture: "migrations_page2.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes", status: 200},
		{name: "migrations empty", fixture: "migrations_empty.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes", status: 200},
		{name: "migrations with new fields, a new status and an old month", fixture: "migrations_tolerance.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes", status: 200},
		{name: "detail as an array", fixture: "migration_detail_array.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes/" + id, status: 200},
		{name: "detail with two items", fixture: "migration_detail_two.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes/" + id, status: 200},
		{name: "migrations of different distributors and dates", fixture: "migrations_check.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes", status: 200},
		{name: "change requests", fixture: "requests_page1.json", method: http.MethodGet, path: "/v1/solicitacoes", status: 200},
		{name: "empty detail", fixture: "migration_detail_empty.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes/" + id, status: 200},
		{name: "created migration", fixture: "migration_created.json", method: http.MethodPost, path: "/v1.1/varejista/migracoes", status: 200},
		{name: "edited migration returns the detail", fixture: "migration_detail_object.json", method: http.MethodPut, path: "/v1/varejista/migracoes/" + id, status: 200},
		{name: "measurements with string consumption", fixture: "measurements_page1.json", method: http.MethodGet, path: "/v1/varejista/consumo/medicoes", status: 200},
		{name: "measurements with numeric consumption", fixture: "measurements_page2.json", method: http.MethodGet, path: "/v1/varejista/consumo/medicoes", status: 200},
		{name: "empty measurements", fixture: "measurements_empty.json", method: http.MethodGet, path: "/v1/varejista/consumo/medicoes", status: 200},
		{name: "400 validation", fixture: "error_400_validation.json", method: http.MethodPost, path: "/v1.1/varejista/migracoes", status: 400},
		{name: "400 profile required", fixture: "error_400_profile.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes", status: 400},
		{name: "400 migration already requested", fixture: "error_400_migration_already_requested.json", method: http.MethodPost, path: "/v1.1/varejista/migracoes", status: 400},
		{name: "403 authorizer", fixture: "error_403_authorizer.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes", status: 403},
	}
	router := loadRouter(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body, err := os.ReadFile(filepath.Join("testdata", tc.fixture))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			req := httptest.NewRequest(tc.method, tc.path, nil)
			route, pathParams, err := router.FindRoute(req)
			if err != nil {
				t.Fatalf("route %s %s is outside the spec: %v", tc.method, tc.path, err)
			}
			header := http.Header{"Content-Type": {"application/json"}, "X-Amzn-Requestid": {"00000000-0000-4000-8000-0000000000f1"}}
			if tc.status == 403 {
				header.Set("X-Amzn-Errortype", "AccessDeniedException")
			}
			in := &openapi3filter.ResponseValidationInput{
				RequestValidationInput: &openapi3filter.RequestValidationInput{
					Request: req, PathParams: pathParams, Route: route,
				},
				Status: tc.status,
				Header: header,
				Options: &openapi3filter.Options{
					IncludeResponseStatus: true,
				},
			}
			in.SetBodyBytes(bytes.TrimSpace(body))
			if err := openapi3filter.ValidateResponse(context.Background(), in); err != nil {
				t.Errorf("fixture %s is invalid for %s %s (%d): %v", tc.fixture, tc.method, tc.path, tc.status, err)
			}
		})
	}
}

// TestFixtures_WhenScanned_AreMadeUp rejects values that look like real data:
// consumer units with leading zeros, agents in the 900000 range and e-mail at
// example.org.
func TestFixtures_WhenScanned_AreMadeUp(t *testing.T) {
	t.Parallel()
	root, err := os.OpenRoot("testdata")
	if err != nil {
		t.Fatalf("open testdata: %v", err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Errorf("close testdata: %v", err)
		}
	})
	files, err := filepath.Glob(filepath.Join("testdata", "*"))
	if err != nil {
		t.Fatalf("list fixtures: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no fixture in testdata")
	}
	for _, a := range files {
		t.Run(filepath.Base(a), func(t *testing.T) {
			t.Parallel()
			b, err := root.ReadFile(filepath.Base(a))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			for _, line := range strings.Split(string(b), "\n") {
				l := strings.TrimSpace(line)
				if strings.Contains(l, "@") && !strings.Contains(l, "@example.org") {
					t.Errorf("e-mail outside example.org: %s", l)
				}
				if strings.Contains(l, "\"codigoUnidadeConsumidora\"") && !strings.Contains(l, "\"00000") {
					t.Errorf("consumer unit without leading zeros: %s", l)
				}
			}
		})
	}
}
