package abertura

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimiter_WhenFourthRequest_Waits(t *testing.T) {
	t.Parallel()

	l := newLimiters(RateLimits{Default: RateLimit{Requests: 3, Per: 300 * time.Millisecond}})
	ctx := context.Background()

	start := time.Now()
	for i := range 3 {
		if err := l.waitFor(ctx, groupGeneral); err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("the burst of 3 waited %v", d)
	}

	start = time.Now()
	if err := l.waitFor(ctx, groupGeneral); err != nil {
		t.Fatalf("fourth request: %v", err)
	}
	if d := time.Since(start); d < 60*time.Millisecond {
		t.Errorf("the fourth request waited only %v, it should wait close to 100 ms", d)
	}
}

func TestLimiter_WhenCtxEndsDuringWait_ReturnsCtxError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     func() (context.Context, context.CancelFunc)
		wantErr error
	}{
		{
			name: "a deadline shorter than the wait returns DeadlineExceeded without waiting",
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 30*time.Millisecond) //nolint:gosec // the test calls cancel
			},
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "cancellation during the wait returns Canceled",
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancelFn := context.WithCancel(context.Background())
				time.AfterFunc(30*time.Millisecond, cancelFn)
				return ctx, cancelFn
			},
			wantErr: context.Canceled,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := newLimiters(RateLimits{Default: RateLimit{Requests: 1, Per: time.Hour}})
			if err := l.waitFor(context.Background(), groupGeneral); err != nil {
				t.Fatal(err)
			}
			ctx, cancelFn := tc.ctx()
			defer cancelFn()

			start := time.Now()
			err := l.waitFor(ctx, groupGeneral)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if d := time.Since(start); d > 2*time.Second {
				t.Errorf("returned only after %v", d)
			}
		})
	}
}

func TestLimiter_MeasurementsBucket(t *testing.T) {
	t.Parallel()

	short := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 30*time.Millisecond) //nolint:gosec // the test calls cancel
	}
	oncePerHour := RateLimit{Requests: 1, Per: time.Hour}
	tests := []struct {
		name             string
		limits           RateLimits
		wantMeasurements bool // the second measurement passes after the general bucket is exhausted
	}{
		{"production: measurements have their own bucket", RateLimits{Default: oncePerHour, Measurements: RateLimit{Requests: 5, Per: time.Hour}}, true},
		{"sandbox: zero Measurements uses the general bucket", RateLimits{Default: oncePerHour}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := newLimiters(tc.limits)
			if err := l.waitFor(context.Background(), groupGeneral); err != nil {
				t.Fatal(err)
			}
			ctx, cancelFn := short()
			defer cancelFn()

			err := l.waitFor(ctx, groupMeasurements)
			if got := err == nil; got != tc.wantMeasurements {
				t.Errorf("measurements passed = %v (%v), want %v", got, err, tc.wantMeasurements)
			}
		})
	}
}

func TestLimiter_MeasurementsBucketWithoutGeneral(t *testing.T) {
	t.Parallel()

	l := newLimiters(RateLimits{Measurements: RateLimit{Requests: 1, Per: time.Hour}})
	ctx, cancelFn := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancelFn()

	if err := l.waitFor(ctx, groupGeneral); err != nil {
		t.Errorf("the general bucket is unlimited: %v", err)
	}
	if err := l.waitFor(ctx, groupMeasurements); err != nil {
		t.Fatalf("first measurement: %v", err)
	}
	if err := l.waitFor(ctx, groupMeasurements); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("second measurement = %v, want DeadlineExceeded", err)
	}
}

func TestLimiter_WhenZeroValue_IsDisabled(t *testing.T) {
	t.Parallel()

	l := newLimiters(RateLimits{})
	start := time.Now()
	for range 1000 {
		if err := l.waitFor(context.Background(), groupMeasurements); err != nil {
			t.Fatal(err)
		}
		if err := l.waitFor(context.Background(), groupGeneral); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("disabled limiter waited %v", d)
	}
}

func TestLimiter_BurstNeverExceeds30(t *testing.T) {
	t.Parallel()

	l := newLimiters(RateLimits{Default: RateLimit{Requests: 900, Per: 5 * time.Minute}})
	ctx, cancelFn := context.WithTimeout(context.Background(), time.Second)
	defer cancelFn()

	for i := range 30 {
		if err := l.waitFor(ctx, groupGeneral); err != nil {
			t.Fatalf("request %d of the burst: %v", i+1, err)
		}
	}
	// The 31st waits ~333 ms (5 min / 900), more than the 50 ms deadline.
	short, cancelFn2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelFn2()
	if err := l.waitFor(short, groupGeneral); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("31st request = %v, want DeadlineExceeded", err)
	}
}

// The token request goes through the general bucket: with 2 per hour, the
// first call spends 1 on the token and 1 on the request, and the second no
// longer fits.
func TestLimiter_TokenSpendsFromGeneralBucket(t *testing.T) {
	t.Parallel()

	srv := newServer(t, sequence(t, stubResponse{status: 200, body: okBody}))
	c, _ := newTestClient(t, srv, WithRateLimits(RateLimits{Default: RateLimit{Requests: 2, Per: time.Hour}}))

	if _, _, err := c.send(context.Background(), readOp(), callConfig{}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	ctx, cancelFn := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelFn()
	_, _, err := c.send(ctx, readOp(), callConfig{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second call = %v, want DeadlineExceeded", err)
	}
	if srv.tokens.Load() != 1 || srv.reqs.Load() != 1 {
		t.Errorf("tokens=%d reqs=%d, want 1 and 1", srv.tokens.Load(), srv.reqs.Load())
	}
}

// Every attempt, retries included, waits for the limiter.
func TestLimiter_RetriesSpendFromBucket(t *testing.T) {
	t.Parallel()

	srv := newServer(t, sequence(t, stubResponse{status: 502}))
	c, _ := newClientWithEnv(t, srv.env(), fixedSource("fake-token-1"), WithAgent(fakeAgent, fakeProfile),
		WithRateLimits(RateLimits{Default: RateLimit{Requests: 2, Per: time.Hour}}))

	ctx, cancelFn := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelFn()
	_, _, err := c.send(ctx, readOp(), callConfig{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded on the third attempt", err)
	}
	if srv.reqs.Load() != 2 {
		t.Errorf("requests = %d, want 2", srv.reqs.Load())
	}
}

// The limiter also blocks the token request when the bucket is exhausted.
func TestLimiter_TokenWaitsForBucket(t *testing.T) {
	t.Parallel()

	srv := newServer(t, sequence(t, stubResponse{status: 200, body: okBody}))
	c, _ := newTestClient(t, srv, WithRateLimits(RateLimits{Default: RateLimit{Requests: 1, Per: time.Hour}}))
	ctx, cancelFn := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelFn()

	// The attempt spends the only item in the bucket; the token request no longer fits.
	_, _, err := c.send(ctx, readOp(), callConfig{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if srv.tokens.Load() != 0 {
		t.Errorf("tokens fetched = %d, want 0", srv.tokens.Load())
	}
	assertNoLeak(t, err.Error(), srv.URL)
}

func TestLimiter_TokenClientPreservesUserClient(t *testing.T) {
	t.Parallel()

	user := &http.Client{Timeout: 7 * time.Second}
	clone := tokenHTTPClient(user, limiters{})
	if clone == user || clone.Timeout != 7*time.Second {
		t.Errorf("the clone should keep the Timeout and be a different client")
	}
	if user.Transport != nil {
		t.Error("the user's client was modified")
	}
}

// TestTokenHTTPClient_WhenRedirect_DoesNotResendForm proves that the token
// client never follows a 307, which would resend the form's client_secret to
// another host, even with a user client (WithHTTPClient) and even if it has a
// CheckRedirect that would follow.
func TestTokenHTTPClient_WhenRedirect_DoesNotResendForm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		user        *http.Client
		wantFollows bool
		wantStatus  int
	}{
		{"the SDK default client does not follow", nil, false, http.StatusTemporaryRedirect},
		{"a user client without CheckRedirect does not follow either", &http.Client{}, false, http.StatusTemporaryRedirect},
		{"a user client with a CheckRedirect that would follow does not follow either", &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return nil }}, false, http.StatusTemporaryRedirect},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var receivedB atomic.Bool
			b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedB.Store(true)
			}))
			defer b.Close()
			a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, b.URL, http.StatusTemporaryRedirect)
			}))
			defer a.Close()

			cli := tokenHTTPClient(tc.user, limiters{})
			resp, err := cli.PostForm(a.URL, url.Values{"client_secret": {"fake-secret"}})
			if err != nil {
				t.Fatalf("PostForm: %v", err)
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Errorf("close body: %v", err)
				}
			}()
			if resp.StatusCode != tc.wantStatus || receivedB.Load() != tc.wantFollows {
				t.Errorf("status = %d, second server received = %v; want %d and %v",
					resp.StatusCode, receivedB.Load(), tc.wantStatus, tc.wantFollows)
			}
		})
	}
}

type trackedBody struct {
	closed atomic.Bool
}

func (c *trackedBody) Read([]byte) (int, error) { return 0, io.EOF }
func (c *trackedBody) Close() error             { c.closed.Store(true); return nil }

// TestLimitedTransport_WhenLimiterFails_ClosesBody proves the
// http.RoundTripper contract: the request body is also closed when the
// transport returns an error without sending.
func TestLimitedTransport_WhenLimiterFails_ClosesBody(t *testing.T) {
	t.Parallel()

	ctx, cancelFn := context.WithCancel(context.Background())
	cancelFn()
	body := &trackedBody{}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://example.org/token", body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	tr := limitedTransport{base: http.DefaultTransport, limits: newLimiters(RateLimits{Default: RateLimit{Requests: 1, Per: time.Hour}})}
	resp, err := tr.RoundTrip(req)
	if resp != nil {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Errorf("close response body: %v", cerr)
		}
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RoundTrip: %v, want context.Canceled", err)
	}
	if !body.closed.Load() {
		t.Error("the request body was not closed")
	}
}
