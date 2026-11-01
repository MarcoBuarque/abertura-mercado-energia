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

// Decimal is an exact decimal number, kept as canonical text (no leading or
// trailing zeros, no exponent). It exists so that no precision is lost on
// values the API returns sometimes as a number and sometimes as a string. The
// zero value means "absent", which differs from "0".
//
// It carries no unit: the API does not declare the unit of consumption.
type Decimal struct {
	text string
}

const (
	// maxDecimalText limits the length of the input text.
	maxDecimalText = 400
	// maxDecimalScale limits the position of the decimal point after the
	// exponent is applied.
	maxDecimalScale = 400
)

// decimalFrom converts a [json.Number] (or numeric text) into a [Decimal].
// Empty text yields the absent decimal. It accepts a sign, a decimal point and
// an exponent.
func decimalFrom(n json.Number) (Decimal, error) {
	s := strings.TrimSpace(string(n))
	if s == "" {
		return Decimal{}, nil
	}
	c, err := canonicalDecimal(s)
	if err != nil {
		return Decimal{}, err
	}
	return Decimal{text: c}, nil
}

// canonicalDecimal returns the canonical text of the decimal in s.
func canonicalDecimal(s string) (string, error) {
	invalid := func() (string, error) {
		return "", fmt.Errorf("abertura: invalid decimal")
	}
	if len(s) > maxDecimalText {
		return "", fmt.Errorf("abertura: decimal longer than %d characters", maxDecimalText)
	}
	negative := false
	switch s[0] {
	case '-':
		negative = true
		s = s[1:]
	case '+':
		s = s[1:]
	}
	mantissa, expText, hasExp := strings.Cut(strings.ToLower(s), "e")
	exp := 0
	if hasExp {
		if expText == "" || len(expText) > 6 {
			return invalid()
		}
		sign := 1
		if expText[0] == '-' || expText[0] == '+' {
			if expText[0] == '-' {
				sign = -1
			}
			expText = expText[1:]
		}
		if expText == "" || !onlyDigits(expText) {
			return invalid()
		}
		v, err := strconv.Atoi(expText)
		if err != nil {
			return invalid()
		}
		exp = sign * v
	}
	whole, fraction, _ := strings.Cut(mantissa, ".")
	if whole == "" && fraction == "" {
		return invalid()
	}
	if !onlyDigits(whole) || !onlyDigits(fraction) {
		return invalid()
	}
	digits := whole + fraction
	position := len(whole) + exp
	for len(digits) > 0 && digits[0] == '0' {
		digits = digits[1:]
		position--
	}
	digits = strings.TrimRight(digits, "0")
	if digits == "" {
		return "0", nil
	}
	if position > maxDecimalScale || position < -maxDecimalScale {
		return "", fmt.Errorf("abertura: decimal outside the supported range")
	}
	var out string
	switch {
	case position <= 0:
		out = "0." + strings.Repeat("0", -position) + digits
	case position >= len(digits):
		out = digits + strings.Repeat("0", position-len(digits))
	default:
		out = digits[:position] + "." + digits[position:]
	}
	if negative {
		out = "-" + out
	}
	return out, nil
}

// onlyDigits reports whether s holds only ASCII digits.
func onlyDigits(s string) bool {
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// IsZero reports whether the decimal is absent. The number "0" is not absent.
func (d Decimal) IsZero() bool {
	return d.text == ""
}

// String returns the canonical text ("12.345", "0.5", "1000"). An absent
// decimal yields empty text.
func (d Decimal) String() string {
	return d.text
}

// Rat returns the exact value. An absent decimal yields zero.
func (d Decimal) Rat() *big.Rat {
	r := new(big.Rat)
	if d.text == "" {
		return r
	}
	if _, ok := r.SetString(d.text); !ok {
		// Cannot happen: the canonical text is always a valid decimal.
		return new(big.Rat)
	}
	return r
}

// Float64 returns the closest float64 value, which may lose precision.
// Prefer [Decimal.Rat] or [Decimal.String] where the result must be
// reproducible. An absent decimal yields zero.
func (d Decimal) Float64() float64 {
	if d.text == "" {
		return 0
	}
	f, err := strconv.ParseFloat(d.text, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0
	}
	return f
}

// UnmarshalJSON accepts a number (12.345), a string ("12.345"), an empty
// string and null. The last two leave the decimal absent.
func (d *Decimal) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		*d = Decimal{}
		return nil
	}
	var text string
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &text); err != nil {
			return fmt.Errorf("abertura: decimal: %w", err)
		}
	} else {
		text = string(b)
	}
	v, err := decimalFrom(json.Number(text))
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// MarshalJSON writes the decimal as a JSON number, or null if absent.
func (d Decimal) MarshalJSON() ([]byte, error) {
	if d.text == "" {
		return []byte("null"), nil
	}
	return []byte(d.text), nil
}
