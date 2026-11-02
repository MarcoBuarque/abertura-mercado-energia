package abertura

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

const (
	// bodyLimit is the maximum number of body bytes APIError keeps.
	bodyLimit = 4 << 10
	// messageLimit is the maximum number of message characters in Error().
	messageLimit = 200
	// codeLimit is the maximum number of code characters in Error().
	codeLimit = 100
)

// APIError is the error returned when the API responds with a status outside
// 2xx.
//
// The API uses two error formats, both with the fields "error" and "message":
// the backend one (400) and the API Gateway authorizer one (403, with the
// x-amzn-ErrorType header). A body that is not JSON also becomes an APIError,
// with an empty Code and Message.
//
// Error() shows the operation, the status, the code and the message (cut at
// 200 characters). It never includes the body or headers; the body is only
// available through RawBody.
type APIError struct {
	// Operation is the operationId from the spec ("listMigrations"); "do" in
	// Client.Do.
	Operation string
	// StatusCode is the HTTP status of the response.
	StatusCode int
	// Code is the "error" field of the body, a stable textual code such as
	// "ERR_MIGRACAO_SOLICITADA". Empty if the body does not carry it.
	Code string
	// Message is the "message" field of the body, written for humans. It is
	// server text and appears in Error(): no message seen in the sandbox
	// carried consumer-unit data, but nothing prevents it. When logging the
	// error where personal data must not go, prefer Operation, StatusCode, Code
	// and RequestID.
	Message string
	// RequestID is the x-amzn-RequestId header, useful for support.
	RequestID string
	// GatewayErrorType is the x-amzn-ErrorType header. When set, it means the
	// API Gateway refused the request before the backend; when empty, the
	// error came from the backend.
	GatewayErrorType string

	// httpMethod is the HTTP method of the call, recorded by the SDK; it feeds
	// Problem().Retryable and Uncertain. Empty in an APIError built by hand.
	httpMethod string
	rawBody    []byte
}

// decodeError builds the APIError of a response with an error status. The body
// was already read by the caller; resp only provides the status and headers
// and may be nil.
func decodeError(op string, resp *http.Response, body []byte) *APIError {
	e := &APIError{Operation: op}
	if resp != nil {
		e.StatusCode = resp.StatusCode
		e.RequestID = resp.Header.Get("x-amzn-RequestId")
		e.GatewayErrorType = resp.Header.Get("x-amzn-ErrorType")
	}

	kept := body
	if len(kept) > bodyLimit {
		kept = kept[:bodyLimit]
	}
	e.rawBody = append([]byte(nil), kept...)

	// Tolerant decoding: extra fields or fields of another type do not fail.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return e
	}
	e.Code = fieldText(fields["error"])
	e.Message = fieldText(fields["message"])
	return e
}

// fieldText returns the text of a JSON field, or "" if it is not text.
func fieldText(raw json.RawMessage) string {
	var s string
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// Error implements error, without the body or headers.
func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "abertura: %s: HTTP %d", e.Operation, e.StatusCode)
	if e.Code != "" {
		b.WriteString(" ")
		b.WriteString(sanitize(e.Code, codeLimit))
	}
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(sanitize(e.Message, messageLimit))
	}
	return b.String()
}

// GoString makes %#v follow Error: without it, it would print the raw body in
// hexadecimal.
func (e *APIError) GoString() string { return e.Error() }

// sanitize replaces control characters with a space and cuts the text at max
// characters, so that text coming from the server cannot break or forge log
// lines.
func sanitize(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

// Is makes the package sentinels match the error: the code ones ("ERR_*") by
// the Code field, ignoring the status; ErrNotFound and ErrRateLimited by
// StatusCode.
func (e *APIError) Is(target error) bool {
	s, ok := target.(*sentinel)
	if !ok {
		return false
	}
	switch {
	case s.code != "":
		return e.Code == s.code
	case s.status != 0:
		return e.StatusCode == s.status
	default:
		return false
	}
}

// RawBody returns a copy of the first 4 KiB of the response body. It is the
// only path to the body: it never appears in Error() or in SDK logs.
func (e *APIError) RawBody() []byte {
	return append([]byte(nil), e.rawBody...)
}

// sentinel is the type of the package value errors, comparable with errors.Is.
type sentinel struct {
	text   string
	code   string // matches APIError.Code
	status int    // matches APIError.StatusCode
}

func (s *sentinel) Error() string { return s.text }

// Sentinels by API error code. They match with errors.Is through the Code field
// of APIError, because the same 400 status has different meanings.
var (
	// ErrInvalidCredential matches the authorizer 403 (ERR_CREDENCIAL_INVALIDA).
	ErrInvalidCredential error = &sentinel{text: "abertura: invalid credential (ERR_CREDENCIAL_INVALIDA)", code: "ERR_CREDENCIAL_INVALIDA"}
	// ErrFieldValidation matches an invalid field in the request
	// (ERR_VALIDACAO_CAMPOS).
	ErrFieldValidation error = &sentinel{text: "abertura: field validation failed (ERR_VALIDACAO_CAMPOS)", code: "ERR_VALIDACAO_CAMPOS"}
	// ErrRetailerProfileRequired matches a missing retailer profile
	// (ERR_PERFIL_VAREJISTA_OBRIGATORIO).
	ErrRetailerProfileRequired error = &sentinel{text: "abertura: retailer profile required (ERR_PERFIL_VAREJISTA_OBRIGATORIO)", code: "ERR_PERFIL_VAREJISTA_OBRIGATORIO"}
	// ErrMigrationAlreadyRequested matches an active migration that already
	// exists for the same consumer unit and distributor (ERR_MIGRACAO_SOLICITADA).
	ErrMigrationAlreadyRequested error = &sentinel{text: "abertura: migration already requested (ERR_MIGRACAO_SOLICITADA)", code: "ERR_MIGRACAO_SOLICITADA"}
	// ErrMonthRangeRequired matches a migration listing requested without the
	// month range (ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO). The SDK validates
	// the range before sending; only [Client.Do] reaches this code.
	ErrMonthRangeRequired error = &sentinel{text: "abertura: month range required (ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO)", code: "ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO"}
	// ErrMonthRangeTooWide matches a range between the start and end months
	// that exceeded the API limit (ERR_MES_REFERENCIA_INICIAL_DIFERENCA). The
	// limit is not published; in the sandbox, on 2026-11-01, 12 months passed
	// and 24 did not (cap between 12 and 24, SOURCES D22). Narrow the range or
	// split the query.
	ErrMonthRangeTooWide error = &sentinel{text: "abertura: month range too wide (ERR_MES_REFERENCIA_INICIAL_DIFERENCA)", code: "ERR_MES_REFERENCIA_INICIAL_DIFERENCA"}
)

// HTTP mechanics sentinels. They match with errors.Is through the StatusCode of
// APIError.
var (
	// ErrNotFound matches a missing resource (404). It is also returned,
	// without an APIError, when a lookup by id comes back empty.
	ErrNotFound error = &sentinel{text: "abertura: not found", status: http.StatusNotFound}
	// ErrRateLimited matches an exceeded request limit (429).
	ErrRateLimited error = &sentinel{text: "abertura: rate limit exceeded", status: http.StatusTooManyRequests}
)

// NaturalKey identifies, on the server side, the resource that a write with an
// uncertain result may have created or changed. It is the input of the Check*
// methods. Only the types of this package implement it.
type NaturalKey interface {
	String() string
	naturalKey()
}

// UncertainResultError indicates that a write may or may not have been
// applied: the request left, but the response does not say (transport error
// after sending, timeout, 5xx, or 2xx with an unreadable body). The SDK does
// not retry the write on its own; check the state with Key and the matching
// Check* method.
//
// Error() does not include Key, which may hold consumer-unit data.
type UncertainResultError struct {
	// Operation is the operationId of the write.
	Operation string
	// Key locates the affected resource; nil in Client.Do.
	Key NaturalKey
	// Err is the cause: transport error, timeout, 5xx *APIError or decoding
	// failure.
	Err error
}

// Error implements error.
func (e *UncertainResultError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("abertura: %s: uncertain result", e.Operation)
	}
	return fmt.Sprintf("abertura: %s: uncertain result: %v", e.Operation, e.Err)
}

// GoString makes %#v follow Error: without it, it would print the whole Key and
// cause.
func (e *UncertainResultError) GoString() string { return e.Error() }

// Unwrap returns the cause, for errors.Is and errors.As.
func (e *UncertainResultError) Unwrap() error { return e.Err }

// stripURL removes the URL from a *url.Error, which carries the query (consumer
// unit and month), but keeps the cause reachable through errors.Is and
// errors.As.
func stripURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}

// transportError is a failure without a usable response (DNS, connection, TLS,
// dropped connection, attempt deadline). Error() and Unwrap follow the cause,
// already without the URL; the type keeps what decide needs so that
// ProblemFrom can say whether retrying is safe.
type transportError struct {
	err        error
	httpMethod string
	wrote      bool // the request headers were written
	timeout    bool // the attempt deadline expired
}

func (e *transportError) Error() string { return e.err.Error() }

func (e *transportError) Unwrap() error { return e.err }

// responseError is a 2xx response that the SDK could not read (interrupted
// body) or understand (invalid JSON, unreadable field). Error() and Unwrap
// follow the cause. Outside an *UncertainResultError, it always comes from a
// read: every write with an unreadable response is uncertain.
type responseError struct{ err error }

func (e *responseError) Error() string { return e.err.Error() }

func (e *responseError) Unwrap() error { return e.err }

// unreadableResponse marks err as an unreadable 2xx response.
func unreadableResponse(err error) error { return &responseError{err: err} }

// tokenFailureError is a failure to obtain the token that is neither an error
// response from the endpoint nor a network failure: a response without
// access_token, or an error from the external source (WithTokenSource).
// Error() and Unwrap follow the cause.
type tokenFailureError struct{ err error }

func (e *tokenFailureError) Error() string { return e.err.Error() }

func (e *tokenFailureError) Unwrap() error { return e.err }
