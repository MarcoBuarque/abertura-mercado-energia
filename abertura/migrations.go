package abertura

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura/internal/gen"
)

// Migrations groups the consumer unit migration operations. Get it from
// [Client].Migrations.
type Migrations struct{ c *Client }

const (
	routeListMigrations = "/v1.1/varejista/migracoes"
	routeMigrationV11   = "/v1.1/varejista/migracoes/{idMigracao}"
	routeMigrationV1    = "/v1/varejista/migracoes/{idMigracao}"
)

// List walks the migrations matching the filter, page by page. Nothing is
// fetched before the first range step, and a break stops the fetching. An
// error is yielded as (zero, err) once and ends the iteration; a repeated
// cursor from the API yields [ErrRepeatedCursor].
//
// MigrationFilter.From and To are required (the API requires the month range);
// without them, or with To before From, the error is returned before any
// request, as [ErrInvalidArgument]. The API also limits the width of the
// range, and the limit is not published: see [ErrMonthRangeTooWide].
//
// The profile of the query is the client's (or the call's, with [WithAgent]);
// MigrationFilter.Profile overrides it.
func (m *Migrations) List(ctx context.Context, f MigrationFilter, opts ...CallOption) iter.Seq2[Migration, error] {
	return paginate(ctx, func(ctx context.Context, cursor string) (Page[Migration], error) {
		return m.ListPage(ctx, f, cursor, opts...)
	})
}

// ListPage fetches one page. An empty cursor asks for the first page; the
// returned [Page].Next can be used as a cursor to resume later.
func (m *Migrations) ListPage(ctx context.Context, f MigrationFilter, cursor string, opts ...CallOption) (Page[Migration], error) {
	const id = "listMigrations"
	if err := validateMonthRange(f); err != nil {
		return Page[Migration]{}, fmt.Errorf("%s: %w", id, err)
	}
	p := m.c.params(newCallConfig(opts...))
	profile := p.profile
	if f.Profile != 0 {
		profile = f.Profile
	}
	q := url.Values{"codigoPerfilVarejista": {strconv.Itoa(profile)}}
	q.Set("mesReferenciaInicial", f.From.String())
	q.Set("mesReferenciaFinal", f.To.String())
	setIf(q, "codigoUnidadeConsumidora", f.ConsumerUnit)
	setIf(q, "statusMigracao", string(f.Status))
	setIf(q, "indexProximaPagina", cursor)

	_, body, err := m.c.send(ctx, operation{
		id: id, httpMethod: http.MethodGet, pathTemplate: routeListMigrations, path: routeListMigrations,
		query: q, group: groupGeneral,
	}, newCallConfig(opts...))
	if err != nil {
		return Page[Migration]{}, err
	}
	var list gen.ListaMigracoes
	if err := decodeOutput(body, &list); err != nil {
		return Page[Migration]{}, fmt.Errorf("%s: %w", id, err)
	}
	page := Page[Migration]{Items: make([]Migration, 0, len(list.Migracao)), Next: list.IndexProximaPagina}
	for _, r := range list.Migracao {
		migration, err := migrationFromSummary(r)
		if err != nil {
			return Page[Migration]{}, fmt.Errorf("%s: %w", id, err)
		}
		page.Items = append(page.Items, migration)
	}
	return page, nil
}

// Get returns a migration by id. The API answers with a one-item list or with
// an object; the SDK accepts both. An empty list yields [ErrNotFound].
//
// If the list has more than one item, the item whose IdMigracao is the
// requested id wins (compared after trimming spaces): if exactly one matches,
// it is returned; if none matches, the result is [ErrNotFound]. If more than
// one matches and all are equal once decoded (field order does not count), the
// first wins.
//
// The detail may carry one entry per reference month of the same migration
// (observed on 2026-11-01: two entries that differed only in dataReferencia;
// see SOURCES D23). In that case the SDK returns the entry of the most recent
// month; the rule is the SDK's, since the API does not document which month is
// the current one. To read all rows, use [Client.Do]. If the entries differ in
// any other field, or some month is unreadable, there is no way to know which
// one applies and the error is an unreadable response.
func (m *Migrations) Get(ctx context.Context, id string, opts ...CallOption) (Migration, error) {
	const op = "getMigration"
	id, err := validateID(op, id)
	if err != nil {
		return Migration{}, err
	}
	_, body, err := m.c.send(ctx, operation{
		id: op, httpMethod: http.MethodGet, pathTemplate: routeMigrationV11,
		path: strings.Replace(routeMigrationV11, "{idMigracao}", url.PathEscape(id), 1), group: groupGeneral,
	}, newCallConfig(opts...))
	if err != nil {
		return Migration{}, err
	}
	items, err := detailsFrom(body)
	if err != nil {
		return Migration{}, fmt.Errorf("%s: %w", op, err)
	}
	if len(items) >= 1 {
		items = slices.DeleteFunc(items, func(d gen.MigracaoDetalhe) bool {
			return strings.TrimSpace(d.IdMigracao) != id
		})
	}
	if len(items) == 0 {
		return Migration{}, fmt.Errorf("%s: %w", op, ErrNotFound)
	}
	chosen := items[0]
	if len(items) > 1 {
		var ok bool
		if chosen, ok = currentEntry(items); !ok {
			return Migration{}, fmt.Errorf("%s: %w", op, unreadableResponse(errors.New("the API returned more than one migration for the id")))
		}
	}
	migration, err := migrationFromDetail(chosen)
	if err != nil {
		return Migration{}, fmt.Errorf("%s: %w", op, err)
	}
	return migration, nil
}

// currentEntry picks one of several entries with the same id. If all are equal
// once decoded, the first wins. If only dataReferencia differs, the most
// recent month wins (the first one, on a tie). If anything else differs, or
// some month cannot be read, it returns false.
func currentEntry(items []gen.MigracaoDetalhe) (gen.MigracaoDetalhe, bool) {
	base := items[0]
	base.DataReferencia = ""
	for _, d := range items[1:] {
		other := d
		other.DataReferencia = ""
		if !reflect.DeepEqual(base, other) {
			return gen.MigracaoDetalhe{}, false
		}
	}
	allEqual := true
	for _, d := range items[1:] {
		if !reflect.DeepEqual(items[0], d) {
			allEqual = false
			break
		}
	}
	if allEqual {
		return items[0], true
	}
	best, bestMonth := items[0], YearMonth{}
	for i, d := range items {
		m, err := ParseYearMonth(d.DataReferencia)
		if err != nil {
			return gen.MigracaoDetalhe{}, false
		}
		if i == 0 || m.Year > bestMonth.Year || m.Year == bestMonth.Year && m.Month > bestMonth.Month {
			best, bestMonth = d, m
		}
	}
	return best, true
}

// Create registers a new migration. The retailer code and the profile come
// from the client (or from the call, with [WithAgent]).
//
// The write is not retried. If the result is uncertain, the error is a
// [*UncertainResultError] with a [MigrationKey]: check it with
// [Migrations.CheckCreated]. Repeating the creation while a migration is
// active yields [ErrMigrationAlreadyRequested].
func (m *Migrations) Create(ctx context.Context, n MigrationRequest, opts ...CallOption) (Migration, error) {
	const op = "createMigration"
	p := m.c.params(newCallConfig(opts...))
	key := MigrationKey{ConsumerUnit: n.ConsumerUnit, Distributor: n.Distributor, Month: n.Month, Profile: p.profile}
	_, body, err := m.c.send(ctx, operation{
		id: op, httpMethod: http.MethodPost, pathTemplate: routeListMigrations, path: routeListMigrations,
		body: migrationRequestToGen(n, p.agent, p.profile), group: groupGeneral, key: key,
	}, newCallConfig(opts...))
	if err != nil {
		return Migration{}, err
	}
	migration, err := writtenMigration(body)
	if err != nil {
		// The server accepted the request, but we do not know what was created.
		return Migration{}, &UncertainResultError{Operation: op, Key: key, Err: fmt.Errorf("%s: %w", op, err)}
	}
	return migration, nil
}

// Edit reads the migration, applies change to the editable fields and sends
// the whole object. Since it is unknown whether the API's PUT is partial or
// total, sending everything gives the same result in both cases, at the cost
// of a GET first.
//
// When the detail has one row per reference month, [Migrations.Get] picks the
// most recent month and Edit sends it in the full PUT body; the semantics of
// the PUT (partial or total, and for which month) have not been verified. If
// the migration spans more than one month, set [MigrationEdit].Month
// explicitly in change.
//
// There is no ETag: another write between the GET and the PUT may be
// overwritten. If the result of the PUT is uncertain, the error is a
// [*UncertainResultError] with an [EditKey]; check it with
// [Migrations.CheckEdited].
func (m *Migrations) Edit(ctx context.Context, id string, change func(*MigrationEdit), opts ...CallOption) (Migration, error) {
	const op = "editMigration"
	if change == nil {
		return Migration{}, invalidArgument("%s: the change function must not be nil", op)
	}
	current, err := m.Get(ctx, id, opts...)
	if err != nil {
		return Migration{}, err
	}
	e := MigrationEdit{
		Month: current.Month, Profile: current.Profile, Document: current.Document,
		Email: current.Email, Status: current.Status,
	}
	change(&e)
	id = strings.TrimSpace(id)
	key := EditKey{ID: id, Desired: e}
	_, body, err := m.c.send(ctx, operation{
		id: op, httpMethod: http.MethodPut, pathTemplate: routeMigrationV1,
		path: strings.Replace(routeMigrationV1, "{idMigracao}", url.PathEscape(id), 1),
		body: editToGen(e), group: groupGeneral, key: key,
	}, newCallConfig(opts...))
	if err != nil {
		return Migration{}, err
	}
	migration, err := writtenMigration(body)
	if err != nil {
		return Migration{}, &UncertainResultError{Operation: op, Key: key, Err: fmt.Errorf("%s: %w", op, err)}
	}
	return migration, nil
}

// CheckCreated reports what exists for the key after an uncertain result in
// [Migrations.Create]. It lists by consumer unit and month, filters by
// distributor and returns what it finds, from newest to oldest. An empty
// result means "not created". The filter ignores the status: an earlier
// migration of the same consumer unit and month (canceled, for example) also
// comes back, so check Status and RequestDate before concluding that the
// creation happened. A MigrationKey with a zero Month (only possible if built
// by hand) fails with [ErrInvalidArgument] before any request, because the
// listing requires a month range.
func (m *Migrations) CheckCreated(ctx context.Context, k MigrationKey, opts ...CallOption) ([]Migration, error) {
	filter := MigrationFilter{From: k.Month, To: k.Month, ConsumerUnit: k.ConsumerUnit, Profile: k.Profile}
	found := make([]Migration, 0)
	for migration, err := range m.List(ctx, filter, opts...) {
		if err != nil {
			return nil, err
		}
		if k.Distributor == 0 || migration.Distributor == k.Distributor {
			found = append(found, migration)
		}
	}
	slices.SortStableFunc(found, func(a, b Migration) int {
		if c := b.RequestDate.Compare(a.RequestDate); c != 0 {
			return c
		}
		if c := b.Month.Year - a.Month.Year; c != 0 {
			return c
		}
		return int(b.Month.Month) - int(a.Month.Month)
	})
	return found, nil
}

// CheckEdited reads the migration and reports whether it already has the
// desired fields of the key. Zero fields in EditKey.Desired are left out of the
// comparison. It returns the migration as it is now.
func (m *Migrations) CheckEdited(ctx context.Context, k EditKey, opts ...CallOption) (Migration, bool, error) {
	current, err := m.Get(ctx, k.ID, opts...)
	if err != nil {
		return Migration{}, false, err
	}
	d := k.Desired
	matches := (d.Month.IsZero() || d.Month == current.Month) &&
		(d.Profile == 0 || d.Profile == current.Profile) &&
		(d.Document.IsZero() || (d.Document.Type() == current.Document.Type() && d.Document.Number() == current.Document.Number())) &&
		(d.Email == "" || d.Email == current.Email) &&
		(d.Status == "" || d.Status == current.Status)
	return current, matches, nil
}

// validateMonthRange checks the month range, which the API requires: without
// mesReferenciaInicial and mesReferenciaFinal it answers 400
// (ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO). The error is returned before any
// request. The cap on the range is enforced by the API ([ErrMonthRangeTooWide]).
func validateMonthRange(f MigrationFilter) error {
	switch {
	case f.From.IsZero() && f.To.IsZero():
		return invalidArgument("the month range is required: fill From and To")
	case f.From.IsZero():
		return invalidArgument("the From field is required")
	case f.To.IsZero():
		return invalidArgument("the To field is required")
	case !f.From.valid():
		return invalidArgument("the From field is not a valid month: use a year greater than zero and a month from 1 to 12")
	case !f.To.valid():
		return invalidArgument("the To field is not a valid month: use a year greater than zero and a month from 1 to 12")
	case f.To.Year*12+int(f.To.Month) < f.From.Year*12+int(f.From.Month):
		return invalidArgument("the To field must not be before From")
	}
	return nil
}

// validateID checks a migration id without repeating it in the message.
func validateID(op, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", invalidArgument("%s: the migration id must not be empty", op)
	}
	return id, nil
}

// detailsFrom reads the body of a detail response, which the API sends as a
// list or as an object.
func detailsFrom(body []byte) ([]gen.MigracaoDetalhe, error) {
	t := bytes.TrimSpace(body)
	var items []gen.MigracaoDetalhe
	if len(t) > 0 && t[0] == '[' {
		if err := decodeOutput(t, &items); err != nil {
			return nil, err
		}
		return items, nil
	}
	if len(t) == 0 || bytes.Equal(t, []byte("null")) {
		return nil, unreadableResponse(errors.New("empty response"))
	}
	var single gen.MigracaoDetalhe
	if err := decodeOutput(t, &single); err != nil {
		return nil, err
	}
	return []gen.MigracaoDetalhe{single}, nil
}

// writtenMigration reads the response of a POST or PUT: a single detail.
func writtenMigration(body []byte) (Migration, error) {
	items, err := detailsFrom(body)
	if err != nil {
		return Migration{}, err
	}
	if len(items) != 1 || items[0].IdMigracao == "" {
		return Migration{}, unreadableResponse(errors.New("the response does not carry an identifiable migration"))
	}
	return migrationFromDetail(items[0])
}

// setIf adds the parameter if the value is not empty.
func setIf(q url.Values, name, value string) {
	if value != "" {
		q.Set(name, value)
	}
}
