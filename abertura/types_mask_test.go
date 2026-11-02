package abertura

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// Fake data that no printing form may show.
const (
	maskEmail        = "person@example.org"
	maskConsumerUnit = "000000000000123"
	maskConsumption  = "98765.4321"
	maskFreeText     = "fake-free-text"
	documentNumber   = "11111111111111"
)

func personalTerms() []string {
	return []string{maskEmail, "person", maskConsumerUnit, "00000000000", maskConsumption, "98765", maskFreeText, documentNumber, "1111111111"}
}

func migrationWithData(t *testing.T) Migration {
	t.Helper()
	return Migration{
		ID: "migration-id-1", ConsumerUnit: maskConsumerUnit, Distributor: 900003, Supplier: 900004,
		Retailer: fakeAgent, Profile: fakeProfile,
		Document: CNPJ(documentNumber), Email: maskEmail,
		Month: NewYearMonth(2027, time.January), Status: MigrationStatusCreated,
		Justification: maskFreeText, Comment: maskFreeText,
	}
}

func measurementWithData(t *testing.T) Measurement {
	t.Helper()
	consumption, err := decimalFrom(json.Number(maskConsumption))
	if err != nil {
		t.Fatalf("decimalFrom: %v", err)
	}
	return Measurement{ID: "measurement-id-1", ConsumerUnit: maskConsumerUnit, Distributor: 900003, Consumption: consumption, Status: MeasurementStatusValidated}
}

// forms prints v in every fmt form and in both slog handlers.
func forms(v any) map[string]string {
	out := map[string]string{
		"%v":  fmt.Sprintf("%v", v),
		"%+v": fmt.Sprintf("%+v", v),
		"%#v": fmt.Sprintf("%#v", v),
		"%s":  fmt.Sprintf("%s", v),
		"%q":  fmt.Sprintf("%q", v),
		"%d":  fmt.Sprintf("%d", v),
		"%t":  fmt.Sprintf("%t", v),
		"%x":  fmt.Sprintf("%x", v),
	}
	var text, js bytes.Buffer
	slog.New(slog.NewTextHandler(&text, nil)).Info("m", "v", v)
	slog.New(slog.NewJSONHandler(&js, nil)).Info("m", "v", v)
	out["slog text"], out["slog json"] = text.String(), js.String()
	return out
}

func TestPublicTypes_WhenPrinted_DoNotShowPersonalData(t *testing.T) {
	t.Parallel()

	migrationFilter := MigrationFilter{From: NewYearMonth(2027, time.January), To: NewYearMonth(2027, time.March), ConsumerUnit: maskConsumerUnit, Status: MigrationStatusCreated, Profile: fakeProfile}
	measurementFilter := MeasurementFilter{ConsumerUnit: maskConsumerUnit, Distributor: 900003, From: time.Date(2027, 1, 1, 0, 0, 0, 0, brasiliaZone), To: time.Date(2027, 1, 31, 0, 0, 0, 0, brasiliaZone), Status: MeasurementStatusValidated}
	tests := []struct {
		name  string
		value any
		want  string // a fragment that may appear, proving the output is not empty
	}{
		{name: "Migration", value: migrationWithData(t), want: "migration-id-1"},
		{name: "*Migration", value: func() *Migration { m := migrationWithData(t); return &m }(), want: "migration-id-1"},
		{name: "Measurement", value: measurementWithData(t), want: "measurement-id-1"},
		{name: "MigrationRequest", value: MigrationRequest{ConsumerUnit: maskConsumerUnit, Distributor: 900003, Document: CNPJ(documentNumber), Email: maskEmail, Month: NewYearMonth(2027, time.January), Comment: maskFreeText}, want: "900003"},
		{name: "MigrationEdit", value: MigrationEdit{Month: NewYearMonth(2027, time.February), Profile: fakeProfile, Document: CNPJ(documentNumber), Email: maskEmail, Status: MigrationStatusCanceled}, want: "CANCELADA"},
		{name: "MigrationFilter", value: migrationFilter, want: "CRIADA"},
		{name: "MeasurementFilter", value: measurementFilter, want: "CONSISTIDA"},
		{name: "Page[Migration]", value: Page[Migration]{Items: []Migration{migrationWithData(t)}, NextCursor: "fake-cursor-2"}, want: "migration-id-1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for form, output := range forms(tc.value) {
				if output == "" {
					t.Errorf("%s: empty output", form)
				}
				for _, banned := range personalTerms() {
					if strings.Contains(output, banned) {
						t.Errorf("%s leaks %q: %s", form, banned, output)
					}
				}
			}
			if s := fmt.Sprintf("%v", tc.value); !strings.Contains(s, tc.want) {
				t.Errorf("%%v = %q, should keep %q (non-personal data)", s, tc.want)
			}
		})
	}
}

func TestPublicTypes_WhenSlice_DoesNotShowPersonalDataInTextHandler(t *testing.T) {
	t.Parallel()

	list := []Migration{migrationWithData(t), migrationWithData(t)}
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("m", "list", list)
	for _, banned := range personalTerms() {
		if strings.Contains(buf.String(), banned) {
			t.Errorf("slog text of []Migration leaks %q: %s", banned, buf.String())
		}
	}
	for _, form := range []string{"%v", "%+v", "%#v"} {
		if s := fmt.Sprintf(form, list); strings.Contains(s, maskConsumerUnit) || strings.Contains(s, maskEmail) {
			t.Errorf("%s of []Migration leaks: %s", form, s)
		}
	}
}

// TestPublicTypes_WhenJSON_CarriesFieldsInClear proves that masking does not
// affect json.Marshal: the JSON is for storing, and the fields remain exported.
func TestPublicTypes_WhenJSON_CarriesFieldsInClear(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
		want  []string
	}{
		{name: "Migration", value: migrationWithData(t), want: []string{maskConsumerUnit, maskEmail}},
		{name: "Measurement", value: measurementWithData(t), want: []string{maskConsumerUnit, maskConsumption}},
		{name: "MigrationRequest", value: MigrationRequest{ConsumerUnit: maskConsumerUnit, Email: maskEmail}, want: []string{maskConsumerUnit, maskEmail}},
		{name: "MigrationFilter", value: MigrationFilter{From: fromMonth, To: toMonth, ConsumerUnit: maskConsumerUnit}, want: []string{maskConsumerUnit}},
		{name: "MeasurementFilter", value: MeasurementFilter{ConsumerUnit: maskConsumerUnit}, want: []string{maskConsumerUnit}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			for _, q := range tc.want {
				if !strings.Contains(string(b), q) {
					t.Errorf("the JSON should carry %q in clear text: %s", q, b)
				}
			}
		})
	}
}
