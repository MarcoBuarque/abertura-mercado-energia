package abertura

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestProblemFrom_WhenFalhaDeTransporte_ClassificaPeloTipo prova que as falhas
// de transporte reais (TLS não confiável, conexão fechada sem resposta, JSON
// inválido num 200) chegam ao catálogo, e que Retryable e Uncertain vêm de
// decidir.
func TestProblemFrom_WhenFalhaDeTransporte_ClassificaPeloTipo(t *testing.T) {
	t.Parallel()

	derruba := func(t *testing.T) http.HandlerFunc { return sequencia(t, resposta{derruba: true}) }
	tests := []struct {
		name          string
		tls           bool
		api           func(t *testing.T) http.HandlerFunc
		chamar        func(c *Client) error
		wantType      string
		wantRetryable bool
		wantUncertain bool
		wantTLS       bool
	}{
		{
			name: "GET com certificado não confiável",
			tls:  true,
			api:  func(t *testing.T) http.HandlerFunc { return sequencia(t, resposta{status: 200, corpo: corpoOK}) },
			chamar: func(c *Client) error {
				_, err := c.Migracoes.ListarPagina(context.Background(), FiltroMigracoes{UC: ucInventada}, "")
				return err
			},
			wantType: "erro-de-transporte", wantRetryable: true, wantTLS: true,
		},
		{
			name: "POST com certificado não confiável não saiu e é seguro repetir",
			tls:  true,
			api:  func(t *testing.T) http.HandlerFunc { return sequencia(t, resposta{status: 200, corpo: corpoOK}) },
			chamar: func(c *Client) error {
				_, err := c.Migracoes.Criar(context.Background(), NovaMigracao{UC: ucInventada})
				return err
			},
			wantType: "erro-de-transporte", wantRetryable: true, wantTLS: true,
		},
		{
			name: "GET com a conexão fechada sem resposta",
			api:  derruba,
			chamar: func(c *Client) error {
				_, err := c.Migracoes.ListarPagina(context.Background(), FiltroMigracoes{UC: ucInventada}, "")
				return err
			},
			wantType: "erro-de-transporte", wantRetryable: true,
		},
		{
			name: "POST com a conexão fechada sem resposta é incerto",
			api:  derruba,
			chamar: func(c *Client) error {
				_, err := c.Migracoes.Criar(context.Background(), NovaMigracao{UC: ucInventada})
				return err
			},
			wantType: "resultado-incerto", wantUncertain: true,
		},
		{
			name: "GET com 200 e JSON inválido",
			api: func(t *testing.T) http.HandlerFunc {
				return sequencia(t, resposta{status: 200, header: map[string]string{"Content-Type": "application/json"}, corpo: `{"migracao": [`})
			},
			chamar: func(c *Client) error {
				_, err := c.Migracoes.ListarPagina(context.Background(), FiltroMigracoes{UC: ucInventada}, "")
				return err
			},
			wantType: "resposta-ilegivel", wantRetryable: true,
		},
		{
			name: "GET com o prazo da tentativa vencido",
			api: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					select {
					case <-r.Context().Done():
					case <-time.After(2 * time.Second):
					}
				}
			},
			chamar: func(c *Client) error {
				_, err := c.Migracoes.ListarPagina(context.Background(), FiltroMigracoes{UC: ucInventada}, "", WithTimeout(50*time.Millisecond))
				return err
			},
			wantType: "tempo-esgotado", wantRetryable: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var c *Client
			if tc.tls {
				srv := httptest.NewTLSServer(tc.api(t))
				t.Cleanup(srv.Close)
				c, _ = novoClienteComEnv(t, Environment{Name: "teste", APIURL: srv.URL, TokenURL: srv.URL + "/sso/oauth/token"},
					fonteFixa("token-inventado"), WithAgent(agenteInventado, perfilInventado), WithRateLimits(RateLimits{}))
			} else {
				s := novoServidor(t, tc.api(t))
				c, _ = novoClienteTeste(t, s, WithRetries(0))
			}
			err := tc.chamar(c)
			if err == nil {
				t.Fatal("quero erro")
			}
			p := ProblemFrom(err)
			if want := CatalogoErrosURL + "#" + tc.wantType; p.Type != want {
				t.Errorf("Type = %q, quero %q (erro: %v)", p.Type, want, err)
			}
			if p.Retryable != tc.wantRetryable {
				t.Errorf("Retryable = %v, quero %v", p.Retryable, tc.wantRetryable)
			}
			if p.Uncertain != tc.wantUncertain {
				t.Errorf("Uncertain = %v, quero %v", p.Uncertain, tc.wantUncertain)
			}
			if tc.wantTLS {
				var ua x509.UnknownAuthorityError
				if !errors.As(err, &ua) {
					t.Errorf("a causa x509 devia continuar acessível por errors.As: %v", err)
				}
			}
		})
	}
}

// TestProblemFrom_WhenTokenFalha_ClassificaPeloTipo cobre as falhas do pedido de
// token: transporte, endpoint com JSON sem access_token e fonte externa.
func TestProblemFrom_WhenTokenFalha_ClassificaPeloTipo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		cliente       func(t *testing.T) *Client
		wantType      string
		wantRetryable bool
	}{
		{
			name: "endpoint de token com conexão fechada",
			cliente: func(t *testing.T) *Client {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					hj, ok := w.(http.Hijacker)
					if !ok {
						t.Error("sem Hijack")
						return
					}
					conn, _, err := hj.Hijack()
					if err != nil {
						t.Errorf("Hijack: %v", err)
						return
					}
					if err := conn.Close(); err != nil {
						t.Errorf("fechar: %v", err)
					}
				}))
				t.Cleanup(srv.Close)
				c, _ := novoClienteComEnv(t, Environment{Name: "teste", APIURL: srv.URL, TokenURL: srv.URL},
					WithCredentials(idInventado, segredoInventado), WithAgent(agenteInventado, perfilInventado), WithRateLimits(RateLimits{}), WithRetries(0))
				return c
			},
			wantType: "erro-de-transporte", wantRetryable: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.cliente(t).Migracoes.ListarPagina(context.Background(), FiltroMigracoes{UC: ucInventada}, "")
			if err == nil {
				t.Fatal("quero erro")
			}
			p := ProblemFrom(err)
			if want := CatalogoErrosURL + "#" + tc.wantType; p.Type != want {
				t.Errorf("Type = %q, quero %q (erro: %v)", p.Type, want, err)
			}
			if p.Retryable != tc.wantRetryable {
				t.Errorf("Retryable = %v, quero %v", p.Retryable, tc.wantRetryable)
			}
		})
	}
}
