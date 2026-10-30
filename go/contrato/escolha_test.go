//go:build sandbox

package contrato

import (
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
