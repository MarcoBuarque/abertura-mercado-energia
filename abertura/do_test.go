package abertura

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type testOutput struct {
	OK bool `json:"ok"`
}

func TestDo_WhenRead_DecodesIntoOutput(t *testing.T) {
	t.Parallel()

	srv := newServer(t, sequence(t, stubResponse{status: 200, body: okBody}))
	c, _ := newTestClient(t, srv)

	t.Run("typed struct", func(t *testing.T) {
		var out testOutput
		if err := c.Do(context.Background(), "GET", "/v1/x", nil, &out); err != nil {
			t.Fatal(err)
		}
		if !out.OK {
			t.Errorf("out = %+v", out)
		}
	})
	t.Run("json.RawMessage", func(t *testing.T) {
		var raw json.RawMessage
		if err := c.Do(context.Background(), "get", "/v1/x", nil, &raw); err != nil {
			t.Fatal(err)
		}
		if string(raw) != okBody {
			t.Errorf("raw = %s", raw)
		}
	})
	t.Run("nil out ignores the body", func(t *testing.T) {
		if err := c.Do(context.Background(), "GET", "/v1/x", nil, nil); err != nil {
			t.Fatal(err)
		}
	})
}

func TestDo_WhenEmptyBody_LeavesOutUntouched(t *testing.T) {
	t.Parallel()

	srv := newServer(t, sequence(t, stubResponse{status: 204}))
	c, _ := newTestClient(t, srv)

	out := testOutput{OK: true}
	if err := c.Do(context.Background(), "PUT", "/v1/x", map[string]int{"a": 1}, &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Errorf("out was changed: %+v", out)
	}
}

func TestDo_WhenCalled_SendsBodyQueryAndCallOptions(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var seen struct {
		httpMethod, path, query, body, agent, profile string
	}
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		read, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading the body: %v", err)
		}
		mu.Lock()
		seen.httpMethod, seen.path, seen.query, seen.body = r.Method, r.URL.Path, r.URL.RawQuery, string(read)
		seen.agent, seen.profile = r.Header.Get("codigoAgente"), r.Header.Get("codigoPerfil")
		mu.Unlock()
		writeBody(t, w, okBody)
	})
	c, _ := newTestClient(t, srv)

	err := c.Do(context.Background(), "POST", "/v1.1/varejista/migracoes?codigoUnidadeConsumidora="+fakeConsumerUnit+"&b=2",
		map[string]string{"k": "v"}, nil, WithAgent(900011, 900012))
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if seen.httpMethod != "POST" || seen.path != "/v1.1/varejista/migracoes" {
		t.Errorf("seen = %+v", seen)
	}
	if seen.query != "b=2&codigoUnidadeConsumidora="+fakeConsumerUnit {
		t.Errorf("query = %q", seen.query)
	}
	if seen.body != `{"k":"v"}` {
		t.Errorf("body = %q", seen.body)
	}
	if seen.agent != "900011" || seen.profile != "900012" {
		t.Errorf("agent/profile = %q/%q", seen.agent, seen.profile)
	}
}

func TestDo_WhenFailure_ReturnsExpectedError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		httpMethod    string
		resp          stubResponse
		out           any
		wantUncertain bool
		wantStatus    int // expected *APIError
		wantIs        error
		wantAny       bool // any error, no specific type
	}{
		{name: "POST 502 becomes uncertain with nil Key", httpMethod: "POST", resp: stubResponse{status: 502}, wantUncertain: true},
		{name: "PUT 503 becomes uncertain with nil Key", httpMethod: "PUT", resp: stubResponse{status: 503}, wantUncertain: true},
		{name: "POST 200 unreadable for out becomes uncertain", httpMethod: "POST", resp: stubResponse{status: 200, body: "<html>"}, out: &testOutput{}, wantUncertain: true},
		{name: "POST 200 with wrong type becomes uncertain", httpMethod: "POST", resp: stubResponse{status: 200, body: `{"ok":"yes"}`}, out: &testOutput{}, wantUncertain: true},
		{name: "GET 200 unreadable is a plain error", httpMethod: "GET", resp: stubResponse{status: 200, body: "<html>"}, out: &testOutput{}, wantAny: true},
		{name: "GET 400 returns APIError from do", httpMethod: "GET", resp: stubResponse{status: 400, body: errorJSON("ERR_VALIDACAO_CAMPOS")}, wantStatus: 400, wantIs: ErrFieldValidation},
		{name: "GET 404 matches ErrNotFound", httpMethod: "GET", resp: stubResponse{status: 404}, wantStatus: 404, wantIs: ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := newServer(t, sequence(t, tc.resp))
			c, _ := newTestClient(t, srv, WithRetries(0))

			err := c.Do(context.Background(), tc.httpMethod, "/v1/x?codigoUnidadeConsumidora="+fakeConsumerUnit, map[string]string{"a": "b"}, tc.out)
			if err == nil {
				t.Fatal("expected an error")
			}
			var uncertain *UncertainResultError
			if got := errors.As(err, &uncertain); got != tc.wantUncertain {
				t.Fatalf("uncertain = %v, want %v (%v)", got, tc.wantUncertain, err)
			}
			if tc.wantUncertain {
				if uncertain.Key != nil || uncertain.Operation != "do" {
					t.Errorf("Key=%v Operation=%q, want nil and \"do\"", uncertain.Key, uncertain.Operation)
				}
			}
			if tc.wantStatus != 0 {
				var api *APIError
				if !errors.As(err, &api) || api.StatusCode != tc.wantStatus || api.Operation != "do" {
					t.Fatalf("error = %v, want APIError %d from do", err, tc.wantStatus)
				}
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("errors.Is(%v) = false", tc.wantIs)
			}
			assertNoLeak(t, err.Error(), srv.URL, "<html>", "yes")
		})
	}
}

func TestDo_WhenInvalidInput_ReturnsErrorWithoutSending(t *testing.T) {
	t.Parallel()

	srv := newServer(t, sequence(t, stubResponse{status: 200, body: okBody}))
	c, _ := newTestClient(t, srv)

	tests := []struct {
		name       string
		httpMethod string
		path       string
		body       any
	}{
		{"empty method", "", "/v1/x", nil},
		{"path without leading slash", "GET", "v1/x", nil},
		{"empty path", "GET", "", nil},
		{"invalid query does not leak", "GET", "/v1/x?codigoUnidadeConsumidora=%zz" + fakeConsumerUnit, nil},
		{"body that is not JSON", "POST", "/v1/x", make(chan int)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := c.Do(context.Background(), tc.httpMethod, tc.path, tc.body, nil)
			if err == nil {
				t.Fatal("expected an error")
			}
			assertNoLeak(t, err.Error())
		})
	}
	if srv.reqs.Load() != 0 {
		t.Errorf("nothing should be sent: %d", srv.reqs.Load())
	}
}

func TestDo_WhenLogging_RouteHasNoQuery(t *testing.T) {
	t.Parallel()

	srv := newServer(t, sequence(t, stubResponse{status: 400, body: errorJSON("ERR_VALIDACAO_CAMPOS")}))
	logger, buf := newLogger()
	c, _ := newTestClient(t, srv, WithLogger(logger))

	err := c.Do(context.Background(), "GET", "/v1.1/varejista/migracoes?codigoUnidadeConsumidora="+fakeConsumerUnit, nil, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	debug := byLevel(logRecords(t, buf), "DEBUG")
	if len(debug) != 1 {
		t.Fatalf("Debug = %d\n%s", len(debug), buf.String())
	}
	if debug[0]["op"] != "do" || debug[0]["route"] != "/v1.1/varejista/migracoes" || debug[0]["code"] != "ERR_VALIDACAO_CAMPOS" {
		t.Errorf("Debug = %v", debug[0])
	}
	assertLogClean(t, buf)
}

func TestDo_WhenRoute_PicksLimiterGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want limitGroup
	}{
		{"migrations use the general bucket", "/v1.1/varejista/migracoes", groupGeneral},
		{"measurements use their own", "/v1/varejista/consumo/medicoes", groupMeasurements},
		{"measurements with id", "/v1/varejista/consumo/medicoes/abc", groupMeasurements},
		{"similar name does not count", "/v1/medicoesx/outra", groupGeneral},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := groupForRoute(tc.path); got != tc.want {
				t.Errorf("groupForRoute(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestDecodeOutput_WhenValueMismatches_DoesNotLeakValue(t *testing.T) {
	t.Parallel()
	type saida struct {
		Consumption json.Number `json:"consumo"`
		Name        string      `json:"name"`
	}
	// The shape of the encoding/json error changes across Go versions (in 1.27
	// Field may be empty and an invalid number becomes an UnmarshalTypeError),
	// so each case accepts any of the fixed texts and forbids the value.
	tests := []struct {
		name      string
		body      string
		wantOne   []string
		wantNever []string
	}{
		{name: "number with decimal comma", body: `{"consumo":"1.234,56"}`,
			wantOne:   []string{"decode response: unexpected JSON", "has an unexpected type (expected json.Number)"},
			wantNever: []string{"1.234", "56"}},
		{name: "text where a number is expected", body: `{"consumo":true}`,
			wantOne: []string{`field "consumo" has an unexpected type (expected json.Number)`, "value has an unexpected type (expected json.Number)"}},
		{name: "number where text is expected", body: `{"name":1.5}`,
			wantOne:   []string{`field "name" has an unexpected type (expected string)`, "value has an unexpected type (expected string)"},
			wantNever: []string{"1.5"}},
		{name: "truncated JSON quotes only the byte", body: `{"consumo":`, wantOne: []string{"response is not valid JSON (byte "}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out saida
			err := decodeOutput([]byte(tc.body), &out)
			if err == nil {
				t.Fatal("want an error")
			}
			found := false
			for _, w := range tc.wantOne {
				if strings.Contains(err.Error(), w) {
					found = true
				}
			}
			if !found {
				t.Errorf("error %q contains none of %q", err, tc.wantOne)
			}
			for _, n := range tc.wantNever {
				if strings.Contains(err.Error(), n) {
					t.Errorf("error %q leaked %q", err, n)
				}
			}
		})
	}
}
