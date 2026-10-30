package abertura

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
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
	Sandbox = Environment{ //nolint:gosec // G101: a TokenURL é pública, da documentação da CCEE; não há credencial aqui
		Name:     "sandbox",
		APIURL:   "https://sandbox-api-abm.ccee.org.br",
		TokenURL: "https://sandbox-api-abm.ccee.org.br/sso/oauth/token",
		// 1.000 requisições por 5 minutos por IP; 900 deixam folga. As medições
		// usam o balde geral.
		RateLimits: RateLimits{Default: RateLimit{Requests: 900, Per: 5 * time.Minute}},
	}
	Production = Environment{ //nolint:gosec // G101: a TokenURL é pública, da documentação da CCEE; não há credencial aqui
		Name:     "production",
		APIURL:   "https://api-abm.ccee.org.br",
		TokenURL: "https://api-abm.ccee.org.br/sso/oauth/token",
		// 1.000 por IP no geral; 10.000 por agente nas medições.
		RateLimits: RateLimits{
			Default:  RateLimit{Requests: 900, Per: 5 * time.Minute},
			Medicoes: RateLimit{Requests: 9000, Per: 5 * time.Minute},
		},
	}
)

const (
	timeoutPadrao = 30 * time.Second
	retriesPadrao = 2
)

// Client fala com a API de abertura de mercado da CCEE.
type Client struct {
	// Migracoes e Medicoes são as operações públicas do SDK. Para uma rota que
	// o SDK ainda não cobre, use [Client.Do].
	Migracoes *Migracoes
	Medicoes  *Medicoes

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

// NewClient cria um cliente para o ambiente. O ambiente é o primeiro
// argumento, sem padrão: nada aponta para produção sem pedido explícito.
//
// Obrigatórios: WithCredentials (ou WithTokenSource) e WithAgent. O SDK não lê
// variáveis de ambiente por conta própria.
//
// O limitador local vale por processo: vários processos no mesmo IP ou agente
// somam requisições na CCEE sem que ele saiba. Toda resposta da API fecha a
// conexão, então cada requisição abre TLS e a latência sobe; os prazos ficam
// por conta de WithTimeout e do ctx de cada chamada.
func NewClient(env Environment, opts ...Option) (*Client, error) {
	var cfg config
	for _, o := range opts {
		if o != nil {
			o.applyClient(&cfg)
		}
	}
	if err := cfg.validar(env); err != nil {
		return nil, err
	}

	c := &Client{
		env:      env,
		apiURL:   strings.TrimRight(env.APIURL, "/"),
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
	c.Migracoes, c.Medicoes = &Migracoes{c: c}, &Medicoes{c: c}
	if cfg.temTimeout {
		c.timeout = cfg.timeout
	}
	if cfg.temRetries {
		c.retries = cfg.retries
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
	if c.http == nil {
		// Sem redirecionamento: os cabeçalhos do agente não saem para outro host.
		c.http = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return c, nil
}

// validar confere a configuração. As mensagens nunca citam o segredo.
func (cfg config) validar(env Environment) error {
	if err := validarURL("APIURL", env.APIURL); err != nil {
		return err
	}
	switch {
	case cfg.tokenSource != nil && cfg.temCredenciais:
		return errors.New("abertura: use WithCredentials ou WithTokenSource, não os dois")
	case cfg.tokenSource == nil && (!cfg.temCredenciais || cfg.clientID == "" || cfg.clientSecret == ""):
		return errors.New("abertura: faltam as credenciais: informe WithCredentials (client_id e client_secret) ou WithTokenSource")
	case cfg.tokenSource == nil:
		if err := validarURL("TokenURL", env.TokenURL); err != nil {
			return err
		}
	}
	if !cfg.temAgente || cfg.agente <= 0 || cfg.perfil <= 0 {
		return errors.New("abertura: falta o agente: informe WithAgent com o código do agente e o do perfil, ambos positivos")
	}
	if cfg.temTimeout && cfg.timeout <= 0 {
		return errors.New("abertura: WithTimeout precisa de uma duração positiva")
	}
	if cfg.temRetries && cfg.retries < 0 {
		return errors.New("abertura: WithRetries não aceita número negativo")
	}
	rl := env.RateLimits
	if cfg.limites != nil {
		rl = *cfg.limites
	}
	for _, g := range []struct {
		nome string
		l    RateLimit
	}{{"Default", rl.Default}, {"Medicoes", rl.Medicoes}} {
		if (g.l.Requests != 0 || g.l.Per != 0) && (g.l.Requests <= 0 || g.l.Per <= 0) {
			return fmt.Errorf("abertura: limite %s inválido: Requests e Per precisam ser ambos positivos, ou ambos zero para desligar", g.nome)
		}
	}
	return nil
}

// validarURL exige uma URL http(s) absoluta, sem query nem fragmento. A
// mensagem não repete a URL.
func validarURL(campo, valor string) error {
	u, err := url.Parse(valor)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("abertura: %s do ambiente precisa ser uma URL http(s) absoluta, sem query", campo)
	}
	return nil
}

// String descreve o cliente sem credenciais: o %+v do fmt imprimiria os campos
// não exportados, e o segredo mora neles.
func (c *Client) String() string {
	return fmt.Sprintf("abertura.Client{ambiente: %q, agente: %d, perfil: %d}", c.env.Name, c.agente, c.perfil)
}

// GoString faz o %#v seguir o String.
func (c *Client) GoString() string { return c.String() }

// userAgent monta o User-Agent do SDK, com o sufixo do usuário se houver.
func userAgent(sufixo string) string {
	ua := "abertura-mercado-energia-go/" + version.Version + " (+https://github.com/MarcoBuarque/abertura-mercado-energia)"
	if sufixo != "" {
		ua += " " + limpar(sufixo, 100)
	}
	return ua
}
