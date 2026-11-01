package gen_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MarcoBuarque/abertura-mercado-energia/go/internal/gen"
)

// Asserções de compilação: cada ação do overlay precisa ter efeito. Se o
// JSONPath de uma ação deixar de casar, o gerador segue calado e o tipo
// volta a ser outro; estas linhas então param de compilar.
var (
	_ json.Number = gen.MigracaoResumo{}.ValorDHC
	_ json.Number = gen.MigracaoResumo{}.ValorMusd
	_ json.Number = gen.MigracaoDetalhe{}.ValorDHC
	_ json.Number = gen.MigracaoDetalhe{}.ValorMusd
	_ json.Number = gen.Medicao{}.Consumo
	_ json.Number = gen.Medicao{}.CodigoAgenteConcessionaria
	_ int         = gen.Token{}.NotBeforePolicy
	// Datas e e-mails do corpo de envio viram string (sem openapi_types).
	_ string = gen.NovaMigracao{}.DataDenuncia
	_ string = gen.NovaMigracao{}.EmailUnidadeConsumidora
	_ string = gen.SolicitacaoSuspensao{}.DataNotificacao
	_ string = gen.EdicaoMigracao{}.EmailUnidadeConsumidora
)

func lerFixture(t *testing.T, nome string) []byte {
	t.Helper()
	raiz, err := os.OpenRoot(filepath.Join("..", "..", "testdata"))
	if err != nil {
		t.Fatalf("abrir testdata: %v", err)
	}
	defer func() {
		if err := raiz.Close(); err != nil {
			t.Errorf("fechar testdata: %v", err)
		}
	}()
	b, err := raiz.ReadFile(nome)
	if err != nil {
		t.Fatalf("lendo fixture %s: %v", nome, err)
	}
	return b
}

func TestListaMigracoes_WhenTolerancia_DecodificaSemErro(t *testing.T) {
	t.Parallel()
	var l gen.ListaMigracoes
	if err := json.Unmarshal(lerFixture(t, "migracoes_tolerancia.json"), &l); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if len(l.Migracao) != 1 {
		t.Fatalf("itens = %d, quero 1", len(l.Migracao))
	}
	m := l.Migracao[0]
	if m.StatusMigracao != "STATUS_NOVO" {
		t.Errorf("StatusMigracao = %q, quero STATUS_NOVO", m.StatusMigracao)
	}
	if m.MesReferencia != "2026-12-01T00:00:00-03:00" {
		t.Errorf("MesReferencia = %q, quero 2026-12-01T00:00:00-03:00", m.MesReferencia)
	}
	if m.ValorDHC != "" || m.Justificativa != "" || m.CodigoAgenteSupridora != 0 {
		t.Errorf("nulos deveriam virar valor zero: %+v", m)
	}
	if l.IndexProximaPagina != "" {
		t.Errorf("cursor nulo deveria virar vazio, veio %q", l.IndexProximaPagina)
	}
}

func TestMedicao_WhenConsumoStringOuNumero_DecodificaOsDois(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		fixture      string
		wantConsumo  string
		wantConcess  string
		wantProxPage string
	}{
		{name: "consumo e concessionária como string", fixture: "medicoes_pagina1.json", wantConsumo: "12.345", wantConcess: "900001", wantProxPage: "cursor-inventado-2"},
		{name: "consumo e concessionária como número", fixture: "medicoes_pagina2.json", wantConsumo: "0.5", wantConcess: "900001"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var l gen.ListaMedicoes
			if err := json.Unmarshal(lerFixture(t, tc.fixture), &l); err != nil {
				t.Fatalf("decodificar: %v", err)
			}
			if len(l.Medicoes) != 1 {
				t.Fatalf("medições = %d, quero 1", len(l.Medicoes))
			}
			m := l.Medicoes[0]
			if m.Consumo.String() != tc.wantConsumo {
				t.Errorf("Consumo = %q, quero %q", m.Consumo, tc.wantConsumo)
			}
			if m.CodigoAgenteConcessionaria.String() != tc.wantConcess {
				t.Errorf("CodigoAgenteConcessionaria = %q, quero %q", m.CodigoAgenteConcessionaria, tc.wantConcess)
			}
			if l.IndexProximaPagina != tc.wantProxPage {
				t.Errorf("cursor = %q, quero %q", l.IndexProximaPagina, tc.wantProxPage)
			}
		})
	}
}

func TestMigracaoDetalhe_WhenArrayOuObjeto_Decodifica(t *testing.T) {
	t.Parallel()
	t.Run("array de um item", func(t *testing.T) {
		t.Parallel()
		var itens []gen.MigracaoDetalhe
		if err := json.Unmarshal(lerFixture(t, "migracao_detalhe_array.json"), &itens); err != nil {
			t.Fatalf("decodificar: %v", err)
		}
		if len(itens) != 1 || itens[0].IdMigracao == "" || itens[0].DataReferencia == "" {
			t.Errorf("itens = %+v", itens)
		}
	})
	t.Run("lista vazia", func(t *testing.T) {
		t.Parallel()
		var itens []gen.MigracaoDetalhe
		if err := json.Unmarshal(lerFixture(t, "migracao_detalhe_vazio.json"), &itens); err != nil {
			t.Fatalf("decodificar: %v", err)
		}
		if len(itens) != 0 {
			t.Errorf("itens = %d, quero 0", len(itens))
		}
	})
	t.Run("objeto", func(t *testing.T) {
		t.Parallel()
		var m gen.MigracaoDetalhe
		if err := json.Unmarshal(lerFixture(t, "migracao_detalhe_objeto.json"), &m); err != nil {
			t.Fatalf("decodificar: %v", err)
		}
		if m.IdMigracao == "" {
			t.Errorf("IdMigracao vazio: %+v", m)
		}
	})
}

func TestErro_WhenFixturesDeErro_Decodifica(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		fixture string
		want    string
	}{
		{name: "validação de campos", fixture: "erro_400_validacao.json", want: "ERR_VALIDACAO_CAMPOS"},
		{name: "perfil obrigatório", fixture: "erro_400_perfil.json", want: "ERR_PERFIL_VAREJISTA_OBRIGATORIO"},
		{name: "migração solicitada", fixture: "erro_400_migracao_solicitada.json", want: "ERR_MIGRACAO_SOLICITADA"},
		{name: "autorizador", fixture: "erro_403_autorizador.json", want: "ERR_CREDENCIAL_INVALIDA"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var e gen.Erro
			if err := json.Unmarshal(lerFixture(t, tc.fixture), &e); err != nil {
				t.Fatalf("decodificar: %v", err)
			}
			if e.Error != tc.want || e.Message == "" {
				t.Errorf("erro = %+v, quero %s", e, tc.want)
			}
		})
	}
}
