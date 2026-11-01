package abertura

import (
	"net/http"
	"testing"
)

// base returns a "clean" outcome with a retry budget; each table case
// changes only what matters.
func base(alt func(*outcome)) outcome {
	r := outcome{maxRetries: 2, renewable: true}
	if alt != nil {
		alt(&r)
	}
	return r
}

// TestDecide_WhenRetryTable_ReturnsExpectedAction has one case per row of
// table 4.4 of the architecture, for GET, POST and PUT. The "row" column points
// to the table situation the case covers.
func TestDecide_WhenRetryTable_ReturnsExpectedAction(t *testing.T) {
	t.Parallel()

	const (
		read   = "GET"
		writeA = "POST"
		writeB = "PUT"
	)
	tests := []struct {
		name       string
		row        string
		httpMethod string
		r          outcome
		want       action
	}{
		// Error before the body is written: retries for every method.
		{"GET error before body written retries", "error before body written", read, base(func(r *outcome) { r.failure = failureTransport }), actionRetry},
		{"POST error before body written retries", "error before body written", writeA, base(func(r *outcome) { r.failure = failureTransport }), actionRetry},
		{"PUT error before body written retries", "error before body written", writeB, base(func(r *outcome) { r.failure = failureTransport }), actionRetry},
		{"POST error before body written without retries is error", "error before body written", writeA, base(func(r *outcome) { r.failure = failureTransport; r.retries = 2 }), actionError},

		// Transport error after the body is written.
		{"GET error after body written retries", "error after body written", read, base(func(r *outcome) { r.failure = failureTransport; r.wrote = true }), actionRetry},
		{"POST error after body written is uncertain", "error after body written", writeA, base(func(r *outcome) { r.failure = failureTransport; r.wrote = true }), actionUncertain},
		{"PUT error after body written is uncertain", "error after body written", writeB, base(func(r *outcome) { r.failure = failureTransport; r.wrote = true }), actionUncertain},
		{"GET error after body written without retries is error", "retries exhausted", read, base(func(r *outcome) { r.failure = failureTransport; r.wrote = true; r.retries = 2 }), actionError},

		// Attempt timeout.
		{"GET timeout after body written retries", "attempt timeout", read, base(func(r *outcome) { r.failure = failureTimeout; r.wrote = true }), actionRetry},
		{"POST timeout after body written is uncertain", "attempt timeout", writeA, base(func(r *outcome) { r.failure = failureTimeout; r.wrote = true }), actionUncertain},
		{"PUT timeout after body written is uncertain", "attempt timeout", writeB, base(func(r *outcome) { r.failure = failureTimeout; r.wrote = true }), actionUncertain},
		{"POST timeout before body written retries", "attempt timeout", writeA, base(func(r *outcome) { r.failure = failureTimeout }), actionRetry},
		{"GET timeout without retries is error", "retries exhausted", read, base(func(r *outcome) { r.failure = failureTimeout; r.wrote = true; r.retries = 2 }), actionError},

		// Caller context canceled or expired.
		{"GET context before body written stops", "caller context", read, base(func(r *outcome) { r.failure = failureContext }), actionError},
		{"GET context after body written stops", "caller context", read, base(func(r *outcome) { r.failure = failureContext; r.wrote = true }), actionError},
		{"POST context before body written is error", "caller context", writeA, base(func(r *outcome) { r.failure = failureContext }), actionError},
		{"POST context after body written is uncertain", "caller context", writeA, base(func(r *outcome) { r.failure = failureContext; r.wrote = true }), actionUncertain},
		{"PUT context after body written is uncertain", "caller context", writeB, base(func(r *outcome) { r.failure = failureContext; r.wrote = true }), actionUncertain},

		// 2xx.
		{"GET 200 succeeds", "2xx", read, base(func(r *outcome) { r.status = 200 }), actionSuccess},
		{"POST 201 succeeds", "2xx", writeA, base(func(r *outcome) { r.status = 201 }), actionSuccess},
		{"PUT 204 succeeds", "2xx", writeB, base(func(r *outcome) { r.status = 204 }), actionSuccess},
		{"POST 200 unreadable is uncertain", "2xx with unreadable body", writeA, base(func(r *outcome) { r.status = 200; r.unreadableBody = true }), actionUncertain},
		{"PUT 200 unreadable is uncertain", "2xx with unreadable body", writeB, base(func(r *outcome) { r.status = 200; r.unreadableBody = true }), actionUncertain},
		{"GET 200 unreadable retries", "2xx with unreadable body", read, base(func(r *outcome) { r.status = 200; r.unreadableBody = true }), actionRetry},
		{"GET 200 unreadable without retries is error", "retries exhausted", read, base(func(r *outcome) { r.status = 200; r.unreadableBody = true; r.retries = 2 }), actionError},

		// Other 4xx.
		{"GET 400 does not retry", "400, 404, other 4xx", read, base(func(r *outcome) { r.status = 400 }), actionError},
		{"POST 400 does not retry", "400, 404, other 4xx", writeA, base(func(r *outcome) { r.status = 400 }), actionError},
		{"PUT 404 does not retry", "400, 404, other 4xx", writeB, base(func(r *outcome) { r.status = 404 }), actionError},
		{"GET 404 does not retry", "400, 404, other 4xx", read, base(func(r *outcome) { r.status = 404 }), actionError},
		{"POST 422 does not retry", "400, 404, other 4xx", writeA, base(func(r *outcome) { r.status = 422 }), actionError},

		// 403 outside rule 4.2.
		{"GET 403 from backend does not retry", "403 outside rule 4.2", read, base(func(r *outcome) { r.status = 403; r.invalidCredential = true; r.tokenAccepted = true }), actionError},
		{"POST 403 from backend does not retry", "403 outside rule 4.2", writeA, base(func(r *outcome) { r.status = 403; r.invalidCredential = true; r.tokenAccepted = true }), actionError},
		{"GET 403 from authorizer with never-accepted token does not retry", "403 outside rule 4.2", read, base(func(r *outcome) { r.status = 403; r.invalidCredential = true; r.gateway = true }), actionError},
		{"POST 403 from authorizer with never-accepted token does not retry", "403 outside rule 4.2", writeA, base(func(r *outcome) { r.status = 403; r.invalidCredential = true; r.gateway = true }), actionError},
		{"GET 403 from authorizer without ERR code does not retry", "403 outside rule 4.2", read, base(func(r *outcome) { r.status = 403; r.gateway = true; r.tokenAccepted = true }), actionError},
		{"GET 403 without ERR code from backend does not retry", "403 outside rule 4.2", read, base(func(r *outcome) { r.status = 403 }), actionError},
		{"GET 403 with WithTokenSource does not retry", "403 outside rule 4.2", read, base(func(r *outcome) {
			r.status = 403
			r.invalidCredential = true
			r.gateway = true
			r.tokenAccepted = true
			r.renewable = false
		}), actionError},
		{"GET 403 already refreshed once does not retry", "403 outside rule 4.2", read, base(func(r *outcome) {
			r.status = 403
			r.invalidCredential = true
			r.gateway = true
			r.tokenAccepted = true
			r.alreadyRefreshed = true
		}), actionError},

		// 403 inside rule 4.2.
		{"GET 403 from authorizer with accepted token refreshes", "403 inside rule 4.2", read, base(func(r *outcome) {
			r.status = 403
			r.invalidCredential = true
			r.gateway = true
			r.tokenAccepted = true
		}), actionRefreshRetry},
		{"POST 403 from authorizer with accepted token refreshes", "403 inside rule 4.2", writeA, base(func(r *outcome) {
			r.status = 403
			r.invalidCredential = true
			r.gateway = true
			r.tokenAccepted = true
		}), actionRefreshRetry},
		{"PUT 403 from authorizer with accepted token refreshes", "403 inside rule 4.2", writeB, base(func(r *outcome) {
			r.status = 403
			r.invalidCredential = true
			r.gateway = true
			r.tokenAccepted = true
		}), actionRefreshRetry},
		{"GET 403 refreshes even with retries exhausted", "403 inside rule 4.2", read, base(func(r *outcome) {
			r.status = 403
			r.invalidCredential = true
			r.gateway = true
			r.tokenAccepted = true
			r.retries = 2
		}), actionRefreshRetry},

		// 429.
		{"GET 429 without mark retries", "429", read, base(func(r *outcome) { r.status = 429 }), actionRetry},
		{"GET 429 with mark retries", "429", read, base(func(r *outcome) { r.status = 429; r.gateway = true }), actionRetry},
		{"POST 429 with mark retries", "429", writeA, base(func(r *outcome) { r.status = 429; r.gateway = true }), actionRetry},
		{"PUT 429 with mark retries", "429", writeB, base(func(r *outcome) { r.status = 429; r.gateway = true }), actionRetry},
		{"POST 429 without mark is error", "429", writeA, base(func(r *outcome) { r.status = 429 }), actionError},
		{"PUT 429 without mark is error", "429", writeB, base(func(r *outcome) { r.status = 429 }), actionError},
		{"GET 429 without retries is error", "retries exhausted", read, base(func(r *outcome) { r.status = 429; r.retries = 2 }), actionError},
		{"POST 429 with mark without retries is error", "retries exhausted", writeA, base(func(r *outcome) { r.status = 429; r.gateway = true; r.retries = 2 }), actionError},

		// 5xx.
		{"GET 500 retries", "500 to 599 except 501", read, base(func(r *outcome) { r.status = 500 }), actionRetry},
		{"GET 502 retries", "500 to 599 except 501", read, base(func(r *outcome) { r.status = 502 }), actionRetry},
		{"GET 503 retries", "500 to 599 except 501", read, base(func(r *outcome) { r.status = 503 }), actionRetry},
		{"GET 504 retries", "500 to 599 except 501", read, base(func(r *outcome) { r.status = 504 }), actionRetry},
		{"POST 500 is uncertain", "500 to 599 except 501", writeA, base(func(r *outcome) { r.status = 500 }), actionUncertain},
		{"POST 502 is uncertain", "500 to 599 except 501", writeA, base(func(r *outcome) { r.status = 502 }), actionUncertain},
		{"POST 503 is uncertain", "500 to 599 except 501", writeA, base(func(r *outcome) { r.status = 503 }), actionUncertain},
		{"POST 504 is uncertain", "500 to 599 except 501", writeA, base(func(r *outcome) { r.status = 504 }), actionUncertain},
		{"PUT 502 is uncertain", "500 to 599 except 501", writeB, base(func(r *outcome) { r.status = 502 }), actionUncertain},
		{"PUT 504 is uncertain", "500 to 599 except 501", writeB, base(func(r *outcome) { r.status = 504 }), actionUncertain},
		{"GET 502 without retries is error", "retries exhausted", read, base(func(r *outcome) { r.status = 502; r.retries = 2 }), actionError},
		{"GET 500 with retries disabled is error", "retries exhausted", read, base(func(r *outcome) { r.status = 500; r.maxRetries = 0 }), actionError},
		{"GET 507 retries", "other 5xx except 501", read, base(func(r *outcome) { r.status = 507 }), actionRetry},
		{"GET 599 retries", "other 5xx except 501", read, base(func(r *outcome) { r.status = 599 }), actionRetry},
		{"POST 507 is uncertain", "other 5xx except 501", writeA, base(func(r *outcome) { r.status = 507 }), actionUncertain},
		{"POST 520 is uncertain", "other 5xx except 501", writeA, base(func(r *outcome) { r.status = 520 }), actionUncertain},
		{"PUT 599 is uncertain", "other 5xx except 501", writeB, base(func(r *outcome) { r.status = 599 }), actionUncertain},
		{"GET 507 without retries is error", "retries exhausted", read, base(func(r *outcome) { r.status = 507; r.retries = 2 }), actionError},
		{"GET 600 does not retry", "outside 5xx", read, base(func(r *outcome) { r.status = 600 }), actionError},
		{"POST 600 is not uncertain", "outside 5xx", writeA, base(func(r *outcome) { r.status = 600 }), actionError},
		{"GET 501 does not retry", "501 is final", read, base(func(r *outcome) { r.status = 501 }), actionError},
		{"POST 501 does not retry", "501 is final", writeA, base(func(r *outcome) { r.status = 501 }), actionError},
		{"GET 302 does not retry", "other statuses (interpretation)", read, base(func(r *outcome) { r.status = http.StatusFound }), actionError},
		{"PATCH 502 is uncertain", "generic write (interpretation)", "PATCH", base(func(r *outcome) { r.status = 502 }), actionUncertain},
		{"HEAD 502 retries", "generic read (interpretation)", "HEAD", base(func(r *outcome) { r.status = 502 }), actionRetry},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := decide(tc.httpMethod, tc.r); got != tc.want {
				t.Errorf("decide(%s, %+v) [%s] = %v, want %v", tc.httpMethod, tc.r, tc.row, got, tc.want)
			}
		})
	}
}

func TestAction_WhenStringified_ReturnsName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a    action
		want string
	}{
		{actionSuccess, "success"},
		{actionRetry, "retry"},
		{actionRefreshRetry, "refresh and retry"},
		{actionError, "error"},
		{actionUncertain, "uncertain"},
		{action(0), "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			if got := tc.a.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFailureName_WhenOutcome_ReturnsLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		d    outcome
		want string
	}{
		{"no failure", outcome{}, ""},
		{"transport", outcome{failure: failureTransport}, "transport"},
		{"timeout", outcome{failure: failureTimeout}, "timeout"},
		{"context", outcome{failure: failureContext}, "context"},
		{"unreadable body", outcome{unreadableBody: true}, "body"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := failureName(tc.d); got != tc.want {
				t.Errorf("failureName = %q, want %q", got, tc.want)
			}
		})
	}
}
