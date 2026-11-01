package abertura

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseYearMonth_WhenAcceptedFormat_ReturnsYearMonth(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    YearMonth
		wantErr bool
	}{
		{name: "year and month", input: "2026-12", want: NewYearMonth(2026, time.December)},
		{name: "month and year", input: "12-2026", want: NewYearMonth(2026, time.December)},
		{name: "date-time with offset", input: "2026-12-01T00:00:00-03:00", want: NewYearMonth(2026, time.December)},
		{name: "date-time with fractional seconds", input: "2026-12-01T00:00:00.123-03:00", want: NewYearMonth(2026, time.December)},
		{name: "date-time near the turn of the month is not converted", input: "2026-12-01T00:30:00+05:00", want: NewYearMonth(2026, time.December)},
		{name: "date only", input: "2026-12-01", want: NewYearMonth(2026, time.December)},
		{name: "date-time without offset", input: "2026-12-01T00:00:00", want: NewYearMonth(2026, time.December)},
		{name: "date-time without offset with fractional seconds", input: "2026-12-01T00:00:00.123", want: NewYearMonth(2026, time.December)},
		{name: "slashes instead of hyphens is an error", input: "2026/12", wantErr: true},
		{name: "date with invalid day is an error", input: "2026-12-32", wantErr: true},
		{name: "surrounding spaces", input: " 2026-01 ", want: NewYearMonth(2026, time.January)},
		{name: "empty is an error", input: "", wantErr: true},
		{name: "month 13 is an error", input: "2026-13", wantErr: true},
		{name: "month 0 is an error", input: "2026-00", wantErr: true},
		{name: "arbitrary text is an error", input: "abc", wantErr: true},
		{name: "single-digit month is an error", input: "2026-1", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseYearMonth(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseYearMonth(%q) = %v, want error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseYearMonth(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseYearMonth(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestYearMonth_WhenRoundTripped_PreservesValue(t *testing.T) {
	tests := []struct {
		name  string
		month YearMonth
		want  string
	}{
		{name: "december", month: NewYearMonth(2026, time.December), want: "2026-12"},
		{name: "january with leading zero", month: NewYearMonth(2027, time.January), want: "2027-01"},
		{name: "short year is padded", month: NewYearMonth(5, time.March), want: "0005-03"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.month.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
			text, err := tc.month.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText: %v", err)
			}
			if string(text) != tc.want {
				t.Errorf("MarshalText() = %q, want %q", text, tc.want)
			}
			var back YearMonth
			if err := back.UnmarshalText(text); err != nil {
				t.Fatalf("UnmarshalText: %v", err)
			}
			if back != tc.month {
				t.Errorf("round trip = %v, want %v", back, tc.month)
			}
		})
	}
}

func TestYearMonth_WhenJSON_UsesText(t *testing.T) {
	type envelope struct {
		M YearMonth `json:"m"`
	}
	b, err := json.Marshal(envelope{M: NewYearMonth(2026, time.December)})
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
	if e.M != NewYearMonth(2026, time.December) {
		t.Errorf("decoded = %v", e.M)
	}
	if err := json.Unmarshal([]byte(`{"m":"junk"}`), &e); err == nil {
		t.Error("want error for invalid month")
	}
}

func TestYearMonth_WhenAbsent_IsZeroWithEmptyText(t *testing.T) {
	tests := []struct {
		name     string
		month    YearMonth
		wantZero bool
	}{
		{name: "zero value", month: YearMonth{}, wantZero: true},
		{name: "filled", month: NewYearMonth(2026, time.June), wantZero: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.month.IsZero(); got != tc.wantZero {
				t.Errorf("IsZero() = %v, want %v", got, tc.wantZero)
			}
			if tc.wantZero && tc.month.String() != "" {
				t.Errorf("String() of zero = %q, want empty", tc.month.String())
			}
		})
	}
	m := NewYearMonth(2026, time.June)
	if err := m.UnmarshalText(nil); err != nil || !m.IsZero() {
		t.Errorf("UnmarshalText empty: m=%v err=%v", m, err)
	}
}

func FuzzParseYearMonth(f *testing.F) {
	for _, s := range []string{"2026-12", "12-2026", "2026-12-01T00:00:00-03:00", "2026-12-01T00:00:00.123-03:00", "", "2026-13", "abc", "0000-01", "9999-12"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		m, err := ParseYearMonth(s)
		if err != nil {
			return
		}
		back, err := ParseYearMonth(m.String())
		if err != nil {
			t.Fatalf("ParseYearMonth(%q) accepted %v, but ParseYearMonth(%q) failed: %v", s, m, m.String(), err)
		}
		if back != m {
			t.Fatalf("ParseYearMonth(%q) = %v, but the round trip gave %v", s, m, back)
		}
	})
}
