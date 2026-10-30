package abertura

import (
	"encoding/json"
	"math/big"
	"testing"
)

func TestDecimal_WhenUnmarshalJSON_AceitaNumeroOuTexto(t *testing.T) {
	tests := []struct {
		name     string
		entrada  string
		want     string
		wantZero bool
		wantErr  bool
	}{
		{name: "número", entrada: `12.345`, want: "12.345"},
		{name: "texto", entrada: `"12.345"`, want: "12.345"},
		{name: "zero numérico não é ausente", entrada: `0`, want: "0"},
		{name: "texto com fração", entrada: `"0.5"`, want: "0.5"},
		{name: "null é ausente", entrada: `null`, wantZero: true},
		{name: "texto vazio é ausente", entrada: `""`, wantZero: true},
		{name: "zeros sobrando são tirados", entrada: `"001.500"`, want: "1.5"},
		{name: "negativo", entrada: `-0.25`, want: "-0.25"},
		{name: "expoente positivo", entrada: `1.5e3`, want: "1500"},
		{name: "expoente negativo", entrada: `12E-3`, want: "0.012"},
		{name: "zero com fração vira 0", entrada: `0.000`, want: "0"},
		{name: "texto não numérico", entrada: `"abc"`, wantErr: true},
		{name: "booleano", entrada: `true`, wantErr: true},
		{name: "objeto", entrada: `{}`, wantErr: true},
		{name: "fração racional não vale", entrada: `"1/2"`, wantErr: true},
		{name: "expoente absurdo", entrada: `1e99999`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var d Decimal
			err := json.Unmarshal([]byte(tc.entrada), &d)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("queria erro, veio %q", d)
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if d.IsZero() != tc.wantZero {
				t.Errorf("IsZero() = %v, queria %v", d.IsZero(), tc.wantZero)
			}
			if d.String() != tc.want {
				t.Errorf("String() = %q, queria %q", d.String(), tc.want)
			}
		})
	}
}

func TestDecimal_WhenSomaDeRacionais_EhExato(t *testing.T) {
	a, err := decimalDe("0.1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := decimalDe("0.2")
	if err != nil {
		t.Fatal(err)
	}
	c, err := decimalDe("0.3")
	if err != nil {
		t.Fatal(err)
	}
	soma := new(big.Rat).Add(a.Rat(), b.Rat())
	if soma.Cmp(c.Rat()) != 0 {
		t.Errorf("0.1 + 0.2 = %v, queria 0.3 exato", soma)
	}
	if a.Float64()+b.Float64() == c.Float64() {
		t.Log("float64 coincidiu, mas só o Rat é garantido")
	}
}

func TestDecimal_WhenAusente_ValoresZerados(t *testing.T) {
	var d Decimal
	if !d.IsZero() || d.String() != "" || d.Float64() != 0 || d.Rat().Sign() != 0 {
		t.Errorf("ausente deveria zerar tudo: %q", d)
	}
	b, err := json.Marshal(d)
	if err != nil || string(b) != "null" {
		t.Errorf("Marshal do ausente = %s, %v", b, err)
	}
}

func TestDecimal_WhenIdaEVolta_PreservaValor(t *testing.T) {
	tests := []struct {
		name    string
		entrada string
		want    float64
	}{
		{name: "com fração", entrada: "12.345", want: 12.345},
		{name: "inteiro", entrada: "1000", want: 1000},
		{name: "pequeno", entrada: "0.001", want: 0.001},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := decimalDe(json.Number(tc.entrada))
			if err != nil {
				t.Fatal(err)
			}
			b, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tc.entrada {
				t.Errorf("Marshal = %s, queria %s", b, tc.entrada)
			}
			if d.Float64() != tc.want {
				t.Errorf("Float64() = %v, queria %v", d.Float64(), tc.want)
			}
		})
	}
}
