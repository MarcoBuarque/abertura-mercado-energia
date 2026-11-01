package abertura

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestProblemFrom_WhenTransportFailure_ClassifiesByType proves that real
// transport failures (untrusted TLS, connection closed without a response,
// invalid JSON in a 200) reach the catalogue, and that Retryable and Uncertain
// come from decide.
func TestProblemFrom_WhenTransportFailure_ClassifiesByType(t *testing.T) {
	t.Parallel()

	drop := func(t *testing.T) http.HandlerFunc { return sequence(t, stubResponse{drop: true}) }
	tests := []struct {
		name          string
		tls           bool
		api           func(t *testing.T) http.HandlerFunc
		call          func(c *Client) error
		wantType      string
		wantRetryable bool
		wantUncertain bool
		wantTLS       bool
	}{
		{
			name: "GET with untrusted certificate",
			tls:  true,
			api:  func(t *testing.T) http.HandlerFunc { return sequence(t, stubResponse{status: 200, body: okBody}) },
			call: func(c *Client) error {
				_, err := c.Migrations.ListPage(context.Background(), MigrationFilter{From: fromMonth, To: toMonth, ConsumerUnit: fakeConsumerUnit}, "")
				return err
			},
			wantType: "transport-error", wantRetryable: true, wantTLS: true,
		},
		{
			name: "POST with untrusted certificate never left and is safe to retry",
			tls:  true,
			api:  func(t *testing.T) http.HandlerFunc { return sequence(t, stubResponse{status: 200, body: okBody}) },
			call: func(c *Client) error {
				_, err := c.Migrations.Create(context.Background(), MigrationRequest{ConsumerUnit: fakeConsumerUnit})
				return err
			},
			wantType: "transport-error", wantRetryable: true, wantTLS: true,
		},
		{
			name: "GET with the connection closed without a response",
			api:  drop,
			call: func(c *Client) error {
				_, err := c.Migrations.ListPage(context.Background(), MigrationFilter{From: fromMonth, To: toMonth, ConsumerUnit: fakeConsumerUnit}, "")
				return err
			},
			wantType: "transport-error", wantRetryable: true,
		},
		{
			name: "POST with the connection closed without a response is uncertain",
			api:  drop,
			call: func(c *Client) error {
				_, err := c.Migrations.Create(context.Background(), MigrationRequest{ConsumerUnit: fakeConsumerUnit})
				return err
			},
			wantType: "uncertain-result", wantUncertain: true,
		},
		{
			name: "GET with 200 and invalid JSON",
			api: func(t *testing.T) http.HandlerFunc {
				return sequence(t, stubResponse{status: 200, header: map[string]string{"Content-Type": "application/json"}, body: `{"migracao": [`})
			},
			call: func(c *Client) error {
				_, err := c.Migrations.ListPage(context.Background(), MigrationFilter{From: fromMonth, To: toMonth, ConsumerUnit: fakeConsumerUnit}, "")
				return err
			},
			wantType: "unreadable-response", wantRetryable: true,
		},
		{
			name: "GET with the attempt deadline exceeded",
			api: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					select {
					case <-r.Context().Done():
					case <-time.After(2 * time.Second):
					}
				}
			},
			call: func(c *Client) error {
				_, err := c.Migrations.ListPage(context.Background(), MigrationFilter{From: fromMonth, To: toMonth, ConsumerUnit: fakeConsumerUnit}, "", WithTimeout(50*time.Millisecond))
				return err
			},
			wantType: "timeout", wantRetryable: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var c *Client
			if tc.tls {
				srv := httptest.NewTLSServer(tc.api(t))
				t.Cleanup(srv.Close)
				c, _ = newClientWithEnv(t, Environment{Name: "test", APIURL: srv.URL, TokenURL: srv.URL + "/sso/oauth/token"},
					fixedSource("token-fake"), WithAgent(fakeAgent, fakeProfile), WithRateLimits(RateLimits{}))
			} else {
				s := newServer(t, tc.api(t))
				c, _ = newTestClient(t, s, WithRetries(0))
			}
			err := tc.call(c)
			if err == nil {
				t.Fatal("want an error")
			}
			p := ProblemFrom(err)
			if want := ErrorCatalogURL + "#" + tc.wantType; p.Type != want {
				t.Errorf("Type = %q, want %q (error: %v)", p.Type, want, err)
			}
			if p.Retryable != tc.wantRetryable {
				t.Errorf("Retryable = %v, want %v", p.Retryable, tc.wantRetryable)
			}
			if p.Uncertain != tc.wantUncertain {
				t.Errorf("Uncertain = %v, want %v", p.Uncertain, tc.wantUncertain)
			}
			if tc.wantTLS {
				var ua x509.UnknownAuthorityError
				if !errors.As(err, &ua) {
					t.Errorf("the x509 cause should stay accessible through errors.As: %v", err)
				}
			}
		})
	}
}

// TestProblemFrom_WhenTokenFailure_ClassifiesByType covers the token request
// failures: transport, an endpoint with JSON lacking access_token, and an
// external source.
func TestProblemFrom_WhenTokenFailure_ClassifiesByType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		client        func(t *testing.T) *Client
		wantType      string
		wantRetryable bool
	}{
		{
			name: "token endpoint with closed connection",
			client: func(t *testing.T) *Client {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					hj, ok := w.(http.Hijacker)
					if !ok {
						t.Error("no Hijack")
						return
					}
					conn, _, err := hj.Hijack()
					if err != nil {
						t.Errorf("Hijack: %v", err)
						return
					}
					if err := conn.Close(); err != nil {
						t.Errorf("close: %v", err)
					}
				}))
				t.Cleanup(srv.Close)
				c, _ := newClientWithEnv(t, Environment{Name: "test", APIURL: srv.URL, TokenURL: srv.URL},
					WithCredentials(fakeClientID, fakeSecret), WithAgent(fakeAgent, fakeProfile), WithRateLimits(RateLimits{}), WithRetries(0))
				return c
			},
			wantType: "transport-error", wantRetryable: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.client(t).Migrations.ListPage(context.Background(), MigrationFilter{From: fromMonth, To: toMonth, ConsumerUnit: fakeConsumerUnit}, "")
			if err == nil {
				t.Fatal("want an error")
			}
			p := ProblemFrom(err)
			if want := ErrorCatalogURL + "#" + tc.wantType; p.Type != want {
				t.Errorf("Type = %q, want %q (error: %v)", p.Type, want, err)
			}
			if p.Retryable != tc.wantRetryable {
				t.Errorf("Retryable = %v, want %v", p.Retryable, tc.wantRetryable)
			}
		})
	}
}
