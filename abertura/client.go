package abertura

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura/internal/version"
)

// Environment points the SDK at an API environment.
type Environment struct {
	// Name appears only in messages.
	Name string
	// APIURL is the base of the routes, without a trailing slash.
	APIURL string
	// TokenURL is the OAuth2 token endpoint.
	TokenURL string
	// RateLimits are the limits of the local rate limiter.
	RateLimits RateLimits
}

// RateLimits are the limits of the local rate limiter, per process.
type RateLimits struct {
	// Default applies to every call. The zero value disables the limiter.
	Default RateLimit
	// Measurements applies to the measurement listing. The zero value makes
	// measurements share the Default bucket (as in the sandbox); set it to
	// give them a bucket of their own (as in production).
	Measurements RateLimit
}

// RateLimit allows Requests requests every Per. The zero value disables it.
type RateLimit struct {
	// Requests is the number of requests allowed in each Per window.
	Requests int
	// Per is the length of the window.
	Per time.Duration
}

// Sandbox returns the sandbox environment described in the public
// documentation. Each call returns a fresh copy, so callers may modify it.
func Sandbox() Environment {
	return Environment{ //nolint:gosec // G101: the TokenURL is public, taken from the documentation; there is no credential here
		Name:     "sandbox",
		APIURL:   "https://sandbox-api-abm.ccee.org.br",
		TokenURL: "https://sandbox-api-abm.ccee.org.br/sso/oauth/token",
		// 1,000 requests per 5 minutes per IP; 900 leaves some headroom.
		// Measurements use the general bucket.
		RateLimits: RateLimits{Default: RateLimit{Requests: 900, Per: 5 * time.Minute}},
	}
}

// Production returns the production environment described in the public
// documentation. Each call returns a fresh copy, so callers may modify it.
func Production() Environment {
	return Environment{ //nolint:gosec // G101: the TokenURL is public, taken from the documentation; there is no credential here
		Name:     "production",
		APIURL:   "https://api-abm.ccee.org.br",
		TokenURL: "https://api-abm.ccee.org.br/sso/oauth/token",
		// 1,000 per IP in general; 10,000 per agent for measurements.
		RateLimits: RateLimits{
			Default:      RateLimit{Requests: 900, Per: 5 * time.Minute},
			Measurements: RateLimit{Requests: 9000, Per: 5 * time.Minute},
		},
	}
}

const (
	defaultTimeout = 30 * time.Second
	defaultRetries = 2
)

// Client talks to the market opening API.
type Client struct {
	// Migrations and Measurements are the public operations of the SDK. For a
	// route the SDK does not cover yet, use [Client.Do].
	Migrations   *Migrations
	Measurements *Measurements

	env            Environment
	apiURL         string
	agent          int
	profile        int
	timeout        time.Duration
	retries        int
	http           *http.Client
	source         tokenProvider
	limits         limiters
	logger         *slog.Logger
	userAgentValue string

	// Injection points for tests.
	sleep func(context.Context, time.Duration) error
	rnd   func() float64
	now   func() time.Time
}

// NewClient creates a client for the environment. The environment is the first
// argument, with no default: nothing points at production without an explicit
// request.
//
// Required: WithCredentials (or WithTokenSource) and WithAgent. The SDK does
// not read environment variables on its own.
//
// The local rate limiter applies per process: several processes on the same IP
// or agent add up requests to the API without it knowing. Every API response
// closes the connection, so each request opens TLS and latency goes up;
// deadlines are up to WithTimeout and the ctx of each call.
func NewClient(env Environment, opts ...Option) (*Client, error) {
	var cfg config
	for _, o := range opts {
		if o != nil {
			o.applyClient(&cfg)
		}
	}
	if err := cfg.validate(env); err != nil {
		return nil, err
	}

	c := &Client{
		env:            env,
		apiURL:         strings.TrimRight(env.APIURL, "/"),
		agent:          cfg.agent,
		profile:        cfg.profile,
		timeout:        defaultTimeout,
		retries:        defaultRetries,
		http:           cfg.httpClient,
		logger:         cfg.logger,
		userAgentValue: userAgent(cfg.uaSuffix),
		sleep:          sleepCtx,
		rnd:            rand.Float64,
		now:            time.Now,
	}
	c.Migrations, c.Measurements = &Migrations{c: c}, &Measurements{c: c}
	if cfg.hasTimeout {
		c.timeout = cfg.timeout
	}
	if cfg.hasRetries {
		c.retries = cfg.retries
	}
	rl := env.RateLimits
	if cfg.limits != nil {
		rl = *cfg.limits
	}
	c.limits = newLimiters(rl)
	if cfg.tokenSource != nil {
		c.source = newExternalSource(cfg.tokenSource)
	} else {
		c.source = newTokenCache(configToken{
			tokenURL: env.TokenURL, clientID: cfg.clientID, clientSecret: cfg.clientSecret,
			httpClient: tokenHTTPClient(cfg.httpClient, c.limits), userAgent: c.userAgentValue,
		})
	}
	if c.http == nil {
		// No redirects: the agent headers do not leave for another host.
		c.http = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return c, nil
}

// validate checks the configuration. The messages never mention the secret.
func (cfg config) validate(env Environment) error {
	if err := validateURL("APIURL", env.APIURL); err != nil {
		return err
	}
	switch {
	case cfg.tokenSource != nil && cfg.hasCredentials:
		return invalidArgument("abertura: use WithCredentials or WithTokenSource, not both")
	case cfg.tokenSource == nil && (!cfg.hasCredentials || cfg.clientID == "" || cfg.clientSecret == ""):
		return invalidArgument("abertura: missing credentials: provide WithCredentials (client_id and client_secret) or WithTokenSource")
	case cfg.tokenSource == nil:
		if err := validateURL("TokenURL", env.TokenURL); err != nil {
			return err
		}
	}
	if !cfg.hasAgent || cfg.agent <= 0 || cfg.profile <= 0 {
		return invalidArgument("abertura: missing agent: provide WithAgent with the agent code and the profile code, both positive")
	}
	if cfg.hasTimeout && cfg.timeout <= 0 {
		return invalidArgument("abertura: WithTimeout needs a positive duration")
	}
	if cfg.hasRetries && cfg.retries < 0 {
		return invalidArgument("abertura: WithRetries does not accept a negative number")
	}
	rl := env.RateLimits
	if cfg.limits != nil {
		rl = *cfg.limits
	}
	for _, g := range []struct {
		name string
		l    RateLimit
	}{{"Default", rl.Default}, {"Measurements", rl.Measurements}} {
		if (g.l.Requests != 0 || g.l.Per != 0) && (g.l.Requests <= 0 || g.l.Per <= 0) {
			return invalidArgument("abertura: invalid limit %s: Requests and Per must both be positive, or both zero to disable", g.name)
		}
	}
	return nil
}

// validateURL requires an absolute http(s) URL with no query or fragment. The
// message does not repeat the URL.
func validateURL(field, value string) error {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return invalidArgument("abertura: the environment %s must be an absolute http(s) URL without a query", field)
	}
	return nil
}

// String describes the client without credentials: fmt's %+v would print the
// unexported fields, and the secret lives in them.
func (c *Client) String() string {
	return fmt.Sprintf("abertura.Client{environment: %q, agent: %d, profile: %d}", c.env.Name, c.agent, c.profile)
}

// GoString makes %#v follow String.
func (c *Client) GoString() string { return c.String() }

// userAgent builds the SDK User-Agent, with the user suffix if there is one.
func userAgent(suffix string) string {
	ua := "abertura-mercado-energia/" + version.Version + " (+https://github.com/MarcoBuarque/abertura-mercado-energia)"
	if suffix != "" {
		ua += " " + sanitize(suffix, 100)
	}
	return ua
}
