package abertura

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura/internal/testserver"
)

const measurementsTemplate = "/v1/varejista/consumo/medicoes"

func testMeasurementFilter() MeasurementFilter {
	return MeasurementFilter{
		ConsumerUnit: fakeConsumerUnit, Distributor: 900001,
		From: time.Date(2027, 2, 1, 3, 0, 0, 0, time.UTC), // midnight in -03:00
		To:   time.Date(2027, 2, 28, 23, 45, 0, 0, brasiliaZone),
	}
}

func TestMeasurementsList_WhenTwoPages_ConsumptionAsStringAndNumber(t *testing.T) {
	t.Parallel()
	c, srv := specClient(t, []testserver.Route{
		stubRoute(http.MethodGet, measurementsTemplate, fixtureResponse(200, "measurements_page1.json"), fixtureResponse(200, "measurements_page2.json")),
	})
	items, err := collectSeq(t, c.Measurements.List(context.Background(), testMeasurementFilter()))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].Consumption.String() != "12.345" || items[1].Consumption.String() != "0.5" {
		t.Errorf("consumptions = %v and %v", items[0].Consumption, items[1].Consumption)
	}
	if items[0].Distributor != 900001 || items[1].Distributor != 900001 {
		t.Errorf("distributors = %d and %d", items[0].Distributor, items[1].Distributor)
	}
	if items[0].Status != MeasurementStatusConsistent || !items[0].Day.Equal(time.Date(2027, 2, 1, 0, 0, 0, 0, brasiliaZone)) {
		t.Errorf("first measurement = %v", items[0])
	}
	reqs := calls(srv, http.MethodGet, measurementsTemplate)
	if len(reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(reqs))
	}
	want := url.Values{
		"codigoUnidadeConsumidora": {fakeConsumerUnit}, "codigoAgenteConcessionaria": {"900001"},
		"dataInicio": {"2027-02-01T00:00:00-03:00"}, "dataFim": {"2027-02-28T23:45:00-03:00"},
	}
	if got := reqs[0].URL.Query(); !equalValues(got, want) {
		t.Errorf("query of the first page = %v, want %v", got, want)
	}
	want.Set("indexProximaPagina", "fake-cursor-2")
	if got := reqs[1].URL.Query(); !equalValues(got, want) {
		t.Errorf("query of the second page = %v, want %v", got, want)
	}
	if reqs[0].URL.Query().Has("codigoPerfilVarejista") {
		t.Error("measurements do not carry the profile in the query")
	}
}

func TestMeasurementsList_WhenStatus_GoesInTheQuery(t *testing.T) {
	t.Parallel()
	c, srv := specClient(t, []testserver.Route{stubRoute(http.MethodGet, measurementsTemplate, fixtureResponse(200, "measurements_empty.json"))})
	f := testMeasurementFilter()
	f.Status = MeasurementStatusConsistent
	items, err := collectSeq(t, c.Measurements.List(context.Background(), f))
	if err != nil || len(items) != 0 {
		t.Fatalf("items = %d, err = %v", len(items), err)
	}
	if got := calls(srv, http.MethodGet, measurementsTemplate)[0].URL.Query().Get("situacaoMedicao"); got != "CONSISTIDA" {
		t.Errorf("situacaoMedicao = %q", got)
	}
}

func TestMeasurementsListPage_WhenCursor_ReturnsTheNext(t *testing.T) {
	t.Parallel()
	c, _ := specClient(t, []testserver.Route{stubRoute(http.MethodGet, measurementsTemplate, fixtureResponse(200, "measurements_page1.json"))})
	page, err := c.Measurements.ListPage(context.Background(), testMeasurementFilter(), "")
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	if len(page.Items) != 1 || page.Next != "fake-cursor-2" {
		t.Errorf("page = %d items, next %q", len(page.Items), page.Next)
	}
}

func TestMeasurementsList_WhenFilterIncomplete_ErrorsWithoutRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*MeasurementFilter)
	}{
		{name: "without consumer unit", change: func(f *MeasurementFilter) { f.ConsumerUnit = "" }},
		{name: "without distributor", change: func(f *MeasurementFilter) { f.Distributor = 0 }},
		{name: "without start", change: func(f *MeasurementFilter) { f.From = time.Time{} }},
		{name: "without end", change: func(f *MeasurementFilter) { f.To = time.Time{} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, srv := specClient(t, []testserver.Route{stubRoute(http.MethodGet, measurementsTemplate, fixtureResponse(200, "measurements_empty.json"))})
			f := testMeasurementFilter()
			tc.change(&f)
			if _, err := collectSeq(t, c.Measurements.List(context.Background(), f)); err == nil {
				t.Fatal("expected an error")
			}
			if _, err := c.Measurements.ListPage(context.Background(), f, ""); err == nil {
				t.Fatal("ListPage: expected an error")
			}
			if n := srv.Count(http.MethodGet, measurementsTemplate); n != 0 {
				t.Errorf("requests = %d, want 0", n)
			}
		})
	}
}
