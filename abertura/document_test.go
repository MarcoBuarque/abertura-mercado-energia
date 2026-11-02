package abertura

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Fake documents, with all digits equal.
const (
	fakeCNPJ = "11111111111111"
	fakeCPF  = "22222222222"
)

func TestDocument_WhenNumber_ReturnsWithoutPunctuation(t *testing.T) {
	tests := []struct {
		name string
		doc  Document
		kind DocumentType
		want string
	}{
		{name: "cnpj with punctuation", doc: CNPJ("11.111.111/1111-11"), kind: DocumentTypeCNPJ, want: fakeCNPJ},
		{name: "cpf with punctuation", doc: CPF("222.222.222-22"), kind: DocumentTypeCPF, want: fakeCPF},
		{name: "alphanumeric cnpj in upper case", doc: CNPJ("1a.111.111/1111-11"), kind: DocumentTypeCNPJ, want: "1A111111111111"},
		{name: "no check digit validation", doc: CPF("123"), kind: DocumentTypeCPF, want: "123"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.doc.Number(); got != tc.want {
				t.Errorf("Number() = %q, want %q", got, tc.want)
			}
			if got := tc.doc.Type(); got != tc.kind {
				t.Errorf("Type() = %q, want %q", got, tc.kind)
			}
		})
	}
}

func TestDocument_WhenPrinted_DoesNotShowNumber(t *testing.T) {
	doc := CNPJ("11.111.111/1111-11")
	migration := Migration{
		ID:           "00000000-0000-0000-0000-000000000001",
		ConsumerUnit: "000000000000001",
		Email:        "contato@example.org",
		Document:     doc,
		Status:       MigrationStatusCreated,
	}
	printf := func(verb string, v any) string { return fmt.Sprintf(verb, v) }
	jsonSlog := func(v any) string { return slogJSONNoTime("value", v) }
	jsonStd := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		return string(b)
	}

	tests := []struct {
		name string
		got  string
	}{
		{name: "%v of the document", got: printf("%v", doc)},
		{name: "%+v of the document", got: printf("%+v", doc)},
		{name: "%#v of the document", got: printf("%#v", doc)},
		{name: "%s of the document", got: printf("%s", doc)}, //nolint:gosimple // exercises the %s verb
		{name: "%d of the document", got: printf("%d", doc)},
		{name: "%x of the document", got: printf("%x", doc)},
		{name: "%v of the pointer", got: printf("%v", &doc)},
		{name: "slog JSON of the document", got: jsonSlog(doc)},
		{name: "json.Marshal of the document", got: jsonStd(doc)},
		{name: "json.Marshal of a map with the document", got: jsonStd(map[string]Document{"d": doc})},
		{name: "slog JSON of the migration document", got: jsonSlog(migration.Document)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("output: %s", tc.got)
			if strings.Contains(tc.got, "111") {
				t.Errorf("the output carries the number (or part of it): %s", tc.got)
			}
			if !strings.Contains(tc.got, "CNPJ") {
				t.Errorf("the output lacks the type: %s", tc.got)
			}
			if !strings.Contains(tc.got, "11") {
				t.Errorf("the output lacks the last two digits: %s", tc.got)
			}
		})
	}
}

// Migration does not include the document in any printing form: neither the
// number nor the type.
func TestDocument_WhenMigrationPrinted_OmitsDocument(t *testing.T) {
	migration := Migration{ID: "00000000-0000-0000-0000-000000000001", Document: CNPJ(fakeCNPJ), Email: "contato@example.org"}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		for name, v := range map[string]any{"value": migration, "pointer": &migration} {
			got := fmt.Sprintf(verb, v)
			if strings.Contains(got, "111") || strings.Contains(got, "CNPJ") || strings.Contains(got, "@") {
				t.Errorf("%s of the %s of the migration carried personal data: %s", verb, name, got)
			}
		}
	}
}

func TestDocument_WhenMigrationInSlog_OmitsDocument(t *testing.T) {
	migration := Migration{ID: "00000000-0000-0000-0000-000000000001", Document: CNPJ(fakeCNPJ)}
	output := slogJSONNoTime("value", migration)
	if strings.Contains(output, "111") || strings.Contains(output, "CNPJ") {
		t.Errorf("slog of the migration carried the document: %s", output)
	}
}

func TestDocument_WhenString_MasksAllButLastTwo(t *testing.T) {
	tests := []struct {
		name string
		doc  Document
		want string
	}{
		{name: "cnpj", doc: CNPJ(fakeCNPJ), want: "CNPJ ************11"},
		{name: "cpf", doc: CPF(fakeCPF), want: "CPF *********22"},
		{name: "short number is fully masked", doc: CPF("12"), want: "CPF **"},
		{name: "absent is empty", doc: Document{}, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.doc.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
			if got := tc.doc.GoString(); got != tc.want {
				t.Errorf("GoString() = %q, want %q", got, tc.want)
			}
			if got := tc.doc.IsZero(); got != (tc.want == "") {
				t.Errorf("IsZero() = %v", got)
			}
		})
	}
}

func TestNewDocument_WhenTypeKnown_BuildsIt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		kind    DocumentType
		number  string
		want    Document
		wantErr error
	}{
		{name: "cpf with punctuation", kind: DocumentTypeCPF, number: "111.111.111-11", want: CPF("11111111111")},
		{name: "cnpj", kind: DocumentTypeCNPJ, number: "11111111000111", want: CNPJ("11111111000111")},
		{name: "unknown type returns ErrInvalidArgument", kind: DocumentType("RG"), number: "1", wantErr: ErrInvalidArgument},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NewDocument(tc.kind, tc.number)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewDocument: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}
