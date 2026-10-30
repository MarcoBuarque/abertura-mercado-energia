package abertura

import (
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// Option configura o Client em NewClient.
type Option interface{ applyClient(*config) }

// CallOption configura uma chamada, sobrepondo o que o Client definiu.
type CallOption interface{ applyCall(*callConfig) }

// config é o que as Option preenchem; NewClient o valida e o converte no Client.
type config struct {
	clientID, clientSecret string
	temCredenciais         bool
	tokenSource            oauth2.TokenSource

	agente, perfil int
	temAgente      bool

	timeout    time.Duration
	temTimeout bool

	retries    int
	temRetries bool

	httpClient *http.Client
	limites    *RateLimits
	logger     *slog.Logger
	sufixoUA   string
}

// callConfig é o que as CallOption preenchem. Campos zerados não sobrepõem o
// Client.
type callConfig struct {
	agente, perfil int
	temAgente      bool
	timeout        time.Duration
}

// novoCallConfig aplica as opções da chamada em ordem.
func novoCallConfig(opts ...CallOption) callConfig {
	var cc callConfig
	for _, o := range opts {
		if o != nil {
			o.applyCall(&cc)
		}
	}
	return cc
}

type optionFunc func(*config)

func (f optionFunc) applyClient(c *config) { f(c) }

// AgentOption define o agente e o perfil, nos cabeçalhos codigoAgente e
// codigoPerfil. Vale como Option (padrão do Client) e como CallOption (só
// naquela chamada).
type AgentOption struct{ agente, perfil int }

// WithAgent define o código do agente e o do perfil de varejista. No
// NewClient os dois são obrigatórios e positivos.
func WithAgent(agente, perfil int) AgentOption {
	return AgentOption{agente: agente, perfil: perfil}
}

func (o AgentOption) applyClient(c *config) {
	c.agente, c.perfil, c.temAgente = o.agente, o.perfil, true
}

func (o AgentOption) applyCall(c *callConfig) {
	c.agente, c.perfil, c.temAgente = o.agente, o.perfil, true
}

// TimeoutOption define o prazo de cada tentativa. Vale como Option e como
// CallOption.
type TimeoutOption struct{ d time.Duration }

// WithTimeout define o prazo de cada tentativa (não o da chamada inteira, que
// é do ctx). O padrão é 30 s. No NewClient precisa ser positivo; numa chamada,
// zero ou negativo é ignorado.
func WithTimeout(d time.Duration) TimeoutOption { return TimeoutOption{d: d} }

func (o TimeoutOption) applyClient(c *config) { c.timeout, c.temTimeout = o.d, true }

func (o TimeoutOption) applyCall(c *callConfig) { c.timeout = o.d }

// WithCredentials define o client_id e o client_secret do fluxo client
// credentials. Não combina com WithTokenSource. O SDK não lê variáveis de
// ambiente por conta própria.
func WithCredentials(clientID, clientSecret string) Option {
	return optionFunc(func(c *config) {
		c.clientID, c.clientSecret, c.temCredenciais = clientID, clientSecret, true
	})
}

// WithTokenSource usa a fonte de token do chamador no lugar de
// WithCredentials. O cache, a renovação e a regra do 403 do autorizador ficam
// por conta da fonte: o SDK não repete uma chamada recusada.
func WithTokenSource(ts oauth2.TokenSource) Option {
	return optionFunc(func(c *config) { c.tokenSource = ts })
}

// WithHTTPClient troca o cliente HTTP. O Timeout dele fica intocado; o prazo
// de cada tentativa é o de WithTimeout.
//
// O cliente padrão do SDK não segue redirecionamentos, para que os cabeçalhos
// do agente não saiam para outro host e um 307 no pedido de token não reenvie
// o formulário com o client_secret. Um cliente próprio segue a política dele,
// também no pedido de token: para manter essa proteção, defina CheckRedirect
// devolvendo http.ErrUseLastResponse.
func WithHTTPClient(hc *http.Client) Option {
	return optionFunc(func(c *config) { c.httpClient = hc })
}

// WithRetries define quantas vezes repetir uma tentativa que a tabela de
// retries manda repetir (padrão 2). Zero desliga os retries.
func WithRetries(n int) Option {
	return optionFunc(func(c *config) { c.retries, c.temRetries = n, true })
}

// WithRateLimits troca os limites do Environment. RateLimits{} desliga o
// limitador local.
func WithRateLimits(l RateLimits) Option {
	return optionFunc(func(c *config) { c.limites = &l })
}

// WithLogger liga o log estruturado do transporte. Sem ele, nada é escrito.
// O SDK nunca loga query, corpo, token nem credenciais.
func WithLogger(l *slog.Logger) Option {
	return optionFunc(func(c *config) { c.logger = l })
}

// WithUserAgent acrescenta um sufixo ao User-Agent do SDK, por exemplo o nome
// e a versão do seu aplicativo.
func WithUserAgent(suffix string) Option {
	return optionFunc(func(c *config) { c.sufixoUA = suffix })
}
