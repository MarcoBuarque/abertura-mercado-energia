package testserver_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura/internal/testserver"
)

const listTemplate = "/v1.1/varejista/migracoes"

// recordingTB captures the failures the server reports, to prove that
// validation really rejects without failing the test that exercises it.
type recordingTB struct {
	testing.TB
	mu   sync.Mutex
	errs []string
}

func (r *recordingTB) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errs = append(r.errs, strings.TrimSpace(sprintf(format, args...)))
}

func (r *recordingTB) failures() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.errs...)
}

// received holds what the tests need from the response, with the body already
// read and closed.
type received struct {
	StatusCode int
	Header     http.Header
}

func get(t testing.TB, url string, headers map[string]string) (received, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close body: %v", err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return received{StatusCode: resp.StatusCode, Header: resp.Header}, string(body)
}

var validHeaders = map[string]string{"codigoAgente": "900002", "codigoPerfil": "900003"}

const validQuery = "?codigoPerfilVarejista=900003&mesReferenciaInicial=2027-01&mesReferenciaFinal=2027-03"

func TestServer_WhenSequence502502200_RespondsInOrderAndRepeatsLast(t *testing.T) {
	t.Parallel()
	srv := testserver.New(t, testserver.Route{
		Method: http.MethodGet, PathTemplate: listTemplate,
		Responses: []testserver.Response{
			{Status: 502, Fixture: "error_502.txt"},
			{Status: 502, Fixture: "error_502.txt"},
			{Status: 200, Fixture: "migrations_page1.json"},
		},
	})
	want := []int{502, 502, 200, 200}
	for i, status := range want {
		resp, body := get(t, srv.URL()+listTemplate+validQuery, validHeaders)
		if resp.StatusCode != status {
			t.Fatalf("call %d: status = %d, want %d", i+1, resp.StatusCode, status)
		}
		if resp.Header.Get("x-amzn-RequestId") == "" {
			t.Errorf("call %d: missing x-amzn-RequestId", i+1)
		}
		if status == 502 && !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
			t.Errorf("call %d: Content-Type = %q, want plain text", i+1, resp.Header.Get("Content-Type"))
		}
		if status == 200 && !strings.Contains(body, "fake-cursor-2") {
			t.Errorf("call %d: body without the fixture: %s", i+1, body)
		}
	}
	if got := srv.Count(http.MethodGet, listTemplate); got != len(want) {
		t.Errorf("Count = %d, want %d", got, len(want))
	}
}

func TestServer_WhenResponseAddsHeader_SendsHeader(t *testing.T) {
	t.Parallel()
	srv := testserver.New(t, testserver.Route{
		Method: http.MethodGet, PathTemplate: listTemplate,
		Responses: []testserver.Response{
			{Status: 403, Fixture: "error_403_authorizer.json"},
			{Status: 429, Fixture: "error_400_validation.json", Headers: map[string]string{"Retry-After": "2"}},
		},
	})
	resp, _ := get(t, srv.URL()+listTemplate+validQuery, validHeaders)
	if got := resp.Header.Get("x-amzn-ErrorType"); got != "AccessDeniedException" {
		t.Errorf("403 without x-amzn-ErrorType, got %q", got)
	}
	resp, _ = get(t, srv.URL()+listTemplate+validQuery, validHeaders)
	if got := resp.Header.Get("Retry-After"); got != "2" {
		t.Errorf("Retry-After = %q, want 2", got)
	}
}

func TestServer_WhenDelay_RespondsLate(t *testing.T) {
	t.Parallel()
	srv := testserver.New(t, testserver.Route{
		Method: http.MethodGet, PathTemplate: listTemplate,
		Responses: []testserver.Response{{Status: 200, Fixture: "migrations_empty.json", Delay: 150 * time.Millisecond}},
	})
	start := time.Now()
	get(t, srv.URL()+listTemplate+validQuery, validHeaders)
	if d := time.Since(start); d < 150*time.Millisecond {
		t.Errorf("responded in %v, want at least 150ms", d)
	}
}

func TestServer_WhenDrop_ClosesConnectionWithoutResponse(t *testing.T) {
	t.Parallel()
	srv := testserver.New(t, testserver.Route{
		Method: http.MethodGet, PathTemplate: listTemplate,
		Responses: []testserver.Response{{Drop: true}},
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL()+listTemplate+validQuery, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	for k, v := range validHeaders {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Errorf("close body: %v", cerr)
		}
		t.Fatalf("expected a connection error, got status %d", resp.StatusCode)
	}
	if got := srv.Count(http.MethodGet, listTemplate); got != 1 {
		t.Errorf("Count = %d, want 1 (the request was read before dropping)", got)
	}
}

func TestServer_WhenRequestOutsideSpec_FailsWithClearMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		path    string
		headers map[string]string
		wantMsg string
	}{
		{name: "without the codigoAgente header", path: listTemplate + validQuery, headers: map[string]string{"codigoPerfil": "900003"}, wantMsg: "codigoAgente"},
		{name: "without the required codigoPerfilVarejista parameter", path: listTemplate + "?mesReferenciaInicial=2027-01&mesReferenciaFinal=2027-03", headers: validHeaders, wantMsg: "codigoPerfilVarejista"},
		{name: "unknown query parameter", path: listTemplate + validQuery + "&unknownParameter=1", headers: validHeaders, wantMsg: "unknownParameter"},
		{name: "path the spec does not have", path: "/v1.1/varejista/inexistente", headers: validHeaders, wantMsg: "inexistente"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := &recordingTB{TB: t}
			srv := testserver.New(rec, testserver.Route{
				Method: http.MethodGet, PathTemplate: listTemplate,
				Responses: []testserver.Response{{Status: 200, Fixture: "migrations_empty.json"}},
			})
			resp, _ := get(t, srv.URL()+tc.path, tc.headers)
			if resp.StatusCode < 400 {
				t.Errorf("status = %d, want an error", resp.StatusCode)
			}
			failures := rec.failures()
			if len(failures) != 1 || !strings.Contains(failures[0], tc.wantMsg) || !strings.Contains(failures[0], "spec") {
				t.Errorf("failures = %q, want one message about the spec mentioning %q", failures, tc.wantMsg)
			}
			if got := srv.Violations(); len(got) != 1 {
				t.Errorf("Violations = %q, want 1", got)
			}
		})
	}
}

func TestServer_WhenRequestValid_ReportsNoViolation(t *testing.T) {
	t.Parallel()
	rec := &recordingTB{TB: t}
	srv := testserver.New(rec, testserver.Route{
		Method: http.MethodGet, PathTemplate: listTemplate,
		Responses: []testserver.Response{{Status: 200, Fixture: "migrations_empty.json"}},
	})
	get(t, srv.URL()+listTemplate+validQuery, validHeaders)
	if f := rec.failures(); len(f) != 0 {
		t.Errorf("unexpected failures: %q", f)
	}
}

func TestServer_WhenRouteNotRegistered_Returns404AndRecordsFailure(t *testing.T) {
	t.Parallel()
	rec := &recordingTB{TB: t}
	srv := testserver.New(rec)
	resp, _ := get(t, srv.URL()+listTemplate+validQuery, validHeaders)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	if f := rec.failures(); len(f) != 1 || !strings.Contains(f[0], "no route registered") {
		t.Errorf("failures = %q", f)
	}
}

func TestServer_Requests_KeepsCopiesWithBody(t *testing.T) {
	t.Parallel()
	srv := testserver.New(t, testserver.Route{
		Method: http.MethodPost, PathTemplate: "/sso/oauth/token",
		Responses: []testserver.Response{{Status: 200, Fixture: "token.json"}},
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/sso/oauth/token",
		strings.NewReader("client_id=fake-client&client_secret=fake-secret&grant_type=client_credentials"))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Errorf("close body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(reqs))
	}
	body, err := io.ReadAll(reqs[0].Body)
	if err != nil {
		t.Fatalf("read kept body: %v", err)
	}
	if !strings.Contains(string(body), "client_id=fake-client") {
		t.Errorf("body kept = %q", body)
	}
	if reqs[0].Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Errorf("header not kept: %v", reqs[0].Header)
	}
}

func sprintf(f string, a ...any) string { return fmt.Sprintf(f, a...) }
