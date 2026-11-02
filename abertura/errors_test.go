package abertura

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"unicode"
)

// responseWith builds a response with a status and headers for the tests.
func responseWith(t *testing.T, status int, headers map[string]string) *http.Response {
	t.Helper()

	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{StatusCode: status, Header: h, Body: http.NoBody}
}

func TestDecodeError_WhenRealFormat_FillsFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		status       int
		headers      map[string]string
		body         string
		wantCode     string
		wantMessage  string
		wantRequest  string
		wantGateway  string
		wantRawBytes int
	}{
		{
			name:        "backend 400 field validation, compact and with milliseconds",
			status:      400,
			headers:     map[string]string{"x-amzn-RequestId": "req-fake-1"},
			body:        `{"timestamp":"2026-01-02T03:04:05.678-03:00","status":400,"error":"ERR_VALIDACAO_CAMPOS","message":"fake invalid field"}`,
			wantCode:    "ERR_VALIDACAO_CAMPOS",
			wantMessage: "fake invalid field",
			wantRequest: "req-fake-1",
		},
		{
			name:        "backend 400 profile required",
			status:      400,
			body:        `{"timestamp":"2026-01-02T03:04:05.678-03:00","status":400,"error":"ERR_PERFIL_VAREJISTA_OBRIGATORIO","message":"fake profile missing"}`,
			wantCode:    "ERR_PERFIL_VAREJISTA_OBRIGATORIO",
			wantMessage: "fake profile missing",
		},
		{
			name:        "backend 400 month range too wide",
			status:      400,
			body:        `{"timestamp":"2026-01-02T03:04:05.678-03:00","status":400,"error":"ERR_MES_REFERENCIA_INICIAL_DIFERENCA","message":"fake wide range"}`,
			wantCode:    "ERR_MES_REFERENCIA_INICIAL_DIFERENCA",
			wantMessage: "fake wide range",
		},
		{
			name:        "backend 400 migration already requested",
			status:      400,
			body:        `{"timestamp":"2026-01-02T03:04:05.678-03:00","status":400,"error":"ERR_MIGRACAO_SOLICITADA","message":"fake repeated migration"}`,
			wantCode:    "ERR_MIGRACAO_SOLICITADA",
			wantMessage: "fake repeated migration",
		},
		{
			name:   "authorizer 403 with spaces, without milliseconds and with gateway type",
			status: 403,
			headers: map[string]string{
				"x-amzn-RequestId": "req-fake-2",
				"x-amzn-ErrorType": "AccessDeniedException",
			},
			body:        `{"timestamp": "2026-01-02T03:04:05-03:00", "status": 403, "error": "ERR_CREDENCIAL_INVALIDA", "message": "fake invalid token"}`,
			wantCode:    "ERR_CREDENCIAL_INVALIDA",
			wantMessage: "fake invalid token",
			wantRequest: "req-fake-2",
			wantGateway: "AccessDeniedException",
		},
		{
			name:        "JSON with extra fields is tolerated",
			status:      400,
			body:        `{"error":"ERR_VALIDACAO_CAMPOS","message":"m","content":{"a":1},"extra":[1,2]}`,
			wantCode:    "ERR_VALIDACAO_CAMPOS",
			wantMessage: "m",
		},
		{
			name:        "error field that is not text becomes an empty code",
			status:      400,
			body:        `{"error":{"x":1},"message":"m"}`,
			wantCode:    "",
			wantMessage: "m",
		},
		{
			name:   "502 with plain text has no code or message",
			status: 502,
			body:   "fake bad gateway",
		},
		{
			name:   "empty body",
			status: 500,
		},
		{
			name:         "10 KiB body keeps only 4 KiB",
			status:       500,
			body:         strings.Repeat("x", 10*1024),
			wantRawBytes: 4096,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp := responseWith(t, tc.status, tc.headers)
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Errorf("closing the body: %v", err)
				}
			}()
			got := decodeError("listMigrations", resp, []byte(tc.body))

			if got.Operation != "listMigrations" {
				t.Errorf("Operation = %q", got.Operation)
			}
			if got.StatusCode != tc.status {
				t.Errorf("StatusCode = %d, want %d", got.StatusCode, tc.status)
			}
			if got.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tc.wantCode)
			}
			if got.Message != tc.wantMessage {
				t.Errorf("Message = %q, want %q", got.Message, tc.wantMessage)
			}
			if got.RequestID != tc.wantRequest {
				t.Errorf("RequestID = %q, want %q", got.RequestID, tc.wantRequest)
			}
			if got.GatewayErrorType != tc.wantGateway {
				t.Errorf("GatewayErrorType = %q, want %q", got.GatewayErrorType, tc.wantGateway)
			}
			if tc.wantRawBytes != 0 && len(got.RawBody()) != tc.wantRawBytes {
				t.Errorf("len(RawBody) = %d, want %d", len(got.RawBody()), tc.wantRawBytes)
			}
			if tc.wantRawBytes == 0 && string(got.RawBody()) != tc.body {
				t.Errorf("RawBody = %q, want the original body", got.RawBody())
			}
		})
	}
}

func TestDecodeError_WhenNilResponse_DoesNotPanic(t *testing.T) {
	t.Parallel()

	got := decodeError("do", nil, []byte(`{"error":"ERR_X","message":"m"}`))
	if got.StatusCode != 0 || got.Code != "ERR_X" {
		t.Errorf("got = %+v", got)
	}
}

func TestAPIError_RawBody_ReturnsCopy(t *testing.T) {
	t.Parallel()

	resp := responseWith(t, 500, nil)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing the body: %v", err)
		}
	}()
	e := decodeError("do", resp, []byte("abc"))
	b := e.RawBody()
	b[0] = 'z'
	if string(e.RawBody()) != "abc" {
		t.Errorf("RawBody changed through external modification: %q", e.RawBody())
	}
}

func TestAPIError_Error_WhenFormatted_HasOperationStatusAndCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          *APIError
		wantContains []string
		wantAbsent   []string
		wantMaxLen   int
	}{
		{
			name:         "operation, status and code",
			err:          &APIError{Operation: "createMigration", StatusCode: 400, Code: "ERR_MIGRACAO_SOLICITADA", Message: "already exists"},
			wantContains: []string{"createMigration", "400", "ERR_MIGRACAO_SOLICITADA", "already exists"},
		},
		{
			name:         "without code or message",
			err:          &APIError{Operation: "do", StatusCode: 502},
			wantContains: []string{"do", "502"},
		},
		{
			name:         "long message is cut at 200 characters",
			err:          &APIError{Operation: "do", StatusCode: 400, Code: "ERR_X", Message: strings.Repeat("ã", 500)},
			wantContains: []string{strings.Repeat("ã", 200)},
			wantAbsent:   []string{strings.Repeat("ã", 201)},
		},
		{
			name:         "carries no body or headers",
			err:          &APIError{Operation: "do", StatusCode: 403, Code: "ERR_CREDENCIAL_INVALIDA", RequestID: "req-fake", GatewayErrorType: "AccessDeniedException", rawBody: []byte("body-fake")},
			wantContains: []string{"403"},
			wantAbsent:   []string{"body-fake", "AccessDeniedException"},
		},
		{
			name:         "line breaks in the message become spaces",
			err:          &APIError{Operation: "do", StatusCode: 400, Message: "line1\nline2\r\x00fim"},
			wantContains: []string{"line1 line2"},
			wantAbsent:   []string{"\n", "\r", "\x00"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := tc.err.Error()
			for _, c := range tc.wantContains {
				if !strings.Contains(s, c) {
					t.Errorf("Error() = %q, missing %q", s, c)
				}
			}
			for _, a := range tc.wantAbsent {
				if strings.Contains(s, a) {
					t.Errorf("Error() = %q, should not contain %q", s, a)
				}
			}
			for _, verb := range []string{"%v", "%+v", "%s", "%#v"} {
				if out := fmt.Sprintf(verb, tc.err); strings.Contains(out, "body-fake") {
					t.Errorf("%s leaked the body: %q", verb, out)
				}
			}
		})
	}
}

func TestAPIError_Is_WhenSentinel_MatchesByCodeOrStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{"invalid credential by code", &APIError{StatusCode: 403, Code: "ERR_CREDENCIAL_INVALIDA"}, ErrInvalidCredential, true},
		{"validation by code", &APIError{StatusCode: 400, Code: "ERR_VALIDACAO_CAMPOS"}, ErrFieldValidation, true},
		{"profile required by code", &APIError{StatusCode: 400, Code: "ERR_PERFIL_VAREJISTA_OBRIGATORIO"}, ErrRetailerProfileRequired, true},
		{"migration requested by code", &APIError{StatusCode: 400, Code: "ERR_MIGRACAO_SOLICITADA"}, ErrMigrationAlreadyRequested, true},
		{"month range required by code", &APIError{StatusCode: 400, Code: "ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO"}, ErrMonthRangeRequired, true},
		{"month range too wide by code", &APIError{StatusCode: 400, Code: "ERR_MES_REFERENCIA_INICIAL_DIFERENCA"}, ErrMonthRangeTooWide, true},
		{"the two range errors are not confused", &APIError{StatusCode: 400, Code: "ERR_MES_REFERENCIA_INICIAL_DIFERENCA"}, ErrMonthRangeRequired, false},
		{"same 400 with another code does not match", &APIError{StatusCode: 400, Code: "ERR_VALIDACAO_CAMPOS"}, ErrMigrationAlreadyRequested, false},
		{"code ignores the status", &APIError{StatusCode: 500, Code: "ERR_MIGRACAO_SOLICITADA"}, ErrMigrationAlreadyRequested, true},
		{"403 without code is not an invalid credential", &APIError{StatusCode: 403}, ErrInvalidCredential, false},
		{"404 by status", &APIError{StatusCode: 404}, ErrNotFound, true},
		{"404 even with a code", &APIError{StatusCode: 404, Code: "ERR_X"}, ErrNotFound, true},
		{"429 by status", &APIError{StatusCode: 429}, ErrRateLimited, true},
		{"400 is not not-found", &APIError{StatusCode: 400}, ErrNotFound, false},
		{"repeated cursor never matches an APIError", &APIError{StatusCode: 400}, ErrRepeatedCursor, false},
		{"sentinel matches itself", ErrRepeatedCursor, ErrRepeatedCursor, true},
		{"through fmt.Errorf %w", fmt.Errorf("context: %w", &APIError{StatusCode: 429}), ErrRateLimited, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := errors.Is(tc.err, tc.target); got != tc.want {
				t.Errorf("errors.Is = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAPIError_As_WhenWrapped_Works(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("layer: %w", &APIError{Operation: "do", StatusCode: 400})
	var api *APIError
	if !errors.As(err, &api) || api.StatusCode != 400 {
		t.Fatalf("errors.As falhou: %v", err)
	}
}

type fakeKey struct{}

func (fakeKey) String() string { return "key-fake" }
func (fakeKey) naturalKey()    {}

func TestUncertainResultError_Unwrap_WhenCause_ExposesCauseAndKey(t *testing.T) {
	t.Parallel()

	cause := &APIError{Operation: "createMigration", StatusCode: 503}
	err := fmt.Errorf("layer: %w", &UncertainResultError{
		Operation: "createMigration",
		Key:       fakeKey{},
		Err:       cause,
	})

	var uncertain *UncertainResultError
	if !errors.As(err, &uncertain) {
		t.Fatal("errors.As did not find UncertainResultError")
	}
	if uncertain.Key == nil || uncertain.Key.String() != "key-fake" {
		t.Errorf("Key = %v", uncertain.Key)
	}
	if !errors.Is(err, cause) {
		t.Error("errors.Is did not reach the cause")
	}
	var api *APIError
	if !errors.As(err, &api) || api.StatusCode != 503 {
		t.Error("errors.As did not reach the cause APIError")
	}
}

func TestUncertainResultError_Error_WhenFormatted_OmitsKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *UncertainResultError
		want []string
	}{
		{"with cause", &UncertainResultError{Operation: "createMigration", Key: fakeKey{}, Err: errors.New("fake timeout")}, []string{"createMigration", "uncertain", "fake timeout"}},
		{"without cause", &UncertainResultError{Operation: "do"}, []string{"do", "uncertain"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := tc.err.Error()
			for _, w := range tc.want {
				if !strings.Contains(s, w) {
					t.Errorf("Error() = %q, missing %q", s, w)
				}
			}
			if strings.Contains(s, "key-fake") {
				t.Errorf("Error() leaked the key: %q", s)
			}
		})
	}
}

func TestStripURL_WhenURLError_RemovesURL(t *testing.T) {
	t.Parallel()

	cause := errors.New("fake connection refused")
	err := stripURL(&url.Error{Op: "Get", URL: "https://example.invalid/x?consumerUnit=000123", Err: cause})
	if strings.Contains(err.Error(), "consumerUnit=000") {
		t.Errorf("the URL leaked: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Error("lost the cause")
	}
	if stripURL(nil) != nil {
		t.Error("nil should stay nil")
	}
}

// FuzzDecodeError proves the tolerant decoding of the error body: it never
// panics, keeps at most 4 KiB of the body and builds a one-line Error() without
// control characters and with the code and message cut.
func FuzzDecodeError(f *testing.F) {
	for _, s := range []struct {
		status int
		body   string
	}{
		{400, `{"timestamp":"2027-01-10T10:00:00.000-03:00","status":400,"error":"ERR_VALIDACAO_CAMPOS","message":"Invented"}`},
		{403, `{"timestamp" : "2027-01-10T10:00:00-03:00", "status" : 403, "error" : "ERR_CREDENCIAL_INVALIDA", "message" : "Invented"}`},
		{502, `Bad Gateway`},
		{500, `{"error":7,"message":null}`},
		{400, `{"error":"ERR_X\n","message":"line1\r\nline2"}`},
		{0, ``},
	} {
		f.Add(s.status, []byte(s.body))
	}
	f.Fuzz(func(t *testing.T, status int, body []byte) {
		e := decodeError("listMigrations", &http.Response{StatusCode: status, Header: http.Header{}}, body)
		if n := len(e.RawBody()); n > bodyLimit {
			t.Fatalf("RawBody with %d bytes, more than %d", n, bodyLimit)
		}
		msg := e.Error()
		if strings.ContainsFunc(msg, unicode.IsControl) {
			t.Fatalf("Error() with control character: %q", msg)
		}
		// "abertura: listMigrations: HTTP <status> <code>: <message>"
		cap := len([]rune(fmt.Sprintf("abertura: listMigrations: HTTP %d", status))) + 1 + codeLimit + 2 + messageLimit
		if n := len([]rune(msg)); n > cap {
			t.Fatalf("Error() with %d characters, more than %d", n, cap)
		}
	})
}
