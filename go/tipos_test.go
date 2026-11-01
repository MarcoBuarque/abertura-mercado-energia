package abertura

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestTipos_WhenStatusMigracao_ConhecidoSoParaOsPublicados(t *testing.T) {
	tests := []struct {
		name   string
		status StatusMigracao
		want   bool
	}{
		{name: "CRIADA", status: StatusCriada, want: true},
		{name: "CONCLUIDA", status: StatusConcluida, want: true},
		{name: "REPROVADA", status: StatusReprovada, want: true},
		{name: "ENCERRADA", status: StatusEncerrada, want: true},
		{name: "CANCELADA", status: StatusCancelada, want: true},
		{name: "EXPIRADA", status: StatusExpirada, want: true},
		{name: "APROVADA_COM_PENDENCIA", status: StatusAprovadaComPendencia, want: true},
		{name: "valor novo da API passa, mas não é conhecido", status: "EM_ANALISE", want: false},
		{name: "vazio não é conhecido", status: "", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.status.Conhecido(); got != tc.want {
				t.Errorf("Conhecido() = %v, queria %v", got, tc.want)
			}
		})
	}
}

func TestTipos_WhenSituacaoMedicao_Conhecido(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		situacao SituacaoMedicao
		want     bool
	}{
		{"consistida é conhecida", SituacaoConsistida, true},
		{"rejeitada é conhecida", SituacaoRejeitada, true},
		{"valor novo não é conhecido", SituacaoMedicao("NOVA"), false},
		{"vazio não é conhecido", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.situacao.Conhecido(); got != tc.want {
				t.Errorf("Conhecido() = %v, queria %v", got, tc.want)
			}
		})
	}
	if SituacaoRejeitada != "REJEITADA" {
		t.Errorf("SituacaoRejeitada = %q", SituacaoRejeitada)
	}
}

func TestTipos_WhenLogValue_NaoVazaDadoPessoal(t *testing.T) {
	consumo, err := decimalDe("987.654")
	if err != nil {
		t.Fatal(err)
	}
	mig := Migracao{
		ID:           "00000000-0000-0000-0000-000000000001",
		UC:           "000000000000001",
		Email:        "pessoa@example.org",
		Documento:    DocumentoCNPJ(cnpjInventado),
		Mes:          MesDe(2026, time.December),
		Status:       StatusCriada,
		ValorDHC:     consumo,
		DataDenuncia: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
	med := Medicao{
		ID:       "00000000-0000-0000-0000-000000000002",
		UC:       "000000000000001",
		Consumo:  consumo,
		Situacao: SituacaoConsistida,
	}

	tests := []struct {
		name        string
		valor       slog.LogValuer
		devePossuir []string
		naoDeve     []string
	}{
		{
			name:        "migração",
			valor:       mig,
			devePossuir: []string{mig.ID, "CRIADA", "2026-12"},
			naoDeve:     []string{"pessoa@example.org", "example.org", cnpjInventado, "1111", "000000000000001", "987.654"},
		},
		{
			name:        "medição",
			valor:       med,
			devePossuir: []string{med.ID, "CONSISTIDA"},
			naoDeve:     []string{"000000000000001", "987.654", "987"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			saida := slogJSONSemTempo("item", tc.valor)
			t.Logf("saída: %s", saida)
			for _, quer := range tc.devePossuir {
				if !strings.Contains(saida, quer) {
					t.Errorf("faltou %q em %s", quer, saida)
				}
			}
			for _, nao := range tc.naoDeve {
				if strings.Contains(saida, nao) {
					t.Errorf("vazou %q em %s", nao, saida)
				}
			}
		})
	}
}

func TestTipos_WhenChaveString_NaoTrazDadoPessoal(t *testing.T) {
	desejado := EdicaoMigracao{
		Mes:       MesDe(2026, time.December),
		Email:     "pessoa@example.org",
		Documento: DocumentoCNPJ(cnpjInventado),
		Status:    StatusCancelada,
	}
	tests := []struct {
		name    string
		chave   interface{ String() string }
		want    string
		naoDeve []string
	}{
		{
			name:    "migração",
			chave:   ChaveMigracao{UC: "000000000000001", Concessionaria: 900001, Mes: MesDe(2026, time.December), Perfil: 900002},
			want:    "migracao uc=***********0001 concessionaria=900001 mes=2026-12 perfil=900002",
			naoDeve: []string{"000000000000001", cnpjInventado},
		},
		{
			name:    "edição",
			chave:   ChaveEdicao{ID: "00000000-0000-0000-0000-000000000001", Desejado: desejado},
			want:    "edicao id=00000000-0000-0000-0000-000000000001 mes=2026-12 perfil=0 status=CANCELADA",
			naoDeve: []string{"example.org", cnpjInventado, "1111"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.chave.String()
			if got != tc.want {
				t.Errorf("String() = %q, queria %q", got, tc.want)
			}
			for _, nao := range tc.naoDeve {
				if strings.Contains(got, nao) {
					t.Errorf("vazou %q em %q", nao, got)
				}
			}
		})
	}
}

func TestTipos_WhenChaveNatural_AsDuasImplementam(t *testing.T) {
	// A interface ChaveNatural é da task 05; aqui só se prova que o método existe.
	var _ interface{ chaveNatural() } = ChaveMigracao{}
	var _ interface{ chaveNatural() } = ChaveEdicao{}
}

func TestTipos_WhenMascarar_MantemSoOsUltimos(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{name: "mantém quatro", in: "123456", n: 4, want: "**3456"},
		{name: "curto demais mascara tudo", in: "123", n: 4, want: "***"},
		{name: "vazio", in: "", n: 4, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := mascarar(tc.in, tc.n); got != tc.want {
				t.Errorf("mascarar(%q, %d) = %q, queria %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}

func TestTipos_WhenMigracaoComoJSON_DocumentoSaiMascarado(t *testing.T) {
	b, err := json.Marshal(Migracao{Documento: DocumentoCNPJ(cnpjInventado)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "111") {
		t.Errorf("JSON traz o número: %s", b)
	}
}
