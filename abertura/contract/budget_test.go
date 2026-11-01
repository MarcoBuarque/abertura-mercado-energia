//go:build sandbox

package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	abertura "github.com/MarcoBuarque/abertura-mercado-energia/abertura"
)

// The tests in this file run offline, against a local server: they prove the
// counter, the stop at the first 403 and the description without values.

func localServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write body: %v", err)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func fetch(t *testing.T, rt http.RoundTripper, url string) error {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func TestBudget_WhenOverTheLimit_Fails(t *testing.T) {
	s := localServer(t, 200, `{"migracao":[]}`)
	o, err := newBudget(http.DefaultTransport, 3)
	if err != nil {
		t.Fatalf("newBudget: %v", err)
	}
	for i := range 3 {
		if err := fetch(t, o, s.URL+"/v1.1/varejista/migracoes"); err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
	}
	if err := fetch(t, o, s.URL+"/v1.1/varejista/migracoes"); !errors.Is(err, errBudgetExhausted) {
		t.Fatalf("the 4th should exhaust the budget: %v", err)
	}
}

func TestBudget_WhenFirst403_StopsTheNext(t *testing.T) {
	s := localServer(t, 403, `{"error":"ERR_CREDENCIAL_INVALIDA","message":"x","status":403}`)
	o, err := newBudget(http.DefaultTransport, 90)
	if err != nil {
		t.Fatalf("newBudget: %v", err)
	}
	if err := fetch(t, o, s.URL+"/v1.1/varejista/migracoes"); err != nil {
		t.Fatalf("the first one goes through: %v", err)
	}
	if err := fetch(t, o, s.URL+"/v1.1/varejista/migracoes"); !errors.Is(err, errAborted) {
		t.Fatalf("the second should stop: %v", err)
	}
	if l := strings.Join(o.drain(), "\n"); !strings.Contains(l, "ERR_CREDENCIAL_INVALIDA") {
		t.Errorf("the line should bring the code: %s", l)
	}
}

func TestBudget_WhenResponseOutsideSpec_RecordsWithoutValues(t *testing.T) {
	const value = "secret-value-123"
	s := localServer(t, 200, `{"migracao":[{"idMigracao":"`+value+`","statusMigracao":7}],"indexProximaPagina":null}`)
	o, err := newBudget(http.DefaultTransport, 90)
	if err != nil {
		t.Fatalf("newBudget: %v", err)
	}
	if err := fetch(t, o, s.URL+"/v1.1/varejista/migracoes"); err != nil {
		t.Fatalf("pegar: %v", err)
	}
	v := o.allViolations()
	if len(v) == 0 {
		t.Fatal("expected a violation (numeric statusMigracao)")
	}
	all := strings.Join(v, "\n") + strings.Join(o.drain(), "\n")
	if strings.Contains(all, value) {
		t.Errorf("value leaked: %s", all)
	}
	if !strings.Contains(all, "idMigracao") {
		t.Errorf("the field names should appear: %s", all)
	}
}

func TestFieldNames(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{name: "object with a list", in: `{"a":1,"migracao":[{"x":"v","y":2},{"z":1}]}`, want: "a,migracao,migracao.x,migracao.y"},
		{name: "list at the root", in: `[{"id":"v"}]`, want: "id"},
		{name: "not JSON", in: `Bad Gateway`, want: "body is not JSON (11 bytes)"},
		{name: "empty", in: `[]`, want: "(none)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := fieldNames([]byte(tc.in)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDocumentFromEnv(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		kind  string
		ok    bool
	}{
		{"eleven characters is a CPF", "000.000.000-00", "CPF", true},
		{"fourteen characters is a CNPJ", "00.000.000/0000-00", "CNPJ", true},
		{"alphanumeric CNPJ", "AB.CDE.FGH/0001-00", "CNPJ", true},
		{"empty is refused", "", "", false},
		{"odd length is refused", "123", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, ok := documentFromEnv(tc.value)
			if ok != tc.ok || string(d.Type()) != tc.kind {
				t.Fatalf("documentFromEnv = %q, %v; want %q, %v", d.Type(), ok, tc.kind, tc.ok)
			}
		})
	}
}

func TestBudget_FieldFormats(t *testing.T) {
	long := strings.Repeat("a1", 30)
	tests := []struct {
		name, in, want string
	}{
		{name: "simple object", in: `{"mesReferencia":"2026-12","outro":"x"}`, want: "mesReferencia=9999-99"},
		{name: "nested in a list, only the first item", in: `{"migracao":[{"dataReferencia":"01-2026"},{"dataReferencia":"2026-01-01"}]}`, want: "dataReferencia=99-9999"},
		{name: "date and time with letters", in: `{"timestamp":"2026-01-02T03:04:05.678-03:00"}`, want: "timestamp=9999-99-99A99:99:99.999-99:99"},
		{name: "first occurrence wins", in: `{"a":{"dataDenuncia":"2026-01-02"},"dataDenuncia":"x"}`, want: "dataDenuncia=9999-99-99"},
		{name: "null", in: `{"dataValidacao":null}`, want: "dataValidacao=null"},
		{name: "a number becomes nines", in: `{"expires_in":300}`, want: "expires_in=999"},
		{name: "accented letter and full-width digit", in: `{"dataSolicitacao":"é１２"}`, want: "dataSolicitacao=A99"},
		{name: "a long value is cut at 40", in: `{"dataSolicitacao":"` + long + `"}`, want: "dataSolicitacao=" + strings.Repeat("A9", 20) + "..."},
		{name: "a field outside the list does not appear", in: `{"emailUnidadeConsumidora":"a@b.c","codigoUnidadeConsumidora":"123"}`, want: ""},
		{name: "several, in name order", in: `{"mesReferencia":"2026-12","dataReferencia":"2026-12-01"}`, want: "dataReferencia=9999-99-99, mesReferencia=9999-99"},
		{name: "not JSON", in: `Bad Gateway`, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := fieldFormats([]byte(tc.in))
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
			if strings.Contains(got, "2026") || strings.Contains(got, "a@b") {
				t.Errorf("value leaked: %q", got)
			}
		})
	}
}

func TestBudget_WhenTemporalField_LineBringsFormatsWithoutValue(t *testing.T) {
	s := localServer(t, 200, `{"migracao":[{"mesReferencia":"2026-12","idMigracao":"secret-value"}]}`)
	o, err := newBudget(http.DefaultTransport, 90)
	if err != nil {
		t.Fatalf("newBudget: %v", err)
	}
	if err := fetch(t, o, s.URL+"/v1.1/varejista/migracoes"); err != nil {
		t.Fatalf("pegar: %v", err)
	}
	l := strings.Join(o.drain(), "\n")
	if !strings.Contains(l, "formats: mesReferencia=9999-99") {
		t.Errorf("the line should bring the mask: %s", l)
	}
	if strings.Contains(l, "2026-12") || strings.Contains(l, "secret-value") {
		t.Errorf("value leaked: %s", l)
	}
}

func TestDescribe_WhenUnreadableFieldError_NamesTheField(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "wrapped unreadable field", err: fmt.Errorf("getMigration: %w", errors.New(`the response field "dataReferencia" has an unreadable value`)), want: "unreadable field in the response: dataReferencia"},
		{name: "nested field", err: errors.New(`x: the response field "a.b_c" has an unreadable value`), want: "unreadable field in the response: a.b_c"},
		{name: "unexpected type, fixed SDK text", err: fmt.Errorf("getMigration: %w", errors.New(`decode response: field "migracao.dataReferencia" has an unexpected type (expected string)`)), want: "field with unexpected type in the response: migracao.dataReferencia"},
		{name: "a typed encoding/json error never prints the value", err: &json.UnmarshalTypeError{Field: "migracao.consumo", Value: "number 1234", Type: reflect.TypeOf(0)}, want: "error of type *json.UnmarshalTypeError"},
		{name: "invalid JSON", err: fmt.Errorf("getMigration: %w", &json.SyntaxError{Offset: 7}), want: "response is not valid JSON (offset 7)"},
		{name: "another error keeps its type", err: errors.New("something with secret-value"), want: "error of type *errors.errorString"},
		{name: "a name with a non-standard character does not match", err: errors.New(`the response field "a b@c" has an unreadable value`), want: "error of type *errors.errorString"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := describe(tc.err); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDescribe_WhenSDKOwnError_PrintsOnlyFixedText(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "more than one migration", err: fmt.Errorf("getMigration: %w", errors.New("the API returned more than one migration for the id")), want: "the API returned more than one migration for the id"},
		{name: "empty response", err: fmt.Errorf("getMigration: %w", errors.New("empty response")), want: "empty response"},
		{name: "no identifiable migration", err: fmt.Errorf("createMigration: %w", errors.New("the response does not bring an identifiable migration")), want: "the response does not bring an identifiable migration"},
		{name: "invalid JSON with byte", err: fmt.Errorf("getMigration: %w", errors.New("response is not valid JSON (byte 42)")), want: "response is not valid JSON (byte 42)"},
		{name: "decode: only the prefix", err: fmt.Errorf("getMigration: decode response: %w", errors.New("secret-value")), want: "decode response: "},
		{name: "not found", err: fmt.Errorf("getMigration: %w", abertura.ErrNotFound), want: "ErrNotFound"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := describe(tc.err)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
			if strings.Contains(got, "secret-value") {
				t.Errorf("value leaked: %q", got)
			}
		})
	}
}

func TestBudget_RootItems(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{name: "list with three", in: `[{"a":1},{"a":2},{"a":3}]`, want: "items at root: 3"},
		{name: "empty list", in: `[]`, want: "items at root: 0"},
		{name: "an object prints nothing", in: `{"a":1}`, want: ""},
		{name: "not JSON prints nothing", in: `Bad Gateway`, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := rootItems([]byte(tc.in)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBudget_WhenRootIsList_LineBringsCount(t *testing.T) {
	s := localServer(t, 200, `[{"idMigracao":"secret-value"},{"idMigracao":"other-secret"}]`)
	o, err := newBudget(http.DefaultTransport, 90)
	if err != nil {
		t.Fatalf("newBudget: %v", err)
	}
	if err := fetch(t, o, s.URL+"/v1.1/varejista/migracoes/00000000-0000-4000-8000-000000000001"); err != nil {
		t.Fatalf("pegar: %v", err)
	}
	l := strings.Join(o.drain(), "\n")
	if !strings.Contains(l, "items at root: 2") {
		t.Errorf("the line should bring the count: %s", l)
	}
	if strings.Contains(l, "secret") {
		t.Errorf("value leaked: %s", l)
	}
}

func TestBudget_RootOf_WhenListInOperation_KeepsTheCount(t *testing.T) {
	s := localServer(t, 200, `[{"a":1},{"a":1},{"a":1}]`)
	o, err := newBudget(http.DefaultTransport, 90)
	if err != nil {
		t.Fatalf("newBudget: %v", err)
	}
	if got := o.rootOf("getMigration"); got != "" {
		t.Fatalf("before any request rootOf = %q, want empty", got)
	}
	if err := fetch(t, o, s.URL+"/v1.1/varejista/migracoes/00000000-0000-4000-8000-000000000001"); err != nil {
		t.Fatalf("pegar: %v", err)
	}
	if got := o.rootOf("getMigration"); got != "items at root: 3; items equal" {
		t.Errorf("rootOf = %q, want %q", got, "items at root: 3; items equal")
	}
}

func TestBudget_CompareItems(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{name: "one item does not compare", in: `[{"a":1}]`, want: ""},
		{name: "an object does not compare", in: `{"a":1}`, want: ""},
		{name: "identical even with a different field order", in: `[{"a":1,"b":"x"},{"b":"x","a":1}]`, want: "items equal"},
		{
			name: "differ in two fields",
			in:   `[{"statusMigracao":"A","dataValidacao":null,"id":"1"},{"statusMigracao":"B","dataValidacao":"2027-01-01","id":"1"}]`,
			want: "items differ; fields that differ: dataValidacao,statusMigracao",
		},
		{name: "field present on one side only", in: `[{"a":1},{"a":1,"z":2}]`, want: "items differ; fields that differ: z"},
		{name: "three items, only the third differs", in: `[{"a":1},{"a":1},{"a":2}]`, want: "items differ; fields that differ: a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := compareItems([]byte(tc.in)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBudget_WhenRootHasTwoDifferentItems_LineAndKeptBringFields(t *testing.T) {
	s := localServer(t, 200, `[{"idMigracao":"secret-value","statusMigracao":"X"},{"idMigracao":"secret-value","statusMigracao":"Y"}]`)
	o, err := newBudget(http.DefaultTransport, 90)
	if err != nil {
		t.Fatalf("newBudget: %v", err)
	}
	if err := fetch(t, o, s.URL+"/v1.1/varejista/migracoes/00000000-0000-4000-8000-000000000001"); err != nil {
		t.Fatalf("pegar: %v", err)
	}
	want := "items at root: 2; items differ; fields that differ: statusMigracao"
	if got := o.rootOf("getMigration"); got != want {
		t.Errorf("rootOf = %q, want %q", got, want)
	}
	l := strings.Join(o.drain(), "\n")
	if !strings.Contains(l, want) {
		t.Errorf("the line should bring %q: %s", want, l)
	}
	if strings.Contains(l, "secret") {
		t.Errorf("value leaked: %s", l)
	}
}

func TestBudget_FieldTimes_WhenRFC3339_PrintsOnlyTheTime(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "UTC and -03:00", in: `{"dataDenuncia":"2027-01-15T00:00:00Z","dataReferencia":"2027-03-01T00:00:00-03:00"}`, want: "dataDenuncia=T00:00:00Z, dataReferencia=T00:00:00-03:00"},
		{name: "fraction of a second", in: `{"dataSolicitacao":"2027-01-10T14:30:15.123-03:00"}`, want: "dataSolicitacao=T14:30:15.123-03:00"},
		{name: "a bare day has no time", in: `{"dataDenuncia":"2027-01-15"}`, want: ""},
		{name: "a field outside the list does not appear", in: `{"emailUnidadeConsumidora":"2027-01-15T00:00:00Z"}`, want: ""},
		{name: "plain text does not match", in: `{"dataValidacao":"secret T00:00:00Z"}`, want: ""},
		{name: "not JSON", in: `Bad Gateway`, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := fieldTimes([]byte(tc.in))
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
			if strings.Contains(got, "2027") {
				t.Errorf("the date leaked: %q", got)
			}
		})
	}
}

func TestBudget_WhenTemporalFieldRFC3339_LineBringsTimes(t *testing.T) {
	s := localServer(t, 200, `{"migracao":[{"dataReferencia":"2026-12-01T00:00:00-03:00"}]}`)
	o, err := newBudget(http.DefaultTransport, 90)
	if err != nil {
		t.Fatalf("newBudget: %v", err)
	}
	if err := fetch(t, o, s.URL+"/v1.1/varejista/migracoes"); err != nil {
		t.Fatalf("pegar: %v", err)
	}
	l := strings.Join(o.drain(), "\n")
	if !strings.Contains(l, "times: dataReferencia=T00:00:00-03:00") {
		t.Errorf("the times line is missing: %s", l)
	}
	if strings.Contains(l, "2026-12") {
		t.Errorf("the date leaked: %s", l)
	}
}
