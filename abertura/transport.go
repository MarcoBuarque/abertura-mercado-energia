package abertura

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	"golang.org/x/oauth2"
)

const (
	// invalidCredentialCode is the API error code of the authorizer 403.
	invalidCredentialCode = "ERR_CREDENCIAL_INVALIDA" //nolint:gosec // G101: API error code name, not a credential
	// gatewayHeader marks responses produced by the API Gateway (authorizer, rate limit).
	gatewayHeader = "x-amzn-ErrorType"
)

// Wait times between attempts (architecture section 4.4).
const (
	baseBackoff    = 500 * time.Millisecond
	backoffCap     = 8 * time.Second
	baseBackoff429 = 2 * time.Second
	backoffCap429  = 30 * time.Second
	retryAfterCap  = 30 * time.Second
)

// action is what the send loop does after an attempt.
type action int

const (
	// actionSuccess returns the response to the caller.
	actionSuccess action = iota + 1
	// actionRetry waits and sends again, spending a retry.
	actionRetry
	// actionRefreshRetry discards the token, fetches another and sends again,
	// without spending a retry. It applies once per call.
	actionRefreshRetry
	// actionError returns the attempt error.
	actionError
	// actionUncertain returns *UncertainResultError: the write may have been
	// applied.
	actionUncertain
)

func (a action) String() string {
	switch a {
	case actionSuccess:
		return "success"
	case actionRetry:
		return "retry"
	case actionRefreshRetry:
		return "refresh and retry"
	case actionError:
		return "error"
	case actionUncertain:
		return "uncertain"
	default:
		return "unknown"
	}
}

// failureKind classifies an attempt that did not get a complete response.
type failureKind int

const (
	// noFailure: there was a response.
	noFailure failureKind = iota
	// failureTransport: DNS, connection, TLS, dropped connection, read.
	failureTransport
	// failureTimeout: the attempt deadline expired (not the caller ctx).
	failureTimeout
	// failureContext: the caller ctx was canceled or expired.
	failureContext
)

// outcome gathers what decide needs to know about an attempt.
type outcome struct {
	failure failureKind
	// wrote: the request headers were written to the connection
	// (httptrace.WroteHeaders). When false nothing left, and a retry is safe.
	wrote bool

	status int
	// gateway: the response carried x-amzn-ErrorType, meaning it came from
	// the API Gateway and not from the backend.
	gateway bool
	// invalidCredential: the body carries the code ERR_CREDENCIAL_INVALIDA.
	invalidCredential bool
	// tokenAccepted: the token in use was already accepted by some response
	// without the authorizer mark.
	tokenAccepted bool
	// renewable: the SDK can fetch another token (false with WithTokenSource).
	renewable bool
	// alreadyRefreshed: the refresh of the 403 rule was already used in this call.
	alreadyRefreshed bool
	// unreadableBody: 2xx response whose body could not be read.
	unreadableBody bool

	// retries is the number of retries already done; maxRetries is the limit (WithRetries).
	retries, maxRetries int
}

// isRead reports whether the method is safe to retry. Any other method
// (POST, PUT, PATCH, DELETE) is treated as a write.
func isRead(httpMethod string) bool {
	return httpMethod == http.MethodGet || httpMethod == http.MethodHead
}

// decide is the retry table of the architecture (section 4.4) as a pure
// function: the method and what the attempt brought determine the action. It
// does no I/O and reads no clock.
func decide(httpMethod string, r outcome) action {
	read := isRead(httpMethod)
	// Retry only while retries remain; once exhausted, the attempt error is returned.
	retry := actionRetry
	if r.retries >= r.maxRetries {
		retry = actionError
	}

	if r.failure != noFailure {
		switch {
		case r.failure == failureContext:
			// The caller gave up: a read stops; a write already sent is uncertain.
			if !read && r.wrote {
				return actionUncertain
			}
			return actionError
		case !r.wrote:
			// Nothing left the machine: a retry is safe for any method.
			return retry
		case read:
			return retry
		default:
			return actionUncertain
		}
	}

	switch {
	case r.status >= 200 && r.status < 300:
		switch {
		case !r.unreadableBody:
			return actionSuccess
		case read:
			return retry
		default:
			// The write was accepted, but what it returned is missing.
			return actionUncertain
		}
	case r.status == http.StatusForbidden:
		// Rule 4.2: authorizer, a token that was already accepted and a
		// refresh not yet used. The API Gateway refuses before the backend,
		// so a retry is safe even for a write.
		if !r.alreadyRefreshed && r.renewable && r.invalidCredential && r.gateway && r.tokenAccepted {
			return actionRefreshRetry
		}
		return actionError
	case r.status == http.StatusTooManyRequests:
		// A write is only retried with the authorizer mark: the gateway
		// limited it, and the backend never saw the request.
		if read || r.gateway {
			return retry
		}
		return actionError
	case r.status >= 500 && r.status <= 599 && r.status != http.StatusNotImplemented:
		// Every 5xx except 501 follows the 500 row: the backend may have
		// applied the write before failing. The 501 (method not implemented)
		// is final and falls to the default.
		if read {
			return retry
		}
		return actionUncertain
	default:
		return actionError
	}
}

// retryWait computes how long to wait before the next retry. retry counts
// the retries already done (0 for the first). A readable Retry-After, in
// seconds or as an HTTP date, wins over the backoff, capped at 30 s. Without
// it, the backoff is exponential with full jitter: rnd() in [0,1) multiplies
// the span base*2^retry, limited to the cap. On 429 the base is 2 s and the
// cap 30 s; otherwise 500 ms and 8 s.
func retryWait(status int, retryAfter string, retry int, now time.Time, rnd func() float64) time.Duration {
	if d, ok := parseRetryAfter(retryAfter, now); ok {
		return min(d, retryAfterCap)
	}
	span, cap := baseBackoff, backoffCap
	if status == http.StatusTooManyRequests {
		span, cap = baseBackoff429, backoffCap429
	}
	for range retry {
		if span >= cap {
			break
		}
		span *= 2
	}
	span = min(span, cap)
	return time.Duration(rnd() * float64(span))
}

// parseRetryAfter understands the header as whole seconds or as an HTTP date.
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	if s, err := strconv.Atoi(value); err == nil {
		if s < 0 {
			return 0, false
		}
		// Clamp before multiplying, to avoid overflowing time.Duration.
		return time.Duration(min(s, int(retryAfterCap/time.Second))) * time.Second, true
	}
	t, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	return max(t.Sub(now), 0), true
}

// sleepCtx waits for d or until ctx ends; in that case it returns the ctx error.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// operation describes an API call for the send loop.
type operation struct {
	id           string     // operationId: "listMigrations"
	httpMethod   string     // GET, POST, PUT
	pathTemplate string     // "/v1.1/varejista/migracoes/{idMigracao}", for the log
	path         string     // already filled in and escaped
	query        url.Values // never goes to the log or to errors
	body         any        // nil on GET; sent as JSON
	group        limitGroup // limiter bucket
	key          NaturalKey // for UncertainResultError
}

// responseLimit is the maximum number of bytes read from a response.
const responseLimit = 32 << 20

// params are the values of a call after combining client and call options.
type params struct {
	agent, profile int
	timeout        time.Duration
}

func (c *Client) params(cc callConfig) params {
	p := params{agent: c.agent, profile: c.profile, timeout: c.timeout}
	if cc.hasAgent {
		p.agent, p.profile = cc.agent, cc.profile
	}
	if cc.timeout > 0 {
		p.timeout = cc.timeout
	}
	return p
}

// attempt is what a single request brought back.
type attempt struct {
	status   int
	header   http.Header
	body     []byte
	err      error // transport error, no response
	errRead  error // error reading the response body
	wrote    bool  // headers were written (httptrace.WroteHeaders)
	duration time.Duration
}

// send is the single send loop: limiter, token, headers, attempt, decision
// (decide) and wait. It returns the status and body of a 2xx response. Any
// other outcome is an error: *APIError, *UncertainResultError, the ctx error,
// or a transport error without the URL.
func (c *Client) send(ctx context.Context, op operation, cc callConfig) (int, []byte, error) {
	var body []byte
	if op.body != nil {
		var err error
		if body, err = json.Marshal(op.body); err != nil {
			return 0, nil, invalidArgumentBecause(err, "%s: encode body: %v", op.id, err)
		}
	}
	want := c.apiURL + op.path
	if len(op.query) > 0 {
		want += "?" + op.query.Encode()
	}
	// The net/http build error quotes the URL, and the URL carries the query;
	// the prototype validates what is left without leaking the text.
	proto, err := http.NewRequestWithContext(ctx, op.httpMethod, want, nil)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			return 0, nil, invalidArgument("%s: invalid path", op.id)
		}
		return 0, nil, invalidArgumentBecause(err, "%s: invalid request: %v", op.id, err)
	}
	p := c.params(cc)

	retries, alreadyRefreshed := 0, false
	for n := 1; ; n++ {
		if err := c.limits.waitFor(ctx, op.group); err != nil {
			return 0, nil, err
		}
		tok, err := c.source.token(ctx)
		if err != nil {
			return 0, nil, err
		}

		t := c.tryOnce(ctx, proto, body, tok, p)
		d, apiErr := c.evaluate(ctx, op, t, tok, retries, alreadyRefreshed)
		c.logAttempt(ctx, op, t, d, apiErr, n)

		switch decide(op.httpMethod, d) {
		case actionSuccess:
			return t.status, t.body, nil
		case actionRetry:
			wait := retryWait(t.status, t.header.Get("Retry-After"), retries, c.now(), c.rnd)
			retries++
			c.warn(ctx, "retry scheduled", op, t, d, slog.Int("attempt", n+1), slog.Int64("wait_ms", wait.Milliseconds()))
			if err := c.sleep(ctx, wait); err != nil {
				return 0, nil, err
			}
		case actionRefreshRetry:
			c.source.invalidate(tok)
			alreadyRefreshed = true
			c.warn(ctx, "token refreshed", op, t, d, slog.Int("attempt", n+1))
		case actionUncertain:
			c.warn(ctx, "uncertain result", op, t, d, slog.Int("attempt", n))
			return 0, nil, &UncertainResultError{Operation: op.id, Key: op.key, Err: c.cause(ctx, op, d, t, apiErr)}
		default:
			if d.failure == failureContext {
				return 0, nil, ctx.Err()
			}
			return 0, nil, c.cause(ctx, op, d, t, apiErr)
		}
	}
}

// tryOnce makes one request, with the attempt deadline, and reads the whole response.
func (c *Client) tryOnce(ctx context.Context, proto *http.Request, body []byte, tok *oauth2.Token, p params) attempt {
	var wrote atomic.Bool
	actx, cancela := context.WithTimeout(ctx, p.timeout)
	defer cancela()
	actx = httptrace.WithClientTrace(actx, &httptrace.ClientTrace{
		WroteHeaders: func() { wrote.Store(true) },
	})

	req := proto.Clone(actx)
	if body != nil {
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		req.Header["Content-Type"] = []string{"application/json"}
	}
	tok.SetAuthHeader(req)
	// The names follow the API documentation; the map avoids canonicalization.
	req.Header["codigoAgente"] = []string{strconv.Itoa(p.agent)}
	req.Header["codigoPerfil"] = []string{strconv.Itoa(p.profile)}
	req.Header["Accept"] = []string{"application/json"}
	req.Header["User-Agent"] = []string{c.userAgentValue}

	start := time.Now()
	t := attempt{}
	resp, err := c.http.Do(req) //nolint:gosec // the URL is the environment the caller chose; calling it is the SDK's purpose
	if err != nil {
		t.err = err
	} else {
		t.status, t.header = resp.StatusCode, resp.Header
		var errClose error
		t.body, t.errRead = io.ReadAll(io.LimitReader(resp.Body, responseLimit))
		if errClose = resp.Body.Close(); errClose != nil && t.errRead == nil {
			t.errRead = errClose
		}
	}
	t.wrote = wrote.Load()
	t.duration = time.Since(start)
	return t
}

// evaluate translates the attempt into the outcome that decide understands.
// It also returns the *APIError of a non-2xx response. It marks the token as
// accepted when the response did not come from the authorizer.
func (c *Client) evaluate(ctx context.Context, op operation, t attempt, tok *oauth2.Token, retries int, alreadyRefreshed bool) (outcome, *APIError) {
	d := outcome{
		wrote:            t.wrote,
		status:           t.status,
		renewable:        c.source.renewable(),
		alreadyRefreshed: alreadyRefreshed,
		tokenAccepted:    c.source.wasAccepted(tok),
		retries:          retries,
		maxRetries:       c.retries,
	}

	failure := t.err
	if failure == nil && t.status >= 200 && t.status <= 299 {
		failure = t.errRead
	}
	switch {
	case failure == nil:
	case ctx.Err() != nil:
		d.failure = failureContext
	case errors.Is(failure, context.DeadlineExceeded):
		d.failure = failureTimeout
	case t.err != nil:
		d.failure = failureTransport
	default:
		d.unreadableBody = true
	}
	var apiErr *APIError
	if t.err == nil {
		d.gateway = t.header.Get(gatewayHeader) != ""
		if !d.gateway {
			c.source.markAccepted(tok)
		}
		if t.status < 200 || t.status > 299 {
			apiErr = decodeError(op.id, &http.Response{StatusCode: t.status, Header: t.header}, t.body)
			apiErr.httpMethod = op.httpMethod
			d.invalidCredential = apiErr.Code == invalidCredentialCode
		}
	}
	return d, apiErr
}

// cause is the error that explains why the attempt failed, without the URL.
func (c *Client) cause(ctx context.Context, op operation, d outcome, t attempt, apiErr *APIError) error {
	switch {
	case d.failure == failureContext:
		return ctx.Err()
	case t.err != nil:
		return fmt.Errorf("%s: %w", op.id, &transportError{
			err: stripURL(t.err), httpMethod: op.httpMethod, wrote: d.wrote, timeout: d.failure == failureTimeout,
		})
	case d.unreadableBody || apiErr == nil:
		return fmt.Errorf("%s: unreadable response body: %w", op.id, unreadableResponse(stripURL(t.errRead)))
	default:
		return apiErr
	}
}

// baseAttributes are the fields of every transport log line. The query,
// body, headers (except the request id), token and credentials never appear.
func baseAttributes(op operation, t attempt) []slog.Attr {
	return []slog.Attr{
		slog.String("op", op.id),
		slog.String("method", op.httpMethod),
		slog.String("route", op.pathTemplate),
		slog.Int("status", t.status),
		slog.String("request_id", sanitize(t.header.Get("x-amzn-RequestId"), 100)),
	}
}

// failureName describes, without details, why the attempt had no usable
// response.
func failureName(d outcome) string {
	switch {
	case d.failure == failureTransport:
		return "transport"
	case d.failure == failureTimeout:
		return "timeout"
	case d.failure == failureContext:
		return "context"
	case d.unreadableBody:
		return "body"
	default:
		return ""
	}
}

// logAttempt writes the Debug line of each attempt.
func (c *Client) logAttempt(ctx context.Context, op operation, t attempt, d outcome, apiErr *APIError, n int) {
	if c.logger == nil || !c.logger.Enabled(ctx, slog.LevelDebug) {
		return
	}
	code := ""
	if apiErr != nil {
		code = sanitize(apiErr.Code, codeLimit)
	}
	attrs := append(baseAttributes(op, t),
		slog.Int64("duration_ms", t.duration.Milliseconds()),
		slog.Int("attempt", n),
		slog.String("code", code),
	)
	if f := failureName(d); f != "" {
		attrs = append(attrs, slog.String("failure", f))
	}
	c.logger.LogAttrs(ctx, slog.LevelDebug, "attempt", attrs...)
}

// warn writes a transport Warn line (retry, refresh, uncertain).
func (c *Client) warn(ctx context.Context, msg string, op operation, t attempt, d outcome, extras ...slog.Attr) {
	if c.logger == nil {
		return
	}
	attrs := append(baseAttributes(op, t), extras...)
	if f := failureName(d); f != "" {
		attrs = append(attrs, slog.String("failure", f))
	}
	c.logger.LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
}
