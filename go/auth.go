package abertura

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const (
	// folgaToken é a antecedência com que o token é renovado antes de vencer.
	folgaToken = 30 * time.Second
	// validadePadraoToken vale quando a resposta não traz expires_in; é o
	// valor observado no sandbox.
	validadePadraoToken = 300 * time.Second
	// timeoutBuscaToken limita cada pedido de token.
	timeoutBuscaToken = 10 * time.Second
	// operacaoToken é o operationId do pedido de token na spec.
	operacaoToken = "obterToken"
)

// fonteToken é a origem do token de acesso que o laço de envio consome. As
// duas implementações são cacheToken (credenciais do cliente) e fonteExterna
// (oauth2.TokenSource do usuário).
//
// O laço de envio usa marcarAceito e foiAceito para a regra do 403 do
// autorizador: só repete com token novo se o rejeitado já tinha sido aceito.
type fonteToken interface {
	// token devolve um token válido, buscando um novo se for preciso. Os
	// valores devolvidos são comparados por identidade em invalidar, marcarAceito
	// e foiAceito.
	token(ctx context.Context) (*oauth2.Token, error)
	// invalidar descarta o token se ainda for o atual; um token antigo não
	// derruba o que já o substituiu.
	invalidar(t *oauth2.Token)
	// marcarAceito registra que o token foi aceito por uma resposta sem a
	// marca do autorizador (x-amzn-ErrorType).
	marcarAceito(t *oauth2.Token)
	// foiAceito diz se o token já foi marcado como aceito.
	foiAceito(t *oauth2.Token) bool
	// renovavel diz se o SDK sabe obter outro token; é falso com
	// WithTokenSource.
	renovavel() bool
}

// configToken reúne o que o cacheToken precisa para pedir tokens.
type configToken struct {
	tokenURL     string
	clientID     string
	clientSecret string
	httpClient   *http.Client // nil usa o cliente padrão
}

// cacheToken guarda o token de client credentials, renova-o antes do vencimento
// (pelo relógio local) e permite descartá-lo por token. Uma só busca acontece de
// cada vez; chamadas concorrentes esperam por ela.
type cacheToken struct {
	cfg        clientcredentials.Config
	httpClient *http.Client

	// agora e timeoutBusca são campos para que os testes injetem o relógio e
	// um prazo curto.
	agora        func() time.Time
	timeoutBusca time.Duration

	busca chan struct{} // semáforo de 1: quem o segura está buscando

	mu      sync.Mutex // protege os campos abaixo
	atual   *oauth2.Token
	venceEm time.Time // até quando atual vale, já descontada a folga
	aceitos map[*oauth2.Token]bool
}

// novoCacheToken cria o cache. As credenciais ficam só aqui, em campos não
// exportados, e String e GoString as escondem.
func novoCacheToken(cfg configToken) *cacheToken {
	return &cacheToken{
		cfg: clientcredentials.Config{
			ClientID:     cfg.clientID,
			ClientSecret: cfg.clientSecret,
			TokenURL:     cfg.tokenURL,
			// Credenciais no formulário, como na coleção da API.
			AuthStyle: oauth2.AuthStyleInParams,
		},
		httpClient:   cfg.httpClient,
		agora:        time.Now,
		timeoutBusca: timeoutBuscaToken,
		busca:        make(chan struct{}, 1),
		aceitos:      make(map[*oauth2.Token]bool),
	}
}

// String evita que %v e %+v imprimam o segredo guardado nos campos.
func (c *cacheToken) String() string { return "abertura.cacheToken{credenciais omitidas}" }

// GoString evita que %#v imprima o segredo.
func (c *cacheToken) GoString() string { return c.String() }

func (c *cacheToken) renovavel() bool { return true }

// valido devolve o token atual se ainda vale. Exige c.mu.
func (c *cacheToken) valido() *oauth2.Token {
	if c.atual != nil && !c.agora().After(c.venceEm) {
		return c.atual
	}
	return nil
}

func (c *cacheToken) token(ctx context.Context) (*oauth2.Token, error) {
	c.mu.Lock()
	t := c.valido()
	c.mu.Unlock()
	if t != nil {
		return t, nil
	}

	// Só uma busca por vez; quem espera respeita o próprio ctx.
	select {
	case c.busca <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-c.busca }()

	// Quem buscava pode ter terminado enquanto esperávamos.
	c.mu.Lock()
	t = c.valido()
	c.mu.Unlock()
	if t != nil {
		return t, nil
	}

	recebidoEm := c.agora()
	novo, err := c.buscar(ctx)
	if err != nil {
		return nil, err
	}

	validade := time.Duration(novo.ExpiresIn) * time.Second
	if validade <= 0 {
		validade = validadePadraoToken
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// Guarda a marca só do token que sai; os demais já não circulam por muito
	// tempo e o mapa não cresce.
	aceitos := make(map[*oauth2.Token]bool, 1)
	if c.atual != nil && c.aceitos[c.atual] {
		aceitos[c.atual] = true
	}
	c.aceitos = aceitos
	c.atual = novo
	c.venceEm = recebidoEm.Add(validade - folgaToken)
	return novo, nil
}

// buscar pede um token novo ao endpoint, com o timeout da busca. Falhas viram
// *APIError (resposta de erro do endpoint) ou um erro de transporte sem URL.
func (c *cacheToken) buscar(ctx context.Context) (*oauth2.Token, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeoutBusca)
	defer cancel()
	if c.httpClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, c.httpClient)
	}

	t, err := c.cfg.Token(ctx)
	if err == nil {
		return t, nil
	}

	// O RetrieveError do oauth2 traz o corpo na mensagem; usamos só status,
	// cabeçalhos e os campos do corpo.
	var re *oauth2.RetrieveError
	if errors.As(err, &re) && re.Response != nil {
		apiErr := decodificarErro(operacaoToken, re.Response, re.Body)
		// Pedir token não tem efeito colateral: repetir é seguro como numa leitura.
		apiErr.metodo = http.MethodGet
		return nil, apiErr
	}
	return nil, fmt.Errorf("%s: %w", operacaoToken, erroSemURL(err))
}

func (c *cacheToken) invalidar(t *oauth2.Token) {
	if t == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.atual == t {
		// Mantém o ponteiro, para lembrar se foi aceito, mas o torna vencido.
		c.venceEm = time.Time{}
	}
}

func (c *cacheToken) marcarAceito(t *oauth2.Token) {
	if t == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Só tokens que este cache emitiu e ainda lembra.
	if t == c.atual || c.aceitos[t] {
		c.aceitos[t] = true
	}
}

func (c *cacheToken) foiAceito(t *oauth2.Token) bool {
	if t == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.aceitos[t]
}

// fonteExterna adapta o oauth2.TokenSource do usuário (WithTokenSource). O
// usuário cuida de cache e renovação, então nada aqui é renovável e a regra do
// 403 não se aplica.
type fonteExterna struct {
	ts oauth2.TokenSource
}

func novaFonteExterna(ts oauth2.TokenSource) *fonteExterna {
	return &fonteExterna{ts: ts}
}

func (f *fonteExterna) token(ctx context.Context) (*oauth2.Token, error) {
	// O TokenSource não recebe ctx; ao menos não começamos com ele vencido.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t, err := f.ts.Token()
	if err != nil {
		return nil, fmt.Errorf("obter token da fonte externa: %w", erroSemURL(err))
	}
	return t, nil
}

func (f *fonteExterna) invalidar(*oauth2.Token)      {}
func (f *fonteExterna) marcarAceito(*oauth2.Token)   {}
func (f *fonteExterna) foiAceito(*oauth2.Token) bool { return false }
func (f *fonteExterna) renovavel() bool              { return false }
