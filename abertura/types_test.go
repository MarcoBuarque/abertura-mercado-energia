package abertura

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestTypes_WhenMigrationStatus_IsKnownOnlyForPublished(t *testing.T) {
	tests := []struct {
		name   string
		status MigrationStatus
		want   bool
	}{
		{name: "CRIADA", status: MigrationStatusCreated, want: true},
		{name: "CONCLUIDA", status: MigrationStatusCompleted, want: true},
		{name: "REPROVADA", status: MigrationStatusRejected, want: true},
		{name: "ENCERRADA", status: MigrationStatusClosed, want: true},
		{name: "CANCELADA", status: MigrationStatusCanceled, want: true},
		{name: "EXPIRADA", status: MigrationStatusExpired, want: true},
		{name: "APROVADA_COM_PENDENCIA", status: MigrationStatusApprovedWithPendingIssues, want: true},
		{name: "new API value passes through but is not known", status: "EM_ANALISE", want: false},
		{name: "empty is not known", status: "", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.status.Known(); got != tc.want {
				t.Errorf("Known() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTypes_WhenMeasurementStatus_ReportsKnown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status MeasurementStatus
		want   bool
	}{
		{"consistent is known", MeasurementStatusConsistent, true},
		{"rejected is known", MeasurementStatusRejected, true},
		{"new value is not known", MeasurementStatus("NOVA"), false},
		{"empty is not known", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.status.Known(); got != tc.want {
				t.Errorf("Known() = %v, want %v", got, tc.want)
			}
		})
	}
	if MeasurementStatusRejected != "REJEITADA" {
		t.Errorf("MeasurementStatusRejected = %q", MeasurementStatusRejected)
	}
}

func TestTypes_WhenLogValue_DoesNotLeakPersonalData(t *testing.T) {
	consumption, err := decimalFrom("987.654")
	if err != nil {
		t.Fatal(err)
	}
	migration := Migration{
		ID:                    "00000000-0000-0000-0000-000000000001",
		ConsumerUnit:          "000000000000001",
		Email:                 "person@example.org",
		Document:              CNPJ(fakeCNPJ),
		Month:                 NewYearMonth(2026, time.December),
		Status:                MigrationStatusCreated,
		DHCAmount:             consumption,
		TerminationNoticeDate: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
	meas := Measurement{
		ID:           "00000000-0000-0000-0000-000000000002",
		ConsumerUnit: "000000000000001",
		Consumption:  consumption,
		Status:       MeasurementStatusConsistent,
	}

	tests := []struct {
		name           string
		value          slog.LogValuer
		mustContain    []string
		mustNotContain []string
	}{
		{
			name:           "migration",
			value:          migration,
			mustContain:    []string{migration.ID, "CRIADA", "2026-12"},
			mustNotContain: []string{"person@example.org", "example.org", fakeCNPJ, "1111", "000000000000001", "987.654"},
		},
		{
			name:           "measurement",
			value:          meas,
			mustContain:    []string{meas.ID, "CONSISTIDA"},
			mustNotContain: []string{"000000000000001", "987.654", "987"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output := slogJSONNoTime("item", tc.value)
			t.Logf("output: %s", output)
			for _, wanted := range tc.mustContain {
				if !strings.Contains(output, wanted) {
					t.Errorf("missing %q in %s", wanted, output)
				}
			}
			for _, banned := range tc.mustNotContain {
				if strings.Contains(output, banned) {
					t.Errorf("leaked %q in %s", banned, output)
				}
			}
		})
	}
}

func TestTypes_WhenKeyString_OmitsPersonalData(t *testing.T) {
	desired := MigrationEdit{
		Month:    NewYearMonth(2026, time.December),
		Email:    "person@example.org",
		Document: CNPJ(fakeCNPJ),
		Status:   MigrationStatusCanceled,
	}
	tests := []struct {
		name           string
		key            interface{ String() string }
		want           string
		mustNotContain []string
	}{
		{
			name:           "migration",
			key:            MigrationKey{ConsumerUnit: "000000000000001", Distributor: 900001, Month: NewYearMonth(2026, time.December), Profile: 900002},
			want:           "migration consumer_unit=***********0001 distributor=900001 month=2026-12 profile=900002",
			mustNotContain: []string{"000000000000001", fakeCNPJ},
		},
		{
			name:           "edit",
			key:            EditKey{ID: "00000000-0000-0000-0000-000000000001", Desired: desired},
			want:           "edit id=00000000-0000-0000-0000-000000000001 month=2026-12 profile=0 status=CANCELADA",
			mustNotContain: []string{"example.org", fakeCNPJ, "1111"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.key.String()
			if got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
			for _, banned := range tc.mustNotContain {
				if strings.Contains(got, banned) {
					t.Errorf("leaked %q in %q", banned, got)
				}
			}
		})
	}
}

func TestTypes_WhenNaturalKey_BothImplementIt(t *testing.T) {
	// Only proves that the method exists; the NaturalKey interface is tested elsewhere.
	var _ interface{ naturalKey() } = MigrationKey{}
	var _ interface{ naturalKey() } = EditKey{}
}

func TestTypes_WhenMask_KeepsOnlyLastCharacters(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{name: "keeps four", in: "123456", n: 4, want: "**3456"},
		{name: "too short masks everything", in: "123", n: 4, want: "***"},
		{name: "empty", in: "", n: 4, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := mask(tc.in, tc.n); got != tc.want {
				t.Errorf("mask(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}

func TestTypes_WhenMigrationAsJSON_DocumentIsMasked(t *testing.T) {
	b, err := json.Marshal(Migration{Document: CNPJ(fakeCNPJ)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "111") {
		t.Errorf("JSON carries the number: %s", b)
	}
}
