// Package testserver provides an HTTP test server that answers with the
// fixtures in testdata/ and validates every incoming request against the
// project's OpenAPI description.
//
// It exists for the SDK tests: it catches wrong parameter names, missing
// headers and paths that are not in the spec. It does not import the abertura
// package, to avoid an import cycle, and it uses kin-openapi, which is
// restricted to tests.
package testserver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura/internal/spec"
)

// Response describes one server response.
type Response struct {
	// Status is the HTTP status code.
	Status int
	// Fixture is the name of a file in testdata/. Empty returns an empty body.
	Fixture string
	// Headers are added to the default headers and override them.
	Headers map[string]string
	// Delay is how long to wait before answering (interrupted if the client gives up).
	Delay time.Duration
	// Drop reads the request and closes the connection without answering.
	Drop bool
}

// Route binds a method and a spec path to a sequence of responses.
type Route struct {
	// Method is the HTTP method, for example "GET".
	Method string
	// PathTemplate is the path as written in the spec, for example "/v1.1/varejista/migracoes/{idMigracao}".
	PathTemplate string
	// Responses are returned in sequence; the last one repeats.
	Responses []Response
}

// fakeRequestID is the x-amzn-RequestId of every response.
const fakeRequestID = "00000000-0000-4000-8000-0000000000f1"

// defaultHeaders applies to every response.
var defaultHeaders = map[string]string{
	"x-amzn-RequestId": fakeRequestID,
}

// statusHeaders applies to responses with the given status: the gateway
// authorizer marks the 403 with x-amzn-ErrorType.
var statusHeaders = map[int]map[string]string{
	http.StatusForbidden: {"x-amzn-ErrorType": "AccessDeniedException"},
}

type record struct {
	req  *http.Request
	body []byte
}

// Server is the test server. Use New.
type Server struct {
	t           testing.TB
	srv         *httptest.Server
	router      routers.Router
	routes      []Route
	testdataDir string
	validate    bool
	mu          sync.Mutex
	counts      map[string]int // key "METHOD pathTemplate": calls served
	position    map[string]int // next response of the sequence
	records     []record
	violations  []string
}

// New starts the server and shuts it down at the end of the test. By default
// it validates every request against the spec: a violation fails the test via
// t.Errorf and the request gets status 400.
func New(t testing.TB, routes ...Route) *Server {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(spec.Abertura)
	if err != nil {
		t.Fatalf("testserver: load spec: %v", err)
	}
	// Without servers, the router matches by path only.
	doc.Servers = nil
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatalf("testserver: build spec router: %v", err)
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("testserver: cannot find the source path")
	}
	s := &Server{
		t:           t,
		router:      router,
		routes:      routes,
		testdataDir: filepath.Join(filepath.Dir(file), "..", "..", "testdata"),
		validate:    true,
		counts:      map[string]int{},
		position:    map[string]int{},
	}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.srv.Close)
	return s
}

// WithoutValidation turns off validation against the spec, for tests that
// deliberately send an invalid request.
func (s *Server) WithoutValidation() *Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.validate = false
	return s
}

// URL returns the base address of the server, without a trailing slash.
func (s *Server) URL() string { return s.srv.URL }

// Count returns how many requests the server handled for the given method and
// path template.
func (s *Server) Count(httpMethod, pathTemplate string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[key(httpMethod, pathTemplate)]
}

// Requests returns copies of the received requests, in order, with a readable
// body. Use it to inspect headers and bodies.
func (s *Server) Requests() []*http.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*http.Request, 0, len(s.records))
	for _, r := range s.records {
		c := r.req.Clone(context.Background())
		c.Body = io.NopCloser(bytes.NewReader(r.body))
		out = append(out, c)
	}
	return out
}

// Violations returns the messages of the requests the spec does not accept.
func (s *Server) Violations() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.violations...)
}

func key(httpMethod, pathTemplate string) string {
	return strings.ToUpper(httpMethod) + " " + pathTemplate
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.t.Errorf("testserver: read request body: %v", err)
		http.Error(w, "error reading body", http.StatusInternalServerError)
		return
	}
	if err := r.Body.Close(); err != nil {
		s.t.Errorf("testserver: close request body: %v", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	s.mu.Lock()
	s.records = append(s.records, record{req: r.Clone(context.Background()), body: body})
	validate := s.validate
	s.mu.Unlock()

	route, pathParams, err := s.router.FindRoute(r)
	if err != nil {
		if validate {
			s.violate(w, http.StatusNotFound, "%s %s is not in the spec: %v", r.Method, r.URL.Path, err)
		} else {
			http.Error(w, "route not in the spec", http.StatusNotFound)
		}
		return
	}
	if validate {
		if msg := s.validateRequest(r, body, route, pathParams); msg != "" {
			s.violate(w, http.StatusBadRequest, "%s %s violates the spec: %s", r.Method, r.URL.Path, msg)
			return
		}
	}

	stub, ok := s.findRoute(r.Method, route.Path)
	if !ok {
		s.t.Errorf("testserver: no route registered for %s", key(r.Method, route.Path))
		http.Error(w, "no route registered", http.StatusNotFound)
		return
	}
	resp := s.next(stub)
	if resp.Delay > 0 {
		select {
		case <-time.After(resp.Delay):
		case <-r.Context().Done():
			return
		}
	}
	if resp.Drop {
		s.drop(w)
		return
	}
	s.writeResponse(w, resp)
}

// validateRequest returns an empty string if the spec accepts the request.
func (s *Server) validateRequest(r *http.Request, body []byte, route *routers.Route, pathParams map[string]string) string {
	// Validation consumes the body; each use gets a fresh reader.
	r.Body = io.NopCloser(bytes.NewReader(body))
	in := &openapi3filter.RequestValidationInput{
		Request:    r,
		PathParams: pathParams,
		Route:      route,
		Options:    &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
	}
	if err := openapi3filter.ValidateRequest(r.Context(), in); err != nil {
		return err.Error()
	}
	// kin-openapi does not reject query parameters the spec does not declare.
	declared := map[string]bool{}
	if route.PathItem != nil {
		for _, p := range route.PathItem.Parameters {
			if p.Value != nil && p.Value.In == openapi3.ParameterInQuery {
				declared[p.Value.Name] = true
			}
		}
	}
	for _, p := range route.Operation.Parameters {
		if p.Value != nil && p.Value.In == openapi3.ParameterInQuery {
			declared[p.Value.Name] = true
		}
	}
	for name := range r.URL.Query() {
		if !declared[name] {
			return fmt.Sprintf("query parameter %q is not declared by operation %s", name, route.Operation.OperationID)
		}
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return ""
}

func (s *Server) violate(w http.ResponseWriter, status int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	s.mu.Lock()
	s.violations = append(s.violations, msg)
	s.mu.Unlock()
	s.t.Errorf("testserver: SDK request violates the spec: %s", msg)
	http.Error(w, "spec violation: "+msg, status)
}

func (s *Server) findRoute(httpMethod, pathTemplate string) (*Route, bool) {
	for i := range s.routes {
		if strings.EqualFold(s.routes[i].Method, httpMethod) && s.routes[i].PathTemplate == pathTemplate {
			return &s.routes[i], true
		}
	}
	return nil, false
}

// next counts the call and picks the response from the sequence.
func (s *Server) next(route *Route) Response {
	k := key(route.Method, route.PathTemplate)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts[k]++
	i := s.position[k]
	if i >= len(route.Responses) {
		i = len(route.Responses) - 1
	}
	s.position[k]++
	if i < 0 {
		return Response{Status: http.StatusNoContent}
	}
	return route.Responses[i]
}

func (s *Server) drop(w http.ResponseWriter) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		s.t.Errorf("testserver: the ResponseWriter does not allow dropping the connection")
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		s.t.Errorf("testserver: drop the connection: %v", err)
		return
	}
	if err := conn.Close(); err != nil {
		s.t.Logf("testserver: close the dropped connection: %v", err)
	}
}

func (s *Server) writeResponse(w http.ResponseWriter, resp Response) {
	var body []byte
	if resp.Fixture != "" {
		b, err := os.ReadFile(filepath.Join(s.testdataDir, resp.Fixture))
		if err != nil {
			s.t.Errorf("testserver: read fixture %s: %v", resp.Fixture, err)
			http.Error(w, "missing fixture", http.StatusInternalServerError)
			return
		}
		body = b
		if strings.HasSuffix(resp.Fixture, ".txt") {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
	}
	for k, v := range defaultHeaders {
		w.Header().Set(k, v)
	}
	for k, v := range statusHeaders[resp.Status] {
		w.Header().Set(k, v)
	}
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(resp.Status)
	if _, err := w.Write(body); err != nil {
		s.t.Logf("testserver: write response: %v", err)
	}
}
