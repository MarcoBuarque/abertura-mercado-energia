package abertura

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseMes_WhenFormatoAceito_ReturnsMes(t *testing.T) {
	tests := []struct {
		name    string
		entrada string
		want    Mes
		wantErr bool
	}{
		{name: "ano e mês", entrada: "2026-12", want: MesDe(2026, time.December)},
		{name: "mês e ano", entrada: "12-2026", want: MesDe(2026, time.December)},
		{name: "data e hora com deslocamento", entrada: "2026-12-01T00:00:00-03:00", want: MesDe(2026, time.December)},
		{name: "data e hora com fração de segundo", entrada: "2026-12-01T00:00:00.123-03:00", want: MesDe(2026, time.December)},
		{name: "data e hora perto da virada não converte o fuso", entrada: "2026-12-01T00:30:00+05:00", want: MesDe(2026, time.December)},
		{name: "só a data", entrada: "2026-12-01", want: MesDe(2026, time.December)},
		{name: "data e hora sem deslocamento", entrada: "2026-12-01T00:00:00", want: MesDe(2026, time.December)},
		{name: "data e hora sem deslocamento com fração de segundo", entrada: "2026-12-01T00:00:00.123", want: MesDe(2026, time.December)},
		{name: "barras em vez de hífens é erro", entrada: "2026/12", wantErr: true},
		{name: "data com dia inválido é erro", entrada: "2026-12-32", wantErr: true},
		{name: "com espaços ao redor", entrada: " 2026-01 ", want: MesDe(2026, time.January)},
		{name: "vazio é erro", entrada: "", wantErr: true},
		{name: "mês 13 é erro", entrada: "2026-13", wantErr: true},
		{name: "mês 0 é erro", entrada: "2026-00", wantErr: true},
		{name: "texto qualquer é erro", entrada: "abc", wantErr: true},
		{name: "mês de um dígito é erro", entrada: "2026-1", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseMes(tc.entrada)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseMes(%q) = %v, queria erro", tc.entrada, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMes(%q) erro inesperado: %v", tc.entrada, err)
			}
			if got != tc.want {
				t.Errorf("ParseMes(%q) = %v, queria %v", tc.entrada, got, tc.want)
			}
		})
	}
}

func TestMes_WhenIdaEVolta_PreservaValor(t *testing.T) {
	tests := []struct {
		name string
		mes  Mes
		want string
	}{
		{name: "dezembro", mes: MesDe(2026, time.December), want: "2026-12"},
		{name: "janeiro com zero à esquerda", mes: MesDe(2027, time.January), want: "2027-01"},
		{name: "ano curto preenchido", mes: MesDe(5, time.March), want: "0005-03"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.mes.String(); got != tc.want {
				t.Errorf("String() = %q, queria %q", got, tc.want)
			}
			texto, err := tc.mes.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText: %v", err)
			}
			if string(texto) != tc.want {
				t.Errorf("MarshalText() = %q, queria %q", texto, tc.want)
			}
			var volta Mes
			if err := volta.UnmarshalText(texto); err != nil {
				t.Fatalf("UnmarshalText: %v", err)
			}
			if volta != tc.mes {
				t.Errorf("ida e volta = %v, queria %v", volta, tc.mes)
			}
		})
	}
}

func TestMes_WhenJSON_UsaTexto(t *testing.T) {
	type envelope struct {
		M Mes `json:"m"`
	}
	b, err := json.Marshal(envelope{M: MesDe(2026, time.December)})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"m":"2026-12"}` {
		t.Errorf("json = %s", b)
	}
	var e envelope
	if err := json.Unmarshal([]byte(`{"m":"12-2026"}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.M != MesDe(2026, time.December) {
		t.Errorf("decodificado = %v", e.M)
	}
	if err := json.Unmarshal([]byte(`{"m":"lixo"}`), &e); err == nil {
		t.Error("queria erro para mês inválido")
	}
}

func TestMes_WhenAusente_IsZeroETextoVazio(t *testing.T) {
	tests := []struct {
		name     string
		mes      Mes
		wantZero bool
	}{
		{name: "valor zero", mes: Mes{}, wantZero: true},
		{name: "preenchido", mes: MesDe(2026, time.June), wantZero: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.mes.IsZero(); got != tc.wantZero {
				t.Errorf("IsZero() = %v, queria %v", got, tc.wantZero)
			}
			if tc.wantZero && tc.mes.String() != "" {
				t.Errorf("String() do zero = %q, queria vazio", tc.mes.String())
			}
		})
	}
	m := MesDe(2026, time.June)
	if err := m.UnmarshalText(nil); err != nil || !m.IsZero() {
		t.Errorf("UnmarshalText vazio: m=%v err=%v", m, err)
	}
}

func FuzzParseMes(f *testing.F) {
	for _, s := range []string{"2026-12", "12-2026", "2026-12-01T00:00:00-03:00", "2026-12-01T00:00:00.123-03:00", "", "2026-13", "abc", "0000-01", "9999-12"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		m, err := ParseMes(s)
		if err != nil {
			return
		}
		volta, err := ParseMes(m.String())
		if err != nil {
			t.Fatalf("ParseMes(%q) aceitou %v, mas ParseMes(%q) falhou: %v", s, m, m.String(), err)
		}
		if volta != m {
			t.Fatalf("ParseMes(%q) = %v, mas a volta deu %v", s, m, volta)
		}
	})
}
