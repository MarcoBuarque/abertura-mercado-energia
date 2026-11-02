package abertura

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fixed(v float64) func() float64 { return func() float64 { return v } }

func TestRetryWait_WhenFullJitter_ReturnsExpectedDelay(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		status     int
		retryAfter string
		retry      int
		rnd        float64
		want       time.Duration
	}{
		{"zero jitter waits zero", http.StatusBadGateway, "", 0, 0, 0},
		{"first 5xx retry, half of the 500 ms base", http.StatusBadGateway, "", 0, 0.5, 250 * time.Millisecond},
		{"second retry doubles the span", http.StatusBadGateway, "", 1, 0.5, 500 * time.Millisecond},
		{"fourth retry, span of 4 s", http.StatusServiceUnavailable, "", 3, 0.5, 2 * time.Second},
		{"8 s cap for 5xx", http.StatusBadGateway, "", 5, 1, 8 * time.Second},
		{"absurd retry does not overflow and stays at the cap", http.StatusBadGateway, "", 200, 1, 8 * time.Second},
		{"transport error uses the 5xx base", 0, "", 0, 1, 500 * time.Millisecond},
		{"429 starts at 2 s", http.StatusTooManyRequests, "", 0, 1, 2 * time.Second},
		{"429 doubles", http.StatusTooManyRequests, "", 2, 1, 8 * time.Second},
		{"429 has a 30 s cap", http.StatusTooManyRequests, "", 10, 1, 30 * time.Second},
		{"Retry-After in seconds beats the backoff", http.StatusTooManyRequests, "5", 0, 1, 5 * time.Second},
		{"Retry-After in seconds also applies to 5xx", http.StatusServiceUnavailable, "3", 0, 0, 3 * time.Second},
		{"Retry-After zero waits zero", http.StatusTooManyRequests, "0", 0, 1, 0},
		{"Retry-After has a 30 s cap", http.StatusTooManyRequests, "120", 0, 1, 30 * time.Second},
		{"Retry-After as HTTP date", http.StatusTooManyRequests, now.Add(10 * time.Second).UTC().Format(http.TimeFormat), 0, 1, 10 * time.Second},
		{"Retry-After as distant HTTP date is capped", http.StatusTooManyRequests, now.Add(time.Hour).UTC().Format(http.TimeFormat), 0, 1, 30 * time.Second},
		{"Retry-After as past date waits zero", http.StatusTooManyRequests, now.Add(-time.Minute).UTC().Format(http.TimeFormat), 0, 1, 0},
		{"Retry-After unreadable falls back to the backoff", http.StatusTooManyRequests, "soon", 0, 1, 2 * time.Second},
		{"Retry-After negative falls back to the backoff", http.StatusTooManyRequests, "-3", 0, 1, 2 * time.Second},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := retryWait(tc.status, tc.retryAfter, tc.retry, now, fixed(tc.rnd))
			if got != tc.want {
				t.Errorf("retryWait = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSleepCtx_WhenCtxEnds_ReturnsCtxError(t *testing.T) {
	t.Parallel()

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name    string
		ctx     context.Context
		d       time.Duration
		wantErr error
	}{
		{"zero duration returns immediately", context.Background(), 0, nil},
		{"short duration finishes", context.Background(), time.Millisecond, nil},
		{"already canceled ctx returns the ctx error", canceled, time.Hour, context.Canceled},
		{"already canceled ctx also applies to zero duration", canceled, 0, context.Canceled},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := sleepCtx(tc.ctx, tc.d); !errors.Is(err, tc.wantErr) {
				t.Errorf("sleepCtx = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestSleepCtx_WhenCtxEndsDuringWait_Interrupts(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := sleepCtx(ctx, time.Hour)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("sleepCtx = %v, want DeadlineExceeded", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("the wait was not interrupted by the ctx")
	}
}

// --- log ---

func newLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

func logRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("invalid log line: %v\n%s", err, buf.String())
		}
		out = append(out, m)
	}
	return out
}

func byLevel(rs []map[string]any, level string) []map[string]any {
	var out []map[string]any
	for _, r := range rs {
		if r["level"] == level {
			out = append(out, r)
		}
	}
	return out
}

// assertLogClean fails if the log contains anything the architecture (section 4.7) forbids.
func assertLogClean(t *testing.T, buf *bytes.Buffer, extras ...string) {
	t.Helper()

	forbidden := []string{
		fakeConsumerUnit, "codigoUnidadeConsumidora", "Bearer", "bearer", "fake-token", fakeSecret, fakeClientID,
		"client_secret", "client_id", "invented message", "access_token", "127.0.0.1", "?",
	}
	for _, p := range append(forbidden, extras...) {
		if strings.Contains(buf.String(), p) {
			t.Errorf("the log contains %q:\n%s", p, buf.String())
		}
	}
}

func TestLog_WhenRetry_WritesDebugPerAttemptAndWarn(t *testing.T) {
	t.Parallel()

	srv := newServer(t, sequence(t,
		stubResponse{status: 502, header: map[string]string{"x-amzn-RequestId": "req-fake-1"}, body: errorJSON("ERR_X")},
		stubResponse{status: 200, header: map[string]string{"x-amzn-RequestId": "req-fake-2"}, body: okBody},
	))
	logger, buf := newLogger()
	c, _ := newTestClient(t, srv, WithLogger(logger))

	if _, _, err := c.send(context.Background(), readOp(), callConfig{}); err != nil {
		t.Fatal(err)
	}

	rs := logRecords(t, buf)
	debug, warn := byLevel(rs, "DEBUG"), byLevel(rs, "WARN")
	if len(debug) != 2 || len(warn) != 1 {
		t.Fatalf("debug=%d warn=%d, want 2 and 1:\n%s", len(debug), len(warn), buf.String())
	}
	for i, d := range debug {
		for _, k := range []string{"op", "method", "route", "status", "duration_ms", "attempt", "request_id", "code"} {
			if _, ok := d[k]; !ok {
				t.Errorf("Debug %d missing field %q: %v", i+1, k, d)
			}
		}
	}
	if debug[0]["op"] != "listMigrations" || debug[0]["method"] != "GET" || debug[0]["route"] != "/v1.1/varejista/migracoes" {
		t.Errorf("Debug 1 = %v", debug[0])
	}
	if debug[0]["status"] != float64(502) || debug[0]["attempt"] != float64(1) || debug[0]["request_id"] != "req-fake-1" || debug[0]["code"] != "ERR_X" {
		t.Errorf("Debug 1 = %v", debug[0])
	}
	if debug[1]["status"] != float64(200) || debug[1]["attempt"] != float64(2) || debug[1]["request_id"] != "req-fake-2" {
		t.Errorf("Debug 2 = %v", debug[1])
	}
	if warn[0]["msg"] != "retry scheduled" || warn[0]["wait_ms"] != float64(500) {
		t.Errorf("Warn = %v", warn[0])
	}
	assertLogClean(t, buf)
}

func TestLog_WhenRefreshOrUncertain_WritesWarn(t *testing.T) {
	t.Parallel()

	authorizer := stubResponse{status: 403, header: gateway(map[string]string{"x-amzn-RequestId": "req-fake-3"}), body: errorJSON("ERR_CREDENCIAL_INVALIDA")}
	ok := stubResponse{status: 200, body: okBody}
	tests := []struct {
		name      string
		responses []stubResponse
		flow      func(c *Client) error
		wantMsgs  []string
	}{
		{
			name:      "token refreshed by the 403 rule",
			responses: []stubResponse{ok, authorizer, ok},
			flow: func(c *Client) error {
				if _, _, err := c.send(context.Background(), readOp(), callConfig{}); err != nil {
					return err
				}
				_, _, err := c.send(context.Background(), readOp(), callConfig{})
				return err
			},
			wantMsgs: []string{"token refreshed"},
		},
		{
			name:      "uncertain result on POST 502",
			responses: []stubResponse{{status: 502, header: map[string]string{"x-amzn-RequestId": "req-fake-4"}}},
			flow: func(c *Client) error {
				_, _, err := c.send(context.Background(), writeOp(http.MethodPost), callConfig{})
				return err
			},
			wantMsgs: []string{"uncertain result"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := newServer(t, sequence(t, tc.responses...))
			logger, buf := newLogger()
			c, _ := newTestClient(t, srv, WithLogger(logger))

			err := tc.flow(c)
			var uncertain *UncertainResultError
			if err != nil && !errors.As(err, &uncertain) {
				t.Fatal(err)
			}
			var msgs []string
			for _, w := range byLevel(logRecords(t, buf), "WARN") {
				msgs = append(msgs, fmt.Sprint(w["msg"]))
				for _, k := range []string{"op", "method", "route", "status"} {
					if _, ok := w[k]; !ok {
						t.Errorf("Warn %v missing field %q", w["msg"], k)
					}
				}
			}
			if strings.Join(msgs, "|") != strings.Join(tc.wantMsgs, "|") {
				t.Errorf("Warn = %v, want %v\n%s", msgs, tc.wantMsgs, buf.String())
			}
			assertLogClean(t, buf)
		})
	}
}

func TestLog_WhenTransportFailure_DoesNotLeakURL(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	env := Environment{APIURL: srv.URL, TokenURL: srv.URL + "/sso/oauth/token"}
	srv.Close()
	logger, buf := newLogger()
	c, _ := newClientWithEnv(t, env, fixedSource("fake-token-1"), WithAgent(fakeAgent, fakeProfile),
		WithRateLimits(RateLimits{}), WithLogger(logger))

	if _, _, err := c.send(context.Background(), readOp(), callConfig{}); err == nil {
		t.Fatal("expected an error")
	}
	debug := byLevel(logRecords(t, buf), "DEBUG")
	if len(debug) != 3 {
		t.Fatalf("Debug = %d, want 3", len(debug))
	}
	if debug[0]["status"] != float64(0) || debug[0]["failure"] != "transport" {
		t.Errorf("Debug = %v", debug[0])
	}
	assertLogClean(t, buf, srv.URL)
}

func TestLog_WhenNoLogger_WritesNothing(t *testing.T) {
	// Not parallel: it swaps the process default logger.
	buf := &bytes.Buffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	srv := newServer(t, sequence(t, stubResponse{status: 502}, stubResponse{status: 200, body: okBody}))
	c, _ := newTestClient(t, srv)
	if _, _, err := c.send(context.Background(), readOp(), callConfig{}); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("without WithLogger the SDK wrote to the default log: %s", buf.String())
	}
}

func TestLog_WhenCalled_DoesNotLogBodyOrQuery(t *testing.T) {
	t.Parallel()

	srv := newServer(t, sequence(t, stubResponse{status: 400, body: errorJSON("ERR_VALIDACAO_CAMPOS")}))
	logger, buf := newLogger()
	c, _ := newTestClient(t, srv, WithLogger(logger))

	for _, op := range []operation{readOp(), writeOp(http.MethodPost)} {
		if _, _, err := c.send(context.Background(), op, callConfig{}); err == nil {
			t.Fatal("expected an error")
		}
	}
	if len(byLevel(logRecords(t, buf), "DEBUG")) != 2 {
		t.Errorf("expected 2 Debug lines:\n%s", buf.String())
	}
	assertLogClean(t, buf)
}
