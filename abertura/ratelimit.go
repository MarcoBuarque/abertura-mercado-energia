package abertura

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// limitGroup selects the bucket of the local limiter.
type limitGroup int

const (
	// groupGeneral applies to everything except measurements.
	groupGeneral limitGroup = iota
	// groupMeasurements is the measurements bucket (higher limit in production).
	groupMeasurements
)

// maxBurst is the burst size for both environments; a limit smaller than
// this (3 per 300 ms, in tests) has a burst of its own size.
const maxBurst = 30

// limiters holds one bucket per group. A nil bucket does not limit.
//
// The limit is per process: several processes with the same IP or agent add
// up requests to the API without this limiter knowing.
type limiters struct {
	general, measurements *rate.Limiter
}

// newBucket creates the bucket for a RateLimit; the zero (or incomplete)
// value is a nil bucket, which does not limit.
func newBucket(l RateLimit) *rate.Limiter {
	if l.Requests <= 0 || l.Per <= 0 {
		return nil
	}
	return rate.NewLimiter(rate.Every(l.Per/time.Duration(l.Requests)), min(maxBurst, l.Requests))
}

// newLimiters builds the buckets. A zero Measurements uses the Default
// bucket (sandbox).
func newLimiters(rl RateLimits) limiters {
	l := limiters{general: newBucket(rl.Default)}
	l.measurements = l.general
	if rl.Measurements != (RateLimit{}) {
		l.measurements = newBucket(rl.Measurements)
	}
	return l
}

// waitFor blocks until the group may send a request, or until ctx ends. A
// deadline shorter than the wait returns DeadlineExceeded right away, without
// waiting needlessly.
func (l limiters) waitFor(ctx context.Context, g limitGroup) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	bucket := l.general
	if g == groupMeasurements {
		bucket = l.measurements
	}
	if bucket == nil {
		return nil
	}
	if err := bucket.Wait(ctx); err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		// Wait refuses up front a wait that would exceed the ctx deadline.
		return fmt.Errorf("local rate limiter: %w", context.DeadlineExceeded)
	}
	return nil
}

// limitedTransport makes each request of the token client wait for the
// general bucket, because the token request also counts toward the API limit.
type limitedTransport struct {
	base   http.RoundTripper
	limits limiters
}

func (t limitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.limits.waitFor(req.Context(), groupGeneral); err != nil {
		// The http.RoundTripper contract requires closing the body on error too.
		if req.Body != nil {
			if cerr := req.Body.Close(); cerr != nil {
				err = errors.Join(err, cerr)
			}
		}
		return nil, err
	}
	return t.base.RoundTrip(req)
}

// tokenHTTPClient returns a copy of the HTTP client whose transport waits for
// the limiter. The copy never follows redirects, even with a client from
// WithHTTPClient: the token form carries the client_secret, and a 307 would
// resend it to another host. The caller's CheckRedirect applies only to API
// calls, not to the token request.
func tokenHTTPClient(hc *http.Client, l limiters) *http.Client {
	var cp http.Client
	if hc != nil {
		cp = *hc
	}
	cp.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	base := cp.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	cp.Transport = limitedTransport{base: base, limits: l}
	return &cp
}
