package abertura

import (
	"fmt"
	"io"
	"log/slog"
	"time"
)

// MigrationStatus is the status of a migration. The set is open: the API may
// return new values, and they pass through without error; use
// [MigrationStatus.Known].
type MigrationStatus string

// Migration statuses published by the API.
const (
	MigrationStatusCreated                   MigrationStatus = "CRIADA"
	MigrationStatusCompleted                 MigrationStatus = "CONCLUIDA"
	MigrationStatusRejected                  MigrationStatus = "REPROVADA"
	MigrationStatusClosed                    MigrationStatus = "ENCERRADA"
	MigrationStatusCanceled                  MigrationStatus = "CANCELADA"
	MigrationStatusExpired                   MigrationStatus = "EXPIRADA"
	MigrationStatusApprovedWithPendingIssues MigrationStatus = "APROVADA_COM_PENDENCIA"
)

// Known reports whether the status is one of those published in this version
// of the SDK.
func (s MigrationStatus) Known() bool {
	switch s {
	case MigrationStatusCreated, MigrationStatusCompleted, MigrationStatusRejected, MigrationStatusClosed,
		MigrationStatusCanceled, MigrationStatusExpired, MigrationStatusApprovedWithPendingIssues:
		return true
	}
	return false
}

// MeasurementStatus is the status of a measurement. The set is open, like
// [MigrationStatus].
type MeasurementStatus string

// Measurement statuses published (see the spec). The public documentation
// cites both; only CONSISTIDA was seen in the sandbox.
const (
	MeasurementStatusValidated MeasurementStatus = "CONSISTIDA"
	MeasurementStatusRejected  MeasurementStatus = "REJEITADA"
)

// Known reports whether the status is one of those published in this version
// of the SDK.
func (s MeasurementStatus) Known() bool {
	return s == MeasurementStatusValidated || s == MeasurementStatusRejected
}

// YesNo is a "SIM" or "NAO" field of the API. The set is open; empty means
// absent.
type YesNo string

// Values of [YesNo].
const (
	YesNoYes YesNo = "SIM"
	YesNoNo  YesNo = "NAO"
)

// Migration is a migration of a consumer unit to the free market, already
// normalized between the list and detail formats of the API. Fields that the
// API did not send hold the zero value (0, empty or absent).
//
// fmt (%v, %+v, %#v, %s) and slog show only the id, the status and the month;
// the fields remain exported and intact for callers who store them in their own
// database. json.Marshal carries the fields in clear text, including the
// consumer unit and the e-mail: the JSON is for storing, never for logs. With
// the slog JSON handler, log the id rather than the whole list, because a
// list goes through json.Marshal.
//
// The JSON is write-only: a serialized Migration does not come back through
// json.Unmarshal, because a Document is masked when serialized and has no way
// to be read back.
type Migration struct {
	// ID is the migration identifier.
	ID string
	// ConsumerUnit is the consumer unit code. Personal data.
	ConsumerUnit string
	// Distributor and Supplier are agent codes; 0 means absent.
	Distributor, Supplier int
	// Retailer is the retailer agent code and Profile is the profile code.
	Retailer, Profile int
	// Document is the consumer unit document. Personal data, always masked when printed.
	Document Document
	// Email is the consumer unit e-mail. Personal data.
	Email string
	// Month is the reference month (mesReferencia in the list, dataReferencia in the detail).
	Month YearMonth
	// Status is the migration status.
	Status MigrationStatus
	// Submarket is the reported submarket.
	Submarket string
	// DHCAmount and MUSDAmount are the DHC and MUSD amounts.
	DHCAmount, MUSDAmount Decimal
	// PenaltyPayment indicates whether a penalty is paid.
	PenaltyPayment YesNo
	// TerminationNoticeDate, RequestDate and ValidationDate are process dates.
	TerminationNoticeDate, RequestDate, ValidationDate time.Time
	// Justification and Comment are free text; ConnectionType is the connection type.
	Justification, Comment, ConnectionType string
	// CERSigningID and SIGAProcessIndicator are auxiliary identifiers.
	CERSigningID, SIGAProcessIndicator string
}

// LogValue implements [slog.LogValuer]. It records only the id, the status and
// the month: logging the whole migration does not leak personal data.
func (m Migration) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", m.ID),
		slog.String("status", string(m.Status)),
		slog.String("month", m.Month.String()),
	)
}

// String describes the migration without personal data: only the id, the
// status and the month. It applies to %v, %+v and %s, and therefore to a
// []Migration printed with fmt or with the slog text handler.
func (m Migration) String() string {
	return fmt.Sprintf("Migration{id=%s status=%s month=%s}", m.ID, m.Status, m.Month)
}

// GoString makes %#v follow String, without the consumer unit, e-mail or document.
func (m Migration) GoString() string { return m.String() }

// Format implements [fmt.Formatter]: every verb prints the masked String.
func (m Migration) Format(f fmt.State, _ rune) { formatMasked(f, m) }

// Measurement is a consumption measurement of a consumer unit.
//
// fmt and slog show only the id and the status; the fields remain exported.
// json.Marshal carries the consumer unit and the consumption in clear text
// (see [Migration]).
type Measurement struct {
	// ID is the measurement identifier (identificadorMedicaoConsumo).
	ID string
	// ConsumerUnit is the consumer unit code. Personal data.
	ConsumerUnit string
	// Distributor is the distributor agent code.
	Distributor int
	// ReadingDate, ReferenceDate and UpdatedAt are the reference day, the consumption
	// reference date and the date of the last update.
	ReadingDate, ReferenceDate, UpdatedAt time.Time
	// Consumption is the measured consumption. The API does not declare the unit.
	Consumption Decimal
	// ConsumptionType is the reported consumption type.
	ConsumptionType string
	// Status is the measurement status.
	Status MeasurementStatus
}

// LogValue implements [slog.LogValuer]. It records only the id and the status:
// no consumer unit and no consumption.
func (m Measurement) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", m.ID),
		slog.String("status", string(m.Status)),
	)
}

// String describes the measurement without personal data: only the id and the
// status.
func (m Measurement) String() string {
	return fmt.Sprintf("Measurement{id=%s status=%s}", m.ID, m.Status)
}

// GoString makes %#v follow String, without the consumer unit and consumption.
func (m Measurement) GoString() string { return m.String() }

// Format implements [fmt.Formatter]: every verb prints the masked String.
func (m Measurement) Format(f fmt.State, _ rune) { formatMasked(f, m) }

// MigrationFilter filters the migration listing. From and To are required;
// zero in the other fields means no filter. fmt and slog mask the consumer
// unit; json.Marshal carries it in clear text, so the JSON is for storing,
// never for logs.
type MigrationFilter struct {
	// From and To delimit the range of reference months, and the API requires
	// both. To must not be earlier than From.
	From, To YearMonth
	// ConsumerUnit filters by consumer unit.
	ConsumerUnit string
	// Status filters by migration status.
	Status MigrationStatus
	// Profile is the profile code; 0 uses the client's profile.
	Profile int
}

// String describes the filter without the consumer unit: only the month range,
// the status and the profile.
func (f MigrationFilter) String() string {
	return fmt.Sprintf("MigrationFilter{from=%s to=%s status=%s profile=%d}", f.From, f.To, f.Status, f.Profile)
}

// GoString makes %#v follow String, without the consumer unit.
func (f MigrationFilter) GoString() string { return f.String() }

// Format implements [fmt.Formatter]: every verb prints the masked String.
func (f MigrationFilter) Format(s fmt.State, _ rune) { formatMasked(s, f) }

// LogValue implements [slog.LogValuer] without the consumer unit.
func (f MigrationFilter) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("from", f.From.String()),
		slog.String("to", f.To.String()),
		slog.String("status", string(f.Status)),
		slog.Int("profile", f.Profile),
	)
}

// MeasurementFilter filters the measurement listing. fmt and slog mask the
// consumer unit; json.Marshal carries it in clear text, so the JSON is for
// storing, never for logs.
type MeasurementFilter struct {
	// ConsumerUnit and Distributor identify the consumer unit.
	ConsumerUnit string
	Distributor  int
	// From and To delimit the period; they are sent as -03:00.
	From, To time.Time
	// Status filters by measurement status.
	Status MeasurementStatus
}

// String describes the filter without the consumer unit: only the distributor,
// the period and the status.
func (f MeasurementFilter) String() string {
	return fmt.Sprintf("MeasurementFilter{distributor=%d from=%s to=%s status=%s}",
		f.Distributor, f.From.Format(layoutDay), f.To.Format(layoutDay), f.Status)
}

// GoString makes %#v follow String, without the consumer unit.
func (f MeasurementFilter) GoString() string { return f.String() }

// Format implements [fmt.Formatter]: every verb prints the masked String.
func (f MeasurementFilter) Format(s fmt.State, _ rune) { formatMasked(s, f) }

// LogValue implements [slog.LogValuer] without the consumer unit.
func (f MeasurementFilter) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("distributor", f.Distributor),
		slog.String("from", f.From.Format(layoutDay)),
		slog.String("to", f.To.Format(layoutDay)),
		slog.String("status", string(f.Status)),
	)
}

// MigrationRequest holds the data needed to create a migration. The retailer
// agent code and the profile come from the client.
type MigrationRequest struct {
	// ConsumerUnit is the consumer unit code. Personal data.
	ConsumerUnit string
	// Distributor is the code of the consumer unit's distributor.
	Distributor int
	// Document is the holder's CPF or CNPJ. Personal data.
	Document Document
	// Email is the holder's contact e-mail. Personal data.
	Email string
	// Month is the reference month of the migration.
	Month YearMonth
	// TerminationNoticeDate is the contract termination notice date. Only the
	// calendar day is sent, as written, in the location of the value: no time
	// zone conversion and no time of day. The zero time omits the field.
	TerminationNoticeDate time.Time
	// Comment is optional free text. It may contain personal data.
	Comment string
}

// String describes the request without the consumer unit, document, e-mail or
// comment: only the distributor and the month.
func (r MigrationRequest) String() string {
	return fmt.Sprintf("MigrationRequest{distributor=%d month=%s}", r.Distributor, r.Month)
}

// GoString makes %#v follow String, without personal data.
func (r MigrationRequest) GoString() string { return r.String() }

// Format implements [fmt.Formatter]: every verb prints the masked String.
func (r MigrationRequest) Format(f fmt.State, _ rune) { formatMasked(f, r) }

// LogValue implements [slog.LogValuer] without personal data.
func (r MigrationRequest) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("distributor", r.Distributor),
		slog.String("month", r.Month.String()),
	)
}

// MigrationEdit holds the editable fields of a migration: only the fields the
// spec describes for the PUT.
type MigrationEdit struct {
	// Month is the reference month sent in the PUT. Set it explicitly when the
	// migration spans more than one month (see [Migrations.Edit]).
	Month YearMonth
	// Profile is the retailer profile code.
	Profile int
	// Document is the holder's CPF or CNPJ. Personal data.
	Document Document
	// Email is the holder's contact e-mail. Personal data.
	Email string
	// Status is the migration status.
	Status MigrationStatus
}

// String describes the edit without document or e-mail: only the month, the
// profile and the status.
func (e MigrationEdit) String() string {
	return fmt.Sprintf("MigrationEdit{month=%s profile=%d status=%s}", e.Month, e.Profile, e.Status)
}

// GoString makes %#v follow String, without personal data.
func (e MigrationEdit) GoString() string { return e.String() }

// Format implements [fmt.Formatter]: every verb prints the masked String.
func (e MigrationEdit) Format(f fmt.State, _ rune) { formatMasked(f, e) }

// LogValue implements [slog.LogValuer] without personal data.
func (e MigrationEdit) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("month", e.Month.String()),
		slog.Int("profile", e.Profile),
		slog.String("status", string(e.Status)),
	)
}

// CreateKey identifies a migration created from its data: a consumer unit
// and distributor with an active migration. It serves to check, after an
// uncertain result, whether the creation happened.
//
// fmt and slog show the consumer unit masked; json.Marshal does not: the JSON
// is meant for storing the key and checking later, and carries the consumer
// unit in clear text. Store it like any other personal data and do not use it
// in logs.
type CreateKey struct {
	// ConsumerUnit is the consumer unit code (UC) of the migration.
	ConsumerUnit string
	// Distributor is the agent code of the distribution utility.
	Distributor int
	// Month is the reference month; the listing requires a month range.
	Month YearMonth
	// Profile is the profile of the query; 0 means the client's profile.
	Profile int
}

// String describes the key without personal data: the consumer unit is masked,
// keeping only its last four characters.
func (k CreateKey) String() string {
	return fmt.Sprintf("migration consumer_unit=%s distributor=%d month=%s profile=%d",
		mask(k.ConsumerUnit, 4), k.Distributor, k.Month, k.Profile)
}

// GoString makes %#v follow String, without the consumer unit in clear text.
func (k CreateKey) GoString() string { return k.String() }

// Format implements [fmt.Formatter]: every verb prints the masked String.
func (k CreateKey) Format(f fmt.State, _ rune) { formatMasked(f, k) }

// LogValue implements [slog.LogValuer] with the masked String: a slog call
// recording the key does not write the consumer unit.
func (k CreateKey) LogValue() slog.Value { return slog.StringValue(k.String()) }

func (CreateKey) naturalKey() {}

// EditKey identifies an edit by the migration id and the desired fields. It
// serves to check, after an uncertain result, whether the edit happened.
//
// fmt and slog show only the id and what changes; json.Marshal carries the
// e-mail in clear text and the document masked, and unmarshaling that JSON
// fails on the document. To store the key, store the fields separately (the
// document through Desired.Document.Number()), as personal data.
type EditKey struct {
	// ID is the migration id.
	ID string
	// Desired holds the fields the edit asked for.
	Desired MigrationEdit
}

// String describes the key without document or e-mail: only the id and what
// changes.
func (k EditKey) String() string {
	return fmt.Sprintf("edit id=%s month=%s profile=%d status=%s",
		k.ID, k.Desired.Month, k.Desired.Profile, k.Desired.Status)
}

// GoString makes %#v follow String, without document or e-mail.
func (k EditKey) GoString() string { return k.String() }

// Format implements [fmt.Formatter]: every verb prints the masked String.
func (k EditKey) Format(f fmt.State, _ rune) { formatMasked(f, k) }

// LogValue implements [slog.LogValuer] with String, without document or e-mail.
func (k EditKey) LogValue() slog.Value { return slog.StringValue(k.String()) }

func (EditKey) naturalKey() {}

// mask replaces every character with '*' except the last n. Text of n
// characters or fewer is fully masked.
func mask(s string, n int) string {
	if s == "" {
		return ""
	}
	if len(s) <= n {
		n = 0
	}
	out := make([]byte, len(s))
	for i := range out {
		out[i] = '*'
	}
	copy(out[len(s)-n:], s[len(s)-n:])
	return string(out)
}

// formatMasked implements [fmt.Formatter] for the types that carry personal
// data: whatever the verb (%v, %+v, %#v, %s, %q, %d, %t, %x), the output is
// the masked String, so no verb reaches the raw fields.
func formatMasked(f fmt.State, v fmt.Stringer) {
	if _, err := io.WriteString(f, v.String()); err != nil {
		// fmt.State has no way to return an error; the write failed at the destination.
		return
	}
}
