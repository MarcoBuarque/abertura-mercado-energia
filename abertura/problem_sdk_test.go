package abertura

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestProblemFrom_WhenUnclassifiedError_UsesNotInCatalog prova que o
// ProblemFrom nunca devolve about:blank: o que não tem entrada própria cai em
// #not-in-catalog, com o título da entrada.
func TestProblemFrom_WhenUnclassifiedError_UsesNotInCatalog(t *testing.T) {
	t.Parallel()

	p := ProblemFrom(errors.New("something else"))
	if want := ErrorCatalogURL + "#not-in-catalog"; p.Type != want {
		t.Errorf("Type = %q, want %q", p.Type, want)
	}
	if p.Title != "Error outside the catalogue" {
		t.Errorf("Title = %q", p.Title)
	}
	if strings.Contains(p.Detail, "something else") {
		t.Errorf("Detail copied the error text: %q", p.Detail)
	}
}

// TestProblem_MarshalJSON_WhenEmptyTypeOrAboutBlank_FollowsRFC proves that an
// empty type is omitted (RFC 9457 treats it as about:blank) and that, with
// about:blank, the title is the standard HTTP status phrase, or is omitted
// without a status.
func TestProblem_MarshalJSON_WhenEmptyTypeOrAboutBlank_FollowsRFC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		p         Problem
		wantType  any
		wantTitle any
	}{
		{name: "zero value omits type and title", p: Problem{}},
		{name: "empty type with status uses the status phrase", p: Problem{Status: 404, Title: "Other"}, wantTitle: "Not Found"},
		{name: "about:blank with status uses the status phrase", p: Problem{Type: "about:blank", Status: 503, Title: "Other"}, wantType: "about:blank", wantTitle: "Service Unavailable"},
		{name: "about:blank without status omits the title", p: Problem{Type: "about:blank", Title: "Other"}, wantType: "about:blank"},
		{name: "catalogue type keeps the title", p: Problem{Type: ErrorCatalogURL + "#not-found", Status: 404, Title: "Resource not found"}, wantType: ErrorCatalogURL + "#not-found", wantTitle: "Resource not found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(tc.p)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if m["type"] != tc.wantType {
				t.Errorf("type = %v, want %v (%s)", m["type"], tc.wantType, b)
			}
			if m["title"] != tc.wantTitle {
				t.Errorf("title = %v, want %v (%s)", m["title"], tc.wantTitle, b)
			}
		})
	}
}

// TestProblemFrom_WhenTokenFailure_UsesTokenRequestFailed covers the token
// request failures that are neither an error response nor a network error.
func TestProblemFrom_WhenTokenFailure_UsesTokenRequestFailed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		client   func(t *testing.T) *Client
		wantType string
	}{
		{
			name: "token endpoint without access_token",
			client: func(t *testing.T) *Client {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					writeBody(t, w, `{"token_type":"Bearer"}`)
				}))
				t.Cleanup(srv.Close)
				c, _ := newClientWithEnv(t, Environment{Name: "test", APIURL: srv.URL, TokenURL: srv.URL},
					WithCredentials(fakeClientID, fakeSecret), WithAgent(fakeAgent, fakeProfile), WithRateLimits(RateLimits{}), WithRetries(0))
				return c
			},
			wantType: "token-request-failed",
		},
		{
			name: "failing external source",
			client: func(t *testing.T) *Client {
				c, _ := newClientWithEnv(t, Environment{Name: "test", APIURL: "http://127.0.0.1:1", TokenURL: "http://127.0.0.1:1"}, //nolint:gosec // G101: invented test URL, not a credential
					WithTokenSource(failingSource{err: errors.New("source unavailable")}), WithAgent(fakeAgent, fakeProfile), WithRateLimits(RateLimits{}), WithRetries(0))
				return c
			},
			wantType: "token-request-failed",
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
			if p.Retryable || p.Uncertain {
				t.Errorf("no information to assert: Retryable=%v Uncertain=%v", p.Retryable, p.Uncertain)
			}
		})
	}
}
