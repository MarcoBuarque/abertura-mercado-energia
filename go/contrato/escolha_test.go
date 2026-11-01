//go:build sandbox

package contrato

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	abertura "github.com/MarcoBuarque/abertura-mercado-energia/go"
)

func TestEscolha_WhenEntreAchadas_PegaUmaAtiva(t *testing.T) {
	m := func(id string, s abertura.StatusMigracao) abertura.Migracao {
		return abertura.Migracao{ID: id, Status: s, UC: "000000000000001", Concessionaria: 900003}
	}
	tests := []struct {
		name    string
		achadas []abertura.Migracao
		wantID  string
		wantOK  bool
	}{
		{name: "sem nenhuma", achadas: nil},
		{name: "só inativas", achadas: []abertura.Migracao{m("a", abertura.StatusCancelada), m("b", abertura.StatusEncerrada), m("c", abertura.StatusExpirada), m("d", abertura.StatusReprovada)}},
		{name: "pula as inativas e pega a primeira ativa", achadas: []abertura.Migracao{m("a", abertura.StatusCancelada), m("b", abertura.StatusCriada), m("c", abertura.StatusConcluida)}, wantID: "b", wantOK: true},
		{name: "status novo e desconhecido conta como ativo", achadas: []abertura.Migracao{m("a", abertura.StatusReprovada), m("b", "EM_ANALISE")}, wantID: "b", wantOK: true},
		{name: "status vazio conta como ativo", achadas: []abertura.Migracao{m("a", "")}, wantID: "a", wantOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := escolherAtiva(tc.achadas)
			if ok != tc.wantOK || got.ID != tc.wantID {
				t.Errorf("escolherAtiva = (%q, %v), quero (%q, %v)", got.ID, ok, tc.wantID, tc.wantOK)
			}
		})
	}
}

func TestEscolha_WhenListaAmpla_FiltraPorUCEConcessionaria(t *testing.T) {
	m := func(id, uc string, conc int) abertura.Migracao {
		return abertura.Migracao{ID: id, UC: uc, Concessionaria: conc, Mes: abertura.MesDe(2027, time.January)}
	}
	lista := []abertura.Migracao{
		m("a", "000000000000001", 900003), m("b", "000000000000002", 900003),
		m("c", "000000000000001", 900004), m("d", "000000000000001", 900003),
	}
	got := daUC(lista, "000000000000001", 900003)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "d" {
		t.Errorf("daUC = %v, quero a e d", got)
	}
	if got := daUC(lista, "000000000000001", 0); len(got) != 3 {
		t.Errorf("concessionária 0 não filtra: %d achadas, quero 3", len(got))
	}
}

func TestOrcamento_WhenCodigoForaDoPadrao_NaoImprimeOTexto(t *testing.T) {
	const texto = "contato@example.org quebra o padrão"
	tests := []struct {
		name  string
		corpo string
		want  string
	}{
		{name: "código no padrão", corpo: `{"error":"ERR_MIGRACAO_SOLICITADA"}`, want: "ERR_MIGRACAO_SOLICITADA"},
		{name: "sem o campo", corpo: `{"message":"x"}`, want: ""},
		{name: "texto livre", corpo: `{"error":"` + texto + `"}`, want: "código fora do padrão"},
		{name: "minúsculas", corpo: `{"error":"err_x"}`, want: "código fora do padrão"},
		{name: "código OAuth2 fixo da RFC 6749", corpo: `{"error":"invalid_request"}`, want: "invalid_request"},
		{name: "código OAuth2 com sufixo", corpo: `{"error":"invalid_requestX"}`, want: "código fora do padrão"},
		{name: "código OAuth2 em maiúsculas", corpo: `{"error":"INVALID_REQUEST"}`, want: "código fora do padrão"},
		{name: "longo demais", corpo: `{"error":"ERR_` + strings.Repeat("A", 81) + `"}`, want: "código fora do padrão"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := codigoDoErro([]byte(tc.corpo)); got != tc.want {
				t.Errorf("codigoDoErro = %q, quero %q", got, tc.want)
			}
		})
	}
	api := &abertura.APIError{Operation: "criarMigracao", StatusCode: 400, Code: texto}
	if d := descreve(api); strings.Contains(d, "contato@") {
		t.Errorf("descreve imprimiu o texto livre do campo error: %s", d)
	} else if !strings.Contains(d, "código fora do padrão") {
		t.Errorf("descreve devia marcar o código fora do padrão: %s", d)
	}
}

// baseFalsa devolve sempre a mesma resposta, sem rede.
type baseFalsa struct {
	status int
	corpo  string
}

func (b baseFalsa) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: b.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(b.corpo))}, nil
}

func TestOrcamento_WhenErroOAuth2_DicaSoParaDescricoesFixas(t *testing.T) {
	tests := []struct {
		name     string
		corpo    string
		wantDica string
		proibido string
	}{
		{name: "descrição fixa duplicated parameter", corpo: `{"error":"invalid_request","error_description":"duplicated parameter"}`, wantDica: "oauth2_error_description: duplicated parameter"},
		{name: "descrição fixa grant_type ausente", corpo: `{"error":"invalid_request","error_description":"Missing form parameter: grant_type"}`, wantDica: "oauth2_error_description: Missing form parameter: grant_type"},
		{name: "descrição livre não aparece", corpo: `{"error":"invalid_request","error_description":"contato@example.org livre"}`, proibido: "contato@"},
		{name: "variação da descrição fixa não aparece", corpo: `{"error":"invalid_request","error_description":"duplicated parameter!"}`, proibido: "duplicated"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o, err := novoOrcamento(baseFalsa{status: 400, corpo: tc.corpo}, 5)
			if err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequest(http.MethodPost, "https://exemplo.invalid/sso/oauth/token", strings.NewReader(""))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := o.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			linhas := strings.Join(o.descarregar(), "\n")
			if !strings.Contains(linhas, "code: invalid_request") {
				t.Errorf("linha sem o código OAuth2: %q", linhas)
			}
			if tc.wantDica != "" && !strings.Contains(linhas, tc.wantDica) {
				t.Errorf("linha sem a dica %q: %q", tc.wantDica, linhas)
			}
			if strings.Contains(linhas, "oauth2_error_description") && tc.wantDica == "" {
				t.Errorf("linha menciona a descrição sem ser fixa: %q", linhas)
			}
			if tc.proibido != "" && strings.Contains(linhas, tc.proibido) {
				t.Errorf("linha vaza %q: %q", tc.proibido, linhas)
			}
		})
	}
}

func TestEscolha_WhenFaixaLarga_ReduzPelaMetadeAteUmMes(t *testing.T) {
	got := []int{96}
	for n := 96; n > 1; {
		n = metadeDaFaixa(n)
		got = append(got, n)
	}
	want := []int{96, 48, 24, 12, 6, 3, 1}
	if len(got) != len(want) {
		t.Fatalf("sequência = %v, quero %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sequência = %v, quero %v", got, want)
		}
	}
	if metadeDaFaixa(1) != 1 {
		t.Errorf("metadeDaFaixa(1) = %d, quero 1", metadeDaFaixa(1))
	}
}

func TestEscolha_WhenFaixaCentrada_TemOTamanhoPedido(t *testing.T) {
	centro := abertura.MesDe(2026, time.November)
	tests := []struct {
		name         string
		n            int
		wantDe, want abertura.Mes
	}{
		{name: "um mês é o próprio mês", n: 1, wantDe: centro, want: centro},
		{name: "três meses", n: 3, wantDe: abertura.MesDe(2026, time.October), want: abertura.MesDe(2026, time.December)},
		{name: "doze meses cruzam o ano", n: 12, wantDe: abertura.MesDe(2026, time.May), want: abertura.MesDe(2027, time.April)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			de, ate := faixaCentrada(centro, tc.n)
			if de != tc.wantDe || ate != tc.want {
				t.Errorf("faixaCentrada = %v a %v, quero %v a %v", de, ate, tc.wantDe, tc.want)
			}
			if got := mesesNaFaixa(de, ate); got != tc.n {
				t.Errorf("mesesNaFaixa = %d, quero %d", got, tc.n)
			}
		})
	}
}

func TestEscolha_WhenBuscarFaixaAceita_EstreitaAteAAPIAceitar(t *testing.T) {
	centro := abertura.MesDe(2026, time.November)
	de0, ate0 := abertura.MesDe(2020, time.January), abertura.MesDe(2027, time.December)
	tests := []struct {
		name         string
		teto         int // maior faixa que o servidor aceita
		wantN        int
		wantRecusada int
		wantTent     int
		wantErr      bool
	}{
		{name: "aceita de primeira", teto: 200, wantN: 96, wantRecusada: 0, wantTent: 1},
		{name: "teto de 12 meses", teto: 12, wantN: 12, wantRecusada: 24, wantTent: 4},
		{name: "teto de 1 mês", teto: 1, wantN: 1, wantRecusada: 3, wantTent: 7},
		{name: "nem um mês passa", teto: 0, wantErr: true, wantTent: 7},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tent := 0
			tenta := func(de, ate abertura.Mes) (abertura.Pagina[abertura.Migracao], error) {
				tent++
				if mesesNaFaixa(de, ate) > tc.teto {
					return abertura.Pagina[abertura.Migracao]{}, abertura.ErrFaixaDeMesesLargaDemais
				}
				return abertura.Pagina[abertura.Migracao]{Proxima: "c"}, nil
			}
			r, err := buscarFaixaAceita(de0, ate0, centro, tenta)
			if tent != tc.wantTent {
				t.Errorf("tentativas = %d, quero %d", tent, tc.wantTent)
			}
			if tc.wantErr {
				if !errors.Is(err, abertura.ErrFaixaDeMesesLargaDemais) {
					t.Fatalf("err = %v, quero ErrFaixaDeMesesLargaDemais", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("erro: %v", err)
			}
			if r.Meses != tc.wantN || r.Recusada != tc.wantRecusada || r.Pagina.Proxima != "c" {
				t.Errorf("resultado = %d meses, recusada %d, pagina %q; quero %d, %d", r.Meses, r.Recusada, r.Pagina.Proxima, tc.wantN, tc.wantRecusada)
			}
		})
	}

	t.Run("outro erro não estreita", func(t *testing.T) {
		tent := 0
		boom := errors.New("boom")
		_, err := buscarFaixaAceita(de0, ate0, centro, func(_, _ abertura.Mes) (abertura.Pagina[abertura.Migracao], error) {
			tent++
			return abertura.Pagina[abertura.Migracao]{}, boom
		})
		if !errors.Is(err, boom) || tent != 1 {
			t.Errorf("err = %v, tentativas = %d; quero boom e 1", err, tent)
		}
	})
}
