package abertura

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura/internal/gen"
)

// jsonTags returns the JSON names of the fields of a generated struct.
func jsonTags(t *testing.T, v any) []string {
	t.Helper()
	typ := reflect.TypeOf(v)
	var tags []string
	for i := range typ.NumField() {
		tag, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if tag == "" || tag == "-" {
			t.Fatalf("%s.%s has no JSON tag", typ.Name(), typ.Field(i).Name)
		}
		tags = append(tags, tag)
	}
	return tags
}

// mapping describes what the converter does with each field of the generated
// model: mapped points to the public destination field; ignored carries the
// reason.
type mapping struct {
	mapped  map[string]string
	ignored map[string]string
	// sample has every field of the model, with valid, non-zero values.
	sample string
}

const compositionReason = "composition \"<distributor>_<consumer unit>\": redundant, already provided by Distributor and ConsumerUnit"

var mappings = map[string]struct {
	fresh func() any
	mapping
	convert func(t *testing.T, v any) reflect.Value
}{
	"gen.MigracaoResumo": {
		fresh: func() any { return &gen.MigracaoResumo{} },
		mapping: mapping{
			mapped: map[string]string{
				"idMigracao":                 "ID",
				"codigoUnidadeConsumidora":   "ConsumerUnit",
				"codigoAgenteConcessionaria": "Distributor",
				"codigoAgenteSupridora":      "Supplier",
				"tipoDocumento":              "Document",
				"numeroDocumento":            "Document",
				"codigoAgenteVarejista":      "Retailer",
				"mesReferencia":              "Month",
				"dataDenuncia":               "TerminationNoticeDate",
				"codigoPerfilVarejista":      "Profile",
				"dataSolicitacao":            "RequestDate",
				"statusMigracao":             "Status",
				"submercado":                 "Submarket",
				"valorDHC":                   "DHCAmount",
				"valorMusd":                  "MUSDAmount",
				"pagamentoMulta":             "PenaltyPayment",
				"justificativa":              "Justification",
				"dataValidacao":              "ValidationDate",
				"emailUnidadeConsumidora":    "Email",
				"comentario":                 "Comment",
				"idCelebracaoCER":            "CERSigningID",
				"indicadorProcessoSiga":      "SIGAProcessIndicator",
			},
			ignored: map[string]string{
				"codigoAgenteConcessionariaUnidadeConsumidora": compositionReason,
			},
			sample: `{"idMigracao":"i1","codigoUnidadeConsumidora":"000000000000001",
"codigoAgenteConcessionariaUnidadeConsumidora":"900001_000000000000001","codigoAgenteConcessionaria":900001,
"codigoAgenteSupridora":900004,"tipoDocumento":"CNPJ","numeroDocumento":"11111111111111","codigoAgenteVarejista":900002,
"mesReferencia":"2027-03-01T00:00:00-03:00","dataDenuncia":"2027-01-15","codigoPerfilVarejista":900003,
"dataSolicitacao":"2027-01-10","statusMigracao":"CRIADA","submercado":"SUL","valorDHC":12.5,"valorMusd":3.25,
"pagamentoMulta":"NAO","justificativa":"j","dataValidacao":"2027-02-20","emailUnidadeConsumidora":"a@example.org",
"comentario":"c","idCelebracaoCER":"cer","indicadorProcessoSiga":"siga"}`,
		},
		convert: func(t *testing.T, v any) reflect.Value {
			t.Helper()
			g, ok := v.(*gen.MigracaoResumo)
			if !ok {
				t.Fatalf("type %T", v)
			}
			m, err := migrationFromSummary(*g)
			if err != nil {
				t.Fatalf("migrationFromSummary: %v", err)
			}
			return reflect.ValueOf(m)
		},
	},
	"gen.MigracaoDetalhe": {
		fresh: func() any { return &gen.MigracaoDetalhe{} },
		mapping: mapping{
			mapped: map[string]string{
				"idMigracao":                 "ID",
				"codigoUnidadeConsumidora":   "ConsumerUnit",
				"codigoAgenteConcessionaria": "Distributor",
				"codigoSupridora":            "Supplier",
				"tipoDocumento":              "Document",
				"numeroDocumento":            "Document",
				"codigoAgenteVarejista":      "Retailer",
				"dataReferencia":             "Month",
				"dataDenuncia":               "TerminationNoticeDate",
				"codigoPerfilVarejista":      "Profile",
				"dataSolicitacao":            "RequestDate",
				"statusMigracao":             "Status",
				"submercado":                 "Submarket",
				"valorDHC":                   "DHCAmount",
				"valorMusd":                  "MUSDAmount",
				"pagamentoMulta":             "PenaltyPayment",
				"justificativa":              "Justification",
				"dataValidacao":              "ValidationDate",
				"emailUnidadeConsumidora":    "Email",
				"comentario":                 "Comment",
				"indicadorProcessoSiga":      "SIGAProcessIndicator",
				"tipoConectado":              "ConnectionType",
			},
			ignored: map[string]string{
				"codigoAgenteConcessionariaUnidadeConsumidora": compositionReason,
			},
			sample: `{"idMigracao":"i1","codigoUnidadeConsumidora":"000000000000001",
"codigoAgenteConcessionariaUnidadeConsumidora":"900001_000000000000001","codigoAgenteConcessionaria":900001,
"codigoSupridora":900004,"tipoDocumento":"CNPJ","numeroDocumento":"11111111111111","codigoAgenteVarejista":900002,
"dataReferencia":"2027-03-01T00:00:00-03:00","dataDenuncia":"2027-01-15","codigoPerfilVarejista":900003,
"dataSolicitacao":"2027-01-10","statusMigracao":"CRIADA","submercado":"SUL","valorDHC":12.5,"valorMusd":3.25,
"pagamentoMulta":"NAO","justificativa":"j","dataValidacao":"2027-02-20","emailUnidadeConsumidora":"a@example.org",
"comentario":"c","indicadorProcessoSiga":"siga","tipoConectado":"tc"}`,
		},
		convert: func(t *testing.T, v any) reflect.Value {
			t.Helper()
			g, ok := v.(*gen.MigracaoDetalhe)
			if !ok {
				t.Fatalf("type %T", v)
			}
			m, err := migrationFromDetail(*g)
			if err != nil {
				t.Fatalf("migrationFromDetail: %v", err)
			}
			return reflect.ValueOf(m)
		},
	},
	"gen.Medicao": {
		fresh: func() any { return &gen.Medicao{} },
		mapping: mapping{
			mapped: map[string]string{
				"identificadorMedicaoConsumo": "ID",
				"codigoUnidadeConsumidora":    "ConsumerUnit",
				"codigoAgenteConcessionaria":  "Distributor",
				"diaReferencia":               "Day",
				"dataReferenciaConsumo":       "Reference",
				"dataAtualizacao":             "UpdatedAt",
				"consumo":                     "Consumption",
				"tipoConsumo":                 "ConsumptionType",
				"situacaoMedicao":             "Status",
			},
			ignored: map[string]string{},
			sample: `{"identificadorMedicaoConsumo":"m1","codigoUnidadeConsumidora":"000000000000001",
"codigoAgenteConcessionaria":"900001","diaReferencia":"2027-02-01","dataReferenciaConsumo":"2027-02-01T00:15:00-03:00",
"consumo":"12.345","tipoConsumo":"tc","dataAtualizacao":"2027-02-02T08:00:00-03:00","situacaoMedicao":"CONSISTIDA"}`,
		},
		convert: func(t *testing.T, v any) reflect.Value {
			t.Helper()
			g, ok := v.(*gen.Medicao)
			if !ok {
				t.Fatalf("type %T", v)
			}
			m, err := measurementFrom(*g)
			if err != nil {
				t.Fatalf("measurementFrom: %v", err)
			}
			return reflect.ValueOf(m)
		},
	},
}

// TestMapping_WhenGeneratedStruct_CoversEveryField fails if a field of the
// generated model is neither mapped nor ignored (with a reason), if the map
// cites a field that no longer exists, or if a mapped field does not reach the
// public type.
func TestMapping_WhenGeneratedStruct_CoversEveryField(t *testing.T) {
	t.Parallel()
	for name, tc := range mappings {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			want := tc.fresh()
			tags := jsonTags(t, reflect.ValueOf(want).Elem().Interface())
			existing := map[string]bool{}
			for _, tag := range tags {
				existing[tag] = true
				_, isMapped := tc.mapped[tag]
				reason, isIgnored := tc.ignored[tag]
				switch {
				case !isMapped && !isIgnored:
					t.Errorf("field %q of the generated model is neither mapped nor ignored", tag)
				case isMapped && isIgnored:
					t.Errorf("field %q is both mapped and ignored", tag)
				case isIgnored && reason == "":
					t.Errorf("ignored field %q has no reason", tag)
				}
			}
			for tag := range tc.mapped {
				if !existing[tag] {
					t.Errorf("the map cites %q, which no longer exists in the generated model", tag)
				}
			}
			for tag := range tc.ignored {
				if !existing[tag] {
					t.Errorf("the ignored map cites %q, which no longer exists in the generated model", tag)
				}
			}

			dec := json.NewDecoder(strings.NewReader(tc.sample))
			dec.DisallowUnknownFields()
			if err := dec.Decode(want); err != nil {
				t.Fatalf("sample does not decode into the generated model: %v", err)
			}
			var sampled map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.sample), &sampled); err != nil {
				t.Fatalf("sample: %v", err)
			}
			for _, tag := range tags {
				if _, ok := sampled[tag]; !ok {
					t.Errorf("the sample does not fill %q", tag)
				}
			}

			public := tc.convert(t, want)
			seen := map[string]bool{}
			for tag, field := range tc.mapped {
				if seen[field] {
					continue
				}
				seen[field] = true
				f := public.FieldByName(field)
				if !f.IsValid() {
					t.Errorf("%q points to %s, which does not exist in the public type", tag, field)
					continue
				}
				if f.IsZero() {
					t.Errorf("%q should fill %s, but it stayed at the zero value", tag, field)
				}
			}
		})
	}
}

var brasilia = time.FixedZone("", -3*60*60)

func TestMigrationFromSummary_WhenVariedFields_Converts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   gen.MigracaoResumo
		want Migration
	}{
		{
			name: "date-time month becomes YearMonth",
			in:   gen.MigracaoResumo{IdMigracao: "i", MesReferencia: "2027-03-01T00:00:00-03:00"},
			want: Migration{ID: "i", Month: NewYearMonth(2027, time.March)},
		},
		{
			name: "MM-YYYY month becomes YearMonth",
			in:   gen.MigracaoResumo{IdMigracao: "i", MesReferencia: "12-2026"},
			want: Migration{ID: "i", Month: NewYearMonth(2026, time.December)},
		},
		{
			name: "supplier comes from codigoAgenteSupridora",
			in:   gen.MigracaoResumo{IdMigracao: "i", CodigoAgenteSupridora: 900004},
			want: Migration{ID: "i", Supplier: 900004},
		},
		{
			name: "absent or null fields stay at the zero value",
			in:   gen.MigracaoResumo{IdMigracao: "i"},
			want: Migration{ID: "i"},
		},
		{
			name: "status and yes/no pass through as received, even when unknown",
			in:   gen.MigracaoResumo{IdMigracao: "i", StatusMigracao: "STATUS_NOVO", PagamentoMulta: "TALVEZ"},
			want: Migration{ID: "i", Status: "STATUS_NOVO", PenaltyPayment: "TALVEZ"},
		},
		{
			name: "day dates become midnight at -03:00",
			in:   gen.MigracaoResumo{IdMigracao: "i", DataDenuncia: "2027-01-15", DataSolicitacao: "2027-01-10", DataValidacao: "2027-02-20"},
			want: Migration{
				ID:                    "i",
				TerminationNoticeDate: time.Date(2027, 1, 15, 0, 0, 0, 0, brasilia),
				RequestDate:           time.Date(2027, 1, 10, 0, 0, 0, 0, brasilia),
				ValidationDate:        time.Date(2027, 2, 20, 0, 0, 0, 0, brasilia),
			},
		},
		{
			name: "CNPJ becomes a Document without punctuation",
			in:   gen.MigracaoResumo{IdMigracao: "i", TipoDocumento: "CNPJ", NumeroDocumento: "11.111.111/1111-11"},
			want: Migration{ID: "i", Document: CNPJ("11111111111111")},
		},
		{
			name: "CPF becomes a Document",
			in:   gen.MigracaoResumo{IdMigracao: "i", TipoDocumento: "CPF", NumeroDocumento: "22222222222"},
			want: Migration{ID: "i", Document: CPF("22222222222")},
		},
		{
			name: "decimal values keep their precision",
			in:   gen.MigracaoResumo{IdMigracao: "i", ValorDHC: "12.50", ValorMusd: "3.25"},
			want: Migration{ID: "i", DHCAmount: testDecimal(t, "12.5"), MUSDAmount: testDecimal(t, "3.25")},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := migrationFromSummary(tc.in)
			if err != nil {
				t.Fatalf("migrationFromSummary: %v", err)
			}
			compare(t, got, tc.want)
		})
	}
}

func TestMigrationFromDetail_WhenVariedFields_Converts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   gen.MigracaoDetalhe
		want Migration
	}{
		{
			name: "dataReferencia becomes the same YearMonth as mesReferencia",
			in:   gen.MigracaoDetalhe{IdMigracao: "i", DataReferencia: "2027-03-01T00:00:00-03:00"},
			want: Migration{ID: "i", Month: NewYearMonth(2027, time.March)},
		},
		{
			name: "dataReferencia with only YYYY-MM is also valid",
			in:   gen.MigracaoDetalhe{IdMigracao: "i", DataReferencia: "2027-03"},
			want: Migration{ID: "i", Month: NewYearMonth(2027, time.March)},
		},
		{
			name: "supplier comes from codigoSupridora",
			in:   gen.MigracaoDetalhe{IdMigracao: "i", CodigoSupridora: 900004},
			want: Migration{ID: "i", Supplier: 900004},
		},
		{
			name: "connection type exists only in the detail",
			in:   gen.MigracaoDetalhe{IdMigracao: "i", TipoConectado: "tc"},
			want: Migration{ID: "i", ConnectionType: "tc"},
		},
		{
			name: "absent fields stay at the zero value",
			in:   gen.MigracaoDetalhe{IdMigracao: "i"},
			want: Migration{ID: "i"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := migrationFromDetail(tc.in)
			if err != nil {
				t.Fatalf("migrationFromDetail: %v", err)
			}
			compare(t, got, tc.want)
		})
	}
}

func TestConverters_WhenInvalidValue_ReturnsErrorWithoutQuotingIt(t *testing.T) {
	t.Parallel()
	const marker = "value-that-must-not-leak"
	tests := []struct {
		name      string
		convert   func() error
		wantField string
	}{
		{
			name:      "invalid month in the list",
			convert:   func() error { _, err := migrationFromSummary(gen.MigracaoResumo{MesReferencia: marker}); return err },
			wantField: "mesReferencia",
		},
		{
			name:      "invalid month in the detail",
			convert:   func() error { _, err := migrationFromDetail(gen.MigracaoDetalhe{DataReferencia: marker}); return err },
			wantField: "dataReferencia",
		},
		{
			name:      "invalid date",
			convert:   func() error { _, err := migrationFromSummary(gen.MigracaoResumo{DataDenuncia: marker}); return err },
			wantField: "dataDenuncia",
		},
		{
			name:      "invalid decimal",
			convert:   func() error { _, err := migrationFromSummary(gen.MigracaoResumo{ValorDHC: marker}); return err },
			wantField: "valorDHC",
		},
		{
			name:      "invalid consumption",
			convert:   func() error { _, err := measurementFrom(gen.Medicao{Consumo: marker}); return err },
			wantField: "consumo",
		},
		{
			name:      "invalid measurement distributor",
			convert:   func() error { _, err := measurementFrom(gen.Medicao{CodigoAgenteConcessionaria: marker}); return err },
			wantField: "codigoAgenteConcessionaria",
		},
		{
			name:      "invalid measurement date",
			convert:   func() error { _, err := measurementFrom(gen.Medicao{DataAtualizacao: marker}); return err },
			wantField: "dataAtualizacao",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.convert()
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantField) {
				t.Errorf("the error does not cite field %q: %v", tc.wantField, err)
			}
			if strings.Contains(err.Error(), marker) {
				t.Errorf("the error quotes the received value: %v", err)
			}
		})
	}
}

func TestMeasurementFrom_WhenVariedFields_Converts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   gen.Medicao
		want Measurement
	}{
		{
			name: "consumption as string",
			in:   gen.Medicao{IdentificadorMedicaoConsumo: "m", Consumo: "12.345"},
			want: Measurement{ID: "m", Consumption: testDecimal(t, "12.345")},
		},
		{
			name: "consumption as number",
			in:   gen.Medicao{IdentificadorMedicaoConsumo: "m", Consumo: "0.5"},
			want: Measurement{ID: "m", Consumption: testDecimal(t, "0.5")},
		},
		{
			name: "distributor as string or number",
			in:   gen.Medicao{IdentificadorMedicaoConsumo: "m", CodigoAgenteConcessionaria: "900001"},
			want: Measurement{ID: "m", Distributor: 900001},
		},
		{
			name: "day and instants at -03:00",
			in: gen.Medicao{
				IdentificadorMedicaoConsumo: "m", DiaReferencia: "2027-02-01",
				DataReferenciaConsumo: "2027-02-01T00:15:00-03:00", DataAtualizacao: "2027-02-02T08:00:00-03:00",
			},
			want: Measurement{
				ID:        "m",
				Day:       time.Date(2027, 2, 1, 0, 0, 0, 0, brasilia),
				Reference: time.Date(2027, 2, 1, 0, 15, 0, 0, brasilia),
				UpdatedAt: time.Date(2027, 2, 2, 8, 0, 0, 0, brasilia),
			},
		},
		{
			name: "unknown status passes through as received",
			in:   gen.Medicao{IdentificadorMedicaoConsumo: "m", SituacaoMedicao: "NOVA"},
			want: Measurement{ID: "m", Status: "NOVA"},
		},
		{
			name: "everything absent stays zero",
			in:   gen.Medicao{},
			want: Measurement{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := measurementFrom(tc.in)
			if err != nil {
				t.Fatalf("measurementFrom: %v", err)
			}
			compare(t, got, tc.want)
		})
	}
}

// TestGeneratedRequest_WhenDocumentInClear_SendsNumber covers the conversion
// from the public type to gen: a Document serializes masked, so sending must
// use Number().
func TestGeneratedRequest_WhenDocumentInClear_SendsNumber(t *testing.T) {
	t.Parallel()
	request := migrationRequestToGen(MigrationRequest{
		ConsumerUnit: "000000000000001", Distributor: 900001,
		Document: CNPJ("11.111.111/1111-11"), Email: "a@example.org",
		Month: NewYearMonth(2027, time.March), TerminationNoticeDate: time.Date(2027, 1, 15, 23, 0, 0, 0, time.UTC),
		Comment: "c",
	}, 900002, 900003)
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	for _, fragment := range []string{
		`"numeroDocumento":"11111111111111"`, `"tipoDocumento":"CNPJ"`, `"mesReferencia":"2027-03"`,
		`"dataDenuncia":"2027-01-15"`, `"codigoAgenteVarejista":900002`, `"codigoPerfilVarejista":900003`,
		`"codigoAgenteConcessionaria":900001`,
	} {
		if !bytes.Contains(body, []byte(fragment)) {
			t.Errorf("body lacks %s: %s", fragment, body)
		}
	}

	edit := editToGen(MigrationEdit{
		Month: NewYearMonth(2027, time.April), Profile: 900003, Document: CPF("22222222222"),
		Email: "new@example.org", Status: MigrationStatusCreated,
	})
	body, err = json.Marshal(edit)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	for _, fragment := range []string{
		`"numeroDocumento":"22222222222"`, `"tipoDocumento":"CPF"`, `"mesReferencia":"2027-04"`,
		`"emailUnidadeConsumidora":"new@example.org"`, `"statusMigracao":"CRIADA"`, `"codigoPerfilVarejista":900003`,
	} {
		if !bytes.Contains(body, []byte(fragment)) {
			t.Errorf("body lacks %s: %s", fragment, body)
		}
	}
}

func testDecimal(t *testing.T, s string) Decimal {
	t.Helper()
	d, err := decimalFrom(json.Number(s))
	if err != nil {
		t.Fatalf("decimalFrom(%q): %v", s, err)
	}
	return d
}

// compare checks two values field by field. Dates compare the instant and the
// offset; the Document compares the clear text.
func compare(t *testing.T, got, want any) {
	t.Helper()
	g, w := reflect.ValueOf(got), reflect.ValueOf(want)
	for i := range g.NumField() {
		name := g.Type().Field(i).Name
		gv, wv := g.Field(i).Interface(), w.Field(i).Interface()
		switch x := gv.(type) {
		case time.Time:
			y, ok := wv.(time.Time)
			if !ok {
				t.Fatalf("%s: different types", name)
			}
			if !x.Equal(y) {
				t.Errorf("%s = %v, want %v", name, x, y)
			} else if !x.IsZero() {
				if _, off := x.Zone(); off != -3*60*60 {
					t.Errorf("%s with offset %d, want -03:00", name, off)
				}
			}
		case Document:
			y, ok := wv.(Document)
			if !ok {
				t.Fatalf("%s: different types", name)
			}
			if x.Type() != y.Type() || x.Number() != y.Number() {
				t.Errorf("%s = %v, want %v", name, x, y)
			}
		default:
			if !reflect.DeepEqual(gv, wv) {
				t.Errorf("%s = %#v, want %#v", name, gv, wv)
			}
		}
	}
}

func TestParseDate_WhenFormat_ReadsInBrasilia(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      string
		want    time.Time
		wantErr bool
	}{
		{name: "empty is the zero time", in: "  ", want: time.Time{}},
		{name: "day only", in: "2027-01-15", want: time.Date(2027, 1, 15, 0, 0, 0, 0, brasiliaZone)},
		{name: "RFC 3339 in UTC goes to -03:00", in: "2027-01-15T03:00:00Z", want: time.Date(2027, 1, 15, 0, 0, 0, 0, brasiliaZone)},
		{name: "date-time without offset", in: "2027-01-15T10:20:30", want: time.Date(2027, 1, 15, 10, 20, 30, 0, brasiliaZone)},
		{name: "date-time without offset with fraction", in: "2027-01-15T10:20:30.123", want: time.Date(2027, 1, 15, 10, 20, 30, 123000000, brasiliaZone)},
		{name: "observed: offset without fraction", in: "2027-01-10T14:30:15-03:00", want: time.Date(2027, 1, 10, 14, 30, 15, 0, brasiliaZone)},
		{name: "observed: offset with three fraction digits", in: "2027-01-10T14:30:15.123-03:00", want: time.Date(2027, 1, 10, 14, 30, 15, 123000000, brasiliaZone)},
		{name: "observed: UTC with Z", in: "2027-01-15T03:00:00Z", want: time.Date(2027, 1, 15, 0, 0, 0, 0, brasiliaZone)},
		{name: "slashes are rejected", in: "2026/12/01", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseDate("dataSolicitacao", tc.in)
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), `"dataSolicitacao"`) {
					t.Fatalf("err = %v, want unreadable field error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDate: %v", err)
			}
			if !got.Equal(tc.want) || got.Location() != tc.want.Location() && !tc.want.IsZero() {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDayString_WhenDateAsWritten_KeepsTheDay(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{name: "UTC midnight keeps the day", in: time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC), want: "2027-01-15"},
		{name: "-03:00 midnight keeps the day", in: time.Date(2027, 1, 15, 0, 0, 0, 0, brasiliaZone), want: "2027-01-15"},
		{name: "23:30 at +09:00 keeps the written day", in: time.Date(2027, 1, 15, 23, 30, 0, 0, time.FixedZone("+09:00", 9*60*60)), want: "2027-01-15"},
		{name: "zero time is omitted", in: time.Time{}, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := dayString(tc.in); got != tc.want {
				t.Errorf("dayString = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseDate_WhenRFC3339InUTC_KeepsTheInstant(t *testing.T) {
	t.Parallel()
	got, err := parseDate("dataDenuncia", "2027-01-15T00:00:00Z")
	if err != nil {
		t.Fatalf("parseDate: %v", err)
	}
	if want := time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("got %v, want the instant %v", got, want)
	}
}
