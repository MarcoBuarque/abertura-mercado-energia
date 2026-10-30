package abertura

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestProblemFrom_WhenErroNaoClassificado_UsaForaDoCatalogo prova que o
// ProblemFrom nunca devolve about:blank: o que não tem entrada própria cai em
// #fora-do-catalogo, com o título da entrada.
func TestProblemFrom_WhenErroNaoClassificado_UsaForaDoCatalogo(t *testing.T) {
	t.Parallel()

	p := ProblemFrom(errors.New("outra coisa"))
	if want := CatalogoErrosURL + "#fora-do-catalogo"; p.Type != want {
		t.Errorf("Type = %q, quero %q", p.Type, want)
	}
	if p.Title != "Erro fora do catálogo" {
		t.Errorf("Title = %q", p.Title)
	}
	if strings.Contains(p.Detail, "outra coisa") {
		t.Errorf("Detail copiou o texto do erro: %q", p.Detail)
	}
}

// TestProblem_MarshalJSON_WhenTypeVazioOuAboutBlank_SegueARFC prova que o
// type vazio sai omitido (a RFC 9457 o trata como about:blank) e que, com
// about:blank, o title é a frase padrão do status HTTP, ou some sem status.
func TestProblem_MarshalJSON_WhenTypeVazioOuAboutBlank_SegueARFC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		p         Problem
		wantType  any
		wantTitle any
	}{
		{name: "valor zero omite type e title", p: Problem{}},
		{name: "type vazio com status usa a frase do status", p: Problem{Status: 404, Title: "Outro"}, wantTitle: "Not Found"},
		{name: "about:blank com status usa a frase do status", p: Problem{Type: "about:blank", Status: 503, Title: "Outro"}, wantType: "about:blank", wantTitle: "Service Unavailable"},
		{name: "about:blank sem status omite o title", p: Problem{Type: "about:blank", Title: "Outro"}, wantType: "about:blank"},
		{name: "type do catálogo mantém o title", p: Problem{Type: CatalogoErrosURL + "#nao-encontrado", Status: 404, Title: "Recurso não encontrado"}, wantType: CatalogoErrosURL + "#nao-encontrado", wantTitle: "Recurso não encontrado"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(tc.p)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if m["type"] != tc.wantType {
				t.Errorf("type = %v, quero %v (%s)", m["type"], tc.wantType, b)
			}
			if m["title"] != tc.wantTitle {
				t.Errorf("title = %v, quero %v (%s)", m["title"], tc.wantTitle, b)
			}
		})
	}
}

// TestProblemFrom_WhenFalhaDeToken_UsaFalhaAoObterToken cobre as falhas do
// pedido de token que não são resposta de erro nem rede.
func TestProblemFrom_WhenFalhaDeToken_UsaFalhaAoObterToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cliente  func(t *testing.T) *Client
		wantType string
	}{
		{
			name: "endpoint de token sem access_token",
			cliente: func(t *testing.T) *Client {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					escrever(t, w, `{"token_type":"Bearer"}`)
				}))
				t.Cleanup(srv.Close)
				c, _ := novoClienteComEnv(t, Environment{Name: "teste", APIURL: srv.URL, TokenURL: srv.URL},
					WithCredentials(idInventado, segredoInventado), WithAgent(agenteInventado, perfilInventado), WithRateLimits(RateLimits{}), WithRetries(0))
				return c
			},
			wantType: "falha-ao-obter-token",
		},
		{
			name: "fonte externa que falha",
			cliente: func(t *testing.T) *Client {
				c, _ := novoClienteComEnv(t, Environment{Name: "teste", APIURL: "http://127.0.0.1:1", TokenURL: "http://127.0.0.1:1"}, //nolint:gosec // G101: URL inventada de teste, não é credencial
					WithTokenSource(fonteQueFalha{err: errors.New("fonte indisponível")}), WithAgent(agenteInventado, perfilInventado), WithRateLimits(RateLimits{}), WithRetries(0))
				return c
			},
			wantType: "falha-ao-obter-token",
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
			if p.Retryable || p.Uncertain {
				t.Errorf("sem informação para afirmar: Retryable=%v Uncertain=%v", p.Retryable, p.Uncertain)
			}
		})
	}
}
