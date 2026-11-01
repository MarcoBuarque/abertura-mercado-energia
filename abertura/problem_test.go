package abertura

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const (
	aboutBlank = "about:blank"
	catalogURL = "https://marcobuarque.github.io/abertura-mercado-energia/errors"
)

func apiError(httpMethod string, status int, code, gateway string) *APIError {
	return &APIError{
		Operation: "createMigration", StatusCode: status, Code: code,
		Message: "API text", RequestID: "req-fake-1",
		GatewayErrorType: gateway, httpMethod: httpMethod,
	}
}

func TestProblemFrom_WhenSDKError_ClassifiesInCatalog(t *testing.T) {
	t.Parallel()

	transportCause := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	tests := []struct {
		name          string
		err           error
		wantType      string
		wantStatus    int
		wantCode      string
		wantRetryable bool
		wantUncertain bool
	}{
		{name: "migration already requested", err: apiError("POST", 400, "ERR_MIGRACAO_SOLICITADA", ""), wantType: "migration-already-requested", wantStatus: 400, wantCode: "ERR_MIGRACAO_SOLICITADA"},
		{name: "field validation", err: apiError("POST", 400, "ERR_VALIDACAO_CAMPOS", ""), wantType: "field-validation", wantStatus: 400, wantCode: "ERR_VALIDACAO_CAMPOS"},
		{name: "profile required", err: apiError("GET", 400, "ERR_PERFIL_VAREJISTA_OBRIGATORIO", ""), wantType: "retailer-profile-required", wantStatus: 400, wantCode: "ERR_PERFIL_VAREJISTA_OBRIGATORIO"},
		{name: "month range required", err: apiError("GET", 400, "ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO", ""), wantType: "month-range-required", wantStatus: 400, wantCode: "ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO"},
		{name: "month range too wide", err: apiError("GET", 400, "ERR_MES_REFERENCIA_INICIAL_DIFERENCA", ""), wantType: "month-range-too-wide", wantStatus: 400, wantCode: "ERR_MES_REFERENCIA_INICIAL_DIFERENCA"},
		{name: "credential refused by the authorizer", err: apiError("GET", 403, "ERR_CREDENCIAL_INVALIDA", "AccessDeniedException"), wantType: "invalid-credential-authorizer", wantStatus: 403, wantCode: "ERR_CREDENCIAL_INVALIDA"},
		{name: "credential refused by the backend", err: apiError("GET", 403, "ERR_CREDENCIAL_INVALIDA", ""), wantType: "invalid-credential-backend", wantStatus: 403, wantCode: "ERR_CREDENCIAL_INVALIDA"},
		{name: "403 without code", err: apiError("GET", 403, "", ""), wantType: "refusal-without-code", wantStatus: 403},
		{name: "API 404", err: apiError("GET", 404, "", ""), wantType: "not-found", wantStatus: 404},
		{name: "wrapped ErrNotFound", err: fmt.Errorf("getMigration: %w", ErrNotFound), wantType: "not-found", wantStatus: 404},
		{name: "429 on read retries", err: apiError("GET", 429, "", ""), wantType: "rate-limited", wantStatus: 429, wantRetryable: true},
		{name: "429 on write without gateway mark does not retry", err: apiError("POST", 429, "", ""), wantType: "rate-limited", wantStatus: 429},
		{name: "429 on write with gateway mark retries", err: apiError("POST", 429, "", "ThrottlingException"), wantType: "rate-limited", wantStatus: 429, wantRetryable: true},
		{name: "500 on read retries", err: apiError("GET", 500, "", ""), wantType: "internal-error", wantStatus: 500, wantRetryable: true},
		{name: "500 on write is uncertain", err: apiError("POST", 500, "", ""), wantType: "internal-error", wantStatus: 500, wantUncertain: true},
		{name: "gateway 502", err: apiError("GET", 502, "", ""), wantType: "gateway-failure", wantStatus: 502, wantRetryable: true},
		{name: "gateway 504 on write", err: apiError("PUT", 504, "", ""), wantType: "gateway-failure", wantStatus: 504, wantUncertain: true},
		{name: "unknown method asserts nothing", err: apiError("", 503, "", ""), wantType: "internal-error", wantStatus: 503},
		{name: "new API code", err: apiError("POST", 400, "ERR_NOVO_DA_API", ""), wantType: "unknown-code", wantStatus: 400, wantCode: "ERR_NOVO_DA_API"},
		{name: "error field that is not a code", err: apiError("POST", 400, "free text", ""), wantType: "refusal-without-code", wantStatus: 400},
		{name: "token request failure", err: &APIError{Operation: tokenOperation, StatusCode: 503, httpMethod: http.MethodGet}, wantType: "token-request-failed", wantStatus: 503, wantRetryable: true},
		{name: "APIError embrulhado com %w", err: fmt.Errorf("layer: %w", apiError("POST", 400, "ERR_MIGRACAO_SOLICITADA", "")), wantType: "migration-already-requested", wantStatus: 400, wantCode: "ERR_MIGRACAO_SOLICITADA"},
		{
			name:     "uncertain with 502 cause",
			err:      &UncertainResultError{Operation: "createMigration", Key: MigrationKey{ConsumerUnit: "000000000000001"}, Err: apiError("POST", 502, "", "")},
			wantType: "uncertain-result", wantStatus: 502, wantUncertain: true,
		},
		{
			name:     "uncertain with transport error",
			err:      fmt.Errorf("layer: %w", &UncertainResultError{Operation: "editMigration", Err: transportCause}),
			wantType: "uncertain-result", wantUncertain: true,
		},
		{name: "repeated cursor", err: fmt.Errorf("listMigrations: %w", ErrRepeatedCursor), wantType: "repeated-cursor", wantRetryable: false},
		{name: "local validation", err: fmt.Errorf("listMeasurements: %w", validateMeasurementFilter(MeasurementFilter{})), wantType: "invalid-argument"},
		{name: "deadline exceeded", err: fmt.Errorf("x: %w", context.DeadlineExceeded), wantType: "timeout", wantRetryable: true},
		{name: "context canceled", err: context.Canceled, wantType: "call-canceled"},
		{name: "network error", err: fmt.Errorf("listMigrations: %w", transportCause), wantType: "transport-error", wantRetryable: true},
		{name: "error foreign to the SDK", err: errors.New("something else"), wantType: "not-in-catalog"},
		{name: "APIError inside errors.Join", err: errors.Join(errors.New("something else"), apiError("POST", 400, "ERR_MIGRACAO_SOLICITADA", "")), wantType: "migration-already-requested", wantStatus: 400, wantCode: "ERR_MIGRACAO_SOLICITADA"},
		{
			name:     "uncertain wins over APIError in errors.Join",
			err:      errors.Join(apiError("GET", 404, "", ""), &UncertainResultError{Operation: "createMigration", Err: apiError("POST", 502, "", "")}),
			wantType: "uncertain-result", wantStatus: 502, wantUncertain: true,
		},
		{name: "deadline inside errors.Join", err: errors.Join(errors.New("close body"), context.DeadlineExceeded), wantType: "timeout", wantRetryable: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := ProblemFrom(tc.err)
			wantType := ErrorCatalogURL + "#" + tc.wantType
			if p.Type != wantType {
				t.Errorf("Type = %q, want %q", p.Type, wantType)
			}
			if p.Title == "" {
				t.Error("Title is empty")
			}
			if p.Status != tc.wantStatus {
				t.Errorf("Status = %d, want %d", p.Status, tc.wantStatus)
			}
			if p.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", p.Code, tc.wantCode)
			}
			if p.Retryable != tc.wantRetryable {
				t.Errorf("Retryable = %v, want %v", p.Retryable, tc.wantRetryable)
			}
			if p.Uncertain != tc.wantUncertain {
				t.Errorf("Uncertain = %v, want %v", p.Uncertain, tc.wantUncertain)
			}
			if p.Instance != "" {
				t.Errorf("Instance = %q, is empty in v0.1", p.Instance)
			}
		})
	}
}

func TestProblemFrom_WhenNil_ReturnsZero(t *testing.T) {
	t.Parallel()
	if p := ProblemFrom(nil); p != (Problem{}) {
		t.Errorf("ProblemFrom(nil) = %+v, want the zero value", p)
	}
}

func TestProblem_WhenAPIError_CarriesOperationAndRequestID(t *testing.T) {
	t.Parallel()
	p := apiError("POST", 400, "ERR_MIGRACAO_SOLICITADA", "").Problem()
	if p.Operation != "createMigration" || p.RequestID != "req-fake-1" {
		t.Errorf("Operation=%q RequestID=%q", p.Operation, p.RequestID)
	}
	if !strings.Contains(p.Detail, "createMigration") {
		t.Errorf("Detail = %q, should mention the operation", p.Detail)
	}
	u := (&UncertainResultError{Operation: "createMigration", Err: apiError("POST", 502, "", "")}).Problem()
	if u.Operation != "createMigration" || u.RequestID != "req-fake-1" {
		t.Errorf("uncertain: Operation=%q RequestID=%q", u.Operation, u.RequestID)
	}
}

// The retry decision comes from decide: this test checks the architecture
// table (section 4.4) independently, so Problem cannot drift from it.
func TestProblem_RetryableAndUncertain_WhenMethodAndStatus_MatchRetryTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		httpMethod    string
		status        int
		gateway       string
		wantRetryable bool
		wantUncertain bool
	}{
		{name: "GET 400", httpMethod: "GET", status: 400},
		{name: "GET 403", httpMethod: "GET", status: 403, gateway: "AccessDeniedException"},
		{name: "GET 404", httpMethod: "GET", status: 404},
		{name: "GET 429", httpMethod: "GET", status: 429, wantRetryable: true},
		{name: "GET 500", httpMethod: "GET", status: 500, wantRetryable: true},
		{name: "GET 502", httpMethod: "GET", status: 502, wantRetryable: true},
		{name: "GET 503", httpMethod: "GET", status: 503, wantRetryable: true},
		{name: "GET 504", httpMethod: "GET", status: 504, wantRetryable: true},
		{name: "POST 400", httpMethod: "POST", status: 400},
		{name: "POST 403", httpMethod: "POST", status: 403, gateway: "AccessDeniedException"},
		{name: "POST 429 without gateway", httpMethod: "POST", status: 429},
		{name: "POST 429 with gateway", httpMethod: "POST", status: 429, gateway: "ThrottlingException", wantRetryable: true},
		{name: "POST 500", httpMethod: "POST", status: 500, wantUncertain: true},
		{name: "PUT 502", httpMethod: "PUT", status: 502, wantUncertain: true},
		{name: "PUT 503", httpMethod: "PUT", status: 503, wantUncertain: true},
		{name: "POST 504", httpMethod: "POST", status: 504, wantUncertain: true},
		{name: "GET 507", httpMethod: "GET", status: 507, wantRetryable: true},
		{name: "POST 520", httpMethod: "POST", status: 520, wantUncertain: true},
		{name: "GET 501 is final", httpMethod: "GET", status: 501},
		{name: "POST 501 is final", httpMethod: "POST", status: 501},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := apiError(tc.httpMethod, tc.status, "", tc.gateway).Problem()
			if p.Retryable != tc.wantRetryable || p.Uncertain != tc.wantUncertain {
				t.Errorf("Retryable=%v Uncertain=%v, want %v e %v", p.Retryable, p.Uncertain, tc.wantRetryable, tc.wantUncertain)
			}
		})
	}
}

func TestProblem_MarshalJSON_WhenComplete_FollowsRFC9457(t *testing.T) {
	t.Parallel()

	p := apiError("POST", 400, "ERR_MIGRACAO_SOLICITADA", "").Problem()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	want := map[string]any{
		"type":      ErrorCatalogURL + "#migration-already-requested",
		"status":    float64(400),
		"code":      "ERR_MIGRACAO_SOLICITADA",
		"operation": "createMigration",
		"requestId": "req-fake-1",
		"retryable": false,
		"uncertain": false,
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %v, want %v", k, m[k], v)
		}
	}
	for _, k := range []string{"title", "detail"} {
		if s, _ := m[k].(string); s == "" {
			t.Errorf("%s missing or empty", k)
		}
	}
	if _, ok := m["instance"]; ok {
		t.Error("instance should be omitted when empty")
	}
}

func TestProblem_MarshalJSON_WhenNoResponse_OmitsEmptyFields(t *testing.T) {
	t.Parallel()

	b, err := json.Marshal(ProblemFrom(errors.New("x")))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, k := range []string{"status", "instance", "code", "operation", "requestId"} {
		if _, ok := m[k]; ok {
			t.Errorf("%s should be omitted: %s", k, b)
		}
	}
	if want := ErrorCatalogURL + "#not-in-catalog"; m["type"] != want {
		t.Errorf("type = %v, want %s", m["type"], want)
	}
}

func TestProblemFrom_WhenMessageHasPersonalData_DoesNotLeak(t *testing.T) {
	t.Parallel()

	sensitive := []string{"000000000000001", "123.456.789-09", "fake.name@example.org", "Fulano Fake"}
	message := "consumer unit 000000000000001 of document 123.456.789-09, contact fake.name@example.org, holder Fulano Fake"
	api := &APIError{
		Operation: "createMigration", StatusCode: 400, Code: "ERR_VALIDACAO_CAMPOS",
		Message: message, RequestID: "req-fake-1", httpMethod: "POST", rawBody: []byte(message),
	}
	dirtyCode := &APIError{Operation: "createMigration", StatusCode: 400, Code: "ERR_" + message, Message: message, httpMethod: "POST"}
	dirtyCause := fmt.Errorf("listMigrations: %s: %w", message, context.DeadlineExceeded)
	errs := map[string]error{
		"api":         api,
		"dirty code":  dirtyCode,
		"uncertain":   &UncertainResultError{Operation: "createMigration", Key: MigrationKey{ConsumerUnit: "000000000000001", Distributor: 900001}, Err: api},
		"dirty cause": dirtyCause,
		"foreign":     errors.New(message),
	}
	for name, err := range errs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b, jerr := json.Marshal(ProblemFrom(err))
			if jerr != nil {
				t.Fatalf("Marshal: %v", jerr)
			}
			for _, s := range sensitive {
				if strings.Contains(string(b), s) {
					t.Errorf("JSON leaked %q: %s", s, b)
				}
			}
		})
	}
	if !strings.Contains(api.Message, "000000000000001") {
		t.Error("the API message must stay accessible in APIError.Message")
	}
}

// --- parity between the table, the code and docs/errors.md ---

var reAnchor = regexp.MustCompile(`<a id="([a-z0-9-]+)"></a>`)

func readFile(t *testing.T, path string) string {
	t.Helper()
	root, err := os.OpenRoot("..")
	if err != nil {
		t.Fatalf("open the repository root: %v", err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Errorf("close the root: %v", err)
		}
	}()
	b, err := root.ReadFile(strings.TrimPrefix(path, "../"))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestCatalog_WhenComparedWithDocs_AnchorsMatchTable(t *testing.T) {
	t.Parallel()

	doc := readFile(t, "../docs/errors.md")
	anchors := map[string]bool{}
	for _, m := range reAnchor.FindAllStringSubmatch(doc, -1) {
		if anchors[m[1]] {
			t.Errorf("duplicate anchor in docs/errors.md: %s", m[1])
		}
		anchors[m[1]] = true
	}
	for slug, e := range catalog {
		if !anchors[slug] {
			t.Errorf("slug %q da tabela has no anchor in docs/errors.md", slug)
		}
		if e.title == "" || e.detail == "" {
			t.Errorf("entrada %q without title or detail", slug)
		}
		if !regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`).MatchString(slug) {
			t.Errorf("slug %q does not match the pattern", slug)
		}
	}
	for a := range anchors {
		if _, ok := catalog[a]; !ok {
			t.Errorf("anchor %q of docs/errors.md is not in the table", a)
		}
	}
	if ErrorCatalogURL != catalogURL {
		t.Errorf("ErrorCatalogURL = %q, want %q", ErrorCatalogURL, catalogURL)
	}
}

// TestCatalog_WhenComparedWithDocs_TitlesMatchTable proves that the title of
// each table entry is the same as the heading right below the anchor and the
// link in the index of docs/errors.md: the Problem and the catalog say the
// same thing.
func TestCatalog_WhenComparedWithDocs_TitlesMatchTable(t *testing.T) {
	t.Parallel()

	doc := readFile(t, "../docs/errors.md")
	for slug, e := range catalog {
		heading := `<a id="` + slug + `"></a>` + "\n## " + e.title + "\n"
		if !strings.Contains(doc, heading) {
			t.Errorf("entrada %q: the heading after the anchor is not %q", slug, "## "+e.title)
		}
		if index := "[" + e.title + "](#" + slug + ")"; !strings.Contains(doc, index) {
			t.Errorf("entrada %q: the index does not contain %q", slug, index)
		}
	}
}

func TestCatalog_WhenSentinelExported_HasEntry(t *testing.T) {
	t.Parallel()

	// Each exported sentinel with the error shape the SDK really returns.
	covered := map[string]error{
		"ErrInvalidCredential":         apiError("GET", 403, "ERR_CREDENCIAL_INVALIDA", "x"),
		"ErrFieldValidation":           apiError("POST", 400, "ERR_VALIDACAO_CAMPOS", ""),
		"ErrRetailerProfileRequired":   apiError("GET", 400, "ERR_PERFIL_VAREJISTA_OBRIGATORIO", ""),
		"ErrMigrationAlreadyRequested": apiError("POST", 400, "ERR_MIGRACAO_SOLICITADA", ""),
		"ErrMonthRangeRequired":        apiError("GET", 400, "ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO", ""),
		"ErrMonthRangeTooWide":         apiError("GET", 400, "ERR_MES_REFERENCIA_INICIAL_DIFERENCA", ""),
		"ErrNotFound":                  ErrNotFound,
		"ErrRateLimited":               apiError("GET", 429, "", ""),
		"ErrRepeatedCursor":            ErrRepeatedCursor,
		"ErrInvalidArgument":           ErrInvalidArgument,
	}
	sentinels := map[string]error{
		"ErrInvalidCredential": ErrInvalidCredential, "ErrFieldValidation": ErrFieldValidation,
		"ErrRetailerProfileRequired": ErrRetailerProfileRequired, "ErrMigrationAlreadyRequested": ErrMigrationAlreadyRequested,
		"ErrMonthRangeRequired": ErrMonthRangeRequired, "ErrMonthRangeTooWide": ErrMonthRangeTooWide,
		"ErrNotFound": ErrNotFound, "ErrRateLimited": ErrRateLimited,
		"ErrRepeatedCursor": ErrRepeatedCursor, "ErrInvalidArgument": ErrInvalidArgument,
	}

	// Ensures the list above did not fall behind: reads the package declarations.
	declared := exportedPackageErrors(t)
	var names []string
	for n := range covered {
		names = append(names, n)
	}
	sort.Strings(names)
	if got := strings.Join(declared, ","); got != strings.Join(names, ",") {
		t.Errorf("sentinels exportadas = %s; o teste cobre %s", got, strings.Join(names, ","))
	}

	for name, err := range covered {
		p := ProblemFrom(fmt.Errorf("layer: %w", err))
		if !strings.HasPrefix(p.Type, ErrorCatalogURL+"#") {
			t.Errorf("%s: Type = %q, expected a catalog entry", name, p.Type)
			continue
		}
		slug := strings.TrimPrefix(p.Type, ErrorCatalogURL+"#")
		if _, ok := catalog[slug]; !ok {
			t.Errorf("%s: slug %q is not in the table", name, slug)
		}
		// The sentinel itself must also match the classified error.
		if !errors.Is(err, sentinels[name]) {
			t.Errorf("%s: errors.Is failed", name)
		}
	}
}

// exportedPackageErrors lists, in order, the exported Err* variables of the
// package files (excluding test files).
func exportedPackageErrors(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	var out []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("ParseFile %s: %v", name, err)
		}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, s := range gd.Specs {
				vs, ok := s.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, n := range vs.Names {
					if n.IsExported() && strings.HasPrefix(n.Name, "Err") {
						out = append(out, n.Name)
					}
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

var reSpecCode = regexp.MustCompile(`(?m)^\s+- (ERR_[A-Z_]+)\s*$`)

func TestCatalog_WhenSpecCode_HasEntryAndDocsMention(t *testing.T) {
	t.Parallel()

	spec := readFile(t, "../openapi/abertura.yaml")
	doc := readFile(t, "../docs/errors.md")
	codes := reSpecCode.FindAllStringSubmatch(spec, -1)
	if len(codes) < 4 {
		t.Fatalf("found %d codes in the spec, expected at least 4", len(codes))
	}
	for _, m := range codes {
		code := m[1]
		p := apiError("POST", 400, code, "x").Problem()
		if strings.HasSuffix(p.Type, "#unknown-code") || strings.HasSuffix(p.Type, "#refusal-without-code") {
			t.Errorf("spec code %s has no entry of its own: %s", code, p.Type)
		}
		if !strings.Contains(doc, code) {
			t.Errorf("spec code %s does not appear in docs/errors.md", code)
		}
	}
}

func TestCatalog_WhenConditionWithoutCode_HasDocsEntry(t *testing.T) {
	t.Parallel()

	doc := readFile(t, "../docs/errors.md")
	for _, name := range []string{"ErrNotFound", "ErrRateLimited", "ErrRepeatedCursor", "ErrInvalidArgument", "UncertainResultError", "CheckCreated", "CheckEdited"} {
		if !strings.Contains(doc, name) {
			t.Errorf("docs/errors.md does not mention %s", name)
		}
	}
}
