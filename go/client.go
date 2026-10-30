package abertura

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/MarcoBuarque/abertura-mercado-energia/go/internal/version"
)

// Environment aponta o SDK para um ambiente da CCEE.
type Environment struct {
	// Name aparece só em mensagens.
	Name string
	// APIURL é a base das rotas, sem barra no fim.
	APIURL string
	// TokenURL é o endpoint de token OAuth2.
	TokenURL string
	// RateLimits são os limites do limitador local.
	RateLimits RateLimits
}

// RateLimits são os limites do limitador local, por processo.
type RateLimits struct{ Default, Medicoes RateLimit }

// RateLimit permite Requests requisições a cada Per. O valor zero desliga.
type RateLimit struct {
	Requests int
	Per      time.Duration
}

// Sandbox e Production são os ambientes da documentação pública.
var (
	Sandbox = Environment{
		Name:     "sandbox",
		APIURL:   "https://sandbox-api-abm.ccee.org.br",
		TokenURL: "https://sandbox-api-abm.ccee.org.br/sso/oauth/token",
	}
	Production = Environment{
		Name:     "production",
		APIURL:   "https://api-abm.ccee.org.br",
		TokenURL: "https://api-abm.ccee.org.br/sso/oauth/token",
	}
)

const (
	timeoutPadrao = 30 * time.Second
	retriesPadrao = 2
)

// Client fala com a API de abertura de mercado da CCEE.
type Client struct {
	env      Environment
	apiURL   string
	agente   int
	perfil   int
	timeout  time.Duration
	retries  int
	http     *http.Client
	fonte    fonteToken
	limites  limitadores
	logger   *slog.Logger
	userAgnt string

	// Pontos de injeção dos testes.
	dormir func(context.Context, time.Duration) error
	rnd    func() float64
	agora  func() time.Time
}

// NewClient cria um cliente para o ambiente.
func NewClient(env Environment, opts ...Option) (*Client, error) {
	var cfg config
	for _, o := range opts {
		o.applyClient(&cfg)
	}

	c := &Client{
		env:      env,
		apiURL:   env.APIURL,
		agente:   cfg.agente,
		perfil:   cfg.perfil,
		timeout:  timeoutPadrao,
		retries:  retriesPadrao,
		http:     cfg.httpClient,
		logger:   cfg.logger,
		userAgnt: userAgent(cfg.sufixoUA),
		dormir:   dormirCtx,
		rnd:      rand.Float64,
		agora:    time.Now,
	}
	if cfg.temTimeout {
		c.timeout = cfg.timeout
	}
	if cfg.temRetries {
		c.retries = cfg.retries
	}
	if c.http == nil {
		c.http = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	rl := env.RateLimits
	if cfg.limites != nil {
		rl = *cfg.limites
	}
	c.limites = novosLimitadores(rl)
	if cfg.tokenSource != nil {
		c.fonte = novaFonteExterna(cfg.tokenSource)
	} else {
		c.fonte = novoCacheToken(configToken{
			tokenURL: env.TokenURL, clientID: cfg.clientID, clientSecret: cfg.clientSecret,
			httpClient: clienteDeToken(cfg.httpClient, c.limites),
		})
	}
	return c, nil
}

// userAgent monta o User-Agent do SDK, com o sufixo do usuário se houver.
func userAgent(sufixo string) string {
	ua := "abertura-mercado-energia-go/" + version.Version + " (+https://github.com/MarcoBuarque/abertura-mercado-energia)"
	if sufixo != "" {
		ua += " " + limpar(sufixo, 100)
	}
	return ua
}
