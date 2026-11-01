package abertura

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

const (
	idInventado      = "id-inventado"
	segredoInventado = "segredo-inventado"
	uaInventado      = "ua-inventado/1.0"
	urlInexistente   = "http://exemplo.invalid/sso/oauth/token"
)

// escrever grava o corpo da resposta e falha o teste se a escrita der erro.
func escrever(t *testing.T, w http.ResponseWriter, corpo string) {
	t.Helper()

	if _, err := io.WriteString(w, corpo); err != nil {
		t.Errorf("escrevendo a resposta: %v", err)
	}
}

// novoCacheSemRede cria um cache que nunca deveria chegar à rede.
func novoCacheSemRede() *cacheToken {
	cfg := configToken{tokenURL: urlInexistente, clientID: idInventado}
	cfg.clientSecret = segredoInventado
	return novoCacheToken(cfg)
}

// relogioFalso é um relógio manual, seguro para uso concorrente.
type relogioFalso struct {
	mu sync.Mutex
	t  time.Time
}

func novoRelogio() *relogioFalso {
	return &relogioFalso{t: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
}

func (r *relogioFalso) agora() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.t
}

func (r *relogioFalso) avancar(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.t = r.t.Add(d)
}

// servidorToken simula o endpoint de token e guarda o que recebeu.
type servidorToken struct {
	*httptest.Server
	buscas atomic.Int64

	mu               sync.Mutex
	formulario       map[string][]string
	basic            bool
	temAuthorization bool
	userAgent        string
	metodo           string
	tipoMidia        string
}

// novoServidorToken devolve tokens numerados (token-inventado-1, 2, ...) com
// expires_in 300. atraso simula latência; falha, se não nula, responde erro.
func novoServidorToken(t *testing.T, atraso time.Duration, falha func(w http.ResponseWriter)) *servidorToken {
	t.Helper()

	s := &servidorToken{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "formulário ilegível", http.StatusBadRequest)
			return
		}
		_, _, temBasic := r.BasicAuth()
		s.mu.Lock()
		s.formulario = r.PostForm
		s.basic = temBasic
		s.userAgent = r.Header.Get("User-Agent")
		s.temAuthorization = r.Header.Get("Authorization") != ""
		s.metodo = r.Method
		s.tipoMidia = r.Header.Get("Content-Type")
		s.mu.Unlock()

		n := s.buscas.Add(1)
		if atraso > 0 {
			select {
			case <-time.After(atraso):
			case <-r.Context().Done():
				return
			}
		}
		if falha != nil {
			falha(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		escrever(t, w, fmt.Sprintf(`{"access_token":"token-inventado-%d","token_type":"Bearer","expires_in":300,"refresh_expires_in":0}`, n))
	}))
	t.Cleanup(s.Close)
	return s
}

func novoCacheDeTeste(s *servidorToken, rel *relogioFalso) *cacheToken {
	c := novoCacheToken(configToken{
		tokenURL:     s.URL,
		clientID:     idInventado,
		clientSecret: segredoInventado,
		httpClient:   s.Client(),
		userAgent:    uaInventado,
	})
	if rel != nil {
		c.agora = rel.agora
	}
	return c
}

func TestCacheToken_WhenPedido_UsaFormularioSemBasic(t *testing.T) {
	t.Parallel()

	s := novoServidorToken(t, 0, nil)
	c := novoCacheDeTeste(s, nil)

	tok, err := c.token(context.Background())
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if tok.AccessToken != "token-inventado-1" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metodo != http.MethodPost {
		t.Errorf("método = %s", s.metodo)
	}
	if !strings.HasPrefix(s.tipoMidia, "application/x-www-form-urlencoded") {
		t.Errorf("Content-Type = %q", s.tipoMidia)
	}
	if s.basic {
		t.Error("o pedido usou cabeçalho Basic; deveria usar o formulário")
	}
	if s.userAgent != uaInventado {
		t.Errorf("User-Agent = %q, quer %q", s.userAgent, uaInventado)
	}
	if s.temAuthorization {
		t.Error("o pedido trouxe cabeçalho Authorization")
	}
	if len(s.formulario) != 2 {
		t.Errorf("formulário tem %d campos (%v), quer só client_id e client_secret", len(s.formulario), s.formulario)
	}
	if _, ok := s.formulario["grant_type"]; ok {
		t.Error("o formulário traz grant_type; o endpoint o rejeita")
	}
	want := map[string]string{
		"client_id":     idInventado,
		"client_secret": segredoInventado,
	}
	for k, v := range want {
		if got := s.formulario[k]; len(got) != 1 || got[0] != v {
			t.Errorf("campo %s = %v, quer %q", k, got, v)
		}
	}
}

func TestCacheToken_WhenRespostaOK_InterpretaCorpo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		corpo     string
		wantToken string
		wantTipo  string
		wantErro  bool
		wantVence time.Duration // validade esperada, quando não há erro
	}{
		{name: "when expires_in presente honra o valor", corpo: `{"access_token":"t1","token_type":"Bearer","expires_in":120}`, wantToken: "t1", wantTipo: "Bearer", wantVence: 120 * time.Second},
		{name: "when sem expires_in assume 300 s", corpo: `{"access_token":"t2","token_type":"Bearer"}`, wantToken: "t2", wantTipo: "Bearer", wantVence: 300 * time.Second},
		{name: "when sem token_type assume Bearer", corpo: `{"access_token":"t3"}`, wantToken: "t3", wantTipo: "Bearer", wantVence: 300 * time.Second},
		{name: "when sem access_token falha", corpo: `{"token_type":"Bearer","expires_in":300}`, wantErro: true},
		{name: "when access_token vazio falha", corpo: `{"access_token":"","expires_in":300}`, wantErro: true},
		{name: "when JSON invalido falha", corpo: `não é json`, wantErro: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				escrever(t, w, tc.corpo)
			}))
			t.Cleanup(srv.Close)
			rel := novoRelogio()
			c := novoCacheToken(configToken{tokenURL: srv.URL, clientID: idInventado, clientSecret: segredoInventado, httpClient: srv.Client()})
			c.agora = rel.agora

			tok, err := c.token(context.Background())
			if tc.wantErro {
				var ft *erroFalhaToken
				if !errors.As(err, &ft) {
					t.Fatalf("err = %v, quer *erroFalhaToken", err)
				}
				if strings.Contains(err.Error(), tc.corpo) || strings.Contains(err.Error(), segredoInventado) {
					t.Errorf("erro vaza corpo ou segredo: %q", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("token: %v", err)
			}
			if tok.AccessToken != tc.wantToken || tok.TokenType != tc.wantTipo {
				t.Errorf("token = %q/%q, quer %q/%q", tok.AccessToken, tok.TokenType, tc.wantToken, tc.wantTipo)
			}
			if want := rel.agora().Add(tc.wantVence); !tok.Expiry.Equal(want) {
				t.Errorf("Expiry = %v, quer %v", tok.Expiry, want)
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if want := rel.agora().Add(tc.wantVence - folgaToken); !c.venceEm.Equal(want) {
				t.Errorf("venceEm = %v, quer %v", c.venceEm, want)
			}
		})
	}
}

func TestCacheToken_WhenConcorrente_BuscaUmaVez(t *testing.T) {
	t.Parallel()

	s := novoServidorToken(t, 100*time.Millisecond, nil)
	c := novoCacheDeTeste(s, nil)

	const n = 50
	var wg sync.WaitGroup
	tokens := make([]*oauth2.Token, n)
	erros := make([]error, n)
	inicio := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-inicio
			tokens[i], erros[i] = c.token(context.Background())
		}()
	}
	close(inicio)
	wg.Wait()

	for i := range n {
		if erros[i] != nil {
			t.Fatalf("goroutine %d: %v", i, erros[i])
		}
		if tokens[i] != tokens[0] {
			t.Fatalf("goroutine %d recebeu outro token", i)
		}
	}
	if got := s.buscas.Load(); got != 1 {
		t.Errorf("buscas = %d, quer 1", got)
	}
}

func TestCacheToken_WhenPassaTempo_RenovaTrintaSegundosAntes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		passado    time.Duration
		wantBuscas int64
		wantToken  string
	}{
		{"logo após o recebimento reaproveita", 0, 1, "token-inventado-1"},
		{"aos 269 s reaproveita", 269 * time.Second, 1, "token-inventado-1"},
		{"aos 270 s ainda vale", 270 * time.Second, 1, "token-inventado-1"},
		{"aos 271 s renova", 271 * time.Second, 2, "token-inventado-2"},
		{"depois do vencimento renova", 400 * time.Second, 2, "token-inventado-2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := novoServidorToken(t, 0, nil)
			rel := novoRelogio()
			c := novoCacheDeTeste(s, rel)

			if _, err := c.token(context.Background()); err != nil {
				t.Fatalf("primeira busca: %v", err)
			}
			rel.avancar(tc.passado)
			tok, err := c.token(context.Background())
			if err != nil {
				t.Fatalf("segunda chamada: %v", err)
			}
			if tok.AccessToken != tc.wantToken {
				t.Errorf("AccessToken = %q, quer %q", tok.AccessToken, tc.wantToken)
			}
			if got := s.buscas.Load(); got != tc.wantBuscas {
				t.Errorf("buscas = %d, quer %d", got, tc.wantBuscas)
			}
		})
	}
}

func TestCacheToken_WhenSemExpiresIn_AssumeTrezentosSegundos(t *testing.T) {
	t.Parallel()

	var buscas atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := buscas.Add(1)
		w.Header().Set("Content-Type", "application/json")
		escrever(t, w, fmt.Sprintf(`{"access_token":"token-inventado-%d","token_type":"Bearer"}`, n))
	}))
	t.Cleanup(srv.Close)

	rel := novoRelogio()
	c := novoCacheToken(configToken{tokenURL: srv.URL, clientID: idInventado, clientSecret: segredoInventado, httpClient: srv.Client()})
	c.agora = rel.agora

	if _, err := c.token(context.Background()); err != nil {
		t.Fatal(err)
	}
	rel.avancar(270 * time.Second)
	if _, err := c.token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if buscas.Load() != 1 {
		t.Fatalf("aos 270 s buscas = %d, quer 1", buscas.Load())
	}
	rel.avancar(time.Second)
	if _, err := c.token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if buscas.Load() != 2 {
		t.Errorf("aos 271 s buscas = %d, quer 2", buscas.Load())
	}
}

func TestCacheToken_Invalidar_WhenTokenAtual_ForcaBuscaNova(t *testing.T) {
	t.Parallel()

	s := novoServidorToken(t, 0, nil)
	c := novoCacheDeTeste(s, nil)

	t1, err := c.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c.invalidar(t1)
	t2, err := c.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if t2.AccessToken == t1.AccessToken {
		t.Errorf("token não mudou: %q", t2.AccessToken)
	}
	if got := s.buscas.Load(); got != 2 {
		t.Errorf("buscas = %d, quer 2", got)
	}
}

func TestCacheToken_Invalidar_WhenTokenAntigo_NaoDerrubaOAtual(t *testing.T) {
	t.Parallel()

	s := novoServidorToken(t, 0, nil)
	c := novoCacheDeTeste(s, nil)

	t1, err := c.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c.invalidar(t1)
	t2, err := c.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Outra goroutine ainda segurava t1 e invalida tarde demais.
	c.invalidar(t1)
	c.invalidar(nil)
	t3, err := c.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if t3 != t2 {
		t.Errorf("invalidar token antigo derrubou o atual: %q -> %q", t2.AccessToken, t3.AccessToken)
	}
	if got := s.buscas.Load(); got != 2 {
		t.Errorf("buscas = %d, quer 2", got)
	}
}

func TestCacheToken_Aceito_WhenMarcado_ValePorToken(t *testing.T) {
	t.Parallel()

	s := novoServidorToken(t, 0, nil)
	c := novoCacheDeTeste(s, nil)

	t1, err := c.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if c.foiAceito(t1) {
		t.Error("token novo não deveria constar como aceito")
	}
	c.marcarAceito(t1)
	if !c.foiAceito(t1) {
		t.Error("token marcado deveria constar como aceito")
	}
	if c.foiAceito(nil) {
		t.Error("nil nunca é aceito")
	}
	c.marcarAceito(nil) // não deve panicar

	// Um token estranho ao cache não vira aceito.
	estranho := &oauth2.Token{AccessToken: "token-inventado-9"}
	c.marcarAceito(estranho)
	if c.foiAceito(estranho) {
		t.Error("token que o cache não emitiu não deveria ser marcado")
	}

	// O token renovado começa sem a marca; o antigo mantém a dele, para que
	// uma resposta atrasada ainda seja avaliada com o que se sabia.
	c.invalidar(t1)
	t2, err := c.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if c.foiAceito(t2) {
		t.Error("token renovado não herda a marca do anterior")
	}
	if !c.foiAceito(t1) {
		t.Error("o token anterior deveria manter a marca")
	}
}

func TestCacheToken_Renovavel_WhenCredenciais_EhVerdadeiro(t *testing.T) {
	t.Parallel()

	c := novoCacheSemRede()
	if !c.renovavel() {
		t.Error("cacheToken deveria ser renovável")
	}
	if c.timeoutBusca != 10*time.Second {
		t.Errorf("timeoutBusca = %v, quer 10s", c.timeoutBusca)
	}
}

func TestFonteExterna_WhenTokenSource_NaoRenova(t *testing.T) {
	t.Parallel()

	externo := &oauth2.Token{AccessToken: "token-inventado-externo", TokenType: "Bearer"}
	var fonte fonteToken = novaFonteExterna(oauth2.StaticTokenSource(externo))

	if fonte.renovavel() {
		t.Error("fonte externa não é renovável")
	}
	tok, err := fonte.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "token-inventado-externo" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}

	// Sem cache nem regra de 403: nada disto tem efeito.
	fonte.marcarAceito(tok)
	if fonte.foiAceito(tok) {
		t.Error("fonte externa nunca marca token como aceito")
	}
	fonte.invalidar(tok)
	if _, err := fonte.token(context.Background()); err != nil {
		t.Errorf("token após invalidar: %v", err)
	}
}

type fonteQueFalha struct{ err error }

func (f fonteQueFalha) Token() (*oauth2.Token, error) { return nil, f.err }

func TestFonteExterna_WhenFonteFalhaOuCtxCancelado_DevolveErro(t *testing.T) {
	t.Parallel()

	causa := errors.New("fonte inventada indisponível")
	fonte := novaFonteExterna(fonteQueFalha{causa})
	if _, err := fonte.token(context.Background()); !errors.Is(err, causa) {
		t.Errorf("err = %v, quer a causa", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	boa := novaFonteExterna(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "token-inventado-externo"}))
	if _, err := boa.token(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, quer context.Canceled", err)
	}
}

func TestCacheToken_WhenEndpointFalha_ViraAPIErrorSemSegredo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		corpo      string
		wantCode   string
		wantStatus int
	}{
		{"400 com erro OAuth2", 400, `{"error":"invalid_request","error_description":"pedido inventado"}`, "invalid_request", 400},
		{"401 com erro OAuth2", 401, `{"error":"invalid_client","error_description":"cliente inventado"}`, "invalid_client", 401},
		{"401 com texto puro", 401, "não autorizado, inventado", "", 401},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := novoServidorToken(t, 0, func(w http.ResponseWriter) {
				w.Header().Set("x-amzn-RequestId", "req-inventado-3")
				w.WriteHeader(tc.status)
				escrever(t, w, tc.corpo)
			})
			c := novoCacheDeTeste(s, nil)

			_, err := c.token(context.Background())
			var api *APIError
			if !errors.As(err, &api) {
				t.Fatalf("err = %T %v, quer *APIError", err, err)
			}
			if api.Operation != "obterToken" || api.StatusCode != tc.wantStatus || api.Code != tc.wantCode {
				t.Errorf("api = %+v", api)
			}
			if api.RequestID != "req-inventado-3" {
				t.Errorf("RequestID = %q", api.RequestID)
			}
			exigirSemSegredo(t, err, c)
		})
	}
}

func TestCacheToken_WhenFalha_NaoCacheiaEBuscaDeNovo(t *testing.T) {
	t.Parallel()

	var ligado atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ligado.Load() {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		escrever(t, w, `{"access_token":"token-inventado-ok","token_type":"Bearer","expires_in":300}`)
	}))
	t.Cleanup(srv.Close)
	c := novoCacheToken(configToken{tokenURL: srv.URL, clientID: idInventado, clientSecret: segredoInventado, httpClient: srv.Client()})

	if _, err := c.token(context.Background()); err == nil {
		t.Fatal("esperava erro na primeira chamada")
	}
	ligado.Store(true)
	tok, err := c.token(context.Background())
	if err != nil {
		t.Fatalf("segunda chamada: %v", err)
	}
	if tok.AccessToken != "token-inventado-ok" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}
}

func TestCacheToken_WhenTransporteFalha_ErroSemSegredoNemURL(t *testing.T) {
	t.Parallel()

	// Reserva uma porta e a fecha: a conexão será recusada.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endereco := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	c := novoCacheToken(configToken{tokenURL: "http://" + endereco + "/sso/oauth/token?uc=000123", clientID: idInventado, clientSecret: segredoInventado})
	_, err = c.token(context.Background())
	if err == nil {
		t.Fatal("esperava erro de transporte")
	}
	exigirSemSegredo(t, err, c)
	if strings.Contains(err.Error(), "uc=000123") {
		t.Errorf("a URL vazou no erro: %v", err)
	}
	if !strings.Contains(err.Error(), "obterToken") {
		t.Errorf("erro sem a operação: %v", err)
	}
}

func TestCacheToken_WhenServidorLento_RespeitaTimeoutDaBusca(t *testing.T) {
	t.Parallel()

	s := novoServidorToken(t, 5*time.Second, nil)
	c := novoCacheDeTeste(s, nil)
	c.timeoutBusca = 50 * time.Millisecond

	inicio := time.Now()
	_, err := c.token(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, quer DeadlineExceeded", err)
	}
	if d := time.Since(inicio); d > 3*time.Second {
		t.Errorf("demorou %v; o timeout não valeu", d)
	}
	exigirSemSegredo(t, err, c)
}

func TestCacheToken_WhenCtxCancelado_InterrompeBusca(t *testing.T) {
	t.Parallel()

	s := novoServidorToken(t, 5*time.Second, nil)
	c := novoCacheDeTeste(s, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	inicio := time.Now()
	_, err := c.token(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, quer Canceled", err)
	}
	if d := time.Since(inicio); d > 3*time.Second {
		t.Errorf("demorou %v; o ctx não valeu", d)
	}
}

func TestCacheToken_WhenOutraBuscaEmAndamento_EsperaRespeitandoOCtx(t *testing.T) {
	t.Parallel()

	s := novoServidorToken(t, 300*time.Millisecond, nil)
	c := novoCacheDeTeste(s, nil)

	primeira := make(chan error, 1)
	go func() {
		_, err := c.token(context.Background())
		primeira <- err
	}()
	// Dá tempo de a primeira chamada assumir a busca.
	for s.buscas.Load() == 0 {
		time.Sleep(time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := c.token(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("espera ignorou o ctx: %v", err)
	}
	if err := <-primeira; err != nil {
		t.Errorf("a primeira busca deveria concluir: %v", err)
	}
}

func TestCacheToken_Formatacao_NaoVazaSegredo(t *testing.T) {
	t.Parallel()

	c := novoCacheSemRede()
	for _, verbo := range []string{"%v", "%+v", "%#v", "%s"} {
		if out := fmt.Sprintf(verbo, c); strings.Contains(out, segredoInventado) {
			t.Errorf("%s vazou o segredo: %q", verbo, out)
		}
	}
}

// exigirSemSegredo falha se o erro, ou a formatação dele, trouxer o segredo.
func exigirSemSegredo(t *testing.T, err error, extras ...any) {
	t.Helper()

	for _, verbo := range []string{"%v", "%+v", "%#v", "%s"} {
		if out := fmt.Sprintf(verbo, err); strings.Contains(out, segredoInventado) {
			t.Errorf("%s do erro vazou o segredo: %q", verbo, out)
		}
	}
	if strings.Contains(err.Error(), segredoInventado) {
		t.Errorf("Error() vazou o segredo: %q", err.Error())
	}
	for _, e := range extras {
		if out := fmt.Sprintf("%+v", e); strings.Contains(out, segredoInventado) {
			t.Errorf("%%+v de %T vazou o segredo: %q", e, out)
		}
	}
}

// corpoFechaMal lê normalmente, mas falha ao fechar.
type corpoFechaMal struct{ io.Reader }

func (corpoFechaMal) Close() error { return errors.New("falha ao fechar o corpo") }

// transporteFalso devolve uma resposta pronta, sem rede.
type transporteFalso func(*http.Request) (*http.Response, error)

func (f transporteFalso) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCacheToken_WhenFechamentoDoCorpoFalha_ViraErroFalhaTokenSemSegredo(t *testing.T) {
	t.Parallel()

	hc := &http.Client{Transport: transporteFalso(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       corpoFechaMal{strings.NewReader(`{"access_token":"t","expires_in":300}`)},
		}, nil
	})}
	c := novoCacheToken(configToken{tokenURL: urlInexistente, clientID: idInventado, clientSecret: segredoInventado, httpClient: hc})

	_, err := c.token(context.Background())
	var ft *erroFalhaToken
	if !errors.As(err, &ft) {
		t.Fatalf("err = %v, quer *erroFalhaToken", err)
	}
	if strings.Contains(err.Error(), segredoInventado) {
		t.Errorf("erro vaza o segredo: %q", err)
	}
}
