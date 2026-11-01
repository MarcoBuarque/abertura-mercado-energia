package abertura

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// doOperation is the operationId of the calls made by Client.Do.
const doOperation = "do"

// Do makes an arbitrary call to the API, through a route the SDK does not
// cover yet.
//
// path is relative to the environment APIURL, starts with "/" and is already
// escaped; the query, if any, goes after the "?". body is encoded as JSON (nil
// sends no body). out receives the decoded 2xx response; it can be a pointer
// to a struct, a *json.RawMessage for the raw body, or nil to ignore it. An
// empty response body leaves out untouched.
//
// Do goes through the same loop as the typed methods: token, headers, rate
// limiter, read retries, errors ([*APIError]) and logging. A write is only
// retried in the two exceptions of the retry table: nothing was sent, or the
// authorizer refused before the backend. A write with no known outcome, or
// with a 2xx response that does not decode into out, returns
// [*UncertainResultError] with a nil Key: the SDK does not know how to check
// it. The route is logged without the query.
//
// There is no validation against the spec and no compatibility guarantee
// across versions.
func (c *Client) Do(ctx context.Context, method, path string, body, out any, opts ...CallOption) error {
	op, err := freeOperation(method, path, body)
	if err != nil {
		return err
	}
	_, respBody, err := c.send(ctx, op, newCallConfig(opts...))
	if err != nil {
		return err
	}
	if err := decodeOutput(respBody, out); err != nil {
		err = fmt.Errorf("%s: %w", doOperation, err)
		if !isRead(op.httpMethod) {
			// The write was accepted, but we do not know what it returned.
			return &UncertainResultError{Operation: doOperation, Err: err}
		}
		return err
	}
	return nil
}

// freeOperation builds the operation of a Do. The errors never quote the path
// or the query, which may carry consumer unit data.
func freeOperation(method, path string, body any) (operation, error) {
	httpMethod := strings.ToUpper(strings.TrimSpace(method))
	if httpMethod == "" {
		return operation{}, invalidArgument("%s: empty method", doOperation)
	}
	if !strings.HasPrefix(path, "/") {
		return operation{}, invalidArgument("%s: the path must start with \"/\"", doOperation)
	}
	route, rawQuery, _ := strings.Cut(path, "?")
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return operation{}, invalidArgument("%s: invalid query", doOperation)
	}
	return operation{
		id:           doOperation,
		httpMethod:   httpMethod,
		pathTemplate: route,
		path:         route,
		query:        query,
		body:         body,
		group:        groupForRoute(route),
	}, nil
}

// groupForRoute sends to the measurements bucket the routes that have the
// "medicoes" segment, which have their own limit in production.
func groupForRoute(route string) limitGroup {
	for seg := range strings.SplitSeq(route, "/") {
		if seg == "medicoes" {
			return groupMeasurements
		}
	}
	return groupGeneral
}

// decodeOutput fills out with the body of a 2xx response.
func decodeOutput(body []byte, out any) error {
	if out == nil || len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if raw, ok := out.(*json.RawMessage); ok && raw != nil {
		*raw = append((*raw)[:0], body...)
		return nil
	}
	err := json.Unmarshal(body, out)
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		// The encoding/json message quotes a piece of the body.
		return unreadableResponse(fmt.Errorf("response is not valid JSON (byte %d)", syntaxErr.Offset))
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		// Error() and Value from encoding/json may carry the response value;
		// the error is not chained and the text carries only the field and the
		// expected type. From Go 1.27 on, Field may be empty (and an invalid
		// number becomes an UnmarshalTypeError); the text adapts and never
		// quotes the value.
		if typeErr.Field == "" {
			return unreadableResponse(fmt.Errorf("decode response: value has an unexpected type (expected %s)", typeErr.Type))
		}
		return unreadableResponse(fmt.Errorf("decode response: field %q has an unexpected type (expected %s)", typeErr.Field, typeErr.Type))
	}
	if err != nil {
		// Other encoding/json errors (an invalid number, for example) also
		// quote the value; they are not chained.
		return unreadableResponse(errors.New("decode response: unexpected JSON"))
	}
	return nil
}
