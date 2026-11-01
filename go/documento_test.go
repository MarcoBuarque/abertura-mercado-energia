package abertura

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Documentos inventados, com todos os dígitos iguais.
const (
	cnpjInventado = "11111111111111"
	cpfInventado  = "22222222222"
)

func TestDocumento_WhenNumero_DevolveSemPontuacao(t *testing.T) {
	tests := []struct {
		name string
		doc  Documento
		tipo TipoDocumento
		want string
	}{
		{name: "cnpj com pontuação", doc: DocumentoCNPJ("11.111.111/1111-11"), tipo: TipoCNPJ, want: cnpjInventado},
		{name: "cpf com pontuação", doc: DocumentoCPF("222.222.222-22"), tipo: TipoCPF, want: cpfInventado},
		{name: "cnpj alfanumérico em maiúsculas", doc: DocumentoCNPJ("1a.111.111/1111-11"), tipo: TipoCNPJ, want: "1A111111111111"},
		{name: "sem validar dígito", doc: DocumentoCPF("123"), tipo: TipoCPF, want: "123"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.doc.Numero(); got != tc.want {
				t.Errorf("Numero() = %q, queria %q", got, tc.want)
			}
			if got := tc.doc.Tipo(); got != tc.tipo {
				t.Errorf("Tipo() = %q, queria %q", got, tc.tipo)
			}
		})
	}
}

func TestDocumento_WhenImpresso_NaoMostraONumero(t *testing.T) {
	doc := DocumentoCNPJ("11.111.111/1111-11")
	mig := Migracao{
		ID:        "00000000-0000-0000-0000-000000000001",
		UC:        "000000000000001",
		Email:     "contato@example.org",
		Documento: doc,
		Status:    StatusCriada,
	}
	impr := func(verbo string, v any) string { return fmt.Sprintf(verbo, v) }
	jsonSlog := func(v any) string { return slogJSONSemTempo("valor", v) }
	jsonStd := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		return string(b)
	}

	tests := []struct {
		name string
		got  string
	}{
		{name: "%v do documento", got: impr("%v", doc)},
		{name: "%+v do documento", got: impr("%+v", doc)},
		{name: "%#v do documento", got: impr("%#v", doc)},
		{name: "%s do documento", got: impr("%s", doc)}, //nolint:gosimple // prova o verbo %s
		{name: "%d do documento", got: impr("%d", doc)},
		{name: "%x do documento", got: impr("%x", doc)},
		{name: "%v do ponteiro", got: impr("%v", &doc)},
		{name: "slog JSON do documento", got: jsonSlog(doc)},
		{name: "json.Marshal do documento", got: jsonStd(doc)},
		{name: "json.Marshal de mapa com documento", got: jsonStd(map[string]Documento{"d": doc})},
		{name: "slog JSON do documento da migração", got: jsonSlog(mig.Documento)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("saída: %s", tc.got)
			if strings.Contains(tc.got, "111") {
				t.Errorf("a saída traz o número (ou parte dele): %s", tc.got)
			}
			if !strings.Contains(tc.got, "CNPJ") {
				t.Errorf("a saída não traz o tipo: %s", tc.got)
			}
			if !strings.Contains(tc.got, "11") {
				t.Errorf("a saída não traz os dois últimos dígitos: %s", tc.got)
			}
		})
	}
}

// Migracao não inclui o documento em nenhuma forma de impressão: nem o número
// nem o tipo (decisão da arquitetura, seção 3.4).
func TestDocumento_WhenMigracaoImpressa_NaoTrazDocumento(t *testing.T) {
	mig := Migracao{ID: "00000000-0000-0000-0000-000000000001", Documento: DocumentoCNPJ(cnpjInventado), Email: "contato@example.org"}
	for _, verbo := range []string{"%v", "%+v", "%#v", "%s"} {
		for nome, v := range map[string]any{"valor": mig, "ponteiro": &mig} {
			got := fmt.Sprintf(verbo, v)
			if strings.Contains(got, "111") || strings.Contains(got, "CNPJ") || strings.Contains(got, "@") {
				t.Errorf("%s do %s da migração trouxe dado pessoal: %s", verbo, nome, got)
			}
		}
	}
}

func TestDocumento_WhenMigracaoNoSlog_NaoTrazDocumento(t *testing.T) {
	mig := Migracao{ID: "00000000-0000-0000-0000-000000000001", Documento: DocumentoCNPJ(cnpjInventado)}
	saida := slogJSONSemTempo("valor", mig)
	if strings.Contains(saida, "111") || strings.Contains(saida, "CNPJ") {
		t.Errorf("slog da migração trouxe o documento: %s", saida)
	}
}

func TestDocumento_WhenString_MascaraTudoMenosDoisUltimos(t *testing.T) {
	tests := []struct {
		name string
		doc  Documento
		want string
	}{
		{name: "cnpj", doc: DocumentoCNPJ(cnpjInventado), want: "CNPJ ************11"},
		{name: "cpf", doc: DocumentoCPF(cpfInventado), want: "CPF *********22"},
		{name: "número curto sai todo mascarado", doc: DocumentoCPF("12"), want: "CPF **"},
		{name: "ausente é vazio", doc: Documento{}, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.doc.String(); got != tc.want {
				t.Errorf("String() = %q, queria %q", got, tc.want)
			}
			if got := tc.doc.GoString(); got != tc.want {
				t.Errorf("GoString() = %q, queria %q", got, tc.want)
			}
			if got := tc.doc.IsZero(); got != (tc.want == "") {
				t.Errorf("IsZero() = %v", got)
			}
		})
	}
}
