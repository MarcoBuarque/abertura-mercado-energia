package abertura

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/MarcoBuarque/abertura-mercado-energia/go/internal/version"
)

// As opções que valem no cliente e na chamada têm de servir aos dois lados,
// com checagem do compilador.
var (
	_ Option     = WithAgent(1, 2)
	_ CallOption = WithAgent(1, 2)
	_ Option     = WithTimeout(time.Second)
	_ CallOption = WithTimeout(time.Second)
)

func TestClient_NewClientValidaAConfiguracao(t *testing.T) {
	t.Parallel()

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "token-inventado-1"})
	agente := WithAgent(agenteInventado, perfilInventado)
	cred := WithCredentials(idInventado, segredoInventado)
	env := Environment{Name: "x", APIURL: "https://api.exemplo.invalid", TokenURL: "https://api.exemplo.invalid/sso/oauth/token"}

	tests := []struct {
		name    string
		env     Environment
		opts    []Option
		wantErr string // trecho da mensagem; vazio = sem erro
	}{
		{"credenciais e agente válidos", env, []Option{cred, agente}, ""},
		{"fonte de token e agente válidos", env, []Option{WithTokenSource(ts), agente}, ""},
		{"sem credenciais nem fonte de token", env, []Option{agente}, "credenciais"},
		{"credenciais sem client_id", env, []Option{WithCredentials("", segredoInventado), agente}, "credenciais"},
		{"credenciais sem client_secret", env, []Option{WithCredentials(idInventado, ""), agente}, "credenciais"},
		{"credenciais e fonte de token juntas", env, []Option{cred, WithTokenSource(ts), agente}, "WithTokenSource"},
		{"sem agente", env, []Option{cred}, "WithAgent"},
		{"agente zero", env, []Option{cred, WithAgent(0, perfilInventado)}, "WithAgent"},
		{"perfil negativo", env, []Option{cred, WithAgent(agenteInventado, -1)}, "WithAgent"},
		{"ambiente sem APIURL", Environment{TokenURL: env.TokenURL}, []Option{cred, agente}, "APIURL"},
		{"APIURL sem esquema", Environment{APIURL: "api.exemplo.invalid", TokenURL: env.TokenURL}, []Option{cred, agente}, "APIURL"},
		{"APIURL com query", Environment{APIURL: "https://api.exemplo.invalid/?a=b", TokenURL: env.TokenURL}, []Option{cred, agente}, "APIURL"},
		{"credenciais sem TokenURL", Environment{APIURL: env.APIURL}, []Option{cred, agente}, "TokenURL"},
		{"fonte de token dispensa TokenURL", Environment{APIURL: env.APIURL}, []Option{WithTokenSource(ts), agente}, ""},
		{"timeout zero", env, []Option{cred, agente, WithTimeout(0)}, "WithTimeout"},
		{"timeout negativo", env, []Option{cred, agente, WithTimeout(-time.Second)}, "WithTimeout"},
		{"retries negativos", env, []Option{cred, agente, WithRetries(-1)}, "WithRetries"},
		{"retries zero é válido", env, []Option{cred, agente, WithRetries(0)}, ""},
		{"limite com requisições e sem janela", env, []Option{cred, agente, WithRateLimits(RateLimits{Default: RateLimit{Requests: 10}})}, "limite"},
		{"limite negativo", env, []Option{cred, agente, WithRateLimits(RateLimits{Medicoes: RateLimit{Requests: -1, Per: time.Minute}})}, "limite"},
		{"limite desligado é válido", env, []Option{cred, agente, WithRateLimits(RateLimits{})}, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, err := NewClient(tc.env, tc.opts...)
			if tc.wantErr == "" {
				if err != nil || c == nil {
					t.Fatalf("NewClient = %v, %v", c, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("esperava erro com %q", tc.wantErr)
			}
			if c != nil {
				t.Error("com erro, o cliente devia ser nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("erro = %q, quer trecho %q", err, tc.wantErr)
			}
			assertSemVazamento(t, err.Error())
		})
	}
}

func TestClient_PadroesEOpcoes(t *testing.T) {
	t.Parallel()

	env := Environment{APIURL: "https://api.exemplo.invalid", TokenURL: "https://api.exemplo.invalid/t"}
	base := []Option{WithCredentials(idInventado, segredoInventado), WithAgent(agenteInventado, perfilInventado)}

	padrao, err := NewClient(env, base...)
	if err != nil {
		t.Fatal(err)
	}
	if padrao.timeout != 30*time.Second || padrao.retries != 2 {
		t.Errorf("padrões: timeout=%v retries=%d, quer 30 s e 2", padrao.timeout, padrao.retries)
	}
	if padrao.agente != agenteInventado || padrao.perfil != perfilInventado {
		t.Errorf("agente/perfil = %d/%d", padrao.agente, padrao.perfil)
	}
	if want := "abertura-mercado-energia-go/" + version.Version + " (+https://github.com/MarcoBuarque/abertura-mercado-energia)"; padrao.userAgnt != want {
		t.Errorf("User-Agent = %q, quer %q", padrao.userAgnt, want)
	}

	custom, err := NewClient(env, append(base, WithTimeout(5*time.Second), WithRetries(4), WithUserAgent("app-inventado/1.0"))...)
	if err != nil {
		t.Fatal(err)
	}
	if custom.timeout != 5*time.Second || custom.retries != 4 {
		t.Errorf("timeout=%v retries=%d", custom.timeout, custom.retries)
	}
	if !strings.HasSuffix(custom.userAgnt, " app-inventado/1.0") {
		t.Errorf("User-Agent = %q", custom.userAgnt)
	}

	// A opção mais tardia vence.
	ultimo, err := NewClient(env, append(base, WithAgent(900011, 900012))...)
	if err != nil {
		t.Fatal(err)
	}
	if ultimo.agente != 900011 || ultimo.perfil != 900012 {
		t.Errorf("agente/perfil = %d/%d", ultimo.agente, ultimo.perfil)
	}
}

func TestClient_OpcoesDeChamada(t *testing.T) {
	t.Parallel()

	c, err := NewClient(Environment{APIURL: "https://a.invalid", TokenURL: "https://a.invalid/t"},
		WithCredentials(idInventado, segredoInventado), WithAgent(agenteInventado, perfilInventado), WithTimeout(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		opts        []CallOption
		wantAgente  int
		wantPerfil  int
		wantTimeout time.Duration
	}{
		{"sem opções usa o cliente", nil, agenteInventado, perfilInventado, 7 * time.Second},
		{"WithAgent troca agente e perfil", []CallOption{WithAgent(900011, 900012)}, 900011, 900012, 7 * time.Second},
		{"WithTimeout troca o prazo", []CallOption{WithTimeout(time.Second)}, agenteInventado, perfilInventado, time.Second},
		{"timeout zero na chamada é ignorado", []CallOption{WithTimeout(0)}, agenteInventado, perfilInventado, 7 * time.Second},
		{"opção nil é ignorada", []CallOption{nil}, agenteInventado, perfilInventado, 7 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := c.parametros(novoCallConfig(tc.opts...))
			if p.agente != tc.wantAgente || p.perfil != tc.wantPerfil || p.timeout != tc.wantTimeout {
				t.Errorf("parametros = %+v", p)
			}
		})
	}
}

func TestClient_AmbientesDaDocumentacaoPublica(t *testing.T) {
	t.Parallel()

	cincoMin := 5 * time.Minute
	tests := []struct {
		name string
		env  Environment
		want Environment
	}{
		{
			name: "Sandbox",
			env:  Sandbox,
			want: Environment{
				Name:       "sandbox",
				APIURL:     "https://sandbox-api-abm.ccee.org.br",
				TokenURL:   "https://sandbox-api-abm.ccee.org.br/sso/oauth/token",
				RateLimits: RateLimits{Default: RateLimit{Requests: 900, Per: cincoMin}},
			},
		},
		{
			name: "Production",
			env:  Production,
			want: Environment{
				Name:       "production",
				APIURL:     "https://api-abm.ccee.org.br",
				TokenURL:   "https://api-abm.ccee.org.br/sso/oauth/token",
				RateLimits: RateLimits{Default: RateLimit{Requests: 900, Per: cincoMin}, Medicoes: RateLimit{Requests: 9000, Per: cincoMin}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.env != tc.want {
				t.Errorf("%s = %+v, quer %+v", tc.name, tc.env, tc.want)
			}
			l := novosLimitadores(tc.env.RateLimits)
			if l.geral == nil || l.geral.Burst() != 30 {
				t.Errorf("rajada do balde geral = %v, quer 30", l.geral)
			}
			c, err := NewClient(tc.env, WithCredentials(idInventado, segredoInventado), WithAgent(agenteInventado, perfilInventado))
			if err != nil {
				t.Fatal(err)
			}
			if c.limites.geral == nil {
				t.Error("o cliente devia ter o limitador do ambiente")
			}
			if (tc.name == "Production") == (c.limites.medicoes == c.limites.geral) {
				t.Errorf("balde de medições compartilhado = %v", c.limites.medicoes == c.limites.geral)
			}
		})
	}
}

func TestClient_WithRateLimitsSobrepoeOAmbiente(t *testing.T) {
	t.Parallel()

	c, err := NewClient(Sandbox, WithCredentials(idInventado, segredoInventado), WithAgent(agenteInventado, perfilInventado),
		WithRateLimits(RateLimits{}))
	if err != nil {
		t.Fatal(err)
	}
	if c.limites.geral != nil || c.limites.medicoes != nil {
		t.Error("RateLimits{} devia desligar o limitador")
	}
}

func TestClient_FormatarNaoVazaOSegredo(t *testing.T) {
	t.Parallel()

	c, err := NewClient(Sandbox, WithCredentials(idInventado, segredoInventado), WithAgent(agenteInventado, perfilInventado))
	if err != nil {
		t.Fatal(err)
	}
	// Um token em cache e uma chamada feita não podem mudar o resultado.
	formas := map[string]string{
		"%v":           fmt.Sprintf("%v", c),
		"%+v":          fmt.Sprintf("%+v", c),
		"%#v":          fmt.Sprintf("%#v", c),
		"%s":           fmt.Sprintf("%s", c),
		"%+v do valor": fmt.Sprintf("%+v", *c),
		"%#v do valor": fmt.Sprintf("%#v", *c),
		"%+v da fonte": fmt.Sprintf("%+v", c.fonte),
		"%#v da fonte": fmt.Sprintf("%#v", c.fonte),
	}
	for forma, texto := range formas {
		for _, proibido := range []string{segredoInventado, "Bearer"} {
			if strings.Contains(texto, proibido) {
				t.Errorf("%s contém %q: %s", forma, proibido, texto)
			}
		}
	}
	if got := c.String(); !strings.Contains(got, "sandbox") {
		t.Errorf("String = %q, devia citar o ambiente", got)
	}
	if c.GoString() != c.String() {
		t.Errorf("GoString = %q, String = %q", c.GoString(), c.String())
	}
}

func TestClient_NaoSegueRedirecionamento(t *testing.T) {
	t.Parallel()

	alvo := novoServidor(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("o redirecionamento foi seguido, com os cabeçalhos do agente")
	})
	srv := novoServidor(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, alvo.URL+"/outro", http.StatusFound)
	})
	c, _ := novoClienteTeste(t, srv)

	_, _, err := c.enviar(context.Background(), opLeitura(), callConfig{})
	if err == nil || !strings.Contains(err.Error(), "HTTP 302") {
		t.Fatalf("erro = %v, quer APIError 302", err)
	}
	if alvo.reqs.Load() != 0 {
		t.Error("o destino do redirecionamento recebeu requisição")
	}
}
