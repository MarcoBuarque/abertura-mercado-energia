//go:build sandbox

// Package contract holds the manual contract test against the sandbox. It
// runs only with the sandbox build tag and credentials in the environment;
// nothing here enters the module that users import.
package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura/internal/spec"
)

// requestLimit is the cap on requests in one round.
const requestLimit = 90

var (
	errBudgetExhausted = errors.New("contract: request budget exhausted")
	errAborted         = errors.New("contract: round aborted by the first 403")
)

// budget is an http.RoundTripper that counts requests, stops at the 91st and
// at the first 403, validates each response against the embedded spec and
// keeps one line per request with the status and field names. It never keeps
// URLs, queries, bodies or values.
type budget struct {
	base   http.RoundTripper
	max    int64
	router routers.Router

	n       atomic.Int64
	aborted atomic.Bool

	mu         sync.Mutex
	lines      []string
	violations []string
	roots      map[string]string
}

func newBudget(base http.RoundTripper, max int64) (*budget, error) {
	doc, err := openapi3.NewLoader().LoadFromData(spec.Abertura)
	if err != nil {
		return nil, fmt.Errorf("load the spec: %w", err)
	}
	doc.Servers = nil // match by path only
	r, err := legacy.NewRouter(doc)
	if err != nil {
		return nil, fmt.Errorf("build the router: %w", err)
	}
	return &budget{base: base, max: max, router: r}, nil
}

// RoundTrip implements http.RoundTripper.
func (b *budget) RoundTrip(req *http.Request) (*http.Response, error) {
	if b.aborted.Load() {
		return nil, errAborted
	}
	if b.n.Add(1) > b.max {
		b.aborted.Store(true)
		return nil, errBudgetExhausted
	}
	resp, err := b.base.RoundTrip(req)
	if err != nil {
		b.note("request %d: transport failure", b.n.Load())
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	if cerr := resp.Body.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, fmt.Errorf("read the response: %w", err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if resp.StatusCode == http.StatusForbidden {
		b.aborted.Store(true)
	}
	op, problem := b.validate(req, resp, body)
	line := fmt.Sprintf("request %d: %s -> %d; fields: %s", b.n.Load(), op, resp.StatusCode, fieldNames(body))
	if n := rootItems(body); n != "" {
		if op == "getMigration" {
			if c := compareItems(body); c != "" {
				n += "; " + c
			}
		}
		line += "; " + n
		b.mu.Lock()
		if b.roots == nil {
			b.roots = map[string]string{}
		}
		b.roots[op] = n
		b.mu.Unlock()
	}
	if f := fieldFormats(body); f != "" {
		line += "; formats: " + f
	}
	if h := fieldTimes(body); h != "" {
		line += "; times: " + h
	}
	if code := errorCode(body); code != "" {
		line += "; code: " + code
	}
	if hint := descriptionHint(body); hint != "" {
		line += "; " + hint
	}
	b.note("%s", line)
	if problem != "" {
		b.mu.Lock()
		b.violations = append(b.violations, fmt.Sprintf("%s (%d): %s", op, resp.StatusCode, problem))
		b.mu.Unlock()
	}
	return resp, nil
}

func (b *budget) note(format string, args ...any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, fmt.Sprintf(format, args...))
}

// drain returns and clears the kept lines.
func (b *budget) drain() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	l := b.lines
	b.lines = nil
	return l
}

// allViolations returns the divergences from the spec seen so far.
func (b *budget) allViolations() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.violations...)
}

// rootOf returns the "items at root: N" of the last response of the operation
// whose root was a list; empty if there was none.
func (b *budget) rootOf(op string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.roots[op]
}

// used returns how many requests have gone through.
func (b *budget) used() int64 { return b.n.Load() }

// validate checks the response against the spec. It returns the operationId
// (or "outside the spec") and, if there is a divergence, a description without
// values.
func (b *budget) validate(req *http.Request, resp *http.Response, body []byte) (string, string) {
	route, params, err := b.router.FindRoute(req)
	if err != nil {
		return "outside the spec", "the route does not exist in the spec"
	}
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route},
		Status:                 resp.StatusCode,
		Header:                 resp.Header,
		Options:                &openapi3filter.Options{IncludeResponseStatus: true},
	}
	in.SetBodyBytes(bytes.TrimSpace(body))
	if err := openapi3filter.ValidateResponse(context.Background(), in); err != nil {
		return route.Operation.OperationID, describeViolation(err)
	}
	return route.Operation.OperationID, ""
}

// describeViolation summarizes the kin-openapi error without values: only the
// field that diverged, or the general reason.
func describeViolation(err error) string {
	var se *openapi3.SchemaError
	if errors.As(err, &se) {
		return fmt.Sprintf("field %q diverges from the schema (%s)", "/"+strings.Join(se.JSONPointer(), "/"), se.SchemaField)
	}
	var re *openapi3filter.ResponseError
	if errors.As(err, &re) {
		return re.Reason
	}
	return "response outside the spec"
}

// fieldNames lists the JSON field names: those of the object and, when a value
// is a list of objects, those of the first item (with a prefix). Never values.
func fieldNames(body []byte) string {
	var v any
	if err := json.Unmarshal(bytes.TrimSpace(body), &v); err != nil {
		return fmt.Sprintf("body is not JSON (%d bytes)", len(body))
	}
	var names []string
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				names = append(names, prefix+k)
				walk(prefix+k+".", child)
			}
		case []any:
			if len(x) > 0 {
				walk(prefix, x[0])
			}
		}
	}
	walk("", v)
	if len(names) == 0 {
		return "(none)"
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// reAPICode is the shape of an API error code.
var reAPICode = regexp.MustCompile(`^ERR_[A-Z0-9_]{1,80}$`)

// oauth2Codes are the six fixed error codes of RFC 6749, section 5.2. The
// token endpoint answers with them; they tell which request failed.
//nolint:gochecknoglobals // read-only table, never written after initialization
var oauth2Codes = map[string]bool{
	"invalid_request": true, "invalid_client": true, "invalid_grant": true,
	"unauthorized_client": true, "unsupported_grant_type": true, "invalid_scope": true,
}

// oauth2Descriptions are the only Keycloak error descriptions that may be
// printed, because they are fixed, known text. Any other is ignored.
//nolint:gochecknoglobals // read-only table, never written after initialization
var oauth2Descriptions = map[string]bool{
	"duplicated parameter":               true,
	"Missing form parameter: grant_type": true,
}

// descriptionHint returns "oauth2_error_description: <text>" only when
// error_description is exactly one of the fixed descriptions; otherwise it
// returns empty, with nothing about the received text (not even its length).
func descriptionHint(body []byte) string {
	var e struct {
		Description string `json:"error_description"`
	}
	if json.Unmarshal(bytes.TrimSpace(body), &e) != nil || !oauth2Descriptions[e.Description] {
		return ""
	}
	return "oauth2_error_description: " + e.Description
}

// safeCode returns the code if it has the shape ERR_ followed by uppercase
// letters, digits and underscores, or if it is one of the six fixed RFC 6749
// codes; otherwise it returns "non-standard code". The field comes from the
// server and could carry free text (with personal data) into the log. Empty
// stays empty.
func safeCode(code string) string {
	if code == "" || reAPICode.MatchString(code) || oauth2Codes[code] {
		return code
	}
	return "non-standard code"
}

// errorCode returns the "error" field of an error body, if present and if it
// has the shape of an API code.
func errorCode(body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(bytes.TrimSpace(body), &e) != nil {
		return ""
	}
	return safeCode(e.Error)
}

// temporalFields are the only date and time fields whose format may be
// described: none of them is personal data.
//nolint:gochecknoglobals // read-only table, never written after initialization
var temporalFields = map[string]bool{
	"mesReferencia": true, "dataReferencia": true, "dataDenuncia": true, "dataSolicitacao": true,
	"dataValidacao": true, "diaReferencia": true, "dataReferenciaConsumo": true,
	"expires_in": true, "timestamp": true,
}

// maxMaskLength is the maximum length, in characters, of a mask.
const maxMaskLength = 40

// mask replaces each Unicode digit with 9 and each Unicode letter with A,
// keeps the other characters and truncates at maxMaskLength with an ellipsis.
// The value itself never leaves here.
func mask(value string) string {
	var b strings.Builder
	n := 0
	for _, r := range value {
		if n == maxMaskLength {
			b.WriteString("...")
			break
		}
		switch {
		case unicode.IsDigit(r):
			r = '9'
		case unicode.IsLetter(r):
			r = 'A'
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// fieldFormats describes the format of the allow-listed temporal fields as
// "field=mask" separated by commas, in name order: the first occurrence of
// each one, searched as in [fieldNames]. Null becomes "null"; a number becomes
// a sequence of nines. Never the value.
func fieldFormats(body []byte) string {
	return joinTemporals(firstTemporals(body), valueMask)
}

// reTime matches the time part of an RFC 3339 value with an offset or Z.
var reTime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2}))$`)

// fieldTimes prints, for the allow-listed temporal fields whose value is an
// RFC 3339 timestamp, only the time and offset part ("T00:00:00Z"). The time
// is not personal data and shows whether the API uses midnight UTC or -03:00.
func fieldTimes(body []byte) string {
	return joinTemporals(firstTemporals(body), func(v any) string {
		s, ok := v.(string)
		if !ok {
			return ""
		}
		if m := reTime.FindStringSubmatch(s); m != nil {
			return m[1]
		}
		return ""
	})
}

// firstTemporals returns the value of the first occurrence of each
// allow-listed temporal field (only the first item of each list).
func firstTemporals(body []byte) map[string]any {
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(body)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil
	}
	found := map[string]any{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if _, seen := found[k]; temporalFields[k] && !seen {
					found[k] = x[k]
				}
				walk(x[k])
			}
		case []any:
			if len(x) > 0 {
				walk(x[0])
			}
		}
	}
	walk(v)
	return found
}

// joinTemporals describes each found field, in name order, omitting those the
// description leaves empty.
func joinTemporals(found map[string]any, describe func(any) string) string {
	names := make([]string, 0, len(found))
	for k := range found {
		names = append(names, k)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, k := range names {
		if d := describe(found[k]); d != "" {
			parts = append(parts, k+"="+d)
		}
	}
	return strings.Join(parts, ", ")
}

// valueMask masks the JSON value of a temporal field.
func valueMask(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return mask(x)
	case json.Number:
		return mask(x.String())
	case bool:
		return "bool"
	case map[string]any:
		return "object"
	default:
		return "list"
	}
}

// rootItems returns "items at root: N" when the JSON root is a list; N is a
// count, never a value. For an object or a body that is not JSON, it returns
// empty.
func rootItems(body []byte) string {
	var v any
	if json.Unmarshal(bytes.TrimSpace(body), &v) != nil {
		return ""
	}
	l, ok := v.([]any)
	if !ok {
		return ""
	}
	return fmt.Sprintf("items at root: %d", len(l))
}

// compareItems, when the root is a list of more than one object, says whether
// the items are equal after re-encoding (field order does not count) and, if
// they differ, which top-level field NAMES change between the first item and
// any other. It never prints values. For the other cases it returns empty.
func compareItems(body []byte) string {
	var v any
	if json.Unmarshal(bytes.TrimSpace(body), &v) != nil {
		return ""
	}
	l, ok := v.([]any)
	if !ok || len(l) < 2 {
		return ""
	}
	first, ok := l[0].(map[string]any)
	if !ok {
		return ""
	}
	differs := map[string]bool{}
	for _, it := range l[1:] {
		other, ok := it.(map[string]any)
		if !ok {
			return "items differ"
		}
		for k, a := range first {
			if !sameJSON(a, other[k]) || !hasKey(other, k) {
				differs[k] = true
			}
		}
		for k := range other {
			if !hasKey(first, k) {
				differs[k] = true
			}
		}
	}
	if len(differs) == 0 {
		return "items equal"
	}
	names := make([]string, 0, len(differs))
	for k := range differs {
		names = append(names, k)
	}
	sort.Strings(names)
	return "items differ; fields that differ: " + strings.Join(names, ",")
}

func hasKey(m map[string]any, k string) bool {
	_, ok := m[k]
	return ok
}

// sameJSON compares two decoded values by their canonical form (keys sorted
// by the encoder).
func sameJSON(a, b any) bool {
	ja, erra := json.Marshal(a)
	jb, errb := json.Marshal(b)
	return erra == nil && errb == nil && bytes.Equal(ja, jb)
}
