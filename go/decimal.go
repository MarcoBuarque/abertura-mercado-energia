package abertura

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// Decimal é um número decimal exato, guardado como texto canônico (sem zeros
// à esquerda nem à direita, sem expoente). Existe para não perder precisão em
// valores que a API devolve ora como número, ora como texto. O valor zero
// significa "ausente", diferente de "0".
//
// Não carrega unidade: a API não declara a do consumo.
type Decimal struct {
	texto string
}

const (
	// maxTextoDecimal limita o tamanho do texto de entrada.
	maxTextoDecimal = 400
	// maxEscalaDecimal limita a posição da vírgula depois de aplicar o expoente.
	maxEscalaDecimal = 400
)

// decimalDe converte um [json.Number] (ou texto numérico) em [Decimal]. Texto
// vazio devolve o decimal ausente. Aceita sinal, ponto decimal e expoente.
func decimalDe(n json.Number) (Decimal, error) {
	s := strings.TrimSpace(string(n))
	if s == "" {
		return Decimal{}, nil
	}
	c, err := canonicoDecimal(s)
	if err != nil {
		return Decimal{}, err
	}
	return Decimal{texto: c}, nil
}

func canonicoDecimal(s string) (string, error) {
	invalido := func() (string, error) {
		return "", fmt.Errorf("abertura: decimal inválido")
	}
	if len(s) > maxTextoDecimal {
		return "", fmt.Errorf("abertura: decimal com mais de %d caracteres", maxTextoDecimal)
	}
	negativo := false
	switch s[0] {
	case '-':
		negativo = true
		s = s[1:]
	case '+':
		s = s[1:]
	}
	mantissa, expTexto, temExp := strings.Cut(strings.ToLower(s), "e")
	exp := 0
	if temExp {
		if expTexto == "" || len(expTexto) > 6 {
			return invalido()
		}
		sinal := 1
		if expTexto[0] == '-' || expTexto[0] == '+' {
			if expTexto[0] == '-' {
				sinal = -1
			}
			expTexto = expTexto[1:]
		}
		if expTexto == "" || !soDigitos(expTexto) {
			return invalido()
		}
		v, err := strconv.Atoi(expTexto)
		if err != nil {
			return invalido()
		}
		exp = sinal * v
	}
	inteira, fracao, _ := strings.Cut(mantissa, ".")
	if inteira == "" && fracao == "" {
		return invalido()
	}
	if !soDigitos(inteira) || !soDigitos(fracao) {
		return invalido()
	}
	digitos := inteira + fracao
	posicao := len(inteira) + exp
	for len(digitos) > 0 && digitos[0] == '0' {
		digitos = digitos[1:]
		posicao--
	}
	digitos = strings.TrimRight(digitos, "0")
	if digitos == "" {
		return "0", nil
	}
	if posicao > maxEscalaDecimal || posicao < -maxEscalaDecimal {
		return "", fmt.Errorf("abertura: decimal fora da faixa suportada")
	}
	var out string
	switch {
	case posicao <= 0:
		out = "0." + strings.Repeat("0", -posicao) + digitos
	case posicao >= len(digitos):
		out = digitos + strings.Repeat("0", posicao-len(digitos))
	default:
		out = digitos[:posicao] + "." + digitos[posicao:]
	}
	if negativo {
		out = "-" + out
	}
	return out, nil
}

func soDigitos(s string) bool {
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// IsZero informa se o decimal está ausente. O número "0" não é ausente.
func (d Decimal) IsZero() bool {
	return d.texto == ""
}

// String devolve o texto canônico ("12.345", "0.5", "1000"). Ausente vira
// texto vazio.
func (d Decimal) String() string {
	return d.texto
}

// Rat devolve o valor exato. Decimal ausente devolve zero.
func (d Decimal) Rat() *big.Rat {
	r := new(big.Rat)
	if d.texto == "" {
		return r
	}
	if _, ok := r.SetString(d.texto); !ok {
		// Não ocorre: o texto canônico sempre é um decimal válido.
		return new(big.Rat)
	}
	return r
}

// Float64 devolve o valor mais próximo em float64, que pode perder precisão.
// Prefira [Decimal.Rat] ou [Decimal.String] onde o resultado precisa ser
// reproduzível. Ausente devolve zero.
func (d Decimal) Float64() float64 {
	if d.texto == "" {
		return 0
	}
	f, err := strconv.ParseFloat(d.texto, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0
	}
	return f
}

// UnmarshalJSON aceita número (12.345), texto ("12.345"), texto vazio e null.
// Os dois últimos deixam o decimal ausente.
func (d *Decimal) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		*d = Decimal{}
		return nil
	}
	var texto string
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &texto); err != nil {
			return fmt.Errorf("abertura: decimal: %w", err)
		}
	} else {
		texto = string(b)
	}
	v, err := decimalDe(json.Number(texto))
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// MarshalJSON escreve o decimal como número JSON, ou null se ausente.
func (d Decimal) MarshalJSON() ([]byte, error) {
	if d.texto == "" {
		return []byte("null"), nil
	}
	return []byte(d.texto), nil
}
