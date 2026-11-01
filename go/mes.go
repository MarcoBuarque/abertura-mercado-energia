package abertura

import (
	"fmt"
	"strings"
	"time"
)

// Mes é um mês de referência (ano e mês), sem dia nem fuso. O valor zero
// significa "ausente".
//
// A API usa três formatos para o mesmo conceito: "2026-12" (envio), "12-2026"
// e data e hora com deslocamento, como "2026-12-01T00:00:00-03:00". [ParseMes]
// aceita os três e, por tolerância, a data "2026-12-01" e a data e hora sem
// deslocamento "2026-12-01T00:00:00".
type Mes struct {
	Ano int
	Mes time.Month
}

// MesDe monta um [Mes] a partir do ano e do mês.
func MesDe(ano int, mes time.Month) Mes {
	return Mes{Ano: ano, Mes: mes}
}

// formatosMes são os formatos aceitos por [ParseMes]. O RFC 3339 e a data e
// hora sem deslocamento também leem fração de segundo, mesmo sem ela no
// layout. Os dois últimos foram acrescentados como hipótese para a
// dataReferencia do detalhe de uma migração, cujo formato ainda não está
// registrado (D20).
var formatosMes = []string{"2006-01", "01-2006", time.RFC3339, "2006-01-02", "2006-01-02T15:04:05"}

// ParseMes lê um mês em "2026-12", "12-2026", "2026-12-01T00:00:00-03:00",
// "2026-12-01" ou "2026-12-01T00:00:00" (as datas e horas com ou sem fração de
// segundo). Na data e hora vale o ano e o mês como escritos, sem conversão de
// fuso. Texto vazio, mês fora de 1 a 12 e qualquer
// outro formato devolvem erro.
func ParseMes(s string) (Mes, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Mes{}, fmt.Errorf("abertura: mês vazio")
	}
	for _, layout := range formatosMes {
		t, err := time.Parse(layout, s)
		if err != nil {
			continue
		}
		return Mes{Ano: t.Year(), Mes: t.Month()}, nil
	}
	return Mes{}, fmt.Errorf("abertura: mês %q inválido: use \"2026-12\", \"12-2026\", \"2026-12-01\" ou data e hora (RFC 3339, com ou sem deslocamento)", s)
}

// IsZero informa se o mês está ausente.
func (m Mes) IsZero() bool {
	return m == Mes{}
}

// String devolve o mês como "2026-12", o formato aceito no envio. O mês
// ausente vira texto vazio.
func (m Mes) String() string {
	if m.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d", m.Ano, int(m.Mes))
}

// MarshalText implementa [encoding.TextMarshaler] com o mesmo texto de [Mes.String].
func (m Mes) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}

// UnmarshalText implementa [encoding.TextUnmarshaler] com [ParseMes]. Texto
// vazio deixa o mês ausente.
func (m *Mes) UnmarshalText(texto []byte) error {
	if len(strings.TrimSpace(string(texto))) == 0 {
		*m = Mes{}
		return nil
	}
	v, err := ParseMes(string(texto))
	if err != nil {
		return err
	}
	*m = v
	return nil
}
