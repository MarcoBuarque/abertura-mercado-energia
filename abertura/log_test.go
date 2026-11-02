package abertura

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura/internal/testserver"
)

// logBuffer is the log buffer shared by the whole suite.
type logBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *logBuffer) text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

var suiteLog = &logBuffer{}

// suiteLogger returns a Debug-level logger that writes to the suite buffer.
// The method tests use it, and TestMain checks the buffer.
func suiteLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(suiteLog, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// forbiddenTerms are the fixture and test data that no log may contain:
// consumer units, documents, e-mails, the secret and the token scheme.
var forbiddenTerms = []string{
	"000000000000001", "000000000000002", "000000000000003", "000000000000004", "000000000000005",
	"11111111111111", "22222222222", "33333333333",
	"@example.org", "contato@", "outro@", "fresh@",
	fakeSecret, fakeClientID, "fake-token", "Bearer",
}

// leaks returns the forbidden terms that appear in text.
func leaks(text string) []string {
	var found []string
	for _, term := range forbiddenTerms {
		if strings.Contains(text, term) {
			found = append(found, term)
		}
	}
	return found
}

// TestMain checks, at the end of the whole suite, that the log written by the
// method tests did not leak personal data.
func TestMain(m *testing.M) {
	code := m.Run()
	if code == 0 {
		if found := leaks(suiteLog.text()); len(found) > 0 {
			log.Fatalf("the suite log contains data that must not leak: %q", found)
		}
	}
	os.Exit(code)
}

// TestLog_WhenMethodsRun_HasNoPersonalData runs the method scenarios (success,
// API error, uncertain result, checks) with a Debug logger on its own buffer
// and looks in it for consumer units, documents, e-mails, the secret and
// "Bearer". None may appear, even when the whole entity is logged.
func TestLog_WhenMethodsRun_HasNoPersonalData(t *testing.T) {
	buf := &logBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := context.Background()
	opt := WithLogger(logger)

	var last Migration
	scenarios := []struct {
		name   string
		routes []testserver.Route
		run    func(t *testing.T, c *Client)
	}{
		{
			name:   "list two pages",
			routes: []testserver.Route{stubRoute(http.MethodGet, listTemplate, fixtureResponse(200, "migrations_page1.json"), fixtureResponse(200, "migrations_page2.json"))},
			run: func(t *testing.T, c *Client) {
				items, err := collectSeq(t, c.Migrations.List(ctx, MigrationFilter{From: fromMonth, To: toMonth, ConsumerUnit: fakeConsumerUnit}))
				if err != nil || len(items) == 0 {
					t.Fatalf("List: %d items, %v", len(items), err)
				}
				last = items[0]
			},
		},
		{
			name:   "get",
			routes: []testserver.Route{stubRoute(http.MethodGet, detailTemplate, fixtureResponse(200, "migration_detail_array.json"))},
			run: func(t *testing.T, c *Client) {
				if _, err := c.Migrations.Get(ctx, testMigrationID); err != nil {
					t.Fatalf("Get: %v", err)
				}
			},
		},
		{
			name:   "create with success",
			routes: []testserver.Route{stubRoute(http.MethodPost, listTemplate, fixtureResponse(200, "migration_created.json"))},
			run: func(t *testing.T, c *Client) {
				if _, err := c.Migrations.Create(ctx, testMigrationRequest()); err != nil {
					t.Fatalf("Create: %v", err)
				}
			},
		},
		{
			name:   "create with migration already requested",
			routes: []testserver.Route{stubRoute(http.MethodPost, listTemplate, fixtureResponse(400, "error_400_migration_already_requested.json"))},
			run: func(t *testing.T, c *Client) {
				if _, err := c.Migrations.Create(ctx, testMigrationRequest()); !errors.Is(err, ErrMigrationAlreadyRequested) {
					t.Fatalf("Create: %v", err)
				}
			},
		},
		{
			name:   "create with uncertain result and check",
			routes: []testserver.Route{stubRoute(http.MethodPost, listTemplate, fixtureResponse(502, "error_502.txt")), stubRoute(http.MethodGet, listTemplate, fixtureResponse(200, "migrations_check.json"))},
			run: func(t *testing.T, c *Client) {
				_, err := c.Migrations.Create(ctx, testMigrationRequest())
				var inc *UncertainResultError
				if !errors.As(err, &inc) {
					t.Fatalf("Create: %v", err)
				}
				key, ok := inc.Key.(CreateKey)
				if !ok {
					t.Fatalf("key = %#v", inc.Key)
				}
				if _, err := c.Migrations.FindCreated(ctx, key); err != nil {
					t.Fatalf("FindCreated: %v", err)
				}
				logger.Warn("uncertain", "key", inc.Key.String(), "error", inc)
			},
		},
		{
			name: "edit and check",
			routes: []testserver.Route{
				stubRoute(http.MethodGet, detailTemplate, fixtureResponse(200, "migration_detail_array.json")),
				stubRoute(http.MethodPut, editTemplate, fixtureResponse(200, "migration_detail_object.json")),
			},
			run: func(t *testing.T, c *Client) {
				migration, err := c.Migrations.Edit(ctx, testMigrationID, func(e *MigrationEdit) { e.Email = "fresh@example.org" })
				if err != nil {
					t.Fatalf("Edit: %v", err)
				}
				key := EditKey{ID: testMigrationID, Desired: MigrationEdit{Email: "fresh@example.org", Document: migration.Document}}
				if _, _, err := c.Migrations.CheckEdited(ctx, key); err != nil {
					t.Fatalf("CheckEdited: %v", err)
				}
				logger.Info("edit", "migration", migration, "key", key.String(), "document", migration.Document)
			},
		},
		{
			name: "edit with uncertain PUT",
			routes: []testserver.Route{
				stubRoute(http.MethodGet, detailTemplate, fixtureResponse(200, "migration_detail_array.json")),
				stubRoute(http.MethodPut, editTemplate, fixtureResponse(502, "error_502.txt")),
			},
			run: func(t *testing.T, c *Client) {
				_, err := c.Migrations.Edit(ctx, testMigrationID, func(e *MigrationEdit) { e.Status = MigrationStatusCanceled })
				var inc *UncertainResultError
				if !errors.As(err, &inc) {
					t.Fatalf("Edit: %v", err)
				}
			},
		},
		{
			name:   "measurements",
			routes: []testserver.Route{stubRoute(http.MethodGet, measurementsTemplate, fixtureResponse(200, "measurements_page1.json"), fixtureResponse(200, "measurements_page2.json"))},
			run: func(t *testing.T, c *Client) {
				items, err := collectSeq(t, c.Measurements.List(ctx, testMeasurementFilter()))
				if err != nil || len(items) != 2 {
					t.Fatalf("List: %d items, %v", len(items), err)
				}
				logger.Info("measurement", "measurement", items[0])
			},
		},
	}
	for _, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := specClient(t, tc.routes, opt)
			tc.run(t, c)
		})
	}
	logger.Info("migration of the first scenario", "migration", last)

	text := buf.text()
	for _, expected := range []string{"op=listMigrations", "op=createMigration", "op=editMigration", "op=listMeasurements", "uncertain result"} {
		if !strings.Contains(text, expected) {
			t.Errorf("the log lacks %q: the test is not exercising the log", expected)
		}
	}
	if found := leaks(text); len(found) > 0 {
		t.Errorf("the log contains data that must not leak: %q\n%s", found, text)
	}
}

// TestKeysAndErrors_WhenFormatted_HaveNoPersonalData logs and formats the
// natural keys and the errors through every path a caller uses
// (slog.JSONHandler, slog.TextHandler, %v, %+v, %#v and %s) and checks that
// nothing leaks the consumer unit, document, e-mail, consumption, token or
// body.
func TestKeysAndErrors_WhenFormatted_HaveNoPersonalData(t *testing.T) {
	t.Parallel()

	body := []byte(`{"error":"ERR_X","message":"m","consumerUnit":"000000000000001","email":"contato@example.org"}`)
	apiErr := &APIError{Operation: "createMigration", StatusCode: 502, rawBody: body, httpMethod: http.MethodPost}
	migKey := CreateKey{ConsumerUnit: fakeConsumerUnit, Distributor: 999001, Month: YearMonth{}, Profile: 1}
	editKey := EditKey{ID: testMigrationID, Desired: MigrationEdit{Email: "fresh@example.org", Profile: 1, Document: CNPJ("11111111000111")}}
	uncertain := &UncertainResultError{Operation: "createMigration", Key: migKey, Err: apiErr}
	uncertainEdit := &UncertainResultError{Operation: "editMigration", Key: editKey, Err: apiErr}

	values := []struct {
		name string
		v    any
	}{
		{"CreateKey", migKey},
		{"CreateKey pointer", &migKey},
		{"EditKey", editKey},
		{"EditKey pointer", &editKey},
		{"APIError", apiErr},
		{"UncertainResultError", uncertain},
		{"UncertainResultError from edit", uncertainEdit},
		{"NaturalKey of the uncertainty", uncertain.Key},
	}
	for _, tc := range values {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			outputs := map[string]string{
				"%v": fmt.Sprintf("%v", tc.v), "%+v": fmt.Sprintf("%+v", tc.v),
				"%#v": fmt.Sprintf("%#v", tc.v), "%s": fmt.Sprintf("%s", tc.v),
				"%q": fmt.Sprintf("%q", tc.v), "%d": fmt.Sprintf("%d", tc.v),
				"%t": fmt.Sprintf("%t", tc.v), "%x": fmt.Sprintf("%x", tc.v),
			}
			var bj, bt bytes.Buffer
			slog.New(slog.NewJSONHandler(&bj, nil)).Info("record", "value", tc.v)
			slog.New(slog.NewTextHandler(&bt, nil)).Info("record", "value", tc.v)
			outputs["slog json"] = bj.String()
			outputs["slog text"] = bt.String()
			for name, output := range outputs {
				if found := leaks(output); len(found) > 0 {
					t.Errorf("%s leaked %q: %s", name, found, output)
				}
				// The raw body, in clear or in hexadecimal.
				if strings.Contains(output, "ERR_X") || strings.Contains(output, "7b226572726f72") || strings.Contains(strings.ToLower(output), "0x7b") {
					t.Errorf("%s leaked the body: %s", name, output)
				}
			}
		})
	}

	t.Run("slog json of the key inside the error", func(t *testing.T) {
		t.Parallel()
		var b bytes.Buffer
		l := slog.New(slog.NewJSONHandler(&b, nil))
		l.Warn("uncertain", "key", uncertain.Key, "error", uncertain, "edit", uncertainEdit.Key)
		if found := leaks(b.String()); len(found) > 0 {
			t.Errorf("leaked %q: %s", found, b.String())
		}
	})
}
