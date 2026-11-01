package abertura

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MarcoBuarque/abertura-mercado-energia/go/internal/gen"
)

// tagsJSON devolve os nomes JSON dos campos de uma struct gerada.
func tagsJSON(t *testing.T, v any) []string {
	t.Helper()
	typ := reflect.TypeOf(v)
	var tags []string
	for i := range typ.NumField() {
		tag, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if tag == "" || tag == "-" {
			t.Fatalf("%s.%s sem tag JSON", typ.Name(), typ.Field(i).Name)
		}
		tags = append(tags, tag)
	}
	return tags
}

// mapeamento descreve o que o conversor faz com cada campo do modelo gerado:
// mapeado aponta o campo público de destino; ignorado traz o motivo.
type mapeamento struct {
	mapeado  map[string]string
	ignorado map[string]string
	// amostra tem todos os campos do modelo, com valores válidos e não zero.
	amostra string
}

const motivoComposicao = "composição \"<concessionária>_<UC>\": redundante, já vem de Concessionaria e UC"

var mapeamentos = map[string]struct {
	novo func() any
	mapeamento
	converter func(t *testing.T, v any) reflect.Value
}{
	"gen.MigracaoResumo": {
		novo: func() any { return &gen.MigracaoResumo{} },
		mapeamento: mapeamento{
			mapeado: map[string]string{
				"idMigracao":                 "ID",
				"codigoUnidadeConsumidora":   "UC",
				"codigoAgenteConcessionaria": "Concessionaria",
				"codigoAgenteSupridora":      "Supridora",
				"tipoDocumento":              "Documento",
				"numeroDocumento":            "Documento",
				"codigoAgenteVarejista":      "Varejista",
				"mesReferencia":              "Mes",
				"dataDenuncia":               "DataDenuncia",
				"codigoPerfilVarejista":      "Perfil",
				"dataSolicitacao":            "DataSolicitacao",
				"statusMigracao":             "Status",
				"submercado":                 "Submercado",
				"valorDHC":                   "ValorDHC",
				"valorMusd":                  "ValorMUSD",
				"pagamentoMulta":             "PagamentoMulta",
				"justificativa":              "Justificativa",
				"dataValidacao":              "DataValidacao",
				"emailUnidadeConsumidora":    "Email",
				"comentario":                 "Comentario",
				"idCelebracaoCER":            "IDCelebracaoCER",
				"indicadorProcessoSiga":      "IndicadorProcessoSiga",
			},
			ignorado: map[string]string{
				"codigoAgenteConcessionariaUnidadeConsumidora": motivoComposicao,
			},
			amostra: `{"idMigracao":"i1","codigoUnidadeConsumidora":"000000000000001",
"codigoAgenteConcessionariaUnidadeConsumidora":"900001_000000000000001","codigoAgenteConcessionaria":900001,
"codigoAgenteSupridora":900004,"tipoDocumento":"CNPJ","numeroDocumento":"11111111111111","codigoAgenteVarejista":900002,
"mesReferencia":"2027-03-01T00:00:00-03:00","dataDenuncia":"2027-01-15","codigoPerfilVarejista":900003,
"dataSolicitacao":"2027-01-10","statusMigracao":"CRIADA","submercado":"SUL","valorDHC":12.5,"valorMusd":3.25,
"pagamentoMulta":"NAO","justificativa":"j","dataValidacao":"2027-02-20","emailUnidadeConsumidora":"a@example.org",
"comentario":"c","idCelebracaoCER":"cer","indicadorProcessoSiga":"siga"}`,
		},
		converter: func(t *testing.T, v any) reflect.Value {
			t.Helper()
			g, ok := v.(*gen.MigracaoResumo)
			if !ok {
				t.Fatalf("tipo %T", v)
			}
			m, err := migracaoDeResumo(*g)
			if err != nil {
				t.Fatalf("migracaoDeResumo: %v", err)
			}
			return reflect.ValueOf(m)
		},
	},
	"gen.MigracaoDetalhe": {
		novo: func() any { return &gen.MigracaoDetalhe{} },
		mapeamento: mapeamento{
			mapeado: map[string]string{
				"idMigracao":                 "ID",
				"codigoUnidadeConsumidora":   "UC",
				"codigoAgenteConcessionaria": "Concessionaria",
				"codigoSupridora":            "Supridora",
				"tipoDocumento":              "Documento",
				"numeroDocumento":            "Documento",
				"codigoAgenteVarejista":      "Varejista",
				"dataReferencia":             "Mes",
				"dataDenuncia":               "DataDenuncia",
				"codigoPerfilVarejista":      "Perfil",
				"dataSolicitacao":            "DataSolicitacao",
				"statusMigracao":             "Status",
				"submercado":                 "Submercado",
				"valorDHC":                   "ValorDHC",
				"valorMusd":                  "ValorMUSD",
				"pagamentoMulta":             "PagamentoMulta",
				"justificativa":              "Justificativa",
				"dataValidacao":              "DataValidacao",
				"emailUnidadeConsumidora":    "Email",
				"comentario":                 "Comentario",
				"indicadorProcessoSiga":      "IndicadorProcessoSiga",
				"tipoConectado":              "TipoConectado",
			},
			ignorado: map[string]string{
				"codigoAgenteConcessionariaUnidadeConsumidora": motivoComposicao,
			},
			amostra: `{"idMigracao":"i1","codigoUnidadeConsumidora":"000000000000001",
"codigoAgenteConcessionariaUnidadeConsumidora":"900001_000000000000001","codigoAgenteConcessionaria":900001,
"codigoSupridora":900004,"tipoDocumento":"CNPJ","numeroDocumento":"11111111111111","codigoAgenteVarejista":900002,
"dataReferencia":"2027-03-01T00:00:00-03:00","dataDenuncia":"2027-01-15","codigoPerfilVarejista":900003,
"dataSolicitacao":"2027-01-10","statusMigracao":"CRIADA","submercado":"SUL","valorDHC":12.5,"valorMusd":3.25,
"pagamentoMulta":"NAO","justificativa":"j","dataValidacao":"2027-02-20","emailUnidadeConsumidora":"a@example.org",
"comentario":"c","indicadorProcessoSiga":"siga","tipoConectado":"tc"}`,
		},
		converter: func(t *testing.T, v any) reflect.Value {
			t.Helper()
			g, ok := v.(*gen.MigracaoDetalhe)
			if !ok {
				t.Fatalf("tipo %T", v)
			}
			m, err := migracaoDeDetalhe(*g)
			if err != nil {
				t.Fatalf("migracaoDeDetalhe: %v", err)
			}
			return reflect.ValueOf(m)
		},
	},
	"gen.Medicao": {
		novo: func() any { return &gen.Medicao{} },
		mapeamento: mapeamento{
			mapeado: map[string]string{
				"identificadorMedicaoConsumo": "ID",
				"codigoUnidadeConsumidora":    "UC",
				"codigoAgenteConcessionaria":  "Concessionaria",
				"diaReferencia":               "Dia",
				"dataReferenciaConsumo":       "Referencia",
				"dataAtualizacao":             "AtualizadaEm",
				"consumo":                     "Consumo",
				"tipoConsumo":                 "TipoConsumo",
				"situacaoMedicao":             "Situacao",
			},
			ignorado: map[string]string{},
			amostra: `{"identificadorMedicaoConsumo":"m1","codigoUnidadeConsumidora":"000000000000001",
"codigoAgenteConcessionaria":"900001","diaReferencia":"2027-02-01","dataReferenciaConsumo":"2027-02-01T00:15:00-03:00",
"consumo":"12.345","tipoConsumo":"tc","dataAtualizacao":"2027-02-02T08:00:00-03:00","situacaoMedicao":"CONSISTIDA"}`,
		},
		converter: func(t *testing.T, v any) reflect.Value {
			t.Helper()
			g, ok := v.(*gen.Medicao)
			if !ok {
				t.Fatalf("tipo %T", v)
			}
			m, err := medicaoDe(*g)
			if err != nil {
				t.Fatalf("medicaoDe: %v", err)
			}
			return reflect.ValueOf(m)
		},
	},
}

// TestMapeamentoCompleto falha se um campo do modelo gerado não estiver nem
// mapeado nem ignorado (com motivo), se o mapa citar um campo que não existe
// mais, ou se um campo mapeado não chegar ao tipo público.
func TestMapeamentoCompleto(t *testing.T) {
	t.Parallel()
	for nome, tc := range mapeamentos {
		t.Run(nome, func(t *testing.T) {
			t.Parallel()
			alvo := tc.novo()
			tags := tagsJSON(t, reflect.ValueOf(alvo).Elem().Interface())
			existentes := map[string]bool{}
			for _, tag := range tags {
				existentes[tag] = true
				_, mapeado := tc.mapeado[tag]
				motivo, ignorado := tc.ignorado[tag]
				switch {
				case !mapeado && !ignorado:
					t.Errorf("campo %q do modelo gerado não está mapeado nem ignorado", tag)
				case mapeado && ignorado:
					t.Errorf("campo %q está mapeado e ignorado ao mesmo tempo", tag)
				case ignorado && motivo == "":
					t.Errorf("campo ignorado %q sem motivo", tag)
				}
			}
			for tag := range tc.mapeado {
				if !existentes[tag] {
					t.Errorf("o mapa cita %q, que não existe mais no modelo gerado", tag)
				}
			}
			for tag := range tc.ignorado {
				if !existentes[tag] {
					t.Errorf("o mapa de ignorados cita %q, que não existe mais no modelo gerado", tag)
				}
			}

			dec := json.NewDecoder(strings.NewReader(tc.amostra))
			dec.DisallowUnknownFields()
			if err := dec.Decode(alvo); err != nil {
				t.Fatalf("amostra não decodifica no modelo gerado: %v", err)
			}
			var amostrados map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.amostra), &amostrados); err != nil {
				t.Fatalf("amostra: %v", err)
			}
			for _, tag := range tags {
				if _, ok := amostrados[tag]; !ok {
					t.Errorf("a amostra não preenche %q", tag)
				}
			}

			pub := tc.converter(t, alvo)
			vistos := map[string]bool{}
			for tag, campo := range tc.mapeado {
				if vistos[campo] {
					continue
				}
				vistos[campo] = true
				f := pub.FieldByName(campo)
				if !f.IsValid() {
					t.Errorf("%q aponta para %s, que não existe no tipo público", tag, campo)
					continue
				}
				if f.IsZero() {
					t.Errorf("%q deveria preencher %s, mas ele ficou no valor zero", tag, campo)
				}
			}
		})
	}
}

var brasilia = time.FixedZone("", -3*60*60)

func TestMigracaoDeResumo_WhenCamposVariados_Converte(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   gen.MigracaoResumo
		want Migracao
	}{
		{
			name: "mês em data e hora vira Mes",
			in:   gen.MigracaoResumo{IdMigracao: "i", MesReferencia: "2027-03-01T00:00:00-03:00"},
			want: Migracao{ID: "i", Mes: MesDe(2027, time.March)},
		},
		{
			name: "mês em MM-AAAA vira Mes",
			in:   gen.MigracaoResumo{IdMigracao: "i", MesReferencia: "12-2026"},
			want: Migracao{ID: "i", Mes: MesDe(2026, time.December)},
		},
		{
			name: "supridora vem de codigoAgenteSupridora",
			in:   gen.MigracaoResumo{IdMigracao: "i", CodigoAgenteSupridora: 900004},
			want: Migracao{ID: "i", Supridora: 900004},
		},
		{
			name: "campos ausentes ou nulos ficam no valor zero",
			in:   gen.MigracaoResumo{IdMigracao: "i"},
			want: Migracao{ID: "i"},
		},
		{
			name: "status e sim ou não passam como vieram, mesmo desconhecidos",
			in:   gen.MigracaoResumo{IdMigracao: "i", StatusMigracao: "STATUS_NOVO", PagamentoMulta: "TALVEZ"},
			want: Migracao{ID: "i", Status: "STATUS_NOVO", PagamentoMulta: "TALVEZ"},
		},
		{
			name: "datas de dia viram meia-noite em -03:00",
			in:   gen.MigracaoResumo{IdMigracao: "i", DataDenuncia: "2027-01-15", DataSolicitacao: "2027-01-10", DataValidacao: "2027-02-20"},
			want: Migracao{
				ID:              "i",
				DataDenuncia:    time.Date(2027, 1, 15, 0, 0, 0, 0, brasilia),
				DataSolicitacao: time.Date(2027, 1, 10, 0, 0, 0, 0, brasilia),
				DataValidacao:   time.Date(2027, 2, 20, 0, 0, 0, 0, brasilia),
			},
		},
		{
			name: "CNPJ vira Documento sem pontuação",
			in:   gen.MigracaoResumo{IdMigracao: "i", TipoDocumento: "CNPJ", NumeroDocumento: "11.111.111/1111-11"},
			want: Migracao{ID: "i", Documento: DocumentoCNPJ("11111111111111")},
		},
		{
			name: "CPF vira Documento",
			in:   gen.MigracaoResumo{IdMigracao: "i", TipoDocumento: "CPF", NumeroDocumento: "22222222222"},
			want: Migracao{ID: "i", Documento: DocumentoCPF("22222222222")},
		},
		{
			name: "valores decimais mantêm a precisão",
			in:   gen.MigracaoResumo{IdMigracao: "i", ValorDHC: "12.50", ValorMusd: "3.25"},
			want: Migracao{ID: "i", ValorDHC: decimalTeste(t, "12.5"), ValorMUSD: decimalTeste(t, "3.25")},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := migracaoDeResumo(tc.in)
			if err != nil {
				t.Fatalf("migracaoDeResumo: %v", err)
			}
			comparar(t, got, tc.want)
		})
	}
}

func TestMigracaoDeDetalhe_WhenCamposVariados_Converte(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   gen.MigracaoDetalhe
		want Migracao
	}{
		{
			name: "dataReferencia vira o mesmo Mes de mesReferencia",
			in:   gen.MigracaoDetalhe{IdMigracao: "i", DataReferencia: "2027-03-01T00:00:00-03:00"},
			want: Migracao{ID: "i", Mes: MesDe(2027, time.March)},
		},
		{
			name: "dataReferencia só com AAAA-MM também vale",
			in:   gen.MigracaoDetalhe{IdMigracao: "i", DataReferencia: "2027-03"},
			want: Migracao{ID: "i", Mes: MesDe(2027, time.March)},
		},
		{
			name: "supridora vem de codigoSupridora",
			in:   gen.MigracaoDetalhe{IdMigracao: "i", CodigoSupridora: 900004},
			want: Migracao{ID: "i", Supridora: 900004},
		},
		{
			name: "tipoConectado só existe no detalhe",
			in:   gen.MigracaoDetalhe{IdMigracao: "i", TipoConectado: "tc"},
			want: Migracao{ID: "i", TipoConectado: "tc"},
		},
		{
			name: "campos ausentes ficam no valor zero",
			in:   gen.MigracaoDetalhe{IdMigracao: "i"},
			want: Migracao{ID: "i"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := migracaoDeDetalhe(tc.in)
			if err != nil {
				t.Fatalf("migracaoDeDetalhe: %v", err)
			}
			comparar(t, got, tc.want)
		})
	}
}

func TestConversores_WhenValorInvalido_DaErroSemCitarOValor(t *testing.T) {
	t.Parallel()
	const marca = "valor-que-nao-pode-vazar"
	tests := []struct {
		name      string
		converter func() error
		wantCampo string
	}{
		{
			name:      "mês inválido na lista",
			converter: func() error { _, err := migracaoDeResumo(gen.MigracaoResumo{MesReferencia: marca}); return err },
			wantCampo: "mesReferencia",
		},
		{
			name:      "mês inválido no detalhe",
			converter: func() error { _, err := migracaoDeDetalhe(gen.MigracaoDetalhe{DataReferencia: marca}); return err },
			wantCampo: "dataReferencia",
		},
		{
			name:      "data inválida",
			converter: func() error { _, err := migracaoDeResumo(gen.MigracaoResumo{DataDenuncia: marca}); return err },
			wantCampo: "dataDenuncia",
		},
		{
			name:      "decimal inválido",
			converter: func() error { _, err := migracaoDeResumo(gen.MigracaoResumo{ValorDHC: marca}); return err },
			wantCampo: "valorDHC",
		},
		{
			name:      "consumo inválido",
			converter: func() error { _, err := medicaoDe(gen.Medicao{Consumo: marca}); return err },
			wantCampo: "consumo",
		},
		{
			name:      "concessionária da medição inválida",
			converter: func() error { _, err := medicaoDe(gen.Medicao{CodigoAgenteConcessionaria: marca}); return err },
			wantCampo: "codigoAgenteConcessionaria",
		},
		{
			name:      "data da medição inválida",
			converter: func() error { _, err := medicaoDe(gen.Medicao{DataAtualizacao: marca}); return err },
			wantCampo: "dataAtualizacao",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.converter()
			if err == nil {
				t.Fatal("esperava erro")
			}
			if !strings.Contains(err.Error(), tc.wantCampo) {
				t.Errorf("o erro não cita o campo %q: %v", tc.wantCampo, err)
			}
			if strings.Contains(err.Error(), marca) {
				t.Errorf("o erro cita o valor recebido: %v", err)
			}
		})
	}
}

func TestMedicaoDe_WhenCamposVariados_Converte(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   gen.Medicao
		want Medicao
	}{
		{
			name: "consumo como texto",
			in:   gen.Medicao{IdentificadorMedicaoConsumo: "m", Consumo: "12.345"},
			want: Medicao{ID: "m", Consumo: decimalTeste(t, "12.345")},
		},
		{
			name: "consumo como número",
			in:   gen.Medicao{IdentificadorMedicaoConsumo: "m", Consumo: "0.5"},
			want: Medicao{ID: "m", Consumo: decimalTeste(t, "0.5")},
		},
		{
			name: "concessionária como texto ou número",
			in:   gen.Medicao{IdentificadorMedicaoConsumo: "m", CodigoAgenteConcessionaria: "900001"},
			want: Medicao{ID: "m", Concessionaria: 900001},
		},
		{
			name: "dia e instantes em -03:00",
			in: gen.Medicao{
				IdentificadorMedicaoConsumo: "m", DiaReferencia: "2027-02-01",
				DataReferenciaConsumo: "2027-02-01T00:15:00-03:00", DataAtualizacao: "2027-02-02T08:00:00-03:00",
			},
			want: Medicao{
				ID:           "m",
				Dia:          time.Date(2027, 2, 1, 0, 0, 0, 0, brasilia),
				Referencia:   time.Date(2027, 2, 1, 0, 15, 0, 0, brasilia),
				AtualizadaEm: time.Date(2027, 2, 2, 8, 0, 0, 0, brasilia),
			},
		},
		{
			name: "situação desconhecida passa como veio",
			in:   gen.Medicao{IdentificadorMedicaoConsumo: "m", SituacaoMedicao: "NOVA"},
			want: Medicao{ID: "m", Situacao: "NOVA"},
		},
		{
			name: "tudo ausente fica zero",
			in:   gen.Medicao{},
			want: Medicao{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := medicaoDe(tc.in)
			if err != nil {
				t.Fatalf("medicaoDe: %v", err)
			}
			comparar(t, got, tc.want)
		})
	}
}

// TestEnvioGerado_WhenDocumentoEmClaro cobre a conversão pública para gen: o
// Documento serializa mascarado, então o envio tem de usar Numero().
func TestEnvioGerado_WhenDocumentoEmClaro_EnviaONumero(t *testing.T) {
	t.Parallel()
	nova := novaMigracaoGen(NovaMigracao{
		UC: "000000000000001", Concessionaria: 900001,
		Documento: DocumentoCNPJ("11.111.111/1111-11"), Email: "a@example.org",
		Mes: MesDe(2027, time.March), DataDenuncia: time.Date(2027, 1, 15, 23, 0, 0, 0, time.UTC),
		Comentario: "c",
	}, 900002, 900003)
	corpo, err := json.Marshal(nova)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	for _, trecho := range []string{
		`"numeroDocumento":"11111111111111"`, `"tipoDocumento":"CNPJ"`, `"mesReferencia":"2027-03"`,
		`"dataDenuncia":"2027-01-15"`, `"codigoAgenteVarejista":900002`, `"codigoPerfilVarejista":900003`,
		`"codigoAgenteConcessionaria":900001`,
	} {
		if !bytes.Contains(corpo, []byte(trecho)) {
			t.Errorf("corpo sem %s: %s", trecho, corpo)
		}
	}

	ed := edicaoGen(EdicaoMigracao{
		Mes: MesDe(2027, time.April), Perfil: 900003, Documento: DocumentoCPF("22222222222"),
		Email: "novo@example.org", Status: StatusCriada,
	})
	corpo, err = json.Marshal(ed)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	for _, trecho := range []string{
		`"numeroDocumento":"22222222222"`, `"tipoDocumento":"CPF"`, `"mesReferencia":"2027-04"`,
		`"emailUnidadeConsumidora":"novo@example.org"`, `"statusMigracao":"CRIADA"`, `"codigoPerfilVarejista":900003`,
	} {
		if !bytes.Contains(corpo, []byte(trecho)) {
			t.Errorf("corpo sem %s: %s", trecho, corpo)
		}
	}
}

func decimalTeste(t *testing.T, s string) Decimal {
	t.Helper()
	d, err := decimalDe(json.Number(s))
	if err != nil {
		t.Fatalf("decimalDe(%q): %v", s, err)
	}
	return d
}

// comparar confere dois valores campo a campo. As datas comparam o instante e
// o deslocamento; o Documento, o texto em claro.
func comparar(t *testing.T, got, want any) {
	t.Helper()
	g, w := reflect.ValueOf(got), reflect.ValueOf(want)
	for i := range g.NumField() {
		nome := g.Type().Field(i).Name
		gv, wv := g.Field(i).Interface(), w.Field(i).Interface()
		switch x := gv.(type) {
		case time.Time:
			y, ok := wv.(time.Time)
			if !ok {
				t.Fatalf("%s: tipos diferentes", nome)
			}
			if !x.Equal(y) {
				t.Errorf("%s = %v, quero %v", nome, x, y)
			} else if !x.IsZero() {
				if _, off := x.Zone(); off != -3*60*60 {
					t.Errorf("%s com deslocamento %d, quero -03:00", nome, off)
				}
			}
		case Documento:
			y, ok := wv.(Documento)
			if !ok {
				t.Fatalf("%s: tipos diferentes", nome)
			}
			if x.Tipo() != y.Tipo() || x.Numero() != y.Numero() {
				t.Errorf("%s = %v, quero %v", nome, x, y)
			}
		default:
			if !reflect.DeepEqual(gv, wv) {
				t.Errorf("%s = %#v, quero %#v", nome, gv, wv)
			}
		}
	}
}

func TestDataDe_WhenFormato_LeEmBrasilia(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      string
		want    time.Time
		wantErr bool
	}{
		{name: "vazio é o tempo zero", in: "  ", want: time.Time{}},
		{name: "só o dia", in: "2027-01-15", want: time.Date(2027, 1, 15, 0, 0, 0, 0, fusoBrasilia)},
		{name: "RFC 3339 em UTC vai para -03:00", in: "2027-01-15T03:00:00Z", want: time.Date(2027, 1, 15, 0, 0, 0, 0, fusoBrasilia)},
		{name: "data e hora sem deslocamento", in: "2027-01-15T10:20:30", want: time.Date(2027, 1, 15, 10, 20, 30, 0, fusoBrasilia)},
		{name: "data e hora sem deslocamento com fração", in: "2027-01-15T10:20:30.123", want: time.Date(2027, 1, 15, 10, 20, 30, 123000000, fusoBrasilia)},
		{name: "barras são recusadas", in: "2026/12/01", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := dataDe("dataSolicitacao", tc.in)
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), `"dataSolicitacao"`) {
					t.Fatalf("err = %v, quero erro de campo ilegível", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("dataDe: %v", err)
			}
			if !got.Equal(tc.want) || got.Location() != tc.want.Location() && !tc.want.IsZero() {
				t.Errorf("got %v, quero %v", got, tc.want)
			}
		})
	}
}
