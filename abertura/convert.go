package abertura

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura/internal/gen"
)

// brasiliaZone is the API's fixed -03:00 offset. It is fixed and needs no
// tzdata: the API dates always come with that offset.
//
//nolint:gochecknoglobals // read-only table, never written after initialization
var brasiliaZone = time.FixedZone("-03:00", -3*60*60)

const (
	layoutDay     = "2006-01-02"
	layoutInstant = "2006-01-02T15:04:05-07:00"
	// layoutNoOffset is the date and time without an offset. As in
	// ParseYearMonth, Go also reads fractional seconds even when the layout has
	// none.
	layoutNoOffset = "2006-01-02T15:04:05"
)

// fieldError describes an unreadable field without repeating the received
// value, which may be consumer unit data.
func fieldError(field string) error {
	return unreadableResponse(fmt.Errorf("abertura: the response field %q has an unreadable value", field))
}

// parseDate reads a day-only date ("2027-01-15"), a date and time without an
// offset ("2027-01-15T10:20:30", with or without fractional seconds) or an
// RFC 3339 instant. The first two are taken as -03:00 and the instant is
// converted to -03:00. The no-offset format is a tolerance with no observed
// occurrence; the sandbox returned RFC 3339 with an offset or Z (D21). Empty
// text yields the zero time.
func parseDate(field, s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.ParseInLocation(layoutDay, s, brasiliaZone); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation(layoutNoOffset, s, brasiliaZone); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.In(brasiliaZone), nil
	}
	return time.Time{}, fieldError(field)
}

// parseMonth reads the reference month in the three formats the API uses.
// Empty text means absent.
func parseMonth(field, s string) (YearMonth, error) {
	if strings.TrimSpace(s) == "" {
		return YearMonth{}, nil
	}
	m, err := ParseYearMonth(s)
	if err != nil {
		return YearMonth{}, fieldError(field)
	}
	return m, nil
}

// decimalField reads a decimal from the generated model.
func decimalField(field string, n json.Number) (Decimal, error) {
	d, err := decimalFrom(n)
	if err != nil {
		return Decimal{}, fieldError(field)
	}
	return d, nil
}

// documentFrom builds the Document from a type and number pair of the response.
func documentFrom(kind, number string) Document {
	if kind == "" && number == "" {
		return Document{}
	}
	return Document{kind: DocumentType(strings.ToUpper(strings.TrimSpace(kind))), number: cleanDocument(number)}
}

// migrationFields holds the fields that the list and detail formats have in
// common, already separated from the format of each. The month enters with
// the name of its source field so that the error message cites the right one.
type migrationFields struct {
	id, consumerUnit                                   string
	distributor, supplier                              int
	retailer, profile                                  int
	documentType, documentNumber, email                string
	monthField, month                                  string
	status, submarket                                  string
	dhcAmount, musdAmount                              json.Number
	penaltyPayment                                     string
	terminationNoticeDate, requestDate, validationDate string
	justification, comment, connectionType             string
	cerSigningID, sigaProcessIndicator                 string
}

// convert builds the public Migration from the shared fields.
func (c migrationFields) convert() (Migration, error) {
	m := Migration{
		ID: c.id, ConsumerUnit: c.consumerUnit,
		Distributor: c.distributor, Supplier: c.supplier,
		Retailer: c.retailer, Profile: c.profile,
		Document:       documentFrom(c.documentType, c.documentNumber),
		Email:          c.email,
		Status:         MigrationStatus(c.status),
		Submarket:      c.submarket,
		PenaltyPayment: YesNo(c.penaltyPayment),
		Justification:  c.justification, Comment: c.comment, ConnectionType: c.connectionType,
		CERSigningID: c.cerSigningID, SIGAProcessIndicator: c.sigaProcessIndicator,
	}
	var err error
	if m.Month, err = parseMonth(c.monthField, c.month); err != nil {
		return Migration{}, err
	}
	if m.DHCAmount, err = decimalField("valorDHC", c.dhcAmount); err != nil {
		return Migration{}, err
	}
	if m.MUSDAmount, err = decimalField("valorMusd", c.musdAmount); err != nil {
		return Migration{}, err
	}
	if m.TerminationNoticeDate, err = parseDate("dataDenuncia", c.terminationNoticeDate); err != nil {
		return Migration{}, err
	}
	if m.RequestDate, err = parseDate("dataSolicitacao", c.requestDate); err != nil {
		return Migration{}, err
	}
	if m.ValidationDate, err = parseDate("dataValidacao", c.validationDate); err != nil {
		return Migration{}, err
	}
	return m, nil
}

// migrationFromSummary converts a migration in the list format (v1.1).
func migrationFromSummary(r gen.MigrationSummary) (Migration, error) {
	return migrationFields{
		id: r.IdMigracao, consumerUnit: r.CodigoUnidadeConsumidora,
		distributor: r.CodigoAgenteConcessionaria, supplier: r.CodigoAgenteSupridora,
		retailer: r.CodigoAgenteVarejista, profile: r.CodigoPerfilVarejista,
		documentType: r.TipoDocumento, documentNumber: r.NumeroDocumento, email: r.EmailUnidadeConsumidora,
		monthField: "mesReferencia", month: r.MesReferencia,
		status: r.StatusMigracao, submarket: r.Submercado,
		dhcAmount: r.ValorDHC, musdAmount: r.ValorMusd, penaltyPayment: r.PagamentoMulta,
		terminationNoticeDate: r.DataDenuncia, requestDate: r.DataSolicitacao, validationDate: r.DataValidacao,
		justification: r.Justificativa, comment: r.Comentario,
		cerSigningID: r.IdCelebracaoCER, sigaProcessIndicator: r.IndicadorProcessoSiga,
	}.convert()
}

// migrationFromDetail converts a migration in the detail format (POST, GET by
// id and PUT).
func migrationFromDetail(d gen.MigrationDetail) (Migration, error) {
	return migrationFields{
		id: d.IdMigracao, consumerUnit: d.CodigoUnidadeConsumidora,
		distributor: d.CodigoAgenteConcessionaria, supplier: d.CodigoSupridora,
		retailer: d.CodigoAgenteVarejista, profile: d.CodigoPerfilVarejista,
		documentType: d.TipoDocumento, documentNumber: d.NumeroDocumento, email: d.EmailUnidadeConsumidora,
		monthField: "dataReferencia", month: d.DataReferencia,
		status: d.StatusMigracao, submarket: d.Submercado,
		dhcAmount: d.ValorDHC, musdAmount: d.ValorMusd, penaltyPayment: d.PagamentoMulta,
		terminationNoticeDate: d.DataDenuncia, requestDate: d.DataSolicitacao, validationDate: d.DataValidacao,
		justification: d.Justificativa, comment: d.Comentario, connectionType: d.TipoConectado,
		sigaProcessIndicator: d.IndicadorProcessoSiga,
	}.convert()
}

// measurementFrom converts a measurement.
func measurementFrom(m gen.Measurement) (Measurement, error) {
	out := Measurement{
		ID: m.IdentificadorMedicaoConsumo, ConsumerUnit: m.CodigoUnidadeConsumidora,
		ConsumptionType: m.TipoConsumo, Status: MeasurementStatus(m.SituacaoMedicao),
	}
	var err error
	if s := strings.TrimSpace(m.CodigoAgenteConcessionaria.String()); s != "" {
		if out.Distributor, err = strconv.Atoi(s); err != nil {
			return Measurement{}, fieldError("codigoAgenteConcessionaria")
		}
	}
	if out.Consumption, err = decimalField("consumo", m.Consumo); err != nil {
		return Measurement{}, err
	}
	if out.Day, err = parseDate("diaReferencia", m.DiaReferencia); err != nil {
		return Measurement{}, err
	}
	if out.Reference, err = parseDate("dataReferenciaConsumo", m.DataReferenciaConsumo); err != nil {
		return Measurement{}, err
	}
	if out.UpdatedAt, err = parseDate("dataAtualizacao", m.DataAtualizacao); err != nil {
		return Measurement{}, err
	}
	return out, nil
}

// dayString writes the calendar day as the caller wrote it, in the caller's
// location, without time zone conversion. The zero time becomes empty text.
func dayString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(layoutDay)
}

// instantString writes an instant with a -03:00 offset.
func instantString(t time.Time) string {
	return t.In(brasiliaZone).Format(layoutInstant)
}

// migrationRequestToGen converts the public request into the POST body. A
// Document serializes masked, so the body carries Number(), which is possible
// because the generated type is internal.
func migrationRequestToGen(n MigrationRequest, agent, profile int) gen.MigrationRequest {
	return gen.MigrationRequest{
		CodigoAgenteConcessionaria: n.Distributor,
		CodigoAgenteVarejista:      agent,
		CodigoPerfilVarejista:      profile,
		CodigoUnidadeConsumidora:   n.ConsumerUnit,
		Comentario:                 n.Comment,
		DataDenuncia:               dayString(n.TerminationNoticeDate),
		EmailUnidadeConsumidora:    n.Email,
		MesReferencia:              n.Month.String(),
		NumeroDocumento:            n.Document.Number(),
		TipoDocumento:              gen.MigrationRequestTipoDocumento(n.Document.Type()),
	}
}

// editToGen converts the public edit into the PUT body.
func editToGen(e MigrationEdit) gen.MigrationEdit {
	return gen.MigrationEdit{
		CodigoPerfilVarejista:   e.Profile,
		EmailUnidadeConsumidora: e.Email,
		MesReferencia:           e.Month.String(),
		NumeroDocumento:         e.Document.Number(),
		StatusMigracao:          string(e.Status),
		TipoDocumento:           gen.MigrationEditTipoDocumento(e.Document.Type()),
	}
}
