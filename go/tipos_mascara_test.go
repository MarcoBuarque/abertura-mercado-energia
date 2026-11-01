package abertura

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// Dados inventados que nenhuma forma de impressão pode mostrar.
const (
	emailMascara      = "pessoa@example.org"
	ucMascara         = "000000000000123"
	consumoMascara    = "98765.4321"
	textoLivreMascara = "texto-livre-inventado"
	numeroDocumento   = "11111111111111"
)

func termosDePessoa() []string {
	return []string{emailMascara, "pessoa", ucMascara, "00000000000", consumoMascara, "98765", textoLivreMascara, numeroDocumento, "1111111111"}
}

func migracaoComDados(t *testing.T) Migracao {
	t.Helper()
	return Migracao{
		ID: "id-migracao-1", UC: ucMascara, Concessionaria: 900003, Supridora: 900004,
		Varejista: agenteInventado, Perfil: perfilInventado,
		Documento: DocumentoCNPJ(numeroDocumento), Email: emailMascara,
		Mes: MesDe(2027, time.January), Status: StatusCriada,
		Justificativa: textoLivreMascara, Comentario: textoLivreMascara,
	}
}

func medicaoComDados(t *testing.T) Medicao {
	t.Helper()
	consumo, err := decimalDe(json.Number(consumoMascara))
	if err != nil {
		t.Fatalf("decimalDe: %v", err)
	}
	return Medicao{ID: "id-medicao-1", UC: ucMascara, Concessionaria: 900003, Consumo: consumo, Situacao: SituacaoConsistida}
}

// formas imprime v em todas as formas de fmt e nos dois handlers do slog.
func formas(v any) map[string]string {
	out := map[string]string{
		"%v":  fmt.Sprintf("%v", v),
		"%+v": fmt.Sprintf("%+v", v),
		"%#v": fmt.Sprintf("%#v", v),
		"%s":  fmt.Sprintf("%s", v),
	}
	var texto, js bytes.Buffer
	slog.New(slog.NewTextHandler(&texto, nil)).Info("m", "v", v)
	slog.New(slog.NewJSONHandler(&js, nil)).Info("m", "v", v)
	out["slog texto"], out["slog json"] = texto.String(), js.String()
	return out
}

func TestTiposPublicos_WhenImpressos_NaoMostramDadoPessoal(t *testing.T) {
	t.Parallel()

	filtroM := FiltroMigracoes{De: MesDe(2027, time.January), Ate: MesDe(2027, time.March), UC: ucMascara, Status: StatusCriada, Perfil: perfilInventado}
	filtroC := FiltroMedicoes{UC: ucMascara, Concessionaria: 900003, Inicio: time.Date(2027, 1, 1, 0, 0, 0, 0, fusoBrasilia), Fim: time.Date(2027, 1, 31, 0, 0, 0, 0, fusoBrasilia), Situacao: SituacaoConsistida}
	tests := []struct {
		name  string
		valor any
		quero string // trecho que pode aparecer, para provar que a saída não é vazia
	}{
		{name: "Migracao", valor: migracaoComDados(t), quero: "id-migracao-1"},
		{name: "*Migracao", valor: func() *Migracao { m := migracaoComDados(t); return &m }(), quero: "id-migracao-1"},
		{name: "Medicao", valor: medicaoComDados(t), quero: "id-medicao-1"},
		{name: "NovaMigracao", valor: NovaMigracao{UC: ucMascara, Concessionaria: 900003, Documento: DocumentoCNPJ(numeroDocumento), Email: emailMascara, Mes: MesDe(2027, time.January), Comentario: textoLivreMascara}, quero: "900003"},
		{name: "EdicaoMigracao", valor: EdicaoMigracao{Mes: MesDe(2027, time.February), Perfil: perfilInventado, Documento: DocumentoCNPJ(numeroDocumento), Email: emailMascara, Status: StatusCancelada}, quero: "CANCELADA"},
		{name: "FiltroMigracoes", valor: filtroM, quero: "CRIADA"},
		{name: "FiltroMedicoes", valor: filtroC, quero: "CONSISTIDA"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for forma, saida := range formas(tc.valor) {
				if saida == "" {
					t.Errorf("%s: saída vazia", forma)
				}
				for _, proibido := range termosDePessoa() {
					if strings.Contains(saida, proibido) {
						t.Errorf("%s vaza %q: %s", forma, proibido, saida)
					}
				}
			}
			if s := fmt.Sprintf("%v", tc.valor); !strings.Contains(s, tc.quero) {
				t.Errorf("%%v = %q, devia manter %q (dado não pessoal)", s, tc.quero)
			}
		})
	}
}

func TestTiposPublicos_WhenSlice_NaoMostraDadoPessoalNoHandlerDeTexto(t *testing.T) {
	t.Parallel()

	lista := []Migracao{migracaoComDados(t), migracaoComDados(t)}
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("m", "lista", lista)
	for _, proibido := range termosDePessoa() {
		if strings.Contains(buf.String(), proibido) {
			t.Errorf("slog texto de []Migracao vaza %q: %s", proibido, buf.String())
		}
	}
	for _, forma := range []string{"%v", "%+v", "%#v"} {
		if s := fmt.Sprintf(forma, lista); strings.Contains(s, ucMascara) || strings.Contains(s, emailMascara) {
			t.Errorf("%s de []Migracao vaza: %s", forma, s)
		}
	}
}

// TestTiposPublicos_WhenJSON_LevaOsCamposEmClaro prova que a máscara não mexe
// no json.Marshal: o JSON é para guardar, e os campos continuam exportados.
func TestTiposPublicos_WhenJSON_LevaOsCamposEmClaro(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		valor any
		quero []string
	}{
		{name: "Migracao", valor: migracaoComDados(t), quero: []string{ucMascara, emailMascara}},
		{name: "Medicao", valor: medicaoComDados(t), quero: []string{ucMascara, consumoMascara}},
		{name: "NovaMigracao", valor: NovaMigracao{UC: ucMascara, Email: emailMascara}, quero: []string{ucMascara, emailMascara}},
		{name: "FiltroMigracoes", valor: FiltroMigracoes{De: deTst, Ate: ateTst, UC: ucMascara}, quero: []string{ucMascara}},
		{name: "FiltroMedicoes", valor: FiltroMedicoes{UC: ucMascara}, quero: []string{ucMascara}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(tc.valor)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			for _, q := range tc.quero {
				if !strings.Contains(string(b), q) {
					t.Errorf("o JSON devia levar %q em claro: %s", q, b)
				}
			}
		})
	}
}
