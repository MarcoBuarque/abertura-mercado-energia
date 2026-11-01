package abertura

import (
	"fmt"
	"strings"
	"time"
)

// YearMonth is a reference month (year and month), with no day or time zone.
// The zero value means "absent".
//
// The API uses three formats for the same concept: "2026-12" (sending),
// "12-2026" and a date and time with an offset, such as
// "2026-12-01T00:00:00-03:00". [ParseYearMonth] accepts all three and, as a
// tolerance, the date "2026-12-01" and the date and time without an offset
// "2026-12-01T00:00:00".
type YearMonth struct {
	// Year is the calendar year.
	Year int
	// Month is the calendar month, from January to December.
	Month time.Month
}

// NewYearMonth builds a [YearMonth] from a year and a month.
func NewYearMonth(year int, month time.Month) YearMonth {
	return YearMonth{Year: year, Month: month}
}

// monthFormats are the formats accepted by [ParseYearMonth]. RFC 3339 and the
// date and time without an offset also read fractional seconds, even when the
// layout has none. The last two are tolerances: the sandbox returned
// dataReferencia as RFC 3339 with an offset (D21), and the Get failure has
// another cause (D23); they stay because they are harmless.
//
//nolint:gochecknoglobals // read-only table, never written after initialization
var monthFormats = []string{"2006-01", "01-2006", time.RFC3339, "2006-01-02", "2006-01-02T15:04:05"}

// ParseYearMonth reads a month in "2026-12", "12-2026",
// "2026-12-01T00:00:00-03:00", "2026-12-01" or "2026-12-01T00:00:00" (the
// date-times with or without fractional seconds). In a date-time the year and
// month count as written, with no time zone conversion. Empty text, a month
// outside 1 to 12 and any other format return an error.
func ParseYearMonth(s string) (YearMonth, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return YearMonth{}, fmt.Errorf("abertura: empty month")
	}
	for _, layout := range monthFormats {
		t, err := time.Parse(layout, s)
		if err != nil {
			continue
		}
		return YearMonth{Year: t.Year(), Month: t.Month()}, nil
	}
	return YearMonth{}, fmt.Errorf("abertura: invalid month %q: use \"2026-12\", \"12-2026\", \"2026-12-01\" or a date-time (RFC 3339, with or without offset)", s)
}

// IsZero reports whether the month is absent.
func (m YearMonth) IsZero() bool {
	return m == YearMonth{}
}

// valid reports whether the month has a year greater than zero and a month
// from 1 to 12. A YearMonth built by hand may not, and would reach the API as
// "2027-00" or "2027-13".
func (m YearMonth) valid() bool {
	return m.Year > 0 && m.Month >= time.January && m.Month <= time.December
}

// String returns the month as "2026-12", the format accepted when sending. An
// absent month yields empty text.
func (m YearMonth) String() string {
	if m.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d", m.Year, int(m.Month))
}

// MarshalText implements [encoding.TextMarshaler] with the same text as
// [YearMonth.String].
func (m YearMonth) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}

// UnmarshalText implements [encoding.TextUnmarshaler] with [ParseYearMonth].
// Empty text leaves the month absent.
func (m *YearMonth) UnmarshalText(text []byte) error {
	if len(strings.TrimSpace(string(text))) == 0 {
		*m = YearMonth{}
		return nil
	}
	v, err := ParseYearMonth(string(text))
	if err != nil {
		return err
	}
	*m = v
	return nil
}
