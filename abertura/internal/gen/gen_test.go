package gen_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura/internal/gen"
)

// Compile-time assertions: every overlay action must take effect. If the
// JSONPath of an action stops matching, the generator stays silent and the
// type changes back; these lines then stop compiling.
var (
	_ json.Number = gen.MigracaoResumo{}.ValorDHC
	_ json.Number = gen.MigracaoResumo{}.ValorMusd
	_ json.Number = gen.MigracaoDetalhe{}.ValorDHC
	_ json.Number = gen.MigracaoDetalhe{}.ValorMusd
	_ json.Number = gen.Medicao{}.Consumo
	_ json.Number = gen.Medicao{}.CodigoAgenteConcessionaria
	_ int         = gen.Token{}.NotBeforePolicy
	// Dates and e-mails of the request body become strings (no openapi_types).
	_ string = gen.NovaMigracao{}.DataDenuncia
	_ string = gen.NovaMigracao{}.EmailUnidadeConsumidora
	_ string = gen.SolicitacaoSuspensao{}.DataNotificacao
	_ string = gen.EdicaoMigracao{}.EmailUnidadeConsumidora
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	root, err := os.OpenRoot(filepath.Join("..", "..", "testdata"))
	if err != nil {
		t.Fatalf("open testdata: %v", err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Errorf("close testdata: %v", err)
		}
	}()
	b, err := root.ReadFile(name)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return b
}

func TestListaMigracoes_WhenTolerantFixture_DecodesWithoutError(t *testing.T) {
	t.Parallel()
	var l gen.ListaMigracoes
	if err := json.Unmarshal(readFixture(t, "migrations_tolerance.json"), &l); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(l.Migracao) != 1 {
		t.Fatalf("items = %d, want 1", len(l.Migracao))
	}
	m := l.Migracao[0]
	if m.StatusMigracao != "STATUS_NOVO" {
		t.Errorf("StatusMigracao = %q, want STATUS_NOVO", m.StatusMigracao)
	}
	if m.MesReferencia != "2026-12-01T00:00:00-03:00" {
		t.Errorf("MesReferencia = %q, want 2026-12-01T00:00:00-03:00", m.MesReferencia)
	}
	if m.ValorDHC != "" || m.Justificativa != "" || m.CodigoAgenteSupridora != 0 {
		t.Errorf("nulls should become the zero value: %+v", m)
	}
	if l.IndexProximaPagina != "" {
		t.Errorf("null cursor should become empty, got %q", l.IndexProximaPagina)
	}
}

func TestMedicao_WhenConsumoIsStringOrNumber_DecodesBoth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		fixture         string
		wantConsumption string
		wantDistributor string
		wantNextCursor  string
	}{
		{name: "consumption and distributor as string", fixture: "measurements_page1.json", wantConsumption: "12.345", wantDistributor: "900001", wantNextCursor: "cursor-inventado-2"},
		{name: "consumption and distributor as number", fixture: "measurements_page2.json", wantConsumption: "0.5", wantDistributor: "900001"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var l gen.ListaMedicoes
			if err := json.Unmarshal(readFixture(t, tc.fixture), &l); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(l.Medicoes) != 1 {
				t.Fatalf("measurements = %d, want 1", len(l.Medicoes))
			}
			m := l.Medicoes[0]
			if m.Consumo.String() != tc.wantConsumption {
				t.Errorf("Consumo = %q, want %q", m.Consumo, tc.wantConsumption)
			}
			if m.CodigoAgenteConcessionaria.String() != tc.wantDistributor {
				t.Errorf("CodigoAgenteConcessionaria = %q, want %q", m.CodigoAgenteConcessionaria, tc.wantDistributor)
			}
			if l.IndexProximaPagina != tc.wantNextCursor {
				t.Errorf("cursor = %q, want %q", l.IndexProximaPagina, tc.wantNextCursor)
			}
		})
	}
}

func TestMigracaoDetalhe_WhenArrayOrObject_Decodes(t *testing.T) {
	t.Parallel()
	t.Run("array with one item", func(t *testing.T) {
		t.Parallel()
		var items []gen.MigracaoDetalhe
		if err := json.Unmarshal(readFixture(t, "migration_detail_array.json"), &items); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(items) != 1 || items[0].IdMigracao == "" || items[0].DataReferencia == "" {
			t.Errorf("items = %+v", items)
		}
	})
	t.Run("empty list", func(t *testing.T) {
		t.Parallel()
		var items []gen.MigracaoDetalhe
		if err := json.Unmarshal(readFixture(t, "migration_detail_empty.json"), &items); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(items) != 0 {
			t.Errorf("items = %d, want 0", len(items))
		}
	})
	t.Run("object", func(t *testing.T) {
		t.Parallel()
		var m gen.MigracaoDetalhe
		if err := json.Unmarshal(readFixture(t, "migration_detail_object.json"), &m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if m.IdMigracao == "" {
			t.Errorf("empty IdMigracao: %+v", m)
		}
	})
}

func TestErro_WhenErrorFixtures_Decode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		fixture string
		want    string
	}{
		{name: "field validation", fixture: "error_400_validation.json", want: "ERR_VALIDACAO_CAMPOS"},
		{name: "profile required", fixture: "error_400_profile.json", want: "ERR_PERFIL_VAREJISTA_OBRIGATORIO"},
		{name: "migration already requested", fixture: "error_400_migration_already_requested.json", want: "ERR_MIGRACAO_SOLICITADA"},
		{name: "authorizer", fixture: "error_403_authorizer.json", want: "ERR_CREDENCIAL_INVALIDA"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var e gen.Erro
			if err := json.Unmarshal(readFixture(t, tc.fixture), &e); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if e.Error != tc.want || e.Message == "" {
				t.Errorf("error = %+v, want %s", e, tc.want)
			}
		})
	}
}
