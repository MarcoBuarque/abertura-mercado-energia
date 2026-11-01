package abertura

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// DocumentType is the type of a person's document: CPF or CNPJ.
type DocumentType string

// Known document types.
const (
	DocumentTypeCPF  DocumentType = "CPF"
	DocumentTypeCNPJ DocumentType = "CNPJ"
)

// Document is a CPF or CNPJ. It is personal data: no printing form (fmt, slog,
// JSON) shows the number. Only [Document.Number] returns it in clear text, by
// explicit call.
//
// The SDK does not validate check digits: the server is the source of the
// rule, and the CNPJ now accepts letters.
type Document struct {
	kind   DocumentType
	number string
}

// CPF builds a CPF [Document]. It strips punctuation and does not validate
// check digits.
func CPF(number string) Document {
	return Document{kind: DocumentTypeCPF, number: cleanDocument(number)}
}

// CNPJ builds a CNPJ [Document]. It strips punctuation and does not validate
// check digits.
func CNPJ(number string) Document {
	return Document{kind: DocumentTypeCNPJ, number: cleanDocument(number)}
}

// cleanDocument keeps only ASCII letters and digits, in upper case.
func cleanDocument(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 'a' + 'A')
		}
	}
	return b.String()
}

// Type returns the document type (empty for the zero value).
func (d Document) Type() DocumentType {
	return d.kind
}

// Number returns the number without punctuation, in clear text. Use it only
// where the number is really needed, such as in a request body; never in logs.
func (d Document) Number() string {
	return d.number
}

// IsZero reports whether the document is absent.
func (d Document) IsZero() bool {
	return d.kind == "" && d.number == ""
}

// String returns the masked document, such as "CNPJ ************11": the type
// and the last two characters. A number of two characters or fewer is fully
// masked. An absent document yields empty text.
func (d Document) String() string {
	if d.IsZero() {
		return ""
	}
	n := len(d.number)
	visible := ""
	if n > 2 {
		visible = d.number[n-2:]
		n -= 2
	}
	return string(d.kind) + " " + strings.Repeat("*", n) + visible
}

// GoString implements [fmt.GoStringer] with the masked text of [Document.String].
func (d Document) GoString() string {
	return d.String()
}

// Format implements [fmt.Formatter]: any verb (%v, %+v, %#v, %s, %d, %x)
// prints only the masked text.
func (d Document) Format(f fmt.State, _ rune) {
	if _, err := io.WriteString(f, d.String()); err != nil {
		// fmt.State has no way to return an error; the write failed at the destination.
		return
	}
}

// LogValue implements [slog.LogValuer] with the masked text.
func (d Document) LogValue() slog.Value {
	return slog.StringValue(d.String())
}

// MarshalText implements [encoding.TextMarshaler] with the masked text. For
// that reason the JSON of a [Document] never carries the number. Sending to
// the API uses [Document.Number] in the internal generated type.
func (d Document) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}
