package abertura

import (
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// Option configures the Client in NewClient.
type Option interface{ applyClient(*config) }

// CallOption configures a single call, overriding what the Client set.
type CallOption interface{ applyCall(*callConfig) }

// config is what the Options fill in; NewClient validates it and turns it into the Client.
type config struct {
	clientID, clientSecret string
	hasCredentials         bool
	tokenSource            oauth2.TokenSource

	agent, profile int
	hasAgent       bool

	timeout    time.Duration
	hasTimeout bool

	retries    int
	hasRetries bool

	httpClient *http.Client
	limits     *RateLimits
	logger     *slog.Logger
	uaSuffix   string
}

// callConfig is what the CallOptions fill in. Zeroed fields do not override the
// Client.
type callConfig struct {
	agent, profile int
	hasAgent       bool
	timeout        time.Duration
}

// newCallConfig applies the call options in order.
func newCallConfig(opts ...CallOption) callConfig {
	var cc callConfig
	for _, o := range opts {
		if o != nil {
			o.applyCall(&cc)
		}
	}
	return cc
}

// optionFunc adapts a function to the Option interface.
type optionFunc func(*config)

func (f optionFunc) applyClient(c *config) { f(c) }

// AgentOption sets the agent and the profile, in the codigoAgente and
// codigoPerfil headers. It works as an Option (the Client default) and as a
// CallOption (that call only).
type AgentOption struct{ agent, profile int }

// WithAgent sets the agent code and the retailer profile code. In NewClient
// both are required and must be positive.
func WithAgent(agent, profile int) AgentOption {
	return AgentOption{agent: agent, profile: profile}
}

func (o AgentOption) applyClient(c *config) {
	c.agent, c.profile, c.hasAgent = o.agent, o.profile, true
}

func (o AgentOption) applyCall(c *callConfig) {
	c.agent, c.profile, c.hasAgent = o.agent, o.profile, true
}

// TimeoutOption sets the deadline of each attempt. It works as an Option and
// as a CallOption.
type TimeoutOption struct{ d time.Duration }

// WithTimeout sets the deadline of each attempt (not of the whole call, which
// belongs to the ctx). The default is 30 s. In NewClient it must be positive;
// in a call, zero or negative is ignored.
func WithTimeout(d time.Duration) TimeoutOption { return TimeoutOption{d: d} }

func (o TimeoutOption) applyClient(c *config) { c.timeout, c.hasTimeout = o.d, true }

func (o TimeoutOption) applyCall(c *callConfig) { c.timeout = o.d }

// WithCredentials sets the client_id and client_secret of the client
// credentials flow. It cannot be combined with WithTokenSource. The SDK does
// not read environment variables on its own.
func WithCredentials(clientID, clientSecret string) Option {
	return optionFunc(func(c *config) {
		c.clientID, c.clientSecret, c.hasCredentials = clientID, clientSecret, true
	})
}

// WithTokenSource uses the caller's token source in place of
// WithCredentials. Caching, renewal and the authorizer 403 rule are left to
// the source: the SDK does not repeat a refused call.
func WithTokenSource(ts oauth2.TokenSource) Option {
	return optionFunc(func(c *config) { c.tokenSource = ts })
}

// WithHTTPClient replaces the HTTP client. Its Timeout is left untouched; the
// deadline of each attempt is the one from WithTimeout.
//
// The default SDK client does not follow redirects, so the agent headers do
// not leave for another host and a 307 on the token request does not resend
// the form with the client_secret. A custom client follows its own policy
// (CheckRedirect) on API calls. The token request uses a copy of it that never
// follows redirects, so the client_secret does not leave for another host;
// the copy's Timeout and Transport are those of your client.
func WithHTTPClient(hc *http.Client) Option {
	return optionFunc(func(c *config) { c.httpClient = hc })
}

// WithRetries sets how many times to retry an attempt that the retry table
// says to retry (default 2). Zero disables retries.
func WithRetries(n int) Option {
	return optionFunc(func(c *config) { c.retries, c.hasRetries = n, true })
}

// WithRateLimits replaces the Environment limits. RateLimits{} disables the
// local rate limiter.
func WithRateLimits(l RateLimits) Option {
	return optionFunc(func(c *config) { c.limits = &l })
}

// WithLogger turns on structured logging of the transport. Without it,
// nothing is written. The SDK never logs the query, body, token or
// credentials.
func WithLogger(l *slog.Logger) Option {
	return optionFunc(func(c *config) { c.logger = l })
}

// WithUserAgent appends a suffix to the SDK User-Agent, for example the name
// and version of your application.
func WithUserAgent(suffix string) Option {
	return optionFunc(func(c *config) { c.uaSuffix = suffix })
}
