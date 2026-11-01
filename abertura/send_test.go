package abertura

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

const (
	fakeConsumerUnit = "000000000000001"
	fakeAgent        = 900001
	fakeProfile      = 900002
	gatewayMark      = "AccessDeniedException"
)

// sleepRecorder records the waits the loop asked for, without really sleeping.
type sleepRecorder struct {
	mu    sync.Mutex
	waits []time.Duration
}

func (r *sleepRecorder) sleep(ctx context.Context, d time.Duration) error {
	r.mu.Lock()
	r.waits = append(r.waits, d)
	r.mu.Unlock()
	return ctx.Err()
}

func (r *sleepRecorder) list() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Duration(nil), r.waits...)
}

// counter counts the requests that go through the HTTP client transport.
type counter struct {
	n    atomic.Int32
	base http.RoundTripper
}

func (c *counter) RoundTrip(r *http.Request) (*http.Response, error) {
	c.n.Add(1)
	return c.base.RoundTrip(r)
}

func newCounter() (*http.Client, *counter) {
	c := &counter{base: http.DefaultTransport}
	return &http.Client{Transport: c}, c
}

// testServer serves the token endpoint and delegates the rest to the API handler.
type testServer struct {
	*httptest.Server
	tokens atomic.Int32 // tokens issued
	reqs   atomic.Int32 // API requests (excluding the token)
}

func newServer(t *testing.T, api http.HandlerFunc) *testServer {
	t.Helper()

	s := &testServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/sso/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		n := s.tokens.Add(1)
		w.Header().Set("Content-Type", "application/json")
		writeBody(t, w, fmt.Sprintf(`{"access_token":"fake-token-%d","token_type":"Bearer","expires_in":300}`, n))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.reqs.Add(1)
		api(w, r)
	})
	// Like the real API (spike section i): every response closes the connection.
	// Without it net/http would retry a GET on its own on a reused connection and
	// the attempt count on the server would not match the SDK count.
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *testServer) env() Environment {
	return Environment{
		Name:     "test",
		APIURL:   s.URL,
		TokenURL: s.URL + "/sso/oauth/token",
	}
}

// newTestClient builds a client against the server, with fake credentials and
// agent, with no real sleeping in the waits and no rate limiter.
func newTestClient(t *testing.T, s *testServer, opts ...Option) (*Client, *sleepRecorder) {
	t.Helper()

	all := append([]Option{
		WithCredentials(fakeClientID, fakeSecret),
		WithAgent(fakeAgent, fakeProfile),
		WithRateLimits(RateLimits{}),
	}, opts...)
	return newClientWithEnv(t, s.env(), all...)
}

func newClientWithEnv(t *testing.T, env Environment, opts ...Option) (*Client, *sleepRecorder) {
	t.Helper()

	c, err := NewClient(env, opts...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	reg := &sleepRecorder{}
	c.sleep = reg.sleep
	c.rnd = func() float64 { return 1 }
	return c, reg
}

func fixedSource(token string) Option {
	return WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}))
}

func readOp() operation {
	return operation{
		id:           "listMigrations",
		httpMethod:   http.MethodGet,
		pathTemplate: "/v1.1/varejista/migracoes",
		path:         "/v1.1/varejista/migracoes",
		query:        url.Values{"codigoUnidadeConsumidora": {fakeConsumerUnit}},
		group:        groupGeneral,
	}
}

var testKey = MigrationKey{ConsumerUnit: fakeConsumerUnit, Distributor: 900003}

func writeOp(httpMethod string) operation {
	return operation{
		id:           "createMigration",
		httpMethod:   httpMethod,
		pathTemplate: "/v1.1/varejista/migracoes",
		path:         "/v1.1/varejista/migracoes",
		body:         map[string]string{"codigoUnidadeConsumidora": fakeConsumerUnit},
		group:        groupGeneral,
		key:          testKey,
	}
}

// stubResponse describes a response of a fake server.
type stubResponse struct {
	status int
	header map[string]string
	body   string
	drop   bool // reads the request and drops the connection without responding
}

func gateway(h map[string]string) map[string]string {
	out := map[string]string{"x-amzn-ErrorType": gatewayMark}
	for k, v := range h {
		out[k] = v
	}
	return out
}

const okBody = `{"ok":true}`

func errorJSON(code string) string {
	return fmt.Sprintf(`{"timestamp":"2026-10-30T11:56:47.181-03:00","status":0,"error":%q,"message":"invented message"}`, code)
}

// sequence responds in order; once exhausted, it repeats the last one.
func sequence(t *testing.T, rs ...stubResponse) http.HandlerFunc {
	t.Helper()

	var n atomic.Int32
	return func(w http.ResponseWriter, r *http.Request) {
		i := min(int(n.Add(1))-1, len(rs)-1)
		dropOrRespond(t, w, r, rs[i])
	}
}

func dropOrRespond(t *testing.T, w http.ResponseWriter, r *http.Request, rs stubResponse) {
	t.Helper()

	if rs.drop {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("the server does not support Hijack")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		if err := conn.Close(); err != nil {
			t.Errorf("closing the connection: %v", err)
		}
		return
	}
	for k, v := range rs.header {
		w.Header().Set(k, v)
	}
	w.WriteHeader(rs.status)
	writeBody(t, w, rs.body)
}

func TestSend_WhenResponseSequence_ReturnsExpectedOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		op            operation
		responses     []stubResponse
		wantAttempts  int32
		wantStatus    int  // expected success
		wantAPIStatus int  // expected *APIError
		wantUncertain bool // expected *UncertainResultError
		wantIs        error
		wantWaits     []time.Duration
	}{
		{
			name:         "GET 502, 502, 200 succeeds on the third",
			op:           readOp(),
			responses:    []stubResponse{{status: 502}, {status: 502}, {status: 200, body: okBody}},
			wantAttempts: 3,
			wantStatus:   200,
			wantWaits:    []time.Duration{500 * time.Millisecond, time.Second},
		},
		{
			name:          "GET 502 three times returns APIError 502",
			op:            readOp(),
			responses:     []stubResponse{{status: 502, body: errorJSON("ERR_X")}},
			wantAttempts:  3,
			wantAPIStatus: 502,
		},
		{
			name:         "GET 500 then 200",
			op:           readOp(),
			responses:    []stubResponse{{status: 500}, {status: 200, body: okBody}},
			wantAttempts: 2,
			wantStatus:   200,
		},
		{
			name:          "GET 400 does not retry and matches the code sentinel",
			op:            readOp(),
			responses:     []stubResponse{{status: 400, body: errorJSON("ERR_VALIDACAO_CAMPOS")}},
			wantAttempts:  1,
			wantAPIStatus: 400,
			wantIs:        ErrFieldValidation,
		},
		{
			name:          "GET 404 does not retry",
			op:            readOp(),
			responses:     []stubResponse{{status: 404}},
			wantAttempts:  1,
			wantAPIStatus: 404,
			wantIs:        ErrNotFound,
		},
		{
			name:         "POST 200 succeeds",
			op:           writeOp(http.MethodPost),
			responses:    []stubResponse{{status: 200, body: okBody}},
			wantAttempts: 1,
			wantStatus:   200,
		},
		{
			name:          "POST 502 becomes uncertain with the operation key",
			op:            writeOp(http.MethodPost),
			responses:     []stubResponse{{status: 502}, {status: 200, body: okBody}},
			wantAttempts:  1,
			wantUncertain: true,
		},
		{
			name:          "PUT 504 becomes uncertain",
			op:            writeOp(http.MethodPut),
			responses:     []stubResponse{{status: 504}},
			wantAttempts:  1,
			wantUncertain: true,
		},
		{
			name:          "POST 400 for a duplicate is an error, not uncertain",
			op:            writeOp(http.MethodPost),
			responses:     []stubResponse{{status: 400, body: errorJSON("ERR_MIGRACAO_SOLICITADA")}},
			wantAttempts:  1,
			wantAPIStatus: 400,
			wantIs:        ErrMigrationAlreadyRequested,
		},
		{
			name:          "POST 429 without the authorizer mark is ErrRateLimited",
			op:            writeOp(http.MethodPost),
			responses:     []stubResponse{{status: 429}, {status: 200, body: okBody}},
			wantAttempts:  1,
			wantAPIStatus: 429,
			wantIs:        ErrRateLimited,
		},
		{
			name:         "POST 429 with the authorizer mark retries",
			op:           writeOp(http.MethodPost),
			responses:    []stubResponse{{status: 429, header: gateway(nil)}, {status: 200, body: okBody}},
			wantAttempts: 2,
			wantStatus:   200,
			wantWaits:    []time.Duration{2 * time.Second},
		},
		{
			name:         "GET 429 retries with the 429 wait",
			op:           readOp(),
			responses:    []stubResponse{{status: 429}, {status: 200, body: okBody}},
			wantAttempts: 2,
			wantStatus:   200,
			wantWaits:    []time.Duration{2 * time.Second},
		},
		{
			name:         "GET 429 with Retry-After uses the header",
			op:           readOp(),
			responses:    []stubResponse{{status: 429, header: map[string]string{"Retry-After": "7"}}, {status: 200, body: okBody}},
			wantAttempts: 2,
			wantStatus:   200,
			wantWaits:    []time.Duration{7 * time.Second},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := newServer(t, sequence(t, tc.responses...))
			c, reg := newTestClient(t, srv)

			status, body, err := c.send(context.Background(), tc.op, callConfig{})

			if got := srv.reqs.Load(); got != tc.wantAttempts {
				t.Errorf("attempts = %d, want %d", got, tc.wantAttempts)
			}
			switch {
			case tc.wantStatus != 0:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if status != tc.wantStatus || string(body) != okBody {
					t.Errorf("status=%d body=%q", status, body)
				}
			case tc.wantUncertain:
				var uncertain *UncertainResultError
				if !errors.As(err, &uncertain) {
					t.Fatalf("error = %v (%T), want *UncertainResultError", err, err)
				}
				if uncertain.Operation != tc.op.id {
					t.Errorf("Operation = %q", uncertain.Operation)
				}
				if uncertain.Key != NaturalKey(testKey) {
					t.Errorf("Key = %v, want the operation key", uncertain.Key)
				}
				var api *APIError
				if !errors.As(err, &api) || api.StatusCode < 500 {
					t.Errorf("the cause should be a 5xx APIError, got %v", uncertain.Err)
				}
			default:
				var api *APIError
				if !errors.As(err, &api) {
					t.Fatalf("error = %v (%T), want *APIError", err, err)
				}
				var uncertain *UncertainResultError
				if errors.As(err, &uncertain) {
					t.Errorf("should not be uncertain: %v", err)
				}
				if api.StatusCode != tc.wantAPIStatus {
					t.Errorf("StatusCode = %d, want %d", api.StatusCode, tc.wantAPIStatus)
				}
				if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
					t.Errorf("errors.Is(%v, %v) = false", err, tc.wantIs)
				}
			}
			if tc.wantWaits != nil {
				got := reg.list()
				if len(got) < len(tc.wantWaits) {
					t.Fatalf("waits = %v, want %v", got, tc.wantWaits)
				}
				for i, w := range tc.wantWaits {
					if got[i] != w {
						t.Errorf("wait[%d] = %v, want %v", i, got[i], w)
					}
				}
			}
		})
	}
}

// With no retries configured (WithRetries(0)) nothing is retried.
func TestSend_WhenNoRetries_DoesNotRetry(t *testing.T) {
	t.Parallel()

	srv := newServer(t, sequence(t, stubResponse{status: 502}, stubResponse{status: 200, body: okBody}))
	c, _ := newTestClient(t, srv, WithRetries(0))

	_, _, err := c.send(context.Background(), readOp(), callConfig{})
	var api *APIError
	if !errors.As(err, &api) || api.StatusCode != 502 {
		t.Fatalf("error = %v, want APIError 502", err)
	}
	if got := srv.reqs.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
}

func TestSend_WhenTransportFailure_RetriesOrIsUncertain(t *testing.T) {
	t.Parallel()

	derruba := stubResponse{drop: true}
	truncated := func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		t.Helper()
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("the server does not support Hijack")
			return
		}
		conn, rw, err := hj.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		writeRaw(t, rw, "HTTP/1.1 200 OK\r\nContent-Length: 50\r\nContent-Type: application/json\r\n\r\n{\"ok\"")
		if err := conn.Close(); err != nil {
			t.Errorf("closing the connection: %v", err)
		}
	}
	slow := func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		t.Helper()
		drain(t, r)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}

	tests := []struct {
		name          string
		op            operation
		api           func(t *testing.T) http.HandlerFunc
		opts          []Option
		wantAttempts  int32
		wantUncertain bool
	}{
		{
			name:         "GET with dropped connection retries and returns a transport error",
			op:           readOp(),
			api:          func(t *testing.T) http.HandlerFunc { return sequence(t, derruba) },
			wantAttempts: 3,
		},
		{
			name:          "POST with dropped connection after reading is uncertain",
			op:            writeOp(http.MethodPost),
			api:           func(t *testing.T) http.HandlerFunc { return sequence(t, derruba) },
			wantAttempts:  1,
			wantUncertain: true,
		},
		{
			name:          "PUT with dropped connection after reading is uncertain",
			op:            writeOp(http.MethodPut),
			api:           func(t *testing.T) http.HandlerFunc { return sequence(t, derruba) },
			wantAttempts:  1,
			wantUncertain: true,
		},
		{
			name: "GET with attempt timeout retries",
			op:   readOp(),
			api: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) { slow(t, w, r) }
			},
			opts:         []Option{WithTimeout(40 * time.Millisecond)},
			wantAttempts: 3,
		},
		{
			name: "POST with attempt timeout is uncertain",
			op:   writeOp(http.MethodPost),
			api: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) { slow(t, w, r) }
			},
			opts:          []Option{WithTimeout(40 * time.Millisecond)},
			wantAttempts:  1,
			wantUncertain: true,
		},
		{
			name: "POST 200 with truncated body is uncertain",
			op:   writeOp(http.MethodPost),
			api: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) { truncated(t, w, r) }
			},
			wantAttempts:  1,
			wantUncertain: true,
		},
		{
			name: "GET 200 with truncated body retries",
			op:   readOp(),
			api: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) { truncated(t, w, r) }
			},
			wantAttempts: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := newServer(t, tc.api(t))
			c, _ := newTestClient(t, srv, tc.opts...)

			_, _, err := c.send(context.Background(), tc.op, callConfig{})
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := srv.reqs.Load(); got != tc.wantAttempts {
				t.Errorf("attempts = %d, want %d", got, tc.wantAttempts)
			}
			var uncertain *UncertainResultError
			if got := errors.As(err, &uncertain); got != tc.wantUncertain {
				t.Errorf("uncertain = %v, want %v (error: %v)", got, tc.wantUncertain, err)
			}
			if tc.wantUncertain && uncertain.Key != NaturalKey(testKey) {
				t.Errorf("Key = %v", uncertain.Key)
			}
			assertNoLeak(t, err.Error(), srv.URL)
		})
	}
}

func writeRaw(t *testing.T, rw *bufio.ReadWriter, s string) {
	t.Helper()

	if _, err := rw.WriteString(s); err != nil {
		t.Errorf("writing: %v", err)
		return
	}
	if err := rw.Flush(); err != nil {
		t.Errorf("flush: %v", err)
	}
}

// drain reads the request body; without it the server does not notice that
// the client gave up and r.Context() never ends.
func drain(t *testing.T, r *http.Request) {
	t.Helper()

	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		t.Errorf("reading the request body: %v", err)
	}
}

// assertNoLeak fails if the text contains the consumer unit from the query or the server URL.
func assertNoLeak(t *testing.T, text string, forbidden ...string) {
	t.Helper()

	for _, p := range append([]string{fakeConsumerUnit, "?", "Bearer", fakeSecret}, forbidden...) {
		if strings.Contains(text, p) {
			t.Errorf("the text leaks %q: %s", p, text)
		}
	}
}

// Nothing left the machine: a refused connection retries even on POST, and is not uncertain.
func TestSend_WhenConnectionRefused_RetriesEvenOnPOST(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	env := Environment{Name: "refused", APIURL: srv.URL, TokenURL: srv.URL + "/sso/oauth/token"}
	srv.Close() // the port stays closed

	tests := []struct {
		name string
		op   operation
	}{
		{"GET", readOp()},
		{"POST", writeOp(http.MethodPost)},
		{"PUT", writeOp(http.MethodPut)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hc, n := newCounter()
			c, reg := newClientWithEnv(t, env, fixedSource("fake-token-1"),
				WithAgent(fakeAgent, fakeProfile), WithRateLimits(RateLimits{}), WithHTTPClient(hc))

			_, _, err := c.send(context.Background(), tc.op, callConfig{})
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := n.n.Load(); got != 3 {
				t.Errorf("attempts = %d, want 3 (1 + 2 retries)", got)
			}
			var uncertain *UncertainResultError
			if errors.As(err, &uncertain) {
				t.Errorf("nothing was written, should not be uncertain: %v", err)
			}
			if len(reg.list()) != 2 {
				t.Errorf("waits = %v, want 2", reg.list())
			}
			assertNoLeak(t, err.Error(), srv.URL)
			var ue *url.Error
			if errors.As(err, &ue) {
				t.Errorf("the *url.Error (with the URL) should not escape: %v", ue)
			}
		})
	}
}

func TestSend_WhenCallerContextCanceled_ReturnsCtxError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		op            operation
		wantUncertain bool
	}{
		{"GET canceled midway returns the ctx error", readOp(), false},
		{"POST canceled after the body is written is uncertain", writeOp(http.MethodPost), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				drain(t, r)
				cancel()
				<-r.Context().Done()
			})
			c, _ := newTestClient(t, srv)

			_, _, err := c.send(ctx, tc.op, callConfig{})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
			var uncertain *UncertainResultError
			if got := errors.As(err, &uncertain); got != tc.wantUncertain {
				t.Errorf("uncertain = %v, want %v", got, tc.wantUncertain)
			}
			if got := srv.reqs.Load(); got != 1 {
				t.Errorf("attempts = %d, want 1", got)
			}
		})
	}
}

func TestSend_WhenContextAlreadyCanceled_DoesNotSend(t *testing.T) {
	t.Parallel()

	srv := newServer(t, sequence(t, stubResponse{status: 200, body: okBody}))
	c, _ := newTestClient(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := c.send(ctx, readOp(), callConfig{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if srv.reqs.Load() != 0 || srv.tokens.Load() != 0 {
		t.Errorf("nothing should be sent: reqs=%d tokens=%d", srv.reqs.Load(), srv.tokens.Load())
	}
}

func TestSend_WhenWaitInterruptedByCtx_ReturnsCtxError(t *testing.T) {
	t.Parallel()

	srv := newServer(t, sequence(t, stubResponse{status: 502}))
	c, _ := newTestClient(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	c.sleep = func(ctx context.Context, d time.Duration) error {
		cancel()
		return ctx.Err()
	}

	_, _, err := c.send(ctx, readOp(), callConfig{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got := srv.reqs.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
}

func TestSend_When403_AppliesRefreshRule(t *testing.T) {
	t.Parallel()

	authorizer := stubResponse{status: 403, header: gateway(nil), body: errorJSON("ERR_CREDENCIAL_INVALIDA")}
	backend := stubResponse{status: 403, body: errorJSON("ERR_CREDENCIAL_INVALIDA")}
	withoutCode := stubResponse{status: 403, header: gateway(nil), body: `{"message":"Forbidden"}`}
	ok := stubResponse{status: 200, body: okBody}

	tests := []struct {
		name       string
		op         operation
		opts       []Option
		responses  []stubResponse
		first      bool // makes a prior call that the token sees accepted
		wantTokens int32
		wantReqs   int32 // API requests in the tested call (and in the first)
		wantErr    bool
		wantIs     error    // expected sentinel in the error
		wantAuth   []string // Authorization seen on each API request
	}{
		{
			name:       "already accepted token: invalidates, fetches another and retries",
			op:         readOp(),
			responses:  []stubResponse{ok, authorizer, ok},
			first:      true,
			wantTokens: 2,
			wantReqs:   3,
			wantAuth:   []string{"Bearer fake-token-1", "Bearer fake-token-1", "Bearer fake-token-2"},
		},
		{
			name:       "POST also refreshes and retries, because the gateway refused before the backend",
			op:         writeOp(http.MethodPost),
			responses:  []stubResponse{ok, authorizer, ok},
			first:      true,
			wantTokens: 2,
			wantReqs:   3,
		},
		{
			name:       "never-accepted token: does not retry",
			op:         readOp(),
			responses:  []stubResponse{authorizer, ok},
			wantTokens: 1,
			wantReqs:   1,
			wantErr:    true,
			wantIs:     ErrInvalidCredential,
		},
		{
			name:       "refreshes only once, even if the fresh token is also refused",
			op:         readOp(),
			responses:  []stubResponse{ok, authorizer, authorizer, ok},
			first:      true,
			wantTokens: 2,
			wantReqs:   3,
			wantErr:    true,
			wantIs:     ErrInvalidCredential,
		},
		{
			name:       "403 from the backend with the same code does not retry",
			op:         readOp(),
			responses:  []stubResponse{ok, backend, ok},
			first:      true,
			wantTokens: 1,
			wantReqs:   2,
			wantErr:    true,
			wantIs:     ErrInvalidCredential,
		},
		{
			name:       "403 from the authorizer without ERR code does not retry",
			op:         readOp(),
			responses:  []stubResponse{ok, withoutCode, ok},
			first:      true,
			wantTokens: 1,
			wantReqs:   2,
			wantErr:    true,
		},
		{
			name:       "WithTokenSource does not refresh",
			op:         readOp(),
			opts:       []Option{fixedSource("fake-token-1")},
			responses:  []stubResponse{ok, authorizer, ok},
			first:      true,
			wantTokens: 0,
			wantReqs:   2,
			wantErr:    true,
			wantIs:     ErrInvalidCredential,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var mu sync.Mutex
			var auths []string
			seq := sequence(t, tc.responses...)
			srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				auths = append(auths, r.Header.Get("Authorization"))
				mu.Unlock()
				seq(w, r)
			})
			var c *Client
			if len(tc.opts) > 0 {
				c, _ = newClientWithEnv(t, srv.env(), append([]Option{WithAgent(fakeAgent, fakeProfile), WithRateLimits(RateLimits{})}, tc.opts...)...)
			} else {
				c, _ = newTestClient(t, srv)
			}

			if tc.first {
				if _, _, err := c.send(context.Background(), readOp(), callConfig{}); err != nil {
					t.Fatalf("first call: %v", err)
				}
			}
			_, _, err := c.send(context.Background(), tc.op, callConfig{})

			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("errors.Is(%v) = false: %v", tc.wantIs, err)
			}
			if got := srv.tokens.Load(); got != tc.wantTokens {
				t.Errorf("tokens fetched = %d, want %d", got, tc.wantTokens)
			}
			if got := srv.reqs.Load(); got != tc.wantReqs {
				t.Errorf("requests = %d, want %d", got, tc.wantReqs)
			}
			if tc.wantAuth != nil {
				mu.Lock()
				defer mu.Unlock()
				if strings.Join(auths, "|") != strings.Join(tc.wantAuth, "|") {
					t.Errorf("Authorization = %v, want %v", auths, tc.wantAuth)
				}
			}
		})
	}
}

func TestSend_WhenCalled_SetsHeadersAndBody(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var seenHeaders []http.Header
	var bodies []string
	var queries []string
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		readBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading the body: %v", err)
		}
		mu.Lock()
		seenHeaders = append(seenHeaders, r.Header.Clone())
		bodies = append(bodies, string(readBody))
		queries = append(queries, r.URL.RawQuery)
		mu.Unlock()
		writeBody(t, w, okBody)
	})
	c, _ := newTestClient(t, srv, WithUserAgent("fake-app/2.0"))

	if _, _, err := c.send(context.Background(), readOp(), callConfig{}); err != nil {
		t.Fatalf("GET: %v", err)
	}
	if _, _, err := c.send(context.Background(), writeOp(http.MethodPost), newCallConfig(WithAgent(900011, 900012))); err != nil {
		t.Fatalf("POST: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seenHeaders) != 2 {
		t.Fatalf("requests = %d", len(seenHeaders))
	}
	get, post := seenHeaders[0], seenHeaders[1]

	if got := get.Get("Authorization"); got != "Bearer fake-token-1" {
		t.Errorf("Authorization = %q", got)
	}
	if get.Get("codigoAgente") != "900001" || get.Get("codigoPerfil") != "900002" {
		t.Errorf("client agent/profile = %q/%q", get.Get("codigoAgente"), get.Get("codigoPerfil"))
	}
	if post.Get("codigoAgente") != "900011" || post.Get("codigoPerfil") != "900012" {
		t.Errorf("call agent/profile = %q/%q", post.Get("codigoAgente"), post.Get("codigoPerfil"))
	}
	if get.Get("Accept") != "application/json" {
		t.Errorf("Accept = %q", get.Get("Accept"))
	}
	ua := get.Get("User-Agent")
	if !strings.HasPrefix(ua, "abertura-mercado-energia/") || !strings.Contains(ua, "(+https://github.com/MarcoBuarque/abertura-mercado-energia)") || !strings.HasSuffix(ua, " fake-app/2.0") {
		t.Errorf("User-Agent = %q", ua)
	}
	if post.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type do POST = %q", post.Get("Content-Type"))
	}
	if bodies[1] != `{"codigoUnidadeConsumidora":"`+fakeConsumerUnit+`"}` {
		t.Errorf("body do POST = %q", bodies[1])
	}
	if get.Get("Content-Type") != "" || bodies[0] != "" {
		t.Errorf("GET without a body should have no Content-Type or body")
	}
	if queries[0] != "codigoUnidadeConsumidora="+fakeConsumerUnit {
		t.Errorf("query = %q", queries[0])
	}
	for i, h := range seenHeaders {
		for k := range h {
			if strings.EqualFold(k, "Idempotency-Key") {
				t.Errorf("request %d sent %s", i, k)
			}
		}
	}
}

func TestSend_WhenCallTimeout_TakesPriority(t *testing.T) {
	t.Parallel()

	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		drain(t, r)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	c, _ := newTestClient(t, srv, WithTimeout(time.Hour), WithRetries(0))

	start := time.Now()
	_, _, err := c.send(context.Background(), readOp(), newCallConfig(WithTimeout(30*time.Millisecond)))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want DeadlineExceeded", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Error("the call timeout was not used")
	}
}

func TestSend_WhenTokenFailure_DoesNotRetry(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/sso/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		writeBody(t, w, `{"error":"invalid_client","error_description":"`+fakeSecret+`"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, _ := newClientWithEnv(t, Environment{APIURL: srv.URL, TokenURL: srv.URL + "/sso/oauth/token"},
		WithCredentials(fakeClientID, fakeSecret), WithAgent(fakeAgent, fakeProfile), WithRateLimits(RateLimits{}))

	_, _, err := c.send(context.Background(), readOp(), callConfig{})
	var api *APIError
	if !errors.As(err, &api) || api.Operation != "getToken" {
		t.Fatalf("error = %v, want APIError from getToken", err)
	}
	if calls.Load() != 1 {
		t.Errorf("token fetches = %d, want 1", calls.Load())
	}
	assertNoLeak(t, err.Error())
}

func TestSend_WhenBadRequestBuild_ReturnsInvalidArgument(t *testing.T) {
	t.Parallel()

	srv := newServer(t, sequence(t, stubResponse{status: 200, body: okBody}))
	c, _ := newTestClient(t, srv)

	noMethod := readOp()
	noMethod.httpMethod = "INVALID METHOD"
	invalidBody := writeOp(http.MethodPost)
	invalidBody.body = make(chan int)
	invalidURL := readOp()
	invalidURL.path = "/%zz" + fakeConsumerUnit

	tests := []struct {
		name string
		op   operation
	}{
		{"invalid method", noMethod},
		{"body that is not JSON", invalidBody},
		{"path that is not a URL does not leak the path", invalidURL},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := c.send(context.Background(), tc.op, callConfig{})
			if err == nil {
				t.Fatal("expected an error")
			}
			assertNoLeak(t, err.Error())
			if !errors.Is(err, ErrInvalidArgument) {
				t.Errorf("an invalid build should match ErrInvalidArgument: %v", err)
			}
		})
	}
	if srv.reqs.Load() != 0 {
		t.Errorf("nothing should be sent: %d", srv.reqs.Load())
	}
}

// Ensures that two clients use independent token sources.
func TestSend_WhenTwoClients_UseIndependentTokenSources(t *testing.T) {
	t.Parallel()

	srv := newServer(t, sequence(t, stubResponse{status: 200, body: okBody}))
	a, _ := newTestClient(t, srv)
	b, _ := newTestClient(t, srv)
	for _, c := range []*Client{a, b} {
		if _, _, err := c.send(context.Background(), readOp(), callConfig{}); err != nil {
			t.Fatal(err)
		}
	}
	if srv.tokens.Load() != 2 {
		t.Errorf("tokens = %d, want 2", srv.tokens.Load())
	}
}
