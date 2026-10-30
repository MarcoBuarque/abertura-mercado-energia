package abertura

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimitador_QuartaRequisicaoEspera(t *testing.T) {
	t.Parallel()

	l := novosLimitadores(RateLimits{Default: RateLimit{Requests: 3, Per: 300 * time.Millisecond}})
	ctx := context.Background()

	inicio := time.Now()
	for i := range 3 {
		if err := l.esperar(ctx, grupoGeral); err != nil {
			t.Fatalf("requisição %d: %v", i+1, err)
		}
	}
	if d := time.Since(inicio); d > 50*time.Millisecond {
		t.Errorf("a rajada de 3 esperou %v", d)
	}

	inicio = time.Now()
	if err := l.esperar(ctx, grupoGeral); err != nil {
		t.Fatalf("quarta requisição: %v", err)
	}
	if d := time.Since(inicio); d < 60*time.Millisecond {
		t.Errorf("a quarta requisição esperou só %v, devia esperar perto de 100 ms", d)
	}
}

func TestLimitador_CtxDuranteAEspera(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     func() (context.Context, context.CancelFunc)
		wantErr error
	}{
		{
			name: "prazo mais curto que a espera devolve DeadlineExceeded sem esperar",
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 30*time.Millisecond)
			},
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "cancelamento durante a espera devolve Canceled",
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancela := context.WithCancel(context.Background())
				time.AfterFunc(30*time.Millisecond, cancela)
				return ctx, cancela
			},
			wantErr: context.Canceled,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := novosLimitadores(RateLimits{Default: RateLimit{Requests: 1, Per: time.Hour}})
			if err := l.esperar(context.Background(), grupoGeral); err != nil {
				t.Fatal(err)
			}
			ctx, cancela := tc.ctx()
			defer cancela()

			inicio := time.Now()
			err := l.esperar(ctx, grupoGeral)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("erro = %v, quer %v", err, tc.wantErr)
			}
			if d := time.Since(inicio); d > 2*time.Second {
				t.Errorf("devolveu só depois de %v", d)
			}
		})
	}
}

func TestLimitador_BaldeDeMedicoes(t *testing.T) {
	t.Parallel()

	curto := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 30*time.Millisecond)
	}
	umaPorHora := RateLimit{Requests: 1, Per: time.Hour}
	tests := []struct {
		name         string
		limites      RateLimits
		wantMedicoes bool // a segunda medição passa depois de esgotar o geral
	}{
		{"produção: medições têm balde próprio", RateLimits{Default: umaPorHora, Medicoes: RateLimit{Requests: 5, Per: time.Hour}}, true},
		{"sandbox: Medicoes zero usa o balde geral", RateLimits{Default: umaPorHora}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := novosLimitadores(tc.limites)
			if err := l.esperar(context.Background(), grupoGeral); err != nil {
				t.Fatal(err)
			}
			ctx, cancela := curto()
			defer cancela()

			err := l.esperar(ctx, grupoMedicoes)
			if got := err == nil; got != tc.wantMedicoes {
				t.Errorf("medições passou = %v (%v), quer %v", got, err, tc.wantMedicoes)
			}
		})
	}
}

func TestLimitador_BaldeDeMedicoesSemGeral(t *testing.T) {
	t.Parallel()

	l := novosLimitadores(RateLimits{Medicoes: RateLimit{Requests: 1, Per: time.Hour}})
	ctx, cancela := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancela()

	if err := l.esperar(ctx, grupoGeral); err != nil {
		t.Errorf("o geral é ilimitado: %v", err)
	}
	if err := l.esperar(ctx, grupoMedicoes); err != nil {
		t.Fatalf("primeira medição: %v", err)
	}
	if err := l.esperar(ctx, grupoMedicoes); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("segunda medição = %v, quer DeadlineExceeded", err)
	}
}

func TestLimitador_ValorZeroDesliga(t *testing.T) {
	t.Parallel()

	l := novosLimitadores(RateLimits{})
	inicio := time.Now()
	for range 1000 {
		if err := l.esperar(context.Background(), grupoMedicoes); err != nil {
			t.Fatal(err)
		}
		if err := l.esperar(context.Background(), grupoGeral); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(inicio); d > time.Second {
		t.Errorf("limitador desligado esperou %v", d)
	}
}

func TestLimitador_RajadaNuncaPassaDe30(t *testing.T) {
	t.Parallel()

	l := novosLimitadores(RateLimits{Default: RateLimit{Requests: 900, Per: 5 * time.Minute}})
	ctx, cancela := context.WithTimeout(context.Background(), time.Second)
	defer cancela()

	for i := range 30 {
		if err := l.esperar(ctx, grupoGeral); err != nil {
			t.Fatalf("requisição %d da rajada: %v", i+1, err)
		}
	}
	// A 31ª espera ~333 ms (5 min / 900), mais que o prazo de 50 ms.
	curto, cancela2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancela2()
	if err := l.esperar(curto, grupoGeral); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("31ª requisição = %v, quer DeadlineExceeded", err)
	}
}

// O pedido de token passa pelo balde geral: com 2 por hora, a primeira
// chamada gasta 1 no token e 1 na requisição, e a segunda já não cabe.
func TestLimitador_TokenGastaDoBaldeGeral(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t, resposta{status: 200, corpo: corpoOK}))
	c, _ := novoClienteTeste(t, srv, WithRateLimits(RateLimits{Default: RateLimit{Requests: 2, Per: time.Hour}}))

	if _, _, err := c.enviar(context.Background(), opLeitura(), callConfig{}); err != nil {
		t.Fatalf("primeira chamada: %v", err)
	}
	ctx, cancela := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancela()
	_, _, err := c.enviar(ctx, opLeitura(), callConfig{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("segunda chamada = %v, quer DeadlineExceeded", err)
	}
	if srv.tokens.Load() != 1 || srv.reqs.Load() != 1 {
		t.Errorf("tokens=%d reqs=%d, quer 1 e 1", srv.tokens.Load(), srv.reqs.Load())
	}
}

// Cada tentativa, inclusive retry, espera o limitador.
func TestLimitador_RetriesGastamDoBalde(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t, resposta{status: 502}))
	c, _ := novoClienteComEnv(t, srv.env(), fonteFixa("token-inventado-1"), WithAgent(agenteInventado, perfilInventado),
		WithRateLimits(RateLimits{Default: RateLimit{Requests: 2, Per: time.Hour}}))

	ctx, cancela := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancela()
	_, _, err := c.enviar(ctx, opLeitura(), callConfig{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("erro = %v, quer DeadlineExceeded na terceira tentativa", err)
	}
	if srv.reqs.Load() != 2 {
		t.Errorf("requisições = %d, quer 2", srv.reqs.Load())
	}
}

// O limitador também barra o pedido de token quando o balde acabou.
func TestLimitador_TokenEsperaOBalde(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t, resposta{status: 200, corpo: corpoOK}))
	c, _ := novoClienteTeste(t, srv, WithRateLimits(RateLimits{Default: RateLimit{Requests: 1, Per: time.Hour}}))
	ctx, cancela := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancela()

	// A tentativa gasta o único item do balde; o pedido de token já não cabe.
	_, _, err := c.enviar(ctx, opLeitura(), callConfig{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("erro = %v, quer DeadlineExceeded", err)
	}
	if srv.tokens.Load() != 0 {
		t.Errorf("tokens buscados = %d, quer 0", srv.tokens.Load())
	}
	assertSemVazamento(t, err.Error(), srv.URL)
}

func TestLimitador_ClienteDeTokenPreservaOClienteDoUsuario(t *testing.T) {
	t.Parallel()

	usuario := &http.Client{Timeout: 7 * time.Second}
	copia := clienteDeToken(usuario, limitadores{})
	if copia == usuario || copia.Timeout != 7*time.Second {
		t.Errorf("a cópia devia manter o Timeout e ser outro cliente")
	}
	if usuario.Transport != nil {
		t.Error("o cliente do usuário foi alterado")
	}
}

// TestClienteDeToken_WhenRedirecionamento_NaoReenviaOFormulario prova que o
// cliente do token nunca segue um 307, que reenviaria o client_secret do
// formulário a outro host, nem com um cliente do usuário (WithHTTPClient) e
// mesmo que ele tenha um CheckRedirect que seguiria.
func TestClienteDeToken_WhenRedirecionamento_NaoReenviaOFormulario(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		usuario    *http.Client
		wantSegue  bool
		wantStatus int
	}{
		{"cliente padrão do SDK não segue", nil, false, http.StatusTemporaryRedirect},
		{"cliente do usuário sem CheckRedirect também não segue", &http.Client{}, false, http.StatusTemporaryRedirect},
		{"cliente do usuário com CheckRedirect que seguiria também não segue", &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return nil }}, false, http.StatusTemporaryRedirect},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var recebeuB atomic.Bool
			b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recebeuB.Store(true)
			}))
			defer b.Close()
			a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, b.URL, http.StatusTemporaryRedirect)
			}))
			defer a.Close()

			cli := clienteDeToken(tc.usuario, limitadores{})
			resp, err := cli.PostForm(a.URL, url.Values{"client_secret": {"segredo-inventado"}})
			if err != nil {
				t.Fatalf("PostForm: %v", err)
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Errorf("fechar corpo: %v", err)
				}
			}()
			if resp.StatusCode != tc.wantStatus || recebeuB.Load() != tc.wantSegue {
				t.Errorf("status = %d, segundo servidor recebeu = %v; quer %d e %v",
					resp.StatusCode, recebeuB.Load(), tc.wantStatus, tc.wantSegue)
			}
		})
	}
}

type corpoRastreado struct {
	fechado atomic.Bool
}

func (c *corpoRastreado) Read([]byte) (int, error) { return 0, io.EOF }
func (c *corpoRastreado) Close() error             { c.fechado.Store(true); return nil }

// TestTransporteLimitado_WhenLimitadorFalha_FechaOCorpo prova o contrato do
// http.RoundTripper: o corpo da requisição é fechado também quando o
// transporte devolve erro sem enviar.
func TestTransporteLimitado_WhenLimitadorFalha_FechaOCorpo(t *testing.T) {
	t.Parallel()

	ctx, cancela := context.WithCancel(context.Background())
	cancela()
	corpo := &corpoRastreado{}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://example.org/token", corpo)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	tr := transporteLimitado{base: http.DefaultTransport, limites: novosLimitadores(RateLimits{Default: RateLimit{Requests: 1, Per: time.Hour}})}
	resp, err := tr.RoundTrip(req)
	if resp != nil {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Errorf("fechar corpo da resposta: %v", cerr)
		}
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RoundTrip: %v, quer context.Canceled", err)
	}
	if !corpo.fechado.Load() {
		t.Error("o corpo da requisição não foi fechado")
	}
}
