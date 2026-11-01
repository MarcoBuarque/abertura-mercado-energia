package abertura

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

const (
	fakeClientID   = "fake-id"
	fakeSecret     = "fake-secret"
	fakeUA         = "fake-ua/1.0"
	unreachableURL = "http://example.invalid/sso/oauth/token"
)

// writeBody writes the response body and fails the test if the write fails.
func writeBody(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()

	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("writing the response: %v", err)
	}
}

// newOfflineCache creates a cache that should never reach the network.
func newOfflineCache() *tokenCache {
	cfg := configToken{tokenURL: unreachableURL, clientID: fakeClientID}
	cfg.clientSecret = fakeSecret
	return newTokenCache(cfg)
}

// fakeClock is a manual clock, safe for concurrent use.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
}

func (r *fakeClock) now() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.t
}

func (r *fakeClock) advance(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.t = r.t.Add(d)
}

// tokenServer simulates the token endpoint and records what it received.
type tokenServer struct {
	*httptest.Server
	fetches atomic.Int64

	mu               sync.Mutex
	form             map[string][]string
	basic            bool
	hasAuthorization bool
	userAgent        string
	rawQuery         string
	size             int64
	transferEncoding []string
	httpMethod       string
	mediaType        string
}

// newTokenServer returns numbered tokens (fake-token-1, 2, ...) with
// expires_in 300. delay simulates latency; failure, if not nil, answers with
// an error.
func newTokenServer(t *testing.T, delay time.Duration, failure func(w http.ResponseWriter)) *tokenServer {
	t.Helper()

	s := &tokenServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "unreadable form", http.StatusBadRequest)
			return
		}
		_, _, hasBasic := r.BasicAuth()
		s.mu.Lock()
		s.form = r.PostForm
		s.basic = hasBasic
		s.rawQuery, s.size, s.transferEncoding = r.URL.RawQuery, r.ContentLength, r.TransferEncoding
		s.userAgent = r.Header.Get("User-Agent")
		s.hasAuthorization = r.Header.Get("Authorization") != ""
		s.httpMethod = r.Method
		s.mediaType = r.Header.Get("Content-Type")
		s.mu.Unlock()

		n := s.fetches.Add(1)
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		if failure != nil {
			failure(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeBody(t, w, fmt.Sprintf(`{"access_token":"fake-token-%d","token_type":"Bearer","expires_in":300,"refresh_expires_in":0}`, n))
	}))
	t.Cleanup(s.Close)
	return s
}

// newTestCache creates a tokenCache that talks to the test token server.
func newTestCache(s *tokenServer, rel *fakeClock) *tokenCache {
	c := newTokenCache(configToken{
		tokenURL:     s.URL,
		clientID:     fakeClientID,
		clientSecret: fakeSecret,
		httpClient:   s.Client(),
		userAgent:    fakeUA,
	})
	if rel != nil {
		c.now = rel.now
	}
	return c
}

func TestTokenCache_WhenRequested_UsesFormWithoutBasic(t *testing.T) {
	t.Parallel()

	s := newTokenServer(t, 0, nil)
	c := newTestCache(s, nil)

	tok, err := c.token(context.Background())
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if tok.AccessToken != "fake-token-1" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.httpMethod != http.MethodPost {
		t.Errorf("method = %s", s.httpMethod)
	}
	if s.mediaType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", s.mediaType)
	}
	if s.basic {
		t.Error("the request used a Basic header; it should use the form")
	}
	if s.rawQuery != "" || s.size <= 0 || len(s.transferEncoding) != 0 {
		t.Errorf("query=%q ContentLength=%d TransferEncoding=%v", s.rawQuery, s.size, s.transferEncoding)
	}
	if s.userAgent != fakeUA {
		t.Errorf("User-Agent = %q, want %q", s.userAgent, fakeUA)
	}
	if s.hasAuthorization {
		t.Error("the request carried an Authorization header")
	}
	if len(s.form) != 2 {
		t.Errorf("form has %d fields (%v), want only client_id and client_secret", len(s.form), s.form)
	}
	if _, ok := s.form["grant_type"]; ok {
		t.Error("the form carries grant_type; the endpoint rejects it")
	}
	want := map[string]string{
		"client_id":     fakeClientID,
		"client_secret": fakeSecret,
	}
	for k, v := range want {
		if got := s.form[k]; len(got) != 1 || got[0] != v {
			t.Errorf("field %s = %v, want %q", k, got, v)
		}
	}
}

func TestTokenCache_WhenResponseOK_ParsesBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantToken    string
		wantType     string
		wantErr      bool
		wantValidity time.Duration // expected validity, when there is no error
	}{
		{name: "when expires_in is present it is honored", body: `{"access_token":"t1","token_type":"Bearer","expires_in":120}`, wantToken: "t1", wantType: "Bearer", wantValidity: 120 * time.Second},
		{name: "when expires_in is missing it assumes 300 s", body: `{"access_token":"t2","token_type":"Bearer"}`, wantToken: "t2", wantType: "Bearer", wantValidity: 300 * time.Second},
		{name: "when token_type is missing it assumes Bearer", body: `{"access_token":"t3"}`, wantToken: "t3", wantType: "Bearer", wantValidity: 300 * time.Second},
		{name: "when expires_in is a numeric string", body: `{"access_token":"t4","expires_in":"300"}`, wantToken: "t4", wantType: "Bearer", wantValidity: 300 * time.Second},
		{name: "when expires_in is a string of 120", body: `{"access_token":"t4","expires_in":"120"}`, wantToken: "t4", wantType: "Bearer", wantValidity: 120 * time.Second},
		{name: "when expires_in is a decimal", body: `{"access_token":"t5","expires_in":300.0}`, wantToken: "t5", wantType: "Bearer", wantValidity: 300 * time.Second},
		{name: "when expires_in is negative it falls back to the default", body: `{"access_token":"t6","expires_in":-5}`, wantToken: "t6", wantType: "Bearer", wantValidity: 300 * time.Second},
		{name: "when expires_in is absurd it falls back to the default", body: `{"access_token":"t7","expires_in":1e15}`, wantToken: "t7", wantType: "Bearer", wantValidity: 300 * time.Second},
		{name: "when expires_in is above 24 h it falls back to the default", body: `{"access_token":"t7","expires_in":86401}`, wantToken: "t7", wantType: "Bearer", wantValidity: 300 * time.Second},
		{name: "when expires_in is not numeric it falls back to the default", body: `{"access_token":"t8","expires_in":"abc"}`, wantToken: "t8", wantType: "Bearer", wantValidity: 300 * time.Second},
		{name: "when access_token is missing it fails", body: `{"token_type":"Bearer","expires_in":300}`, wantErr: true},
		{name: "when access_token is empty it fails", body: `{"access_token":"","expires_in":300}`, wantErr: true},
		{name: "when the JSON is invalid it fails", body: `not json`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				writeBody(t, w, tc.body)
			}))
			t.Cleanup(srv.Close)
			rel := newFakeClock()
			c := newTokenCache(configToken{tokenURL: srv.URL, clientID: fakeClientID, clientSecret: fakeSecret, httpClient: srv.Client()})
			c.now = rel.now

			tok, err := c.token(context.Background())
			if tc.wantErr {
				var ft *tokenFailureError
				if !errors.As(err, &ft) {
					t.Fatalf("err = %v, want *tokenFailureError", err)
				}
				if strings.Contains(err.Error(), tc.body) || strings.Contains(err.Error(), fakeSecret) {
					t.Errorf("error leaks body or secret: %q", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("token: %v", err)
			}
			if tok.AccessToken != tc.wantToken || tok.TokenType != tc.wantType {
				t.Errorf("token = %q/%q, want %q/%q", tok.AccessToken, tok.TokenType, tc.wantToken, tc.wantType)
			}
			if want := rel.now().Add(tc.wantValidity); !tok.Expiry.Equal(want) {
				t.Errorf("Expiry = %v, want %v", tok.Expiry, want)
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if want := rel.now().Add(tc.wantValidity - tokenSlack); !c.expiresAt.Equal(want) {
				t.Errorf("expiresAt = %v, want %v", c.expiresAt, want)
			}
		})
	}
}

func TestTokenCache_WhenConcurrent_FetchesOnce(t *testing.T) {
	t.Parallel()

	s := newTokenServer(t, 100*time.Millisecond, nil)
	c := newTestCache(s, nil)

	const n = 50
	var wg sync.WaitGroup
	tokens := make([]*oauth2.Token, n)
	errs := make([]error, n)
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tokens[i], errs[i] = c.token(context.Background())
		}()
	}
	close(start)
	wg.Wait()

	for i := range n {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v", i, errs[i])
		}
		if tokens[i] != tokens[0] {
			t.Fatalf("goroutine %d received a different token", i)
		}
	}
	if got := s.fetches.Load(); got != 1 {
		t.Errorf("fetches = %d, want 1", got)
	}
}

func TestTokenCache_WhenTimePasses_RefreshesThirtySecondsEarly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		elapsed     time.Duration
		wantFetches int64
		wantToken   string
	}{
		{"right after receipt it reuses", 0, 1, "fake-token-1"},
		{"at 269 s it reuses", 269 * time.Second, 1, "fake-token-1"},
		{"at 270 s it is still valid", 270 * time.Second, 1, "fake-token-1"},
		{"at 271 s it refreshes", 271 * time.Second, 2, "fake-token-2"},
		{"after expiry it refreshes", 400 * time.Second, 2, "fake-token-2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newTokenServer(t, 0, nil)
			rel := newFakeClock()
			c := newTestCache(s, rel)

			if _, err := c.token(context.Background()); err != nil {
				t.Fatalf("first lookup: %v", err)
			}
			rel.advance(tc.elapsed)
			tok, err := c.token(context.Background())
			if err != nil {
				t.Fatalf("second call: %v", err)
			}
			if tok.AccessToken != tc.wantToken {
				t.Errorf("AccessToken = %q, want %q", tok.AccessToken, tc.wantToken)
			}
			if got := s.fetches.Load(); got != tc.wantFetches {
				t.Errorf("fetches = %d, want %d", got, tc.wantFetches)
			}
		})
	}
}

func TestTokenCache_WhenExpiresInMissing_AssumesThreeHundredSeconds(t *testing.T) {
	t.Parallel()

	var fetches atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := fetches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		writeBody(t, w, fmt.Sprintf(`{"access_token":"fake-token-%d","token_type":"Bearer"}`, n))
	}))
	t.Cleanup(srv.Close)

	rel := newFakeClock()
	c := newTokenCache(configToken{tokenURL: srv.URL, clientID: fakeClientID, clientSecret: fakeSecret, httpClient: srv.Client()})
	c.now = rel.now

	if _, err := c.token(context.Background()); err != nil {
		t.Fatal(err)
	}
	rel.advance(270 * time.Second)
	if _, err := c.token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fetches.Load() != 1 {
		t.Fatalf("at 270 s fetches = %d, want 1", fetches.Load())
	}
	rel.advance(time.Second)
	if _, err := c.token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fetches.Load() != 2 {
		t.Errorf("at 271 s fetches = %d, want 2", fetches.Load())
	}
}

func TestTokenCache_Invalidate_WhenCurrentToken_ForcesNewFetch(t *testing.T) {
	t.Parallel()

	s := newTokenServer(t, 0, nil)
	c := newTestCache(s, nil)

	t1, err := c.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c.invalidate(t1)
	t2, err := c.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if t2.AccessToken == t1.AccessToken {
		t.Errorf("token did not change: %q", t2.AccessToken)
	}
	if got := s.fetches.Load(); got != 2 {
		t.Errorf("fetches = %d, want 2", got)
	}
}

func TestTokenCache_Invalidate_WhenOldToken_KeepsCurrent(t *testing.T) {
	t.Parallel()

	s := newTokenServer(t, 0, nil)
	c := newTestCache(s, nil)

	t1, err := c.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c.invalidate(t1)
	t2, err := c.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Another goroutine still held t1 and invalidates too late.
	c.invalidate(t1)
	c.invalidate(nil)
	t3, err := c.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if t3 != t2 {
		t.Errorf("invalidating an old token discarded the current one: %q -> %q", t2.AccessToken, t3.AccessToken)
	}
	if got := s.fetches.Load(); got != 2 {
		t.Errorf("fetches = %d, want 2", got)
	}
}

func TestTokenCache_Accepted_WhenMarked_AppliesPerToken(t *testing.T) {
	t.Parallel()

	s := newTokenServer(t, 0, nil)
	c := newTestCache(s, nil)

	t1, err := c.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if c.wasAccepted(t1) {
		t.Error("a fresh token should not be listed as accepted")
	}
	c.markAccepted(t1)
	if !c.wasAccepted(t1) {
		t.Error("a marked token should be listed as accepted")
	}
	if c.wasAccepted(nil) {
		t.Error("nil is never accepted")
	}
	c.markAccepted(nil) // must not panic

	// A token foreign to the cache does not become accepted.
	foreign := &oauth2.Token{AccessToken: "fake-token-9"}
	c.markAccepted(foreign)
	if c.wasAccepted(foreign) {
		t.Error("a token the cache did not issue should not be marked")
	}

	// The refreshed token starts without the mark; the old one keeps its own,
	// so a late response is still evaluated with what was known.
	c.invalidate(t1)
	t2, err := c.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if c.wasAccepted(t2) {
		t.Error("the refreshed token does not inherit the previous mark")
	}
	if !c.wasAccepted(t1) {
		t.Error("the previous token should keep its mark")
	}
}

func TestTokenCache_Renewable_WhenCredentials_IsTrue(t *testing.T) {
	t.Parallel()

	c := newOfflineCache()
	if !c.renewable() {
		t.Error("tokenCache should be renewable")
	}
	if c.fetchTimeout != 10*time.Second {
		t.Errorf("fetchTimeout = %v, want 10s", c.fetchTimeout)
	}
}

func TestExternalSource_WhenTokenSource_DoesNotRenew(t *testing.T) {
	t.Parallel()

	external := &oauth2.Token{AccessToken: "fake-token-external", TokenType: "Bearer"}
	var source tokenProvider = newExternalSource(oauth2.StaticTokenSource(external))

	if source.renewable() {
		t.Error("an external source is not renewable")
	}
	tok, err := source.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "fake-token-external" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}

	// No cache and no 403 rule: none of this has any effect.
	source.markAccepted(tok)
	if source.wasAccepted(tok) {
		t.Error("an external source never marks a token as accepted")
	}
	source.invalidate(tok)
	if _, err := source.token(context.Background()); err != nil {
		t.Errorf("token after invalidate: %v", err)
	}
}

type failingSource struct{ err error }

func (f failingSource) Token() (*oauth2.Token, error) { return nil, f.err }

func TestExternalSource_WhenSourceFailsOrCtxCanceled_ReturnsError(t *testing.T) {
	t.Parallel()

	cause := errors.New("fake source unavailable")
	source := newExternalSource(failingSource{cause})
	if _, err := source.token(context.Background()); !errors.Is(err, cause) {
		t.Errorf("err = %v, want the cause", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	good := newExternalSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fake-token-external"}))
	if _, err := good.token(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestTokenCache_WhenEndpointFails_BecomesAPIErrorWithoutSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		body       string
		wantCode   string
		wantStatus int
	}{
		{"400 with OAuth2 error", 400, `{"error":"invalid_request","error_description":"fake request"}`, "invalid_request", 400},
		{"401 with OAuth2 error", 401, `{"error":"invalid_client","error_description":"fake client"}`, "invalid_client", 401},
		{"401 with plain text", 401, "unauthorized, fake", "", 401},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newTokenServer(t, 0, func(w http.ResponseWriter) {
				w.Header().Set("x-amzn-RequestId", "fake-req-3")
				w.WriteHeader(tc.status)
				writeBody(t, w, tc.body)
			})
			c := newTestCache(s, nil)

			_, err := c.token(context.Background())
			var api *APIError
			if !errors.As(err, &api) {
				t.Fatalf("err = %T %v, want *APIError", err, err)
			}
			if api.Operation != "getToken" || api.StatusCode != tc.wantStatus || api.Code != tc.wantCode {
				t.Errorf("api = %+v", api)
			}
			if api.RequestID != "fake-req-3" {
				t.Errorf("RequestID = %q", api.RequestID)
			}
			requireNoSecret(t, err, c)
		})
	}
}

func TestTokenCache_WhenFailure_DoesNotCacheAndFetchesAgain(t *testing.T) {
	t.Parallel()

	var enabled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !enabled.Load() {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeBody(t, w, `{"access_token":"fake-token-ok","token_type":"Bearer","expires_in":300}`)
	}))
	t.Cleanup(srv.Close)
	c := newTokenCache(configToken{tokenURL: srv.URL, clientID: fakeClientID, clientSecret: fakeSecret, httpClient: srv.Client()})

	if _, err := c.token(context.Background()); err == nil {
		t.Fatal("expected an error on the first call")
	}
	enabled.Store(true)
	tok, err := c.token(context.Background())
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if tok.AccessToken != "fake-token-ok" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}
}

func TestTokenCache_WhenTransportFails_ErrorWithoutSecretOrURL(t *testing.T) {
	t.Parallel()

	// Reserve a port and close it: the connection will be refused.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	c := newTokenCache(configToken{tokenURL: "http://" + address + "/sso/oauth/token?consumerUnit=000123", clientID: fakeClientID, clientSecret: fakeSecret})
	_, err = c.token(context.Background())
	if err == nil {
		t.Fatal("expected a transport error")
	}
	requireNoSecret(t, err, c)
	if strings.Contains(err.Error(), "consumerUnit=000123") {
		t.Errorf("the URL leaked into the error: %v", err)
	}
	if !strings.Contains(err.Error(), "getToken") {
		t.Errorf("error without the operation: %v", err)
	}
}

func TestTokenCache_WhenServerSlow_HonorsFetchTimeout(t *testing.T) {
	t.Parallel()

	s := newTokenServer(t, 5*time.Second, nil)
	c := newTestCache(s, nil)
	c.fetchTimeout = 50 * time.Millisecond

	start := time.Now()
	_, err := c.token(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("took %v; the timeout did not apply", d)
	}
	requireNoSecret(t, err, c)
}

func TestTokenCache_WhenCtxCanceled_InterruptsFetch(t *testing.T) {
	t.Parallel()

	s := newTokenServer(t, 5*time.Second, nil)
	c := newTestCache(s, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := c.token(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want Canceled", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("took %v; the ctx did not apply", d)
	}
}

func TestTokenCache_WhenAnotherFetchInProgress_WaitsHonoringCtx(t *testing.T) {
	t.Parallel()

	s := newTokenServer(t, 300*time.Millisecond, nil)
	c := newTestCache(s, nil)

	first := make(chan error, 1)
	go func() {
		_, err := c.token(context.Background())
		first <- err
	}()
	// Give the first call time to take the lookup.
	for s.fetches.Load() == 0 {
		time.Sleep(time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := c.token(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("the wait ignored the ctx: %v", err)
	}
	if err := <-first; err != nil {
		t.Errorf("the first lookup should complete: %v", err)
	}
}

func TestTokenCache_Formatting_DoesNotLeakSecret(t *testing.T) {
	t.Parallel()

	c := newOfflineCache()
	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		if out := fmt.Sprintf(verb, c); strings.Contains(out, fakeSecret) {
			t.Errorf("%s leaked the secret: %q", verb, out)
		}
	}
}

// requireNoSecret fails if the error, or its formatting, carries the secret.
func requireNoSecret(t *testing.T, err error, extras ...any) {
	t.Helper()

	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		if out := fmt.Sprintf(verb, err); strings.Contains(out, fakeSecret) {
			t.Errorf("%s of the error leaked the secret: %q", verb, out)
		}
	}
	if strings.Contains(err.Error(), fakeSecret) {
		t.Errorf("Error() leaked the secret: %q", err.Error())
	}
	for _, e := range extras {
		if out := fmt.Sprintf("%+v", e); strings.Contains(out, fakeSecret) {
			t.Errorf("%%+v of %T leaked the secret: %q", e, out)
		}
	}
}

// badCloseBody reads normally, but fails on close.
type badCloseBody struct{ io.Reader }

func (badCloseBody) Close() error { return errors.New("failure closing the body") }

// fakeTransport returns a canned response, without network.
type fakeTransport func(*http.Request) (*http.Response, error)

func (f fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTokenCache_WhenBodyCloseFails_BecomesTokenFailureErrorWithoutSecret(t *testing.T) {
	t.Parallel()

	hc := &http.Client{Transport: fakeTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       badCloseBody{strings.NewReader(`{"access_token":"t","expires_in":300}`)},
		}, nil
	})}
	c := newTokenCache(configToken{tokenURL: unreachableURL, clientID: fakeClientID, clientSecret: fakeSecret, httpClient: hc})

	_, err := c.token(context.Background())
	var ft *tokenFailureError
	if !errors.As(err, &ft) {
		t.Fatalf("err = %v, want *tokenFailureError", err)
	}
	if strings.Contains(err.Error(), fakeSecret) {
		t.Errorf("error leaks the secret: %q", err)
	}
}

func TestTokenCache_WhenTokenRedirects_DoesNotFollowOrLeakSecret(t *testing.T) {
	t.Parallel()

	var fetches atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		http.Redirect(w, r, "/outro", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	// A nil httpClient uses the SDK's private client, which does not follow redirects.
	c := newTokenCache(configToken{tokenURL: srv.URL, clientID: fakeClientID, clientSecret: fakeSecret})

	_, err := c.token(context.Background())
	var api *APIError
	if !errors.As(err, &api) || api.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("err = %v, want *APIError with status 307", err)
	}
	if fetches.Load() != 1 {
		t.Errorf("fetches = %d, want 1 (the 307 must not be followed)", fetches.Load())
	}
	if strings.Contains(err.Error(), fakeSecret) {
		t.Errorf("error leaks the secret: %q", err)
	}
}

func TestTokenCache_WhenOAuth2Error_CodeYesFreeTextNo(t *testing.T) {
	t.Parallel()

	s := newTokenServer(t, 0, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		writeBody(t, w, `{"error":"invalid_request","error_description":"text-that-must-not-leak"}`)
	})
	c := newTestCache(s, nil)

	_, err := c.token(context.Background())
	var api *APIError
	if !errors.As(err, &api) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if api.Code != "invalid_request" || api.StatusCode != http.StatusBadRequest {
		t.Errorf("Code=%q status=%d", api.Code, api.StatusCode)
	}
	if strings.Contains(err.Error(), "text-that-must-not-leak") || strings.Contains(err.Error(), fakeSecret) {
		t.Errorf("error leaks free text or secret: %q", err)
	}
}
