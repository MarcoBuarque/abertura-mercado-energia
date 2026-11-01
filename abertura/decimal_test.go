package abertura

import (
	"encoding/json"
	"math/big"
	"testing"
)

func TestDecimal_WhenUnmarshalJSON_AcceptsNumberOrString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     string
		wantZero bool
		wantErr  bool
	}{
		{name: "number", input: `12.345`, want: "12.345"},
		{name: "string", input: `"12.345"`, want: "12.345"},
		{name: "numeric zero is not absent", input: `0`, want: "0"},
		{name: "string with fraction", input: `"0.5"`, want: "0.5"},
		{name: "null is absent", input: `null`, wantZero: true},
		{name: "empty string is absent", input: `""`, wantZero: true},
		{name: "extra zeros are trimmed", input: `"001.500"`, want: "1.5"},
		{name: "negative", input: `-0.25`, want: "-0.25"},
		{name: "positive exponent", input: `1.5e3`, want: "1500"},
		{name: "negative exponent", input: `12E-3`, want: "0.012"},
		{name: "zero with fraction becomes 0", input: `0.000`, want: "0"},
		{name: "non-numeric string", input: `"abc"`, wantErr: true},
		{name: "boolean", input: `true`, wantErr: true},
		{name: "object", input: `{}`, wantErr: true},
		{name: "rational fraction is invalid", input: `"1/2"`, wantErr: true},
		{name: "absurd exponent", input: `1e99999`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var d Decimal
			err := json.Unmarshal([]byte(tc.input), &d)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %q", d)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d.IsZero() != tc.wantZero {
				t.Errorf("IsZero() = %v, want %v", d.IsZero(), tc.wantZero)
			}
			if d.String() != tc.want {
				t.Errorf("String() = %q, want %q", d.String(), tc.want)
			}
		})
	}
}

func TestDecimal_WhenAddingRationals_IsExact(t *testing.T) {
	a, err := decimalFrom("0.1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := decimalFrom("0.2")
	if err != nil {
		t.Fatal(err)
	}
	c, err := decimalFrom("0.3")
	if err != nil {
		t.Fatal(err)
	}
	sum := new(big.Rat).Add(a.Rat(), b.Rat())
	if sum.Cmp(c.Rat()) != 0 {
		t.Errorf("0.1 + 0.2 = %v, want exactly 0.3", sum)
	}
	if a.Float64()+b.Float64() == c.Float64() {
		t.Log("float64 matched, but only Rat is guaranteed")
	}
}

func TestDecimal_WhenAbsent_ReturnsZeroValues(t *testing.T) {
	var d Decimal
	if !d.IsZero() || d.String() != "" || d.Float64() != 0 || d.Rat().Sign() != 0 {
		t.Errorf("absent should zero everything: %q", d)
	}
	b, err := json.Marshal(d)
	if err != nil || string(b) != "null" {
		t.Errorf("Marshal of absent = %s, %v", b, err)
	}
}

func TestDecimal_WhenRoundTripped_PreservesValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  float64
	}{
		{name: "with fraction", input: "12.345", want: 12.345},
		{name: "integer", input: "1000", want: 1000},
		{name: "small", input: "0.001", want: 0.001},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := decimalFrom(json.Number(tc.input))
			if err != nil {
				t.Fatal(err)
			}
			b, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tc.input {
				t.Errorf("Marshal = %s, want %s", b, tc.input)
			}
			if d.Float64() != tc.want {
				t.Errorf("Float64() = %v, want %v", d.Float64(), tc.want)
			}
		})
	}
}
