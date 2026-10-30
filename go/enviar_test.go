package abertura

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

const (
	ucInventada     = "000000000000001"
	agenteInventado = 900001
	perfilInventado = 900002
	marcaGateway    = "AccessDeniedException"
)

// registro guarda as esperas que o laço pediu, sem dormir de verdade.
type registro struct {
	mu      sync.Mutex
	esperas []time.Duration
}

func (r *registro) dormir(ctx context.Context, d time.Duration) error {
	r.mu.Lock()
	r.esperas = append(r.esperas, d)
	r.mu.Unlock()
	return ctx.Err()
}

func (r *registro) lista() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Duration(nil), r.esperas...)
}

// contador conta as requisições que passam pelo transporte do cliente HTTP.
type contador struct {
	n    atomic.Int32
	base http.RoundTripper
}

func (c *contador) RoundTrip(r *http.Request) (*http.Response, error) {
	c.n.Add(1)
	return c.base.RoundTrip(r)
}

func novoContador() (*http.Client, *contador) {
	c := &contador{base: http.DefaultTransport}
	return &http.Client{Transport: c}, c
}

// servidorTeste serve o endpoint de token e delega o resto ao handler da API.
type servidorTeste struct {
	*httptest.Server
	tokens atomic.Int32 // tokens emitidos
	reqs   atomic.Int32 // requisições à API (fora o token)
}

func novoServidor(t *testing.T, api http.HandlerFunc) *servidorTeste {
	t.Helper()

	s := &servidorTeste{}
	mux := http.NewServeMux()
	mux.HandleFunc("/sso/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		n := s.tokens.Add(1)
		w.Header().Set("Content-Type", "application/json")
		escrever(t, w, fmt.Sprintf(`{"access_token":"token-inventado-%d","token_type":"Bearer","expires_in":300}`, n))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.reqs.Add(1)
		api(w, r)
	})
	// Como a API real (spike §i): toda resposta fecha a conexão. Sem isso o
	// net/http repetiria um GET sozinho numa conexão reaproveitada e a contagem
	// de tentativas no servidor não bateria com a do SDK.
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *servidorTeste) env() Environment {
	return Environment{
		Name:     "teste",
		APIURL:   s.URL,
		TokenURL: s.URL + "/sso/oauth/token",
	}
}

// novoClienteTeste monta um cliente contra o servidor, com credenciais e
// agente inventados, sem dormir nas esperas e sem limitador.
func novoClienteTeste(t *testing.T, s *servidorTeste, opts ...Option) (*Client, *registro) {
	t.Helper()

	todas := append([]Option{
		WithCredentials(idInventado, segredoInventado),
		WithAgent(agenteInventado, perfilInventado),
		WithRateLimits(RateLimits{}),
	}, opts...)
	return novoClienteComEnv(t, s.env(), todas...)
}

func novoClienteComEnv(t *testing.T, env Environment, opts ...Option) (*Client, *registro) {
	t.Helper()

	c, err := NewClient(env, opts...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	reg := &registro{}
	c.dormir = reg.dormir
	c.rnd = func() float64 { return 1 }
	return c, reg
}

func fonteFixa(token string) Option {
	return WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}))
}

func opLeitura() operacao {
	return operacao{
		id:      "listarMigracoes",
		metodo:  http.MethodGet,
		modelo:  "/v1.1/varejista/migracoes",
		caminho: "/v1.1/varejista/migracoes",
		query:   url.Values{"codigoUnidadeConsumidora": {ucInventada}},
		grupo:   grupoGeral,
	}
}

var chaveTeste = ChaveMigracao{UC: ucInventada, Concessionaria: 900003}

func opEscrita(metodo string) operacao {
	return operacao{
		id:      "criarMigracao",
		metodo:  metodo,
		modelo:  "/v1.1/varejista/migracoes",
		caminho: "/v1.1/varejista/migracoes",
		corpo:   map[string]string{"codigoUnidadeConsumidora": ucInventada},
		grupo:   grupoGeral,
		chave:   chaveTeste,
	}
}

// resposta descreve uma resposta de um servidor falso.
type resposta struct {
	status  int
	header  map[string]string
	corpo   string
	derruba bool // lê a requisição e derruba a conexão sem responder
}

func gateway(h map[string]string) map[string]string {
	out := map[string]string{"x-amzn-ErrorType": marcaGateway}
	for k, v := range h {
		out[k] = v
	}
	return out
}

const corpoOK = `{"ok":true}`

func erroJSON(codigo string) string {
	return fmt.Sprintf(`{"timestamp":"2026-10-30T11:56:47.181-03:00","status":0,"error":%q,"message":"mensagem inventada"}`, codigo)
}

// sequencia responde na ordem; esgotada, repete a última.
func sequencia(t *testing.T, rs ...resposta) http.HandlerFunc {
	t.Helper()

	var n atomic.Int32
	return func(w http.ResponseWriter, r *http.Request) {
		i := min(int(n.Add(1))-1, len(rs)-1)
		derrubarOuResponder(t, w, r, rs[i])
	}
}

func derrubarOuResponder(t *testing.T, w http.ResponseWriter, r *http.Request, rs resposta) {
	t.Helper()

	if rs.derruba {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("o servidor não suporta Hijack")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		if err := conn.Close(); err != nil {
			t.Errorf("fechando a conexão: %v", err)
		}
		return
	}
	for k, v := range rs.header {
		w.Header().Set(k, v)
	}
	w.WriteHeader(rs.status)
	escrever(t, w, rs.corpo)
}

func TestEnviar_Sequencias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		op             operacao
		respostas      []resposta
		wantTentativas int32
		wantStatus     int  // sucesso esperado
		wantAPIStatus  int  // *APIError esperado
		wantIncerto    bool // *UncertainResultError esperado
		wantIs         error
		wantEsperas    []time.Duration
	}{
		{
			name:           "GET 502, 502, 200 dá sucesso na terceira",
			op:             opLeitura(),
			respostas:      []resposta{{status: 502}, {status: 502}, {status: 200, corpo: corpoOK}},
			wantTentativas: 3,
			wantStatus:     200,
			wantEsperas:    []time.Duration{500 * time.Millisecond, time.Second},
		},
		{
			name:           "GET 502 três vezes devolve APIError 502",
			op:             opLeitura(),
			respostas:      []resposta{{status: 502, corpo: erroJSON("ERR_X")}},
			wantTentativas: 3,
			wantAPIStatus:  502,
		},
		{
			name:           "GET 500 e depois 200",
			op:             opLeitura(),
			respostas:      []resposta{{status: 500}, {status: 200, corpo: corpoOK}},
			wantTentativas: 2,
			wantStatus:     200,
		},
		{
			name:           "GET 400 não repete e casa com a sentinela do código",
			op:             opLeitura(),
			respostas:      []resposta{{status: 400, corpo: erroJSON("ERR_VALIDACAO_CAMPOS")}},
			wantTentativas: 1,
			wantAPIStatus:  400,
			wantIs:         ErrValidacaoCampos,
		},
		{
			name:           "GET 404 não repete",
			op:             opLeitura(),
			respostas:      []resposta{{status: 404}},
			wantTentativas: 1,
			wantAPIStatus:  404,
			wantIs:         ErrNotFound,
		},
		{
			name:           "POST 200 dá sucesso",
			op:             opEscrita(http.MethodPost),
			respostas:      []resposta{{status: 200, corpo: corpoOK}},
			wantTentativas: 1,
			wantStatus:     200,
		},
		{
			name:           "POST 502 vira incerto com a chave da operação",
			op:             opEscrita(http.MethodPost),
			respostas:      []resposta{{status: 502}, {status: 200, corpo: corpoOK}},
			wantTentativas: 1,
			wantIncerto:    true,
		},
		{
			name:           "PUT 504 vira incerto",
			op:             opEscrita(http.MethodPut),
			respostas:      []resposta{{status: 504}},
			wantTentativas: 1,
			wantIncerto:    true,
		},
		{
			name:           "POST 400 de duplicata é erro, não incerto",
			op:             opEscrita(http.MethodPost),
			respostas:      []resposta{{status: 400, corpo: erroJSON("ERR_MIGRACAO_SOLICITADA")}},
			wantTentativas: 1,
			wantAPIStatus:  400,
			wantIs:         ErrMigracaoSolicitada,
		},
		{
			name:           "POST 429 sem a marca do autorizador é ErrRateLimited",
			op:             opEscrita(http.MethodPost),
			respostas:      []resposta{{status: 429}, {status: 200, corpo: corpoOK}},
			wantTentativas: 1,
			wantAPIStatus:  429,
			wantIs:         ErrRateLimited,
		},
		{
			name:           "POST 429 com a marca do autorizador repete",
			op:             opEscrita(http.MethodPost),
			respostas:      []resposta{{status: 429, header: gateway(nil)}, {status: 200, corpo: corpoOK}},
			wantTentativas: 2,
			wantStatus:     200,
			wantEsperas:    []time.Duration{2 * time.Second},
		},
		{
			name:           "GET 429 repete com a espera de 429",
			op:             opLeitura(),
			respostas:      []resposta{{status: 429}, {status: 200, corpo: corpoOK}},
			wantTentativas: 2,
			wantStatus:     200,
			wantEsperas:    []time.Duration{2 * time.Second},
		},
		{
			name:           "GET 429 com Retry-After usa o cabeçalho",
			op:             opLeitura(),
			respostas:      []resposta{{status: 429, header: map[string]string{"Retry-After": "7"}}, {status: 200, corpo: corpoOK}},
			wantTentativas: 2,
			wantStatus:     200,
			wantEsperas:    []time.Duration{7 * time.Second},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := novoServidor(t, sequencia(t, tc.respostas...))
			c, reg := novoClienteTeste(t, srv)

			status, corpo, err := c.enviar(context.Background(), tc.op, callConfig{})

			if got := srv.reqs.Load(); got != tc.wantTentativas {
				t.Errorf("tentativas = %d, quer %d", got, tc.wantTentativas)
			}
			switch {
			case tc.wantStatus != 0:
				if err != nil {
					t.Fatalf("erro inesperado: %v", err)
				}
				if status != tc.wantStatus || string(corpo) != corpoOK {
					t.Errorf("status=%d corpo=%q", status, corpo)
				}
			case tc.wantIncerto:
				var incerto *UncertainResultError
				if !errors.As(err, &incerto) {
					t.Fatalf("erro = %v (%T), quer *UncertainResultError", err, err)
				}
				if incerto.Operation != tc.op.id {
					t.Errorf("Operation = %q", incerto.Operation)
				}
				if incerto.Key != ChaveNatural(chaveTeste) {
					t.Errorf("Key = %v, quer a chave da operação", incerto.Key)
				}
				var api *APIError
				if !errors.As(err, &api) || api.StatusCode < 500 {
					t.Errorf("a causa devia ser um APIError 5xx, veio %v", incerto.Err)
				}
			default:
				var api *APIError
				if !errors.As(err, &api) {
					t.Fatalf("erro = %v (%T), quer *APIError", err, err)
				}
				var incerto *UncertainResultError
				if errors.As(err, &incerto) {
					t.Errorf("não devia ser incerto: %v", err)
				}
				if api.StatusCode != tc.wantAPIStatus {
					t.Errorf("StatusCode = %d, quer %d", api.StatusCode, tc.wantAPIStatus)
				}
				if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
					t.Errorf("errors.Is(%v, %v) = false", err, tc.wantIs)
				}
			}
			if tc.wantEsperas != nil {
				got := reg.lista()
				if len(got) < len(tc.wantEsperas) {
					t.Fatalf("esperas = %v, quer %v", got, tc.wantEsperas)
				}
				for i, w := range tc.wantEsperas {
					if got[i] != w {
						t.Errorf("espera[%d] = %v, quer %v", i, got[i], w)
					}
				}
			}
		})
	}
}

// Sem retries configurados (WithRetries(0)) nada se repete.
func TestEnviar_SemRetries(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t, resposta{status: 502}, resposta{status: 200, corpo: corpoOK}))
	c, _ := novoClienteTeste(t, srv, WithRetries(0))

	_, _, err := c.enviar(context.Background(), opLeitura(), callConfig{})
	var api *APIError
	if !errors.As(err, &api) || api.StatusCode != 502 {
		t.Fatalf("erro = %v, quer APIError 502", err)
	}
	if got := srv.reqs.Load(); got != 1 {
		t.Errorf("tentativas = %d, quer 1", got)
	}
}

func TestEnviar_FalhasDeTransporte(t *testing.T) {
	t.Parallel()

	derruba := resposta{derruba: true}
	truncada := func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		t.Helper()
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("o servidor não suporta Hijack")
			return
		}
		conn, rw, err := hj.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		escreverBruto(t, rw, "HTTP/1.1 200 OK\r\nContent-Length: 50\r\nContent-Type: application/json\r\n\r\n{\"ok\"")
		if err := conn.Close(); err != nil {
			t.Errorf("fechando a conexão: %v", err)
		}
	}
	lento := func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		t.Helper()
		drenar(t, r)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}

	tests := []struct {
		name           string
		op             operacao
		api            func(t *testing.T) http.HandlerFunc
		opts           []Option
		wantTentativas int32
		wantIncerto    bool
	}{
		{
			name:           "GET com conexão derrubada repete e devolve erro de transporte",
			op:             opLeitura(),
			api:            func(t *testing.T) http.HandlerFunc { return sequencia(t, derruba) },
			wantTentativas: 3,
		},
		{
			name:           "POST com conexão derrubada depois de ler é incerto",
			op:             opEscrita(http.MethodPost),
			api:            func(t *testing.T) http.HandlerFunc { return sequencia(t, derruba) },
			wantTentativas: 1,
			wantIncerto:    true,
		},
		{
			name:           "PUT com conexão derrubada depois de ler é incerto",
			op:             opEscrita(http.MethodPut),
			api:            func(t *testing.T) http.HandlerFunc { return sequencia(t, derruba) },
			wantTentativas: 1,
			wantIncerto:    true,
		},
		{
			name: "GET com timeout da tentativa repete",
			op:   opLeitura(),
			api: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) { lento(t, w, r) }
			},
			opts:           []Option{WithTimeout(40 * time.Millisecond)},
			wantTentativas: 3,
		},
		{
			name: "POST com timeout da tentativa é incerto",
			op:   opEscrita(http.MethodPost),
			api: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) { lento(t, w, r) }
			},
			opts:           []Option{WithTimeout(40 * time.Millisecond)},
			wantTentativas: 1,
			wantIncerto:    true,
		},
		{
			name: "POST 200 com corpo truncado é incerto",
			op:   opEscrita(http.MethodPost),
			api: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) { truncada(t, w, r) }
			},
			wantTentativas: 1,
			wantIncerto:    true,
		},
		{
			name: "GET 200 com corpo truncado repete",
			op:   opLeitura(),
			api: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) { truncada(t, w, r) }
			},
			wantTentativas: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := novoServidor(t, tc.api(t))
			c, _ := novoClienteTeste(t, srv, tc.opts...)

			_, _, err := c.enviar(context.Background(), tc.op, callConfig{})
			if err == nil {
				t.Fatal("esperava erro")
			}
			if got := srv.reqs.Load(); got != tc.wantTentativas {
				t.Errorf("tentativas = %d, quer %d", got, tc.wantTentativas)
			}
			var incerto *UncertainResultError
			if got := errors.As(err, &incerto); got != tc.wantIncerto {
				t.Errorf("incerto = %v, quer %v (erro: %v)", got, tc.wantIncerto, err)
			}
			if tc.wantIncerto && incerto.Key != ChaveNatural(chaveTeste) {
				t.Errorf("Key = %v", incerto.Key)
			}
			assertSemVazamento(t, err.Error(), srv.URL)
		})
	}
}

func escreverBruto(t *testing.T, rw *bufio.ReadWriter, s string) {
	t.Helper()

	if _, err := rw.WriteString(s); err != nil {
		t.Errorf("escrevendo: %v", err)
		return
	}
	if err := rw.Flush(); err != nil {
		t.Errorf("flush: %v", err)
	}
}

// drenar lê o corpo da requisição; sem isso o servidor não percebe que o
// cliente desistiu e r.Context() nunca termina.
func drenar(t *testing.T, r *http.Request) {
	t.Helper()

	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		t.Errorf("lendo o corpo da requisição: %v", err)
	}
}

// assertSemVazamento falha se o texto traz a UC da query ou a URL do servidor.
func assertSemVazamento(t *testing.T, texto string, proibidos ...string) {
	t.Helper()

	for _, p := range append([]string{ucInventada, "?", "Bearer", segredoInventado}, proibidos...) {
		if strings.Contains(texto, p) {
			t.Errorf("o texto vaza %q: %s", p, texto)
		}
	}
}

// Nada saiu da máquina: conexão recusada repete até em POST, e não é incerto.
func TestEnviar_ConexaoRecusadaRepeteAteEmPOST(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	env := Environment{Name: "recusado", APIURL: srv.URL, TokenURL: srv.URL + "/sso/oauth/token"}
	srv.Close() // a porta fica fechada

	tests := []struct {
		name string
		op   operacao
	}{
		{"GET", opLeitura()},
		{"POST", opEscrita(http.MethodPost)},
		{"PUT", opEscrita(http.MethodPut)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hc, n := novoContador()
			c, reg := novoClienteComEnv(t, env, fonteFixa("token-inventado-1"),
				WithAgent(agenteInventado, perfilInventado), WithRateLimits(RateLimits{}), WithHTTPClient(hc))

			_, _, err := c.enviar(context.Background(), tc.op, callConfig{})
			if err == nil {
				t.Fatal("esperava erro")
			}
			if got := n.n.Load(); got != 3 {
				t.Errorf("tentativas = %d, quer 3 (1 + 2 retries)", got)
			}
			var incerto *UncertainResultError
			if errors.As(err, &incerto) {
				t.Errorf("nada foi escrito, não devia ser incerto: %v", err)
			}
			if len(reg.lista()) != 2 {
				t.Errorf("esperas = %v, quer 2", reg.lista())
			}
			assertSemVazamento(t, err.Error(), srv.URL)
			var ue *url.Error
			if errors.As(err, &ue) {
				t.Errorf("o *url.Error (com a URL) não devia sair: %v", ue)
			}
		})
	}
}

func TestEnviar_ContextoDoChamador(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		op          operacao
		wantIncerto bool
	}{
		{"GET cancelado no meio devolve o erro do ctx", opLeitura(), false},
		{"POST cancelado depois de escrever é incerto", opEscrita(http.MethodPost), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancela := context.WithCancel(context.Background())
			defer cancela()
			srv := novoServidor(t, func(w http.ResponseWriter, r *http.Request) {
				drenar(t, r)
				cancela()
				<-r.Context().Done()
			})
			c, _ := novoClienteTeste(t, srv)

			_, _, err := c.enviar(ctx, tc.op, callConfig{})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("erro = %v, quer context.Canceled", err)
			}
			var incerto *UncertainResultError
			if got := errors.As(err, &incerto); got != tc.wantIncerto {
				t.Errorf("incerto = %v, quer %v", got, tc.wantIncerto)
			}
			if got := srv.reqs.Load(); got != 1 {
				t.Errorf("tentativas = %d, quer 1", got)
			}
		})
	}
}

func TestEnviar_ContextoJaCanceladoNaoEnvia(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t, resposta{status: 200, corpo: corpoOK}))
	c, _ := novoClienteTeste(t, srv)
	ctx, cancela := context.WithCancel(context.Background())
	cancela()

	_, _, err := c.enviar(ctx, opLeitura(), callConfig{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("erro = %v, quer context.Canceled", err)
	}
	if srv.reqs.Load() != 0 || srv.tokens.Load() != 0 {
		t.Errorf("nada devia ser enviado: reqs=%d tokens=%d", srv.reqs.Load(), srv.tokens.Load())
	}
}

func TestEnviar_EsperaInterrompidaPeloCtx(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t, resposta{status: 502}))
	c, _ := novoClienteTeste(t, srv)
	ctx, cancela := context.WithCancel(context.Background())
	c.dormir = func(ctx context.Context, d time.Duration) error {
		cancela()
		return ctx.Err()
	}

	_, _, err := c.enviar(ctx, opLeitura(), callConfig{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("erro = %v, quer context.Canceled", err)
	}
	if got := srv.reqs.Load(); got != 1 {
		t.Errorf("tentativas = %d, quer 1", got)
	}
}

func TestEnviar_Regra403(t *testing.T) {
	t.Parallel()

	autorizador := resposta{status: 403, header: gateway(nil), corpo: erroJSON("ERR_CREDENCIAL_INVALIDA")}
	backend := resposta{status: 403, corpo: erroJSON("ERR_CREDENCIAL_INVALIDA")}
	semCodigo := resposta{status: 403, header: gateway(nil), corpo: `{"message":"Forbidden"}`}
	ok := resposta{status: 200, corpo: corpoOK}

	tests := []struct {
		name       string
		op         operacao
		opts       []Option
		respostas  []resposta
		primeira   bool // faz antes uma chamada que o token vê aceita
		wantTokens int32
		wantReqs   int32 // requisições à API na chamada testada (e na primeira)
		wantErr    bool
		wantIs     error    // sentinela esperada no erro
		wantAuth   []string // Authorization visto em cada requisição à API
	}{
		{
			name:       "token já aceito: invalida, busca outro e repete",
			op:         opLeitura(),
			respostas:  []resposta{ok, autorizador, ok},
			primeira:   true,
			wantTokens: 2,
			wantReqs:   3,
			wantAuth:   []string{"Bearer token-inventado-1", "Bearer token-inventado-1", "Bearer token-inventado-2"},
		},
		{
			name:       "POST também renova e repete, porque o gateway recusou antes do backend",
			op:         opEscrita(http.MethodPost),
			respostas:  []resposta{ok, autorizador, ok},
			primeira:   true,
			wantTokens: 2,
			wantReqs:   3,
		},
		{
			name:       "token nunca aceito: não repete",
			op:         opLeitura(),
			respostas:  []resposta{autorizador, ok},
			wantTokens: 1,
			wantReqs:   1,
			wantErr:    true,
			wantIs:     ErrCredencialInvalida,
		},
		{
			name:       "renova uma vez só, mesmo que o token novo também seja recusado",
			op:         opLeitura(),
			respostas:  []resposta{ok, autorizador, autorizador, ok},
			primeira:   true,
			wantTokens: 2,
			wantReqs:   3,
			wantErr:    true,
			wantIs:     ErrCredencialInvalida,
		},
		{
			name:       "403 do backend com o mesmo código não repete",
			op:         opLeitura(),
			respostas:  []resposta{ok, backend, ok},
			primeira:   true,
			wantTokens: 1,
			wantReqs:   2,
			wantErr:    true,
			wantIs:     ErrCredencialInvalida,
		},
		{
			name:       "403 do autorizador sem código ERR não repete",
			op:         opLeitura(),
			respostas:  []resposta{ok, semCodigo, ok},
			primeira:   true,
			wantTokens: 1,
			wantReqs:   2,
			wantErr:    true,
		},
		{
			name:       "WithTokenSource não renova",
			op:         opLeitura(),
			opts:       []Option{fonteFixa("token-inventado-1")},
			respostas:  []resposta{ok, autorizador, ok},
			primeira:   true,
			wantTokens: 0,
			wantReqs:   2,
			wantErr:    true,
			wantIs:     ErrCredencialInvalida,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var mu sync.Mutex
			var auths []string
			seq := sequencia(t, tc.respostas...)
			srv := novoServidor(t, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				auths = append(auths, r.Header.Get("Authorization"))
				mu.Unlock()
				seq(w, r)
			})
			var c *Client
			if len(tc.opts) > 0 {
				c, _ = novoClienteComEnv(t, srv.env(), append([]Option{WithAgent(agenteInventado, perfilInventado), WithRateLimits(RateLimits{})}, tc.opts...)...)
			} else {
				c, _ = novoClienteTeste(t, srv)
			}

			if tc.primeira {
				if _, _, err := c.enviar(context.Background(), opLeitura(), callConfig{}); err != nil {
					t.Fatalf("primeira chamada: %v", err)
				}
			}
			_, _, err := c.enviar(context.Background(), tc.op, callConfig{})

			if (err != nil) != tc.wantErr {
				t.Fatalf("erro = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("errors.Is(%v) = false: %v", tc.wantIs, err)
			}
			if got := srv.tokens.Load(); got != tc.wantTokens {
				t.Errorf("tokens buscados = %d, quer %d", got, tc.wantTokens)
			}
			if got := srv.reqs.Load(); got != tc.wantReqs {
				t.Errorf("requisições = %d, quer %d", got, tc.wantReqs)
			}
			if tc.wantAuth != nil {
				mu.Lock()
				defer mu.Unlock()
				if strings.Join(auths, "|") != strings.Join(tc.wantAuth, "|") {
					t.Errorf("Authorization = %v, quer %v", auths, tc.wantAuth)
				}
			}
		})
	}
}

func TestEnviar_Cabecalhos(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var vistos []http.Header
	var corpos []string
	var queries []string
	srv := novoServidor(t, func(w http.ResponseWriter, r *http.Request) {
		lido, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("lendo o corpo: %v", err)
		}
		mu.Lock()
		vistos = append(vistos, r.Header.Clone())
		corpos = append(corpos, string(lido))
		queries = append(queries, r.URL.RawQuery)
		mu.Unlock()
		escrever(t, w, corpoOK)
	})
	c, _ := novoClienteTeste(t, srv, WithUserAgent("app-inventado/2.0"))

	if _, _, err := c.enviar(context.Background(), opLeitura(), callConfig{}); err != nil {
		t.Fatalf("GET: %v", err)
	}
	if _, _, err := c.enviar(context.Background(), opEscrita(http.MethodPost), novoCallConfig(WithAgent(900011, 900012))); err != nil {
		t.Fatalf("POST: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(vistos) != 2 {
		t.Fatalf("requisições = %d", len(vistos))
	}
	get, post := vistos[0], vistos[1]

	if got := get.Get("Authorization"); got != "Bearer token-inventado-1" {
		t.Errorf("Authorization = %q", got)
	}
	if get.Get("codigoAgente") != "900001" || get.Get("codigoPerfil") != "900002" {
		t.Errorf("agente/perfil do cliente = %q/%q", get.Get("codigoAgente"), get.Get("codigoPerfil"))
	}
	if post.Get("codigoAgente") != "900011" || post.Get("codigoPerfil") != "900012" {
		t.Errorf("agente/perfil da chamada = %q/%q", post.Get("codigoAgente"), post.Get("codigoPerfil"))
	}
	if get.Get("Accept") != "application/json" {
		t.Errorf("Accept = %q", get.Get("Accept"))
	}
	ua := get.Get("User-Agent")
	if !strings.HasPrefix(ua, "abertura-mercado-energia-go/") || !strings.Contains(ua, "(+https://github.com/MarcoBuarque/abertura-mercado-energia)") || !strings.HasSuffix(ua, " app-inventado/2.0") {
		t.Errorf("User-Agent = %q", ua)
	}
	if post.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type do POST = %q", post.Get("Content-Type"))
	}
	if corpos[1] != `{"codigoUnidadeConsumidora":"`+ucInventada+`"}` {
		t.Errorf("corpo do POST = %q", corpos[1])
	}
	if get.Get("Content-Type") != "" || corpos[0] != "" {
		t.Errorf("GET sem corpo não devia ter Content-Type nem corpo")
	}
	if queries[0] != "codigoUnidadeConsumidora="+ucInventada {
		t.Errorf("query = %q", queries[0])
	}
	for i, h := range vistos {
		for k := range h {
			if strings.EqualFold(k, "Idempotency-Key") {
				t.Errorf("requisição %d enviou %s", i, k)
			}
		}
	}
}

func TestEnviar_TimeoutDaChamadaTemPrioridade(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, func(w http.ResponseWriter, r *http.Request) {
		drenar(t, r)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	c, _ := novoClienteTeste(t, srv, WithTimeout(time.Hour), WithRetries(0))

	inicio := time.Now()
	_, _, err := c.enviar(context.Background(), opLeitura(), novoCallConfig(WithTimeout(30*time.Millisecond)))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("erro = %v, quer DeadlineExceeded", err)
	}
	if time.Since(inicio) > 3*time.Second {
		t.Error("o timeout da chamada não foi usado")
	}
}

func TestEnviar_FalhaNoTokenNaoRepete(t *testing.T) {
	t.Parallel()

	var chamadas atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/sso/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		chamadas.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		escrever(t, w, `{"error":"invalid_client","error_description":"`+segredoInventado+`"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, _ := novoClienteComEnv(t, Environment{APIURL: srv.URL, TokenURL: srv.URL + "/sso/oauth/token"},
		WithCredentials(idInventado, segredoInventado), WithAgent(agenteInventado, perfilInventado), WithRateLimits(RateLimits{}))

	_, _, err := c.enviar(context.Background(), opLeitura(), callConfig{})
	var api *APIError
	if !errors.As(err, &api) || api.Operation != "obterToken" {
		t.Fatalf("erro = %v, quer APIError de obterToken", err)
	}
	if chamadas.Load() != 1 {
		t.Errorf("buscas de token = %d, quer 1", chamadas.Load())
	}
	assertSemVazamento(t, err.Error())
}

func TestEnviar_ErrosDeMontagem(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t, resposta{status: 200, corpo: corpoOK}))
	c, _ := novoClienteTeste(t, srv)

	semMetodo := opLeitura()
	semMetodo.metodo = "METODO INVALIDO"
	corpoInvalido := opEscrita(http.MethodPost)
	corpoInvalido.corpo = make(chan int)
	urlInvalida := opLeitura()
	urlInvalida.caminho = "/%zz" + ucInventada

	tests := []struct {
		name string
		op   operacao
	}{
		{"método inválido", semMetodo},
		{"corpo que não vira JSON", corpoInvalido},
		{"caminho que não vira URL não vaza o caminho", urlInvalida},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := c.enviar(context.Background(), tc.op, callConfig{})
			if err == nil {
				t.Fatal("esperava erro")
			}
			assertSemVazamento(t, err.Error())
		})
	}
	if srv.reqs.Load() != 0 {
		t.Errorf("nada devia ser enviado: %d", srv.reqs.Load())
	}
}

// Garante que dois clientes usam fontes de token independentes.
func TestEnviar_ClientesIndependentes(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t, resposta{status: 200, corpo: corpoOK}))
	a, _ := novoClienteTeste(t, srv)
	b, _ := novoClienteTeste(t, srv)
	for _, c := range []*Client{a, b} {
		if _, _, err := c.enviar(context.Background(), opLeitura(), callConfig{}); err != nil {
			t.Fatal(err)
		}
	}
	if srv.tokens.Load() != 2 {
		t.Errorf("tokens = %d, quer 2", srv.tokens.Load())
	}
}
