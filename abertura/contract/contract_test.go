//go:build sandbox

package contract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura"
)

// Script (go test -tags sandbox ./contract/... -count=1 -v):
//
//	ABERTURA_CLIENT_ID, ABERTURA_CLIENT_SECRET, ABERTURA_AGENTE, ABERTURA_PERFIL,
//	ABERTURA_CONCESSIONARIA   read
//	ABERTURA_CONTRATO_ESCRITA=1, ABERTURA_UC and ABERTURA_DOCUMENTO   turn
//	    writing on (opt-in). The consumer unit and the document (11-character
//	    CPF or 14-character CNPJ, with a valid check digit) come from the
//	    official sandbox documentation and only from the environment, never
//	    from the repository. If any is missing, the write steps call t.Skip.
//
// WARNING: the write round may leave an active migration on a fictitious
// consumer unit of the shared sandbox (Create and the following edits cannot
// be undone by this test). Run the write round knowing that.
//
// Only the sandbox is targeted. It prints status, Code and field names; never
// values. It stops at the 91st request and at the first 403.
//
// Read (no effect on the server): token, one-month list, list without
// profile, list without span, Get, measurements and up to 3 pages. The wide
// span is searched once: it starts at 12 months ending in the current month
// and, on each ErrMonthRangeTooWide, is halved (centered on the current
// month) until the API accepts; the following steps reuse the accepted span.
// Each response gets, besides the field names, the format mask of the
// temporal fields (digit = 9, letter = A), never the value. Write (creates and
// changes records in the sandbox): Create, FindCreated, Edit, CheckEdited and
// the partial PUT through Do. In the write round, ErrFieldValidation on
// Create, a check that finds no migration (after searching the consumer unit
// over a wide month span when Create returned ErrMigrationAlreadyRequested)
// and the lack of an active migration (not CANCELADA, ENCERRADA, EXPIRADA or
// REPROVADA) become a finding plus t.Skip (a divergence to record, not an SDK
// failure). An uncertain result on Create is checked with FindCreated and, if
// the migration exists, the round goes on; any other error fails.

type config struct {
	id, secret                  string
	agent, profile, distributor int
	write                       bool
	consumerUnit                string
	document                    abertura.Document
	// noWriteReason explains, in the t.Skip message, why writing is off.
	noWriteReason string
}

// documentFromEnv builds the document from ABERTURA_DOCUMENTO: 11 characters
// are a CPF, 14 are a CNPJ (which now accepts letters). Any other length is
// refused.
func documentFromEnv(value string) (abertura.Document, bool) {
	var d abertura.Document
	switch n := len(abertura.CPF(value).Number()); n {
	case 11:
		d = abertura.CPF(value)
	case 14:
		d = abertura.CNPJ(value)
	default:
		return abertura.Document{}, false
	}
	return d, true
}

func readConfig(t *testing.T) config {
	t.Helper()
	var c config
	c.id, c.secret = os.Getenv("ABERTURA_CLIENT_ID"), os.Getenv("ABERTURA_CLIENT_SECRET")
	nums := map[string]*int{"ABERTURA_AGENTE": &c.agent, "ABERTURA_PERFIL": &c.profile, "ABERTURA_CONCESSIONARIA": &c.distributor}
	missing := c.id == "" || c.secret == ""
	for name, dest := range nums {
		n, err := strconv.Atoi(os.Getenv(name))
		if err != nil || n <= 0 {
			missing = true
			continue
		}
		*dest = n
	}
	if missing {
		t.Skip("contract: set ABERTURA_CLIENT_ID, ABERTURA_CLIENT_SECRET, ABERTURA_AGENTE, ABERTURA_PERFIL and ABERTURA_CONCESSIONARIA")
	}
	c.consumerUnit = os.Getenv("ABERTURA_UC")
	doc, docOK := documentFromEnv(os.Getenv("ABERTURA_DOCUMENTO"))
	c.document = doc
	switch {
	case os.Getenv("ABERTURA_CONTRATO_ESCRITA") != "1":
		c.noWriteReason = "writing is off: set ABERTURA_CONTRATO_ESCRITA=1, ABERTURA_UC and ABERTURA_DOCUMENTO"
	case c.consumerUnit == "":
		c.noWriteReason = "writing is off: ABERTURA_UC is missing (a valid consumer unit from the official documentation)"
	case !docOK:
		c.noWriteReason = "writing is off: ABERTURA_DOCUMENTO is missing (11-character CPF or 14-character CNPJ, with a valid check digit)"
	default:
		c.write = true
	}
	return c
}

// wideSearchLimit limits the items read in the consumer-unit search over a
// wide span, so as not to spend the request budget.
const wideSearchLimit = 200

var brasilia = time.FixedZone("-03:00", -3*60*60)

// describe describes an error without URL or body: only status and Code.
func describe(err error) string {
	var api *abertura.APIError
	if errors.As(err, &api) {
		return fmt.Sprintf("APIError op=%s status=%d code=%q", api.Operation, api.StatusCode, safeCode(api.Code))
	}
	var uncertain *abertura.UncertainResultError
	if errors.As(err, &uncertain) {
		return "uncertain result in " + uncertain.Operation
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return fmt.Sprintf("response is not valid JSON (offset %d)", syntaxErr.Offset)
	}
	if m := reUnreadableField.FindStringSubmatch(err.Error()); m != nil {
		return "unreadable field in the response: " + m[1]
	}
	if m := reUnexpectedType.FindStringSubmatch(err.Error()); m != nil {
		return "field with unexpected type in the response: " + m[1]
	}
	if errors.Is(err, abertura.ErrNotFound) {
		return "ErrNotFound"
	}
	if t := sdkFixedText(err.Error()); t != "" {
		return t
	}
	return fmt.Sprintf("error of type %T", err)
}

// reUnreadableField matches the fixed text of the SDK conversion error
// (fieldError in convert.go). The field name is not a value.
var reUnreadableField = regexp.MustCompile(`the response field "([A-Za-z0-9_.]+)" has an unreadable value`)

// reUnexpectedType matches the fixed text the SDK writes when a field arrives
// with a different type than expected (do.go): only the field name is
// printed, never the value nor the encoding/json error.
var reUnexpectedType = regexp.MustCompile(`field "([A-Za-z0-9_.\[\]]+)" has an unexpected type`)

// sdkFixedTexts are the unreadable-response texts the SDK itself writes
// (migrations.go, do.go). The type that carries them is not exported, so the
// text is matched; only the matched fixed text is printed, nothing beyond it.
var sdkFixedTexts = []string{
	"the API returned more than one migration for the id",
	"empty response",
	"the response does not carry an identifiable migration",
	"decode response: unexpected JSON",
	"decode response: ",
}

// reInvalidJSON matches the fixed text of the SDK syntax error (do.go).
var reInvalidJSON = regexp.MustCompile(`response is not valid JSON \(byte \d+\)`)

// sdkFixedText returns the SDK fixed text contained in the message, or empty.
func sdkFixedText(msg string) string {
	if m := reInvalidJSON.FindString(msg); m != "" {
		return m
	}
	for _, t := range sdkFixedTexts {
		if strings.Contains(msg, t) {
			return t
		}
	}
	return ""
}

// rootOr returns the item count of the root, or "root: object" when the root
// was not a list.
func rootOr(root string) string {
	if root == "" {
		return "root: object"
	}
	return root
}

func TestContract(t *testing.T) {
	cfg := readConfig(t)
	o, err := newBudget(http.DefaultTransport, requestLimit)
	if err != nil {
		t.Fatalf("budget: %v", err)
	}
	hc := &http.Client{Transport: o, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	c, err := abertura.NewClient(abertura.Sandbox(),
		abertura.WithCredentials(cfg.id, cfg.secret),
		abertura.WithAgent(cfg.agent, cfg.profile),
		abertura.WithHTTPClient(hc),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()
	var findings []string
	finding := func(t *testing.T, format string, args ...any) {
		t.Helper()
		l := fmt.Sprintf(format, args...)
		findings = append(findings, l)
		t.Log("FINDING: " + l)
	}
	// step runs a subtest, skips if the round aborted and dumps the lines.
	step := func(name string, f func(t *testing.T)) {
		t.Run(name, func(t *testing.T) {
			if o.aborted.Load() {
				t.Skip("round aborted (first 403 or budget exhausted)")
			}
			t.Cleanup(func() {
				for _, l := range o.drain() {
					t.Log(l)
				}
			})
			f(t)
		})
	}

	today := time.Now().In(brasilia)
	month := abertura.NewYearMonth(today.Year(), today.Month())
	spanFrom, spanTo := initialSpan(month)
	var first abertura.Migration

	// wideSpan looks up, the first time, the widest month span the API
	// accepts: it starts at 12 months ending in the current month
	// (spanFrom to spanTo; cap measured on 01/11/2026 between 12 and 24) and, on
	// each ErrMonthRangeTooWide, halves it, centered on the current month
	// (12, 6, 3, 1), with at most maxSpanAttempts requests.
	// The result is kept: the steps that list over a wide span use the
	// accepted span and the first page already read.
	var (
		spanDone bool
		span     acceptedSpan
		spanErr  error
	)
	wideSpan := func(t *testing.T) (acceptedSpan, error) {
		t.Helper()
		if spanDone {
			return span, spanErr
		}
		spanDone = true
		span, spanErr = findAcceptedSpan(spanFrom, spanTo, month, func(from, to abertura.YearMonth) (abertura.Page[abertura.Migration], error) {
			return c.Migrations.ListPage(ctx, abertura.MigrationFilter{From: from, To: to}, "")
		})
		switch {
		case spanErr != nil:
			finding(t, "month span: no tested span was accepted (%s)", describe(spanErr))
		case span.Refused == 0:
			finding(t, "span of %d months accepted (cap between 12 and 24, measured on 01/11/2026)", span.Months)
		default:
			finding(t, "span of %d months accepted; one of %d refused (cap between %d and %d)", span.Months, span.Refused, span.Months, span.Refused)
		}
		return span, spanErr
	}

	step("read/token and one-month list", func(t *testing.T) {
		pag, err := c.Migrations.ListPage(ctx, abertura.MigrationFilter{From: month, To: month}, "")
		var api *abertura.APIError
		if errors.As(err, &api) && api.Operation == "getToken" {
			finding(t, "token: the endpoint refused the SDK request; %s", describe(err))
			t.Fatalf("no token, no contract")
		}
		if err != nil {
			t.Fatalf("ListPage: %s", describe(err))
		}
		finding(t, "token: accepted with only client_id and client_secret in the form (no grant_type)")
		outside := 0
		for _, m := range pag.Items {
			if m.Month != month {
				outside++
			}
		}
		finding(t, "one-month list: %d items, %d with a reference month outside the YYYY-MM filter (0 with items = the filter holds; D2), cursor present: %v",
			len(pag.Items), outside, pag.NextCursor != "")
		if len(pag.Items) > 0 {
			first = pag.Items[0]
		}
	})

	step("read/list without profile through Do", func(t *testing.T) {
		q := url.Values{"mesReferenciaInicial": {month.String()}, "mesReferenciaFinal": {month.String()}}
		var raw json.RawMessage
		err := c.Do(ctx, http.MethodGet, "/v1.1/varejista/migracoes?"+q.Encode(), nil, &raw)
		switch {
		case errors.Is(err, abertura.ErrRetailerProfileRequired):
			finding(t, "list without profile in the query: ErrRetailerProfileRequired, as the spec says")
		case err == nil:
			finding(t, "list without profile in the query: DIVERGES, the API answered 200")
			t.Error("expected ErrRetailerProfileRequired")
		default:
			finding(t, "list without profile in the query: error different from expected: %s", describe(err))
			t.Error("expected ErrRetailerProfileRequired")
		}
	})

	step("read/list without month span through Do", func(t *testing.T) {
		q := url.Values{"codigoPerfilVarejista": {strconv.Itoa(cfg.profile)}}
		var raw json.RawMessage
		err := c.Do(ctx, http.MethodGet, "/v1.1/varejista/migracoes?"+q.Encode(), nil, &raw)
		if errors.Is(err, abertura.ErrMonthRangeRequired) {
			finding(t, "month span: required, ErrMonthRangeRequired (%s)", describe(err))
			return
		}
		if err != nil {
			finding(t, "month span: seems required, but with another error (%s)", describe(err))
			return
		}
		finding(t, "month span: NOT required (200)")
	})

	step("read/Get do first item", func(t *testing.T) {
		if first.ID == "" {
			fx, err := wideSpan(t)
			if err != nil {
				t.Fatalf("wide list: %s", describe(err))
			}
			pag := fx.Page
			if len(pag.Items) == 0 {
				t.Skip("the sandbox has no visible migrations for this profile")
			}
			first = pag.Items[0]
		}
		m, err := c.Migrations.Get(ctx, first.ID)
		if err != nil {
			finding(t, "Get: %s; %s (the name of the unreadable field, if any, is the datum to record in SOURCES.md)", describe(err), rootOr(o.rootOf("getMigration")))
			t.Fatalf("Get: %s", describe(err))
		}
		finding(t, "Get: ok; %s; same status as the list: %v; same month as the list: %v", rootOr(o.rootOf("getMigration")), m.Status == first.Status, m.Month == first.Month)
	})

	step("read/measurements", func(t *testing.T) {
		if first.ConsumerUnit == "" {
			t.Skip("no consumer unit from a migration to query measurements")
		}
		pag, err := c.Measurements.ListPage(ctx, abertura.MeasurementFilter{
			ConsumerUnit: first.ConsumerUnit, Distributor: cfg.distributor,
			From: time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, brasilia).AddDate(0, -1, 0),
			To:   time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, brasilia).Add(-15 * time.Minute),
		}, "")
		if err != nil {
			finding(t, "measurements: %s", describe(err))
			t.Fatalf("ListPage: %s", describe(err))
		}
		finding(t, "measurements: %d items, cursor present: %v", len(pag.Items), pag.NextCursor != "")
	})

	step("read/up to 3 pages over a wide span", func(t *testing.T) {
		fx, err := wideSpan(t)
		if err != nil {
			t.Fatalf("page 1: %s", describe(err))
		}
		pag := fx.Page
		cursor, seen := "", 0
		for pageNo := 1; pageNo <= 3; pageNo++ {
			if pageNo > 1 {
				pag, err = c.Migrations.ListPage(ctx, abertura.MigrationFilter{From: fx.From, To: fx.To}, cursor)
				if err != nil {
					t.Fatalf("page %d: %s", pageNo, describe(err))
				}
			}
			seen += len(pag.Items)
			if pag.NextCursor == "" {
				finding(t, "pagination: %d page(s), %d items, cursor did NOT appear on the last one", pageNo, seen)
				return
			}
			if pag.NextCursor == cursor {
				finding(t, "pagination: repeated cursor on page %d (ErrRepeatedCursor protects List)", pageNo)
				return
			}
			cursor = pag.NextCursor
		}
		finding(t, "pagination: the cursor appeared and led to 3 pages, %d items", seen)
	})

	if !cfg.write {
		t.Run("write", func(t *testing.T) {
			t.Skip(cfg.noWriteReason)
		})
	} else {
		var id string
		step("write/Create and FindCreated", func(t *testing.T) {
			key := abertura.CreateKey{ConsumerUnit: cfg.consumerUnit, Distributor: cfg.distributor, Month: nextMonth(month)}
			_, err := c.Migrations.Create(ctx, abertura.MigrationRequest{
				ConsumerUnit: cfg.consumerUnit, Distributor: cfg.distributor,
				Document: cfg.document, Email: "contract@example.org",
				Month: key.Month, TerminationNoticeDate: today, Comment: "automated contract",
			})
			var uncertain *abertura.UncertainResultError
			requested := false
			switch {
			case err == nil:
				finding(t, "Create: 200")
			case errors.Is(err, abertura.ErrMigrationAlreadyRequested):
				requested = true
				finding(t, "Create: ErrMigrationAlreadyRequested (there was already an active migration)")
			case errors.Is(err, abertura.ErrFieldValidation):
				// The API refused a field (consumer unit, document or month from the
				// environment): it is a finding about the test data, not an SDK defect.
				finding(t, "Create: ErrFieldValidation with the consumer unit and document from the environment; check the values and the month. The write step was skipped")
				t.Skip("Create refused by field validation (finding recorded)")
			case errors.As(err, &uncertain):
				// The result is uncertain: check below, as the catalog says.
				finding(t, "Create: uncertain result (%s); checking with FindCreated", describe(err))
			default:
				finding(t, "Create: %s", describe(err))
				t.Fatalf("Create: %s", describe(err))
			}
			found, err := c.Migrations.FindCreated(ctx, key)
			if err != nil {
				t.Fatalf("FindCreated: %s", describe(err))
			}
			finding(t, "FindCreated: %d migration(s) for the key", len(found))
			if len(found) == 0 && requested {
				// With ErrMigrationAlreadyRequested the active migration may be in
				// another month or profile: search the consumer unit over a wide month span.
				finding(t, "FindCreated: empty for the key month; searching the consumer unit over a wide month span")
				fx, err := wideSpan(t)
				if err != nil {
					t.Fatalf("wide span: %s", describe(err))
				}
				n := 0
				for m, err := range c.Migrations.List(ctx, abertura.MigrationFilter{From: fx.From, To: fx.To, ConsumerUnit: cfg.consumerUnit}) {
					if err != nil {
						t.Fatalf("wide list by consumer unit: %s", describe(err))
					}
					if n++; n > wideSearchLimit {
						finding(t, "wide list by consumer unit: stopped at %d items", wideSearchLimit)
						break
					}
					found = append(found, m)
				}
				found = ofConsumerUnit(found, cfg.consumerUnit, cfg.distributor)
				finding(t, "wide span: %d migration(s) of the consumer unit and the distributor", len(found))
			}
			if len(found) == 0 {
				finding(t, "Create left no locatable migration after the check; the following steps were skipped")
				t.Skip("no migration found after Create (finding recorded)")
			}
			active, ok := pickActive(found)
			if !ok {
				finding(t, "none of the %d migration(s) of the consumer unit is active (all canceled, closed, expired or rejected); the following steps were skipped", len(found))
				t.Skip("no active migration to edit (finding recorded)")
			}
			id = active.ID
		})
		step("write/Edit and CheckEdited", func(t *testing.T) {
			if id == "" {
				t.Skip("no migration")
			}
			const email = "contract-edited@example.org"
			if _, err := c.Migrations.Edit(ctx, id, func(e *abertura.MigrationEdit) { e.Email = email }); err != nil {
				finding(t, "Edit (GET + PUT of the whole object): %s", describe(err))
				t.Fatalf("Edit: %s", describe(err))
			}
			finding(t, "Edit (GET + PUT of the whole object): ok")
			_, matches, err := c.Migrations.CheckEdited(ctx, abertura.EditKey{ID: id, Desired: abertura.MigrationEdit{Email: email}})
			if err != nil {
				t.Fatalf("CheckEdited: %s", describe(err))
			}
			finding(t, "CheckEdited: matches = %v", matches)
			if !matches {
				t.Error("the e-mail should have changed")
			}
		})
		step("write/partial PUT through Do", func(t *testing.T) {
			if id == "" {
				t.Skip("no migration")
			}
			before, err := c.Migrations.Get(ctx, id)
			if err != nil {
				t.Fatalf("Get before: %s", describe(err))
			}
			const email = "contract-partial@example.org"
			err = c.Do(ctx, http.MethodPut, "/v1/varejista/migracoes/"+url.PathEscape(id),
				map[string]string{"emailUnidadeConsumidora": email}, nil)
			if err != nil {
				finding(t, "partial PUT: refused (%s): the PUT seems to be total", describe(err))
				return
			}
			after, err := c.Migrations.Get(ctx, id)
			if err != nil {
				t.Fatalf("Get after: %s", describe(err))
			}
			preserved := after.Month == before.Month && after.Profile == before.Profile &&
				after.Document.Number() == before.Document.Number() && after.Status == before.Status
			finding(t, "partial PUT: accepted; e-mail changed: %v; other fields preserved: %v (if preserved, the PUT is partial)",
				after.Email == email, preserved)
		})
	}

	t.Run("summary", func(t *testing.T) {
		t.Logf("requests used: %d of %d", o.used(), requestLimit)
		for _, v := range o.allViolations() {
			t.Errorf("response diverges from the spec: %s", v)
		}
		t.Log("For openapi/SOURCES.md (D?, x-confidence, x-verified-at):")
		for _, a := range findings {
			t.Log("  - " + a)
		}
	})
}

// nextMonth returns the following month.
func nextMonth(m abertura.YearMonth) abertura.YearMonth {
	if m.Month == time.December {
		return abertura.NewYearMonth(m.Year+1, time.January)
	}
	return abertura.NewYearMonth(m.Year, m.Month+1)
}
