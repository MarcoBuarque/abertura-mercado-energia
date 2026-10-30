//go:build sandbox

package contrato

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Os testes deste arquivo rodam offline, contra um servidor local: provam o
// contador, a parada no primeiro 403 e a descrição sem valores.

func servidorLocal(t *testing.T, status int, corpo string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(corpo)); err != nil {
			t.Errorf("escrever: %v", err)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func pegar(t *testing.T, rt http.RoundTripper, url string) error {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("requisição: %v", err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func TestOrcamento_WhenPassaDoLimite_Falha(t *testing.T) {
	s := servidorLocal(t, 200, `{"migracao":[]}`)
	o, err := novoOrcamento(http.DefaultTransport, 3)
	if err != nil {
		t.Fatalf("novoOrcamento: %v", err)
	}
	for i := range 3 {
		if err := pegar(t, o, s.URL+"/v1.1/varejista/migracoes"); err != nil {
			t.Fatalf("requisição %d: %v", i+1, err)
		}
	}
	if err := pegar(t, o, s.URL+"/v1.1/varejista/migracoes"); !errors.Is(err, errOrcamento) {
		t.Fatalf("a 4ª devia estourar o orçamento: %v", err)
	}
}

func TestOrcamento_WhenPrimeiro403_ParaAsSeguintes(t *testing.T) {
	s := servidorLocal(t, 403, `{"error":"ERR_CREDENCIAL_INVALIDA","message":"x","status":403}`)
	o, err := novoOrcamento(http.DefaultTransport, 90)
	if err != nil {
		t.Fatalf("novoOrcamento: %v", err)
	}
	if err := pegar(t, o, s.URL+"/v1.1/varejista/migracoes"); err != nil {
		t.Fatalf("a primeira passa: %v", err)
	}
	if err := pegar(t, o, s.URL+"/v1.1/varejista/migracoes"); !errors.Is(err, errAbortada) {
		t.Fatalf("a segunda devia parar: %v", err)
	}
	if l := strings.Join(o.descarregar(), "\n"); !strings.Contains(l, "ERR_CREDENCIAL_INVALIDA") {
		t.Errorf("a linha devia trazer o code: %s", l)
	}
}

func TestOrcamento_WhenRespostaForaDaSpec_RegistraSemValores(t *testing.T) {
	const valor = "valor-secreto-123"
	s := servidorLocal(t, 200, `{"migracao":[{"idMigracao":"`+valor+`","statusMigracao":7}],"indexProximaPagina":null}`)
	o, err := novoOrcamento(http.DefaultTransport, 90)
	if err != nil {
		t.Fatalf("novoOrcamento: %v", err)
	}
	if err := pegar(t, o, s.URL+"/v1.1/varejista/migracoes"); err != nil {
		t.Fatalf("pegar: %v", err)
	}
	v := o.todasViolacoes()
	if len(v) == 0 {
		t.Fatal("esperava uma violação (statusMigracao numérico)")
	}
	all := strings.Join(v, "\n") + strings.Join(o.descarregar(), "\n")
	if strings.Contains(all, valor) {
		t.Errorf("valor vazou: %s", all)
	}
	if !strings.Contains(all, "idMigracao") {
		t.Errorf("os nomes de campo deviam aparecer: %s", all)
	}
}

func TestNomesDeCampos(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{name: "objeto com lista", in: `{"a":1,"migracao":[{"x":"v","y":2},{"z":1}]}`, want: "a,migracao,migracao.x,migracao.y"},
		{name: "lista na raiz", in: `[{"id":"v"}]`, want: "id"},
		{name: "não JSON", in: `Bad Gateway`, want: "corpo não JSON (11 bytes)"},
		{name: "vazio", in: `[]`, want: "(nenhum)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nomesDeCampos([]byte(tc.in)); got != tc.want {
				t.Errorf("got %q, quero %q", got, tc.want)
			}
		})
	}
}
