//go:build sandbox

package contrato

import (
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
