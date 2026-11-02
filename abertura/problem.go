package abertura

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// ProblemContentType is the RFC 9457 (Problem Details for HTTP APIs) media
// type. Use it in the Content-Type header when you return a [Problem] as JSON.
const ProblemContentType = "application/problem+json"

// ErrorCatalogURL is the error catalog page. The Type field of a [Problem]
// is this URL plus "#" and the slug of the entry.
//
// The page is generated from docs/errors.md by "make docs"
// (out/docs/errors/index.html, with one id per slug) and published on GitHub
// Pages, next to the Redoc description, by the release-openapi workflow at
// every final openapi/vX.Y.Z tag (-rc.N pre-releases do not publish) and only
// while the repository is public. The /errors page responds once openapi/v0.2.0
// is published; until then the catalog can be read in docs/errors.md and the
// v0.1 page at /erros stays live.
const ErrorCatalogURL = "https://marcobuarque.github.io/abertura-mercado-energia/errors"

// Problem is an SDK error in the RFC 9457 format (Problem Details for HTTP
// APIs). The API returns only four fields (timestamp, status, error, message);
// the SDK delivers this format on top of it, with a title and an explanation
// written by the project and linked to the catalog at [ErrorCatalogURL].
//
// A Problem never carries the raw API message, which may mention a consumer
// unit, a document or an e-mail: Detail is catalog text. The original
// message stays in [APIError].Message, for callers who decide to use it. It
// also carries no token, secret, URL with a query string or body.
//
// To return a Problem from an HTTP server, write it with the Content-Type
// [ProblemContentType]. Status is the one from the API response; if your
// server responds with another status, set Status to the same value before
// encoding, as the RFC asks.
type Problem struct {
	// Type is the URI of the catalog entry (".../errors#migration-already-requested").
	// [ProblemFrom] always fills it, using "#not-in-catalog" for what has no
	// entry of its own. Empty, or "about:blank", is a Problem built by hand for
	// an error with no specific semantics (RFC 9457, 4.2.1): the JSON omits the
	// empty type and uses the standard HTTP status phrase as the title.
	Type string
	// Title is the short title of the entry, in English. In the JSON of a
	// Problem with an empty Type or "about:blank", the title is the standard
	// status phrase (and is omitted when there is no Status), as the RFC asks.
	Title string
	// Status is the HTTP status of the API response. It is 0 when there was no
	// response (network failure, local validation), and is then omitted from
	// the JSON. The [ErrNotFound] of a lookup by id that came back empty
	// carries 404, even though the API answered 200.
	Status int
	// Detail explains this occurrence with catalog text and the operation
	// name. It never carries personal data.
	Detail string
	// Instance identifies this occurrence. It is always empty in this version
	// and omitted from the JSON.
	Instance string

	// Code is the API code ("ERR_MIGRACAO_SOLICITADA"), if there is one and it
	// has the form ERR_ followed by uppercase letters, digits and underscores.
	Code string
	// Operation is the operationId of the call ("createMigration"), if the SDK
	// knows it.
	Operation string
	// RequestID is the x-amzn-RequestId of the response, useful for support.
	RequestID string
	// Retryable says whether it is safe to retry the same call now, without
	// checking anything first. It follows the SDK retry table: reads retry on
	// 429 and 5xx (except 501); writes only when nothing was sent or the
	// gateway throttled.
	Retryable bool
	// Uncertain marks a write with an uncertain result: check the state
	// (FindCreated, CheckEdited) before retrying.
	Uncertain bool
}

// problemJSON is the serialized form of a [Problem].
type problemJSON struct {
	Type      string `json:"type,omitempty"`
	Title     string `json:"title,omitempty"`
	Status    int    `json:"status,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Instance  string `json:"instance,omitempty"`
	Code      string `json:"code,omitempty"`
	Operation string `json:"operation,omitempty"`
	RequestID string `json:"requestId,omitempty"`
	Retryable bool   `json:"retryable"`
	Uncertain bool   `json:"uncertain"`
}

// MarshalJSON writes the Problem as in RFC 9457: type, title, status, detail
// and instance, plus the extensions code, operation, requestId, retryable and
// uncertain (camelCase). Empty text fields and status 0 are omitted; retryable
// and uncertain are always written, because "false" is information too.
//
// With an empty Type or "about:blank", the title is the standard HTTP status
// phrase (RFC 9457, 4.2.1), and is omitted when Status is 0 or unknown.
func (p Problem) MarshalJSON() ([]byte, error) {
	if p.Type == "" || p.Type == "about:blank" {
		p.Title = http.StatusText(p.Status)
	}
	return json.Marshal(problemJSON(p))
}

// ErrInvalidArgument matches, with errors.Is, the local validation errors: the
// SDK refused the call before sending it (incomplete filter, empty id, Do path
// without "/"). Nothing was sent, and retrying without a fix is pointless.
var ErrInvalidArgument error = &sentinel{text: "abertura: invalid argument"}

// argumentError is a local validation error. Error() keeps the original text;
// Is makes the error match [ErrInvalidArgument].
type argumentError struct {
	msg string
	err error // cause, if any (for example, the encoding/json error)
}

func (e *argumentError) Error() string { return e.msg }

func (e *argumentError) Unwrap() error { return e.err }

func (e *argumentError) Is(target error) bool { return target == ErrInvalidArgument }

// invalidArgument builds a local validation error.
func invalidArgument(format string, a ...any) error {
	return &argumentError{msg: fmt.Sprintf(format, a...)}
}

// invalidArgumentBecause is invalidArgument with a cause, which stays
// reachable through errors.As. The text is that of format, without the cause.
func invalidArgumentBecause(err error, format string, a ...any) error {
	return &argumentError{msg: fmt.Sprintf(format, a...), err: err}
}

// catalogEntry is a catalog entry; the anchor in docs/errors.md is the slug.
type catalogEntry struct {
	title  string
	detail string
}

// catalog is the only source, in the code, of slugs and titles. The parity
// test checks each slug against the anchors in docs/errors.md.
//
//nolint:gochecknoglobals // read-only table, never written after initialization
var catalog = map[string]catalogEntry{
	"invalid-credential-authorizer": {
		title:  "Credential refused by the authorizer",
		detail: "The API authorizer refused the token, or the agent and profile, before the request reached the backend. Nothing was processed.",
	},
	"invalid-credential-backend": {
		title:  "Credential refused by the backend",
		detail: "The API backend answered 403 with ERR_CREDENCIAL_INVALIDA, without the authorizer mark. Check the credential, agent and profile; the SDK does not retry.",
	},
	"field-validation": {
		title:  "Invalid field in the request",
		detail: "The API rejected a field of the request. It validates one field at a time and stops at the first problem.",
	},
	"retailer-profile-required": {
		title:  "Retailer profile required",
		detail: "The API requires the retailer profile in this query and did not receive it.",
	},
	"migration-already-requested": {
		title:  "Migration already requested",
		detail: "An active migration already exists for the same consumer unit and distributor. If the previous creation was uncertain, it probably took effect: check.",
	},
	"month-range-required": {
		title:  "Month range required",
		detail: "The migration listing requires both months of the range (mesReferenciaInicial and mesReferenciaFinal, YYYY-MM) and the API did not receive them.",
	},
	"month-range-too-wide": {
		title:  "Month range too wide",
		detail: "The API limits the distance between the start and end months, and the requested range exceeded the limit. The limit is not published; narrow the range or split the query into parts.",
	},
	"unknown-code": {
		title:  "Error code unknown to the SDK",
		detail: "The API returned an ERR_* code that the SDK does not know yet. The code is in code; the API text stays in APIError.Message.",
	},
	"refusal-without-code": {
		title:  "Refusal without an API code",
		detail: "The API refused the request without a recognizable ERR_* code. Use the status and the request id to investigate.",
	},
	"not-found": {
		title:  "Resource not found",
		detail: "The API does not have the requested resource, or the lookup by id came back empty.",
	},
	"rate-limited": {
		title:  "Rate limit exceeded",
		detail: "The API or the gateway throttled the request rate. Wait before trying again.",
	},
	"internal-error": {
		title:  "API internal error",
		detail: "The API backend failed to process the request.",
	},
	"gateway-failure": {
		title:  "API gateway failure",
		detail: "The API gateway returned 502 or 504, possibly without the API error format.",
	},
	"transport-error": {
		title:  "Network failure",
		detail: "The request did not complete because of a network failure (DNS, connection or TLS).",
	},
	"timeout": {
		title:  "Timeout",
		detail: "The attempt deadline, or the call context deadline, expired before the response.",
	},
	"call-canceled": {
		title:  "Call canceled",
		detail: "The call context was canceled before the call finished.",
	},
	"uncertain-result": {
		title:  "Uncertain write result",
		detail: "The write was sent, but the response does not say whether it was applied. The SDK does not retry: check the state first.",
	},
	"unreadable-response": {
		title:  "Unreadable response",
		detail: "The API answered successfully, but the body could not be read or understood. For a write, the result is uncertain; for a read, retrying is safe.",
	},
	"repeated-cursor": {
		title:  "Repeated pagination cursor",
		detail: "The API returned an already used cursor as the next page; the SDK stopped so it would not loop forever.",
	},
	"token-request-failed": {
		title:  "Token request failed",
		detail: "The token endpoint refused the request or failed. Check client_id, client_secret and the environment.",
	},
	"invalid-argument": {
		title:  "Invalid argument",
		detail: "The SDK refused the call before sending it: an argument is missing or extra. Nothing was sent.",
	},
	"not-in-catalog": {
		title:  "Error outside the catalog",
		detail: "The error does not come from the SDK, or has no catalog entry.",
	},
}

// validCode returns code if it has the form of an API code (ERR_ followed by
// uppercase letters, digits and underscores, up to 80 characters after the
// prefix), and an empty string otherwise. The field comes from the server and
// must not carry free text into the JSON.
func validCode(code string) string {
	rest, ok := strings.CutPrefix(code, "ERR_")
	if !ok || rest == "" || len(rest) > 80 {
		return ""
	}
	for _, r := range rest {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return ""
		}
	}
	return code
}

// onlyIdentifier returns s if it has only letters, digits and underscores (an
// operationId), and an empty string otherwise.
func onlyIdentifier(s string) string {
	if s == "" || len(s) > 60 {
		return ""
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return ""
		}
	}
	return s
}

// newProblem builds the Problem for a catalog entry.
func newProblem(slug string, status int, op string) Problem {
	e := catalog[slug]
	p := Problem{Type: ErrorCatalogURL + "#" + slug, Title: e.title, Status: status, Detail: e.detail}
	if op = onlyIdentifier(op); op != "" {
		p.Operation = op
		p.Detail = "Operation " + op + ": " + e.detail
	}
	return p
}

// ProblemFrom translates an SDK error into a [Problem]. It accepts the error
// directly, wrapped with %w or inside errors.Join: the lookup is that of
// errors.As and errors.Is, and an [*UncertainResultError] in the tree wins over
// an [*APIError]. An error the SDK does not classify (including one that does
// not come from the SDK) becomes the "not-in-catalog" entry, without the error
// text; the type is never "about:blank". ProblemFrom(nil) returns the zero
// value.
func ProblemFrom(err error) Problem {
	if err == nil {
		return Problem{}
	}
	var inc *UncertainResultError
	if errors.As(err, &inc) {
		return inc.Problem()
	}
	var api *APIError
	if errors.As(err, &api) {
		return api.Problem()
	}
	var et *transportError
	var er *responseError
	var ft *tokenFailureError
	var ne net.Error
	switch {
	case errors.As(err, &et):
		return et.problem()
	case errors.As(err, &er):
		return er.problem()
	case errors.Is(err, ErrNotFound):
		return newProblem("not-found", http.StatusNotFound, "")
	case errors.Is(err, ErrRepeatedCursor):
		return newProblem("repeated-cursor", 0, "")
	case errors.Is(err, ErrInvalidArgument):
		return newProblem("invalid-argument", 0, "")
	case errors.Is(err, context.DeadlineExceeded):
		p := newProblem("timeout", 0, "")
		p.Retryable = true
		return p
	case errors.Is(err, context.Canceled):
		return newProblem("call-canceled", 0, "")
	case errors.As(err, &ft):
		return newProblem("token-request-failed", 0, tokenOperation)
	case errors.As(err, &ne) && ne.Timeout():
		p := newProblem("timeout", 0, "")
		p.Retryable = true
		return p
	case errors.As(err, &ne):
		p := newProblem("transport-error", 0, "")
		p.Retryable = true
		return p
	}
	return newProblem("not-in-catalog", 0, "")
}

// problem translates the transport failure. It tells whether the attempt
// deadline expired or something else failed; Retryable and Uncertain come from
// decide, using the call method and what was written.
func (e *transportError) problem() Problem {
	slug, failure := "transport-error", failureTransport
	if e.timeout {
		slug, failure = "timeout", failureTimeout
	}
	p := newProblem(slug, 0, "")
	applyDecision(&p, e.httpMethod, outcome{failure: failure, wrote: e.wrote})
	return p
}

// problem translates the unreadable response, which outside an
// UncertainResultError always comes from a read.
func (e *responseError) problem() Problem {
	p := newProblem("unreadable-response", 0, "")
	applyDecision(&p, http.MethodGet, outcome{status: http.StatusOK, unreadableBody: true})
	return p
}

// applyDecision fills Retryable and Uncertain from the decision of decide. A
// maxRetries of 1 only makes the decision "retry", not "exhausted".
func applyDecision(p *Problem, httpMethod string, d outcome) {
	d.maxRetries = 1
	switch decide(httpMethod, d) {
	case actionRetry:
		p.Retryable = true
	case actionUncertain:
		p.Uncertain = true
	}
}

// slug tells which catalog entry describes the error.
func (e *APIError) slug(code string) string {
	switch {
	case e.Operation == tokenOperation:
		return "token-request-failed"
	case code == "ERR_CREDENCIAL_INVALIDA" && e.GatewayErrorType != "":
		return "invalid-credential-authorizer"
	case code == "ERR_CREDENCIAL_INVALIDA":
		return "invalid-credential-backend"
	case code == "ERR_VALIDACAO_CAMPOS":
		return "field-validation"
	case code == "ERR_PERFIL_VAREJISTA_OBRIGATORIO":
		return "retailer-profile-required"
	case code == "ERR_MIGRACAO_SOLICITADA":
		return "migration-already-requested"
	case code == "ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO":
		return "month-range-required"
	case code == "ERR_MES_REFERENCIA_INICIAL_DIFERENCA":
		return "month-range-too-wide"
	case code != "":
		return "unknown-code"
	case e.StatusCode == http.StatusNotFound:
		return "not-found"
	case e.StatusCode == http.StatusTooManyRequests:
		return "rate-limited"
	case e.StatusCode == http.StatusBadGateway, e.StatusCode == http.StatusGatewayTimeout:
		return "gateway-failure"
	case e.StatusCode >= 500:
		return "internal-error"
	default:
		return "refusal-without-code"
	}
}

// Problem translates the error into a [Problem]. Detail comes from the
// catalog; the API message is left out (use e.Message). Retryable and
// Uncertain follow the same decision function as the transport, based on the
// call method the SDK recorded; an APIError built by hand has no method and
// asserts neither.
func (e *APIError) Problem() Problem {
	code := validCode(e.Code)
	p := newProblem(e.slug(code), e.StatusCode, e.Operation)
	p.Code = code
	p.RequestID = sanitize(e.RequestID, 100)
	if e.httpMethod != "" {
		applyDecision(&p, e.httpMethod, outcome{
			status:            e.StatusCode,
			gateway:           e.GatewayErrorType != "",
			invalidCredential: code == invalidCredentialCode,
		})
	}
	return p
}

// Problem translates the error into a [Problem] with the "uncertain-result"
// entry. Status, Code and RequestID come from the cause, when it is an API
// response; Detail mentions only the title of the cause, never the Key (which
// may hold the consumer unit).
func (e *UncertainResultError) Problem() Problem {
	p := newProblem("uncertain-result", 0, e.Operation)
	p.Uncertain = true
	if e.Err == nil {
		return p
	}
	cause := ProblemFrom(e.Err)
	p.Status, p.Code, p.RequestID = cause.Status, cause.Code, cause.RequestID
	p.Detail += " Cause: " + cause.Title + "."
	return p
}
