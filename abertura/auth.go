package abertura

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

const (
	// tokenSlack is how long before expiry the token is refreshed.
	tokenSlack = 30 * time.Second
	// defaultTokenValidity applies when the response carries no expires_in; it
	// is the value observed in the sandbox.
	defaultTokenValidity = 300 * time.Second
	// tokenFetchTimeout bounds each token request.
	tokenFetchTimeout = 10 * time.Second
	// tokenOperation is the operationId of the token request in the spec.
	tokenOperation = "getToken"
)

// tokenProvider is the source of the access token that the send loop consumes.
// The two implementations are tokenCache (client credentials) and
// externalSource (the caller's oauth2.TokenSource).
//
// The send loop uses markAccepted and wasAccepted for the authorizer's 403
// rule: it retries with a fresh token only if the rejected one had already
// been accepted.
type tokenProvider interface {
	// token returns a valid token, fetching a fresh one if needed. Returned
	// values are compared by identity in invalidate, markAccepted and
	// wasAccepted.
	token(ctx context.Context) (*oauth2.Token, error)
	// invalidate discards the token if it is still the current one; an old
	// token does not discard the one that already replaced it.
	invalidate(t *oauth2.Token)
	// markAccepted records that the token was accepted by a response without
	// the authorizer mark (x-amzn-ErrorType).
	markAccepted(t *oauth2.Token)
	// wasAccepted reports whether the token has been marked as accepted.
	wasAccepted(t *oauth2.Token) bool
	// renewable reports whether the SDK can obtain another token; it is false
	// with WithTokenSource.
	renewable() bool
}

// configToken holds what the tokenCache needs to request tokens.
type configToken struct {
	tokenURL     string
	clientID     string
	clientSecret string
	httpClient   *http.Client // nil uses the default client
	userAgent    string       // SDK User-Agent; empty omits the header
}

// tokenCache keeps the client-credentials token, refreshes it before expiry
// (by the local clock) and lets callers discard it by token. Only one lookup
// happens at a time; concurrent calls wait for it.
type tokenCache struct {
	cfg        configToken
	httpClient *http.Client

	// now and fetchTimeout are fields so tests can inject the clock and a
	// short deadline.
	now          func() time.Time
	fetchTimeout time.Duration

	lookup chan struct{} // semaphore of 1: whoever holds it is fetching

	mu        sync.Mutex // guards the fields below
	current   *oauth2.Token
	expiresAt time.Time // until when current is valid, slack already subtracted
	accepted  map[*oauth2.Token]bool
}

// newTokenCache creates the cache. The credentials live only here, in
// unexported fields, and String and GoString hide them. The token request
// form carries only client_id and client_secret. Fact: the request with
// grant_type was refused with 400 on 2026-11-01. Hypothesis: the public
// collection sends only those two fields, and the endpoint does not accept
// the third.
func newTokenCache(cfg configToken) *tokenCache {
	return &tokenCache{
		cfg:          cfg,
		httpClient:   cfg.httpClient,
		now:          time.Now,
		fetchTimeout: tokenFetchTimeout,
		lookup:       make(chan struct{}, 1),
		accepted:     make(map[*oauth2.Token]bool),
	}
}

// String keeps %v and %+v from printing the secret stored in the fields.
func (c *tokenCache) String() string { return "abertura.tokenCache{credentials omitted}" }

// GoString keeps %#v from printing the secret.
func (c *tokenCache) GoString() string { return c.String() }

func (c *tokenCache) renewable() bool { return true }

// valid returns the current token if it is still valid. It requires c.mu.
func (c *tokenCache) valid() *oauth2.Token {
	if c.current != nil && !c.now().After(c.expiresAt) {
		return c.current
	}
	return nil
}

// token returns a valid cached token, fetching a fresh one when needed.
func (c *tokenCache) token(ctx context.Context) (*oauth2.Token, error) {
	c.mu.Lock()
	t := c.valid()
	c.mu.Unlock()
	if t != nil {
		return t, nil
	}

	// Only one lookup at a time; whoever waits honors its own ctx.
	select {
	case c.lookup <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-c.lookup }()

	// Whoever was fetching may have finished while we waited.
	c.mu.Lock()
	t = c.valid()
	c.mu.Unlock()
	if t != nil {
		return t, nil
	}

	receivedAt := c.now()
	fresh, err := c.fetch(ctx)
	if err != nil {
		return nil, err
	}

	validity := time.Duration(fresh.ExpiresIn) * time.Second
	if validity <= 0 {
		validity = defaultTokenValidity
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// Keep the mark only for the token being replaced; the others stop
	// circulating soon and the map does not grow.
	accepted := make(map[*oauth2.Token]bool, 1)
	if c.current != nil && c.accepted[c.current] {
		accepted[c.current] = true
	}
	c.accepted = accepted
	c.current = fresh
	c.expiresAt = receivedAt.Add(validity - tokenSlack)
	return fresh, nil
}

// tokenResponse is the success body of the token endpoint.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	// ExpiresIn accepts a number (300, 300.0) or numeric text ("300"); the
	// validity comes from validityOf, which never fails.
	ExpiresIn json.RawMessage `json:"expires_in"`
}

// maxTokenValidity is the cap accepted for expires_in; above it the response
// is treated as carrying no validity.
const maxTokenValidity = 24 * time.Hour

// validityOf interprets expires_in. Missing, unreadable, non-positive or above
// maxTokenValidity falls back to defaultTokenValidity, so the lookup does not
// fail on an auxiliary field and the time arithmetic does not overflow
// time.Duration.
func validityOf(raw json.RawMessage) time.Duration {
	text := strings.TrimSpace(string(raw))
	if strings.HasPrefix(text, `"`) {
		if err := json.Unmarshal(raw, &text); err != nil {
			return defaultTokenValidity
		}
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil || math.IsNaN(seconds) || seconds < 1 || seconds > maxTokenValidity.Seconds() {
		return defaultTokenValidity
	}
	return time.Duration(seconds) * time.Second
}

// fetch requests a fresh token from the endpoint, with the lookup timeout. The
// form carries only client_id and client_secret (no grant_type; see
// newTokenCache). Failures become *APIError (error response from the
// endpoint) or a transport error without URL. No error mentions the body, the
// form or the secret.
func (c *tokenCache) fetch(ctx context.Context) (*oauth2.Token, error) {
	ctx, cancel := context.WithTimeout(ctx, c.fetchTimeout)
	defer cancel()

	form := url.Values{"client_id": {c.cfg.clientID}, "client_secret": {c.cfg.clientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", tokenOperation, &tokenFailureError{err: stripURL(err)})
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if c.cfg.userAgent != "" {
		req.Header["User-Agent"] = []string{c.cfg.userAgent}
	}

	hc := c.httpClient
	if hc == nil {
		// Private client without redirects: following a 307/308 would resend
		// the form with the client_secret. In production the client comes from
		// tokenHTTPClient; this case only happens without it.
		hc = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := hc.Do(req)
	if err != nil {
		// Network failure while requesting the token: requesting a token has no
		// side effect, so retrying is as safe as for a read.
		var ue *url.Error
		switch {
		case errors.Is(err, context.Canceled):
			return nil, fmt.Errorf("%s: %w", tokenOperation, stripURL(err))
		case errors.As(err, &ue):
			return nil, fmt.Errorf("%s: %w", tokenOperation, &transportError{
				err: stripURL(err), httpMethod: http.MethodGet, wrote: true,
				timeout: errors.Is(err, context.DeadlineExceeded),
			})
		default:
			return nil, fmt.Errorf("%s: %w", tokenOperation, &tokenFailureError{err: stripURL(err)})
		}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit))
	// As in transport.go: a close failure counts as a read failure if the
	// read itself did not fail.
	if cerr := resp.Body.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		return nil, fmt.Errorf("%s: read response: %w", tokenOperation, &tokenFailureError{err: stripURL(err)})
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := decodeError(tokenOperation, resp, body)
		// Requesting a token has no side effect: retrying is as safe as for a read.
		apiErr.httpMethod = http.MethodGet
		return nil, apiErr
	}

	var r tokenResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", tokenOperation, &tokenFailureError{err: errors.New("token response is not valid JSON")})
	}
	if r.AccessToken == "" {
		return nil, fmt.Errorf("%s: %w", tokenOperation, &tokenFailureError{err: errors.New("token response without access_token")})
	}
	if r.TokenType == "" {
		r.TokenType = "Bearer"
	}
	validity := validityOf(r.ExpiresIn)
	t := &oauth2.Token{AccessToken: r.AccessToken, TokenType: r.TokenType, ExpiresIn: int64(validity / time.Second)}
	t.Expiry = c.now().Add(validity)
	return t, nil
}

func (c *tokenCache) invalidate(t *oauth2.Token) {
	if t == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == t {
		// Keep the pointer, to remember whether it was accepted, but make it expired.
		c.expiresAt = time.Time{}
	}
}

func (c *tokenCache) markAccepted(t *oauth2.Token) {
	if t == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Only tokens this cache issued and still remembers.
	if t == c.current || c.accepted[t] {
		c.accepted[t] = true
	}
}

func (c *tokenCache) wasAccepted(t *oauth2.Token) bool {
	if t == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accepted[t]
}

// externalSource adapts the caller's oauth2.TokenSource (WithTokenSource). The
// caller handles caching and renewal, so nothing here is renewable and the
// 403 rule does not apply.
type externalSource struct {
	ts oauth2.TokenSource
}

func newExternalSource(ts oauth2.TokenSource) *externalSource {
	return &externalSource{ts: ts}
}

func (f *externalSource) token(ctx context.Context) (*oauth2.Token, error) {
	// The TokenSource takes no ctx; at least we do not start with it expired.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t, err := f.ts.Token()
	if err != nil {
		return nil, fmt.Errorf("get token from external source: %w", &tokenFailureError{err: stripURL(err)})
	}
	return t, nil
}

func (f *externalSource) invalidate(*oauth2.Token)       {}
func (f *externalSource) markAccepted(*oauth2.Token)     {}
func (f *externalSource) wasAccepted(*oauth2.Token) bool { return false }
func (f *externalSource) renewable() bool                { return false }
