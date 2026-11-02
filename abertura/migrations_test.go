package abertura

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura/internal/testserver"
)

// fromMonth and toMonth are the month range the listing requires.
var fromMonth, toMonth = NewYearMonth(2027, time.January), NewYearMonth(2027, time.March)

const (
	listTemplate    = "/v1.1/varejista/migracoes"
	detailTemplate  = "/v1.1/varejista/migracoes/{idMigracao}"
	editTemplate    = "/v1/varejista/migracoes/{idMigracao}"
	testMigrationID = "00000000-0000-4000-8000-000000000001"
)

// tokenRoute serves the token from the fixtures. The testserver validates the
// token request against the spec.
var tokenRoute = testserver.Route{
	Method: http.MethodPost, PathTemplate: "/sso/oauth/token",
	Responses: []testserver.Response{{Status: http.StatusOK, Fixture: "token.json"}},
}

// stubRoute builds a testserver route for a method and a path template.
func stubRoute(httpMethod, pathTemplate string, responses ...testserver.Response) testserver.Route {
	return testserver.Route{Method: httpMethod, PathTemplate: pathTemplate, Responses: responses}
}

// fixtureResponse builds a testserver response from a fixture file.
func fixtureResponse(status int, fixture string) testserver.Response {
	return testserver.Response{Status: status, Fixture: fixture}
}

// specClient builds a client against the testserver, which validates every
// request against the spec. The suite logger (log_test.go) is attached.
func specClient(t *testing.T, routes []testserver.Route, opts ...Option) (*Client, *testserver.Server) {
	t.Helper()
	srv := testserver.New(t, append([]testserver.Route{tokenRoute}, routes...)...)
	all := append([]Option{
		WithCredentials(fakeClientID, fakeSecret),
		WithAgent(fakeAgent, fakeProfile),
		WithRateLimits(RateLimits{}),
		WithLogger(suiteLogger()),
	}, opts...)
	c, _ := newClientWithEnv(t, Environment{
		Name: "test", APIURL: srv.URL(), TokenURL: srv.URL() + "/sso/oauth/token",
	}, all...)
	return c, srv
}

// calls returns the API requests (without the token) for the path.
func calls(srv *testserver.Server, httpMethod, path string) []*http.Request {
	var out []*http.Request
	for _, r := range srv.Requests() {
		if r.Method == httpMethod && r.URL.Path == path {
			out = append(out, r)
		}
	}
	return out
}

// bodyJSON decodes the JSON body of a request into a map.
func bodyJSON(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	return m
}

// collectSeq drains an iterator, returning the items seen before any error.
func collectSeq[T any](t *testing.T, seq func(func(T, error) bool)) ([]T, error) {
	t.Helper()
	var items []T
	for item, err := range seq {
		if err != nil {
			return items, err
		}
		items = append(items, item)
	}
	return items, nil
}

func TestMigrationsList_WhenTwoPages_WalksEverything(t *testing.T) {
	t.Parallel()
	c, srv := specClient(t, []testserver.Route{
		stubRoute(http.MethodGet, listTemplate, fixtureResponse(200, "migrations_page1.json"), fixtureResponse(200, "migrations_page2.json")),
	})
	items, err := collectSeq(t, c.Migrations.List(context.Background(), MigrationFilter{From: fromMonth, To: toMonth}))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
	if items[0].ID != testMigrationID || items[0].Month != NewYearMonth(2027, time.March) || items[0].Document.Number() != "11111111111111" {
		t.Errorf("first item badly converted: %v", items[0])
	}
	reqs := calls(srv, http.MethodGet, listTemplate)
	if len(reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(reqs))
	}
	if reqs[0].URL.Query().Has("indexProximaPagina") {
		t.Errorf("the first page carries no cursor: %s", reqs[0].URL.RawQuery)
	}
	if got := reqs[1].URL.Query().Get("indexProximaPagina"); got != "fake-cursor-2" {
		t.Errorf("cursor of the second page = %q", got)
	}
}

func TestMigrationsList_WhenFilters_GoInTheQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		filter MigrationFilter
		opts   []CallOption
		want   url.Values
	}{
		{
			name:   "only the range carries the client profile and the months",
			filter: MigrationFilter{From: fromMonth, To: toMonth},
			want:   url.Values{"codigoPerfilVarejista": {"900002"}, "mesReferenciaInicial": {"2027-01"}, "mesReferenciaFinal": {"2027-03"}},
		},
		{
			name:   "filter Profile overrides the client one",
			filter: MigrationFilter{From: fromMonth, To: toMonth, Profile: 900077},
			want:   url.Values{"codigoPerfilVarejista": {"900077"}, "mesReferenciaInicial": {"2027-01"}, "mesReferenciaFinal": {"2027-03"}},
		},
		{
			name:   "WithAgent on the call overrides the query profile",
			filter: MigrationFilter{From: fromMonth, To: toMonth},
			opts:   []CallOption{WithAgent(900005, 900006)},
			want:   url.Values{"codigoPerfilVarejista": {"900006"}, "mesReferenciaInicial": {"2027-01"}, "mesReferenciaFinal": {"2027-03"}},
		},
		{
			name: "months, consumer unit and status",
			filter: MigrationFilter{
				From: NewYearMonth(2027, time.January), To: NewYearMonth(2027, time.December),
				ConsumerUnit: fakeConsumerUnit, Status: MigrationStatusCompleted,
			},
			want: url.Values{
				"codigoPerfilVarejista": {"900002"}, "mesReferenciaInicial": {"2027-01"}, "mesReferenciaFinal": {"2027-12"},
				"codigoUnidadeConsumidora": {fakeConsumerUnit}, "statusMigracao": {"CONCLUIDA"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, srv := specClient(t, []testserver.Route{stubRoute(http.MethodGet, listTemplate, fixtureResponse(200, "migrations_empty.json"))})
			if _, err := collectSeq(t, c.Migrations.List(context.Background(), tc.filter, tc.opts...)); err != nil {
				t.Fatalf("List: %v", err)
			}
			reqs := calls(srv, http.MethodGet, listTemplate)
			if len(reqs) != 1 {
				t.Fatalf("requests = %d, want 1", len(reqs))
			}
			if got := reqs[0].URL.Query(); !equalValues(got, tc.want) {
				t.Errorf("query = %v, want %v", got, tc.want)
			}
		})
	}
}

func equalValues(a, b url.Values) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if !slices.Equal(v, b[k]) {
			return false
		}
	}
	return true
}

func TestMigrationsList_WhenBreak_FetchesNoMore(t *testing.T) {
	t.Parallel()
	c, srv := specClient(t, []testserver.Route{
		stubRoute(http.MethodGet, listTemplate, fixtureResponse(200, "migrations_page1.json"), fixtureResponse(200, "migrations_page2.json")),
	})
	for _, err := range c.Migrations.List(context.Background(), MigrationFilter{From: fromMonth, To: toMonth}) {
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		break
	}
	if n := srv.Count(http.MethodGet, listTemplate); n != 1 {
		t.Errorf("requests = %d, want 1", n)
	}
}

func TestMigrationsList_WhenError_ReturnsItOnce(t *testing.T) {
	t.Parallel()
	c, _ := specClient(t, []testserver.Route{stubRoute(http.MethodGet, listTemplate, fixtureResponse(400, "error_400_profile.json"))})
	items, err := collectSeq(t, c.Migrations.List(context.Background(), MigrationFilter{From: fromMonth, To: toMonth}))
	if !errors.Is(err, ErrRetailerProfileRequired) {
		t.Fatalf("err = %v, want ErrRetailerProfileRequired", err)
	}
	if len(items) != 0 {
		t.Errorf("items = %d, want 0", len(items))
	}
}

func TestMigrationsListPage_WhenCursor_ReturnsTheNext(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		fixture   string
		cursor    string
		wantItems int
		wantNext  string
	}{
		{name: "first page brings the cursor", fixture: "migrations_page1.json", wantItems: 2, wantNext: "fake-cursor-2"},
		{name: "resuming with a cursor and being the last", fixture: "migrations_page2.json", cursor: "fake-cursor-2", wantItems: 1},
		{name: "empty", fixture: "migrations_empty.json", wantItems: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, srv := specClient(t, []testserver.Route{stubRoute(http.MethodGet, listTemplate, fixtureResponse(200, tc.fixture))})
			page, err := c.Migrations.ListPage(context.Background(), MigrationFilter{From: fromMonth, To: toMonth}, tc.cursor)
			if err != nil {
				t.Fatalf("ListPage: %v", err)
			}
			if len(page.Items) != tc.wantItems || page.NextCursor != tc.wantNext {
				t.Errorf("page = %d items, next %q; want %d and %q", len(page.Items), page.NextCursor, tc.wantItems, tc.wantNext)
			}
			reqs := calls(srv, http.MethodGet, listTemplate)
			if got := reqs[0].URL.Query().Get("indexProximaPagina"); got != tc.cursor {
				t.Errorf("cursor sent = %q, want %q", got, tc.cursor)
			}
		})
	}
}

func TestMigrationsList_WhenListingHasUnknownValues_Converts(t *testing.T) {
	t.Parallel()
	c, _ := specClient(t, []testserver.Route{stubRoute(http.MethodGet, listTemplate, fixtureResponse(200, "migrations_tolerance.json"))})
	items, err := collectSeq(t, c.Migrations.List(context.Background(), MigrationFilter{From: fromMonth, To: toMonth}))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Status != "STATUS_NOVO" || items[0].Status.Known() || items[0].Month != NewYearMonth(2026, time.December) {
		t.Errorf("item = %+v", items)
	}
}

func TestMigrationsGet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		stubResponse testserver.Response
		wantID       string
		wantErr      func(error) bool
	}{
		{name: "one-item array", stubResponse: fixtureResponse(200, "migration_detail_array.json"), wantID: testMigrationID},
		{name: "object", stubResponse: fixtureResponse(200, "migration_detail_object.json"), wantID: testMigrationID},
		{
			name: "empty array is ErrNotFound", stubResponse: fixtureResponse(200, "migration_detail_empty.json"),
			wantErr: func(err error) bool { return errors.Is(err, ErrNotFound) },
		},
		{
			name: "one item with another id is ErrNotFound", stubResponse: fixtureResponse(200, "migration_detail_one_other_id.json"),
			wantErr: func(err error) bool { return errors.Is(err, ErrNotFound) },
		},
		{name: "three items with the id in the middle returns the right one", stubResponse: fixtureResponse(200, "migration_detail_three_with_id.json"), wantID: testMigrationID},
		{
			name: "three items without the id is ErrNotFound", stubResponse: fixtureResponse(200, "migration_detail_three_without_id.json"),
			wantErr: func(err error) bool { return errors.Is(err, ErrNotFound) },
		},
		{name: "repeated id with identical content returns the item", stubResponse: fixtureResponse(200, "migration_detail_id_repeated.json"), wantID: testMigrationID},
		{
			name: "repeated id with different content is an unreadable response", stubResponse: fixtureResponse(200, "migration_detail_id_repeated_different.json"),
			wantErr: func(err error) bool {
				var er *responseError
				return errors.As(err, &er) && !errors.Is(err, ErrNotFound) && strings.Contains(err.Error(), "more than one")
			},
		},
		{name: "different months, oldest first, returns the most recent month", stubResponse: fixtureResponse(200, "migration_detail_months_oldest_first.json"), wantID: testMigrationID},
		{name: "different months, newest first, returns the most recent month", stubResponse: fixtureResponse(200, "migration_detail_months_newest_first.json"), wantID: testMigrationID},
		{
			name: "different month and status is an unreadable response", stubResponse: fixtureResponse(200, "migration_detail_months_and_status.json"),
			wantErr: func(err error) bool {
				var er *responseError
				return errors.As(err, &er) && !errors.Is(err, ErrNotFound) && strings.Contains(err.Error(), "more than one")
			},
		},
		{
			name: "server 404 is ErrNotFound", stubResponse: testserver.Response{Status: 404},
			wantErr: func(err error) bool { return errors.Is(err, ErrNotFound) },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, srv := specClient(t, []testserver.Route{stubRoute(http.MethodGet, detailTemplate, tc.stubResponse)})
			m, err := c.Migrations.Get(context.Background(), testMigrationID)
			if tc.wantErr != nil {
				if !tc.wantErr(err) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if m.ID != tc.wantID || m.Month != NewYearMonth(2027, time.March) {
				t.Errorf("migration = %v", m)
			}
			if got := calls(srv, http.MethodGet, "/v1.1/varejista/migracoes/"+testMigrationID); len(got) != 1 {
				t.Errorf("requests for the id = %d, want 1", len(got))
			}
		})
	}
}

func TestMigrationsGet_WhenEmptyID_MakesNoRequest(t *testing.T) {
	t.Parallel()
	c, srv := specClient(t, []testserver.Route{stubRoute(http.MethodGet, detailTemplate, fixtureResponse(200, "migration_detail_array.json"))})
	if _, err := c.Migrations.Get(context.Background(), " "); err == nil {
		t.Fatal("expected an error")
	}
	if n := srv.Count(http.MethodGet, detailTemplate); n != 0 {
		t.Errorf("requests = %d, want 0", n)
	}
}

func testMigrationRequest() MigrationRequest {
	return MigrationRequest{
		ConsumerUnit: fakeConsumerUnit, Distributor: 900001,
		Document: CNPJ("11.111.111/1111-11"), Email: "contato@example.org",
		Month: NewYearMonth(2027, time.March), TerminationNoticeDate: time.Date(2027, 1, 15, 0, 0, 0, 0, brasiliaZone),
		Comment: "Made up: example comment",
	}
}

func TestMigrationsCreate_WhenSuccess_SendsTheDocumentInTheClear(t *testing.T) {
	t.Parallel()
	c, srv := specClient(t, []testserver.Route{stubRoute(http.MethodPost, listTemplate, fixtureResponse(200, "migration_created.json"))})
	m, err := c.Migrations.Create(context.Background(), testMigrationRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.ID != "00000000-0000-4000-8000-000000000005" || m.Status != MigrationStatusCreated {
		t.Errorf("migration = %v", m)
	}
	reqs := calls(srv, http.MethodPost, listTemplate)
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(reqs))
	}
	body := bodyJSON(t, reqs[0])
	want := map[string]any{
		"numeroDocumento": "11111111111111", "tipoDocumento": "CNPJ", "mesReferencia": "2027-03",
		"dataDenuncia": "2027-01-15", "codigoUnidadeConsumidora": fakeConsumerUnit,
		"codigoAgenteConcessionaria": float64(900001), "codigoAgenteVarejista": float64(fakeAgent),
		"codigoPerfilVarejista": float64(fakeProfile), "emailUnidadeConsumidora": "contato@example.org",
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("body[%s] = %v, want %v", k, body[k], v)
		}
	}
}

func TestMigrationsCreate_WhenFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		stubResponse  testserver.Response
		wantSentinel  error
		wantUncertain bool
	}{
		{name: "400 migration already requested", stubResponse: fixtureResponse(400, "error_400_migration_already_requested.json"), wantSentinel: ErrMigrationAlreadyRequested},
		{name: "400 validation", stubResponse: fixtureResponse(400, "error_400_validation.json"), wantSentinel: ErrFieldValidation},
		{name: "502 is an uncertain result", stubResponse: fixtureResponse(502, "error_502.txt"), wantUncertain: true},
		{
			name: "2xx with an unreadable body is an uncertain result", stubResponse: fixtureResponse(200, "error_502.txt"),
			wantUncertain: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, srv := specClient(t, []testserver.Route{stubRoute(http.MethodPost, listTemplate, tc.stubResponse, fixtureResponse(200, "migration_created.json"))})
			_, err := c.Migrations.Create(context.Background(), testMigrationRequest())
			if n := srv.Count(http.MethodPost, listTemplate); n != 1 {
				t.Errorf("POSTs = %d, want 1 (writes are not retried)", n)
			}
			if tc.wantSentinel != nil {
				if !errors.Is(err, tc.wantSentinel) {
					t.Fatalf("err = %v, want %v", err, tc.wantSentinel)
				}
				var uncertain *UncertainResultError
				if errors.As(err, &uncertain) {
					t.Errorf("should not be uncertain: %v", err)
				}
				return
			}
			var uncertain *UncertainResultError
			if !errors.As(err, &uncertain) {
				t.Fatalf("err = %v, want *UncertainResultError", err)
			}
			want := CreateKey{ConsumerUnit: fakeConsumerUnit, Distributor: 900001, Month: NewYearMonth(2027, time.March), Profile: fakeProfile}
			if uncertain.Operation != "createMigration" || uncertain.Key != want {
				t.Errorf("uncertain = %q %v, want createMigration %v", uncertain.Operation, uncertain.Key, want)
			}
		})
	}
}

func TestMigrationsCheckCreated(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		fixture string
		key     CreateKey
		wantIDs []string // newest first
	}{
		{
			name: "filters by distributor and sorts newest first", fixture: "migrations_check.json",
			key:     CreateKey{ConsumerUnit: fakeConsumerUnit, Distributor: 900001, Month: NewYearMonth(2027, time.March)},
			wantIDs: []string{"00000000-0000-4000-8000-000000000013", "00000000-0000-4000-8000-000000000011"},
		},
		{
			name: "distributor without migration gives empty", fixture: "migrations_check.json",
			key: CreateKey{ConsumerUnit: fakeConsumerUnit, Distributor: 900042, Month: NewYearMonth(2027, time.March)},
		},
		{
			name: "empty list gives empty", fixture: "migrations_empty.json",
			key: CreateKey{ConsumerUnit: fakeConsumerUnit, Distributor: 900001, Month: NewYearMonth(2027, time.March)},
		},
		{
			name: "key Profile overrides the client one", fixture: "migrations_empty.json",
			key: CreateKey{ConsumerUnit: fakeConsumerUnit, Distributor: 900001, Month: NewYearMonth(2027, time.March), Profile: 900077},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, srv := specClient(t, []testserver.Route{stubRoute(http.MethodGet, listTemplate, fixtureResponse(200, tc.fixture))})
			got, err := c.Migrations.FindCreated(context.Background(), tc.key)
			if err != nil {
				t.Fatalf("FindCreated: %v", err)
			}
			if got == nil {
				t.Error("the empty result must be a non-nil slice")
			}
			var ids []string
			for _, m := range got {
				ids = append(ids, m.ID)
			}
			if !slices.Equal(ids, tc.wantIDs) {
				t.Errorf("ids = %v, want %v", ids, tc.wantIDs)
			}
			q := calls(srv, http.MethodGet, listTemplate)[0].URL.Query()
			wantProfile := "900002"
			if tc.key.Profile != 0 {
				wantProfile = "900077"
			}
			if q.Get("codigoUnidadeConsumidora") != fakeConsumerUnit || q.Get("mesReferenciaInicial") != "2027-03" ||
				q.Get("mesReferenciaFinal") != "2027-03" || q.Get("codigoPerfilVarejista") != wantProfile {
				t.Errorf("query = %v", q)
			}
		})
	}
}

func TestMigrationsEdit_WhenSuccess_DoesGetThenPutWithTheWholeObject(t *testing.T) {
	t.Parallel()
	c, srv := specClient(t, []testserver.Route{
		stubRoute(http.MethodGet, detailTemplate, fixtureResponse(200, "migration_detail_array.json")),
		stubRoute(http.MethodPut, editTemplate, fixtureResponse(200, "migration_detail_object.json")),
	})
	m, err := c.Migrations.Edit(context.Background(), testMigrationID, func(e *MigrationEdit) { e.Email = "fresh@example.org" })
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if m.ID == "" {
		t.Errorf("empty migration: %v", m)
	}
	var order []string
	for _, r := range srv.Requests() {
		if r.URL.Path != "/sso/oauth/token" {
			order = append(order, r.Method)
		}
	}
	if !slices.Equal(order, []string{http.MethodGet, http.MethodPut}) {
		t.Fatalf("order = %v, want GET and PUT", order)
	}
	puts := calls(srv, http.MethodPut, "/v1/varejista/migracoes/"+testMigrationID)
	body := bodyJSON(t, puts[0])
	want := map[string]any{
		"mesReferencia": "2027-03", "codigoPerfilVarejista": float64(900003), "tipoDocumento": "CNPJ",
		"numeroDocumento": "11111111111111", "emailUnidadeConsumidora": "fresh@example.org", "statusMigracao": "CRIADA",
	}
	if len(body) != len(want) {
		t.Errorf("body = %v, want exactly %v", body, want)
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("body[%s] = %v, want %v", k, body[k], v)
		}
	}
}

func TestMigrationsEdit_WhenFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		get      testserver.Response
		put      testserver.Response
		change   func(*MigrationEdit)
		wantPuts int
		check    func(*testing.T, error)
	}{
		{
			name: "PUT 502 is uncertain with an EditKey", get: fixtureResponse(200, "migration_detail_array.json"), put: fixtureResponse(502, "error_502.txt"),
			change: func(e *MigrationEdit) { e.Email = "fresh@example.org" }, wantPuts: 1,
			check: func(t *testing.T, err error) {
				var uncertain *UncertainResultError
				if !errors.As(err, &uncertain) {
					t.Fatalf("err = %v, want uncertain", err)
				}
				k, ok := uncertain.Key.(EditKey)
				if !ok || k.ID != testMigrationID || k.Desired.Email != "fresh@example.org" || k.Desired.Profile != 900003 {
					t.Errorf("key = %#v", uncertain.Key)
				}
			},
		},
		{
			name: "missing migration never reaches the PUT", get: fixtureResponse(200, "migration_detail_empty.json"), put: fixtureResponse(200, "migration_detail_object.json"),
			change: func(*MigrationEdit) {}, wantPuts: 0,
			check: func(t *testing.T, err error) {
				if !errors.Is(err, ErrNotFound) {
					t.Errorf("err = %v, want ErrNotFound", err)
				}
			},
		},
		{
			name: "nil change function", get: fixtureResponse(200, "migration_detail_array.json"), put: fixtureResponse(200, "migration_detail_object.json"),
			change: nil, wantPuts: 0,
			check: func(t *testing.T, err error) {
				if err == nil {
					t.Error("expected an error")
				}
			},
		},
		{
			name: "PUT 400 is an API error", get: fixtureResponse(200, "migration_detail_array.json"), put: fixtureResponse(400, "error_400_validation.json"),
			change: func(e *MigrationEdit) { e.Status = MigrationStatusCanceled }, wantPuts: 1,
			check: func(t *testing.T, err error) {
				if !errors.Is(err, ErrFieldValidation) {
					t.Errorf("err = %v, want ErrFieldValidation", err)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, srv := specClient(t, []testserver.Route{
				stubRoute(http.MethodGet, detailTemplate, tc.get),
				stubRoute(http.MethodPut, editTemplate, tc.put),
			})
			_, err := c.Migrations.Edit(context.Background(), testMigrationID, tc.change)
			tc.check(t, err)
			if n := srv.Count(http.MethodPut, editTemplate); n != tc.wantPuts {
				t.Errorf("PUTs = %d, want %d", n, tc.wantPuts)
			}
		})
	}
}

func TestMigrationsCheckEdited(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		desired     MigrationEdit
		wantMatches bool
	}{
		{name: "fields that match", desired: MigrationEdit{Email: "contato@example.org", Profile: 900003, Status: MigrationStatusCreated}, wantMatches: true},
		{name: "month and document that match", desired: MigrationEdit{Month: NewYearMonth(2027, time.March), Document: CNPJ("11111111111111")}, wantMatches: true},
		{name: "zero field is ignored", desired: MigrationEdit{}, wantMatches: true},
		{name: "different e-mail", desired: MigrationEdit{Email: "fresh@example.org"}},
		{name: "different status", desired: MigrationEdit{Status: MigrationStatusCanceled}},
		{name: "different month", desired: MigrationEdit{Month: NewYearMonth(2027, time.April)}},
		{name: "different profile", desired: MigrationEdit{Profile: 900099}},
		{name: "different document", desired: MigrationEdit{Document: CPF("22222222222")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := specClient(t, []testserver.Route{stubRoute(http.MethodGet, detailTemplate, fixtureResponse(200, "migration_detail_array.json"))})
			m, matches, err := c.Migrations.CheckEdited(context.Background(), EditKey{ID: testMigrationID, Desired: tc.desired})
			if err != nil {
				t.Fatalf("CheckEdited: %v", err)
			}
			if matches != tc.wantMatches {
				t.Errorf("matches = %v, want %v", matches, tc.wantMatches)
			}
			if m.ID != testMigrationID {
				t.Errorf("current migration = %v", m)
			}
		})
	}
}

func TestMigrationsCheckEdited_WhenMissing_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	c, _ := specClient(t, []testserver.Route{stubRoute(http.MethodGet, detailTemplate, fixtureResponse(200, "migration_detail_empty.json"))})
	_, matches, err := c.Migrations.CheckEdited(context.Background(), EditKey{ID: testMigrationID})
	if !errors.Is(err, ErrNotFound) || matches {
		t.Errorf("matches = %v, err = %v", matches, err)
	}
}

// The natural keys must satisfy the interface; the compiler checks it.
var (
	_ NaturalKey = CreateKey{}
	_ NaturalKey = EditKey{}
)

func TestMigrationsList_WhenMonthRangeInvalid_FailsWithoutRequest(t *testing.T) {
	t.Parallel()
	jan, fev := NewYearMonth(2027, time.January), NewYearMonth(2027, time.February)
	tests := []struct {
		name      string
		filter    MigrationFilter
		wantErr   bool
		wantField string
	}{
		{name: "neither From nor To", filter: MigrationFilter{}, wantErr: true, wantField: "From"},
		{name: "only From", filter: MigrationFilter{From: jan}, wantErr: true, wantField: "To"},
		{name: "only To", filter: MigrationFilter{To: jan}, wantErr: true, wantField: "From"},
		{name: "To before From", filter: MigrationFilter{From: fev, To: jan}, wantErr: true, wantField: "To"},
		{name: "month 0 in From", filter: MigrationFilter{From: YearMonth{Year: 2027, Month: 0}, To: jan}, wantErr: true, wantField: "From"},
		{name: "month 13 in From", filter: MigrationFilter{From: YearMonth{Year: 2027, Month: 13}, To: YearMonth{Year: 2028, Month: time.January}}, wantErr: true, wantField: "From"},
		{name: "month 0 in To", filter: MigrationFilter{From: jan, To: YearMonth{Year: 2027, Month: 0}}, wantErr: true, wantField: "To"},
		{name: "month 13 in To", filter: MigrationFilter{From: jan, To: YearMonth{Year: 2027, Month: 13}}, wantErr: true, wantField: "To"},
		{name: "year 0 in From", filter: MigrationFilter{From: YearMonth{Year: 0, Month: time.March}, To: jan}, wantErr: true, wantField: "From"},
		{name: "one-month range", filter: MigrationFilter{From: jan, To: jan}},
		{name: "valid range", filter: MigrationFilter{From: jan, To: fev}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, via := range []string{"ListPage", "List"} {
				c, srv := specClient(t, []testserver.Route{
					stubRoute(http.MethodGet, listTemplate, fixtureResponse(200, "migrations_empty.json")),
				})
				var err error
				if via == "ListPage" {
					_, err = c.Migrations.ListPage(context.Background(), tc.filter, "")
				} else {
					_, err = collectSeq(t, c.Migrations.List(context.Background(), tc.filter))
				}
				n := srv.Count(http.MethodGet, listTemplate)
				if !tc.wantErr {
					if err != nil || n != 1 {
						t.Errorf("%s: err = %v, requests = %d; want nil and 1", via, err, n)
					}
					continue
				}
				if !errors.Is(err, ErrInvalidArgument) {
					t.Errorf("%s: err = %v, want ErrInvalidArgument", via, err)
				} else if !strings.Contains(err.Error(), tc.wantField) {
					t.Errorf("%s: the message %q does not mention the field %s", via, err.Error(), tc.wantField)
				}
				if n != 0 || len(srv.Requests()) != 0 {
					t.Errorf("%s: %d request(s) were made before the local validation", via, len(srv.Requests()))
				}
			}
		})
	}
}
