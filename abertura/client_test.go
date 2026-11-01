package abertura

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura/internal/version"
)

// Options that apply to both the client and the call must serve both sides,
// checked by the compiler.
var (
	_ Option     = WithAgent(1, 2)
	_ CallOption = WithAgent(1, 2)
	_ Option     = WithTimeout(time.Second)
	_ CallOption = WithTimeout(time.Second)
)

func TestNewClient_WhenInvalidConfiguration_ReturnsError(t *testing.T) {
	t.Parallel()

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "token-inventado-1"})
	agent := WithAgent(fakeAgent, fakeProfile)
	cred := WithCredentials(fakeClientID, fakeSecret)
	env := Environment{Name: "x", APIURL: "https://api.example.invalid", TokenURL: "https://api.example.invalid/sso/oauth/token"} //nolint:gosec // G101: invented test URL, not a credential

	tests := []struct {
		name    string
		env     Environment
		opts    []Option
		wantErr string // message fragment; empty = no error
	}{
		{"valid credentials and agent", env, []Option{cred, agent}, ""},
		{"valid token source and agent", env, []Option{WithTokenSource(ts), agent}, ""},
		{"no credentials and no token source", env, []Option{agent}, "credentials"},
		{"credentials without client_id", env, []Option{WithCredentials("", fakeSecret), agent}, "credentials"},
		{"credentials without client_secret", env, []Option{WithCredentials(fakeClientID, ""), agent}, "credentials"},
		{"credentials and token source together", env, []Option{cred, WithTokenSource(ts), agent}, "WithTokenSource"},
		{"no agent", env, []Option{cred}, "WithAgent"},
		{"zero agent", env, []Option{cred, WithAgent(0, fakeProfile)}, "WithAgent"},
		{"negative profile", env, []Option{cred, WithAgent(fakeAgent, -1)}, "WithAgent"},
		{"environment without APIURL", Environment{TokenURL: env.TokenURL}, []Option{cred, agent}, "APIURL"},
		{"APIURL without scheme", Environment{APIURL: "api.example.invalid", TokenURL: env.TokenURL}, []Option{cred, agent}, "APIURL"},
		{"APIURL with query", Environment{APIURL: "https://api.example.invalid/?a=b", TokenURL: env.TokenURL}, []Option{cred, agent}, "APIURL"},
		{"credentials without TokenURL", Environment{APIURL: env.APIURL}, []Option{cred, agent}, "TokenURL"},
		{"token source does not need TokenURL", Environment{APIURL: env.APIURL}, []Option{WithTokenSource(ts), agent}, ""},
		{"zero timeout", env, []Option{cred, agent, WithTimeout(0)}, "WithTimeout"},
		{"negative timeout", env, []Option{cred, agent, WithTimeout(-time.Second)}, "WithTimeout"},
		{"negative retries", env, []Option{cred, agent, WithRetries(-1)}, "WithRetries"},
		{"zero retries is valid", env, []Option{cred, agent, WithRetries(0)}, ""},
		{"limit with requests and no window", env, []Option{cred, agent, WithRateLimits(RateLimits{Default: RateLimit{Requests: 10}})}, "limit"},
		{"negative limit", env, []Option{cred, agent, WithRateLimits(RateLimits{Measurements: RateLimit{Requests: -1, Per: time.Minute}})}, "limit"},
		{"disabled limit is valid", env, []Option{cred, agent, WithRateLimits(RateLimits{})}, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, err := NewClient(tc.env, tc.opts...)
			if tc.wantErr == "" {
				if err != nil || c == nil {
					t.Fatalf("NewClient = %v, %v", c, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error containing %q", tc.wantErr)
			}
			if c != nil {
				t.Error("with an error, the client should be nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want fragment %q", err, tc.wantErr)
			}
			if !errors.Is(err, ErrInvalidArgument) {
				t.Errorf("NewClient validation error should match ErrInvalidArgument: %v", err)
			}
			if p := ProblemFrom(err); !strings.HasSuffix(p.Type, "#invalid-argument") {
				t.Errorf("Type = %q, want #invalid-argument", p.Type)
			}
			assertNoLeak(t, err.Error())
		})
	}
}

func TestNewClient_WhenDefaultsAndOptions_AppliesThem(t *testing.T) {
	t.Parallel()

	env := Environment{APIURL: "https://api.example.invalid", TokenURL: "https://api.example.invalid/t"} //nolint:gosec // G101: invented test URL, not a credential
	base := []Option{WithCredentials(fakeClientID, fakeSecret), WithAgent(fakeAgent, fakeProfile)}

	defaults, err := NewClient(env, base...)
	if err != nil {
		t.Fatal(err)
	}
	if defaults.timeout != 30*time.Second || defaults.retries != 2 {
		t.Errorf("defaults: timeout=%v retries=%d, want 30 s and 2", defaults.timeout, defaults.retries)
	}
	if defaults.agent != fakeAgent || defaults.profile != fakeProfile {
		t.Errorf("agent/profile = %d/%d", defaults.agent, defaults.profile)
	}
	if want := "abertura-mercado-energia/" + version.Version + " (+https://github.com/MarcoBuarque/abertura-mercado-energia)"; defaults.userAgentValue != want {
		t.Errorf("User-Agent = %q, want %q", defaults.userAgentValue, want)
	}

	custom, err := NewClient(env, append(base, WithTimeout(5*time.Second), WithRetries(4), WithUserAgent("fake-app/1.0"))...)
	if err != nil {
		t.Fatal(err)
	}
	if custom.timeout != 5*time.Second || custom.retries != 4 {
		t.Errorf("timeout=%v retries=%d", custom.timeout, custom.retries)
	}
	if !strings.HasSuffix(custom.userAgentValue, " fake-app/1.0") {
		t.Errorf("User-Agent = %q", custom.userAgentValue)
	}

	// The latest option wins.
	last, err := NewClient(env, append(base, WithAgent(900011, 900012))...)
	if err != nil {
		t.Fatal(err)
	}
	if last.agent != 900011 || last.profile != 900012 {
		t.Errorf("agent/profile = %d/%d", last.agent, last.profile)
	}
}

func TestClient_WhenCallOptions_OverrideClientSettings(t *testing.T) {
	t.Parallel()

	c, err := NewClient(Environment{APIURL: "https://a.invalid", TokenURL: "https://a.invalid/t"}, //nolint:gosec // G101: invented test URL, not a credential
		WithCredentials(fakeClientID, fakeSecret), WithAgent(fakeAgent, fakeProfile), WithTimeout(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		opts        []CallOption
		wantAgent   int
		wantProfile int
		wantTimeout time.Duration
	}{
		{"no options uses the client", nil, fakeAgent, fakeProfile, 7 * time.Second},
		{"WithAgent replaces agent and profile", []CallOption{WithAgent(900011, 900012)}, 900011, 900012, 7 * time.Second},
		{"WithTimeout replaces the deadline", []CallOption{WithTimeout(time.Second)}, fakeAgent, fakeProfile, time.Second},
		{"zero timeout in a call is ignored", []CallOption{WithTimeout(0)}, fakeAgent, fakeProfile, 7 * time.Second},
		{"nil option is ignored", []CallOption{nil}, fakeAgent, fakeProfile, 7 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := c.params(newCallConfig(tc.opts...))
			if p.agent != tc.wantAgent || p.profile != tc.wantProfile || p.timeout != tc.wantTimeout {
				t.Errorf("params = %+v", p)
			}
		})
	}
}

func TestEnvironments_WhenPublicDocumentation_MatchExpectedValues(t *testing.T) {
	t.Parallel()

	fiveMin := 5 * time.Minute
	tests := []struct {
		name string
		env  Environment
		want Environment
	}{
		{
			name: "Sandbox",
			env:  Sandbox(),
			want: Environment{ //nolint:gosec // G101: public URLs from the documentation, not credentials
				Name:       "sandbox",
				APIURL:     "https://sandbox-api-abm.ccee.org.br",
				TokenURL:   "https://sandbox-api-abm.ccee.org.br/sso/oauth/token",
				RateLimits: RateLimits{Default: RateLimit{Requests: 900, Per: fiveMin}},
			},
		},
		{
			name: "Production",
			env:  Production(),
			want: Environment{ //nolint:gosec // G101: public URLs from the documentation, not credentials
				Name:       "production",
				APIURL:     "https://api-abm.ccee.org.br",
				TokenURL:   "https://api-abm.ccee.org.br/sso/oauth/token",
				RateLimits: RateLimits{Default: RateLimit{Requests: 900, Per: fiveMin}, Measurements: RateLimit{Requests: 9000, Per: fiveMin}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.env != tc.want {
				t.Errorf("%s = %+v, want %+v", tc.name, tc.env, tc.want)
			}
			l := newLimiters(tc.env.RateLimits)
			if l.general == nil || l.general.Burst() != 30 {
				t.Errorf("burst of the general bucket = %v, want 30", l.general)
			}
			c, err := NewClient(tc.env, WithCredentials(fakeClientID, fakeSecret), WithAgent(fakeAgent, fakeProfile))
			if err != nil {
				t.Fatal(err)
			}
			if c.limits.general == nil {
				t.Error("the client should have the environment limiter")
			}
			if (tc.name == "Production") == (c.limits.measurements == c.limits.general) {
				t.Errorf("measurements bucket shared = %v", c.limits.measurements == c.limits.general)
			}
		})
	}
}

func TestNewClient_WhenWithRateLimits_OverridesEnvironment(t *testing.T) {
	t.Parallel()

	c, err := NewClient(Sandbox(), WithCredentials(fakeClientID, fakeSecret), WithAgent(fakeAgent, fakeProfile),
		WithRateLimits(RateLimits{}))
	if err != nil {
		t.Fatal(err)
	}
	if c.limits.general != nil || c.limits.measurements != nil {
		t.Error("RateLimits{} should disable the limiter")
	}
}

func TestClient_WhenFormatted_DoesNotLeakSecret(t *testing.T) {
	t.Parallel()

	c, err := NewClient(Sandbox(), WithCredentials(fakeClientID, fakeSecret), WithAgent(fakeAgent, fakeProfile))
	if err != nil {
		t.Fatal(err)
	}
	// A cached token and a call already made must not change the result.
	forms := map[string]string{
		"%v":                fmt.Sprintf("%v", c),
		"%+v":               fmt.Sprintf("%+v", c),
		"%#v":               fmt.Sprintf("%#v", c),
		"%s":                fmt.Sprint(c),
		"%+v of the value":  fmt.Sprintf("%+v", *c),
		"%#v of the value":  fmt.Sprintf("%#v", *c),
		"%+v of the source": fmt.Sprintf("%+v", c.source),
		"%#v of the source": fmt.Sprintf("%#v", c.source),
	}
	for form, text := range forms {
		for _, forbidden := range []string{fakeSecret, "Bearer"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s contains %q: %s", form, forbidden, text)
			}
		}
	}
	if got := c.String(); !strings.Contains(got, "sandbox") {
		t.Errorf("String = %q, should mention the environment", got)
	}
	if c.GoString() != c.String() {
		t.Errorf("GoString = %q, String = %q", c.GoString(), c.String())
	}
}

func TestClient_WhenRedirected_DoesNotFollow(t *testing.T) {
	t.Parallel()

	want := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the redirect was followed, with the agent headers")
	})
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, want.URL+"/other", http.StatusFound)
	})
	c, _ := newTestClient(t, srv)

	_, _, err := c.send(context.Background(), readOp(), callConfig{})
	if err == nil || !strings.Contains(err.Error(), "HTTP 302") {
		t.Fatalf("error = %v, want APIError 302", err)
	}
	if want.reqs.Load() != 0 {
		t.Error("the redirect target received a request")
	}
}
