package abertura

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"strconv"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura/internal/gen"
)

// Measurements groups the consumption measurement operations. Get it from
// [Client].Measurements.
type Measurements struct{ c *Client }

const routeMeasurements = "/v1/varejista/consumo/medicoes"

// List walks the measurements matching the filter, page by page, with the same
// guarantees as [Migrations.List]. ConsumerUnit, Distributor, From and To are
// required; without them the error is returned before any request.
func (m *Measurements) List(ctx context.Context, f MeasurementFilter, opts ...CallOption) iter.Seq2[Measurement, error] {
	return paginate(ctx, func(ctx context.Context, cursor string) (Page[Measurement], error) {
		return m.ListPage(ctx, f, cursor, opts...)
	})
}

// ListPage fetches one page of measurements. An empty cursor asks for the
// first page. From and To are sent in -03:00. The query does not carry the
// profile, only the header does.
func (m *Measurements) ListPage(ctx context.Context, f MeasurementFilter, cursor string, opts ...CallOption) (Page[Measurement], error) {
	const id = "listMeasurements"
	if err := validateMeasurementFilter(f); err != nil {
		return Page[Measurement]{}, fmt.Errorf("%s: %w", id, err)
	}
	q := url.Values{
		"codigoUnidadeConsumidora":   {f.ConsumerUnit},
		"codigoAgenteConcessionaria": {strconv.Itoa(f.Distributor)},
		"dataInicio":                 {instantString(f.From)},
		"dataFim":                    {instantString(f.To)},
	}
	setIf(q, "situacaoMedicao", string(f.Status))
	setIf(q, "indexProximaPagina", cursor)

	_, body, err := m.c.send(ctx, operation{
		id: id, httpMethod: http.MethodGet, pathTemplate: routeMeasurements, path: routeMeasurements,
		query: q, group: groupMeasurements,
	}, newCallConfig(opts...))
	if err != nil {
		return Page[Measurement]{}, err
	}
	var list gen.MeasurementList
	if err := decodeOutput(body, &list); err != nil {
		return Page[Measurement]{}, fmt.Errorf("%s: %w", id, err)
	}
	page := Page[Measurement]{Items: make([]Measurement, 0, len(list.Medicoes)), Next: list.IndexProximaPagina}
	for _, g := range list.Medicoes {
		meas, err := measurementFrom(g)
		if err != nil {
			return Page[Measurement]{}, fmt.Errorf("%s: %w", id, err)
		}
		page.Items = append(page.Items, meas)
	}
	return page, nil
}

// validateMeasurementFilter checks the four fields the API requires. The error
// does not mention the consumer unit.
func validateMeasurementFilter(f MeasurementFilter) error {
	switch {
	case f.ConsumerUnit == "":
		return invalidArgument("the consumer unit is required")
	case f.Distributor <= 0:
		return invalidArgument("the distributor is required")
	case f.From.IsZero() || f.To.IsZero():
		return invalidArgument("From and To are required")
	}
	return nil
}
