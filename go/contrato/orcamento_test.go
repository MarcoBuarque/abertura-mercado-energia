//go:build sandbox

package contrato

import (
	"errors"
	"fmt"
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

func TestDocumentoDoAmbiente(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		valor string
		tipo  string
		ok    bool
	}{
		{"onze caracteres é CPF", "000.000.000-00", "CPF", true},
		{"catorze caracteres é CNPJ", "00.000.000/0000-00", "CNPJ", true},
		{"CNPJ alfanumérico", "AB.CDE.FGH/0001-00", "CNPJ", true},
		{"vazio é recusado", "", "", false},
		{"tamanho estranho é recusado", "123", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, ok := documentoDoAmbiente(tc.valor)
			if ok != tc.ok || string(d.Tipo()) != tc.tipo {
				t.Fatalf("documentoDoAmbiente = %q, %v; quer %q, %v", d.Tipo(), ok, tc.tipo, tc.ok)
			}
		})
	}
}

func TestOrcamento_FormatosDeCampos(t *testing.T) {
	longa := strings.Repeat("a1", 30)
	tests := []struct {
		name, in, want string
	}{
		{name: "objeto simples", in: `{"mesReferencia":"2026-12","outro":"x"}`, want: "mesReferencia=9999-99"},
		{name: "aninhado em lista só o primeiro item", in: `{"migracao":[{"dataReferencia":"01-2026"},{"dataReferencia":"2026-01-01"}]}`, want: "dataReferencia=99-9999"},
		{name: "data e hora com letras", in: `{"timestamp":"2026-01-02T03:04:05.678-03:00"}`, want: "timestamp=9999-99-99A99:99:99.999-99:99"},
		{name: "primeira ocorrência vence", in: `{"a":{"dataDenuncia":"2026-01-02"},"dataDenuncia":"x"}`, want: "dataDenuncia=9999-99-99"},
		{name: "nulo", in: `{"dataValidacao":null}`, want: "dataValidacao=null"},
		{name: "número vira noves", in: `{"expires_in":300}`, want: "expires_in=999"},
		{name: "valor longo é cortado em 40", in: `{"dataSolicitacao":"` + longa + `"}`, want: "dataSolicitacao=" + strings.Repeat("A9", 20) + "…"},
		{name: "campo fora da lista não aparece", in: `{"emailUnidadeConsumidora":"a@b.c","codigoUnidadeConsumidora":"123"}`, want: ""},
		{name: "vários, em ordem de nome", in: `{"mesReferencia":"2026-12","dataReferencia":"2026-12-01"}`, want: "dataReferencia=9999-99-99, mesReferencia=9999-99"},
		{name: "não JSON", in: `Bad Gateway`, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatosDeCampos([]byte(tc.in))
			if got != tc.want {
				t.Errorf("got %q, quero %q", got, tc.want)
			}
			if strings.Contains(got, "2026") || strings.Contains(got, "a@b") {
				t.Errorf("valor vazou: %q", got)
			}
		})
	}
}

func TestOrcamento_WhenCampoTemporal_LinhaTrazFormatosSemValor(t *testing.T) {
	s := servidorLocal(t, 200, `{"migracao":[{"mesReferencia":"2026-12","idMigracao":"valor-secreto"}]}`)
	o, err := novoOrcamento(http.DefaultTransport, 90)
	if err != nil {
		t.Fatalf("novoOrcamento: %v", err)
	}
	if err := pegar(t, o, s.URL+"/v1.1/varejista/migracoes"); err != nil {
		t.Fatalf("pegar: %v", err)
	}
	l := strings.Join(o.descarregar(), "\n")
	if !strings.Contains(l, "formatos: mesReferencia=9999-99") {
		t.Errorf("a linha devia trazer a máscara: %s", l)
	}
	if strings.Contains(l, "2026-12") || strings.Contains(l, "valor-secreto") {
		t.Errorf("valor vazou: %s", l)
	}
}

func TestOrcamento_Descreve_WhenErroDeCampoIlegivel_NomeiaOCampo(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "campo ilegível embrulhado", err: fmt.Errorf("obterMigracao: %w", errors.New(`o campo "dataReferencia" da resposta tem um valor ilegível`)), want: "campo ilegível na resposta: dataReferencia"},
		{name: "campo aninhado", err: errors.New(`x: o campo "a.b_c" da resposta tem um valor ilegível`), want: "campo ilegível na resposta: a.b_c"},
		{name: "outro erro mantém o tipo", err: errors.New("algo com valor-secreto"), want: "erro do tipo *errors.errorString"},
		{name: "nome com caractere fora do padrão não casa", err: errors.New(`o campo "a b@c" da resposta tem um valor ilegível`), want: "erro do tipo *errors.errorString"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := descreve(tc.err); got != tc.want {
				t.Errorf("got %q, quero %q", got, tc.want)
			}
		})
	}
}
