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

func fixo(v float64) func() float64 { return func() float64 { return v } }

func TestEspera_BackoffComJitterCheio(t *testing.T) {
	t.Parallel()

	agora := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		status     int
		retryAfter string
		retry      int
		rnd        float64
		want       time.Duration
	}{
		{"jitter zero espera zero", http.StatusBadGateway, "", 0, 0, 0},
		{"primeiro retry de 5xx, metade da base de 500 ms", http.StatusBadGateway, "", 0, 0.5, 250 * time.Millisecond},
		{"segundo retry dobra a faixa", http.StatusBadGateway, "", 1, 0.5, 500 * time.Millisecond},
		{"quarto retry, faixa de 4 s", http.StatusServiceUnavailable, "", 3, 0.5, 2 * time.Second},
		{"teto de 8 s para 5xx", http.StatusBadGateway, "", 5, 1, 8 * time.Second},
		{"retry absurdo não estoura e fica no teto", http.StatusBadGateway, "", 200, 1, 8 * time.Second},
		{"erro de transporte usa a base de 5xx", 0, "", 0, 1, 500 * time.Millisecond},
		{"429 começa em 2 s", http.StatusTooManyRequests, "", 0, 1, 2 * time.Second},
		{"429 dobra", http.StatusTooManyRequests, "", 2, 1, 8 * time.Second},
		{"429 tem teto de 30 s", http.StatusTooManyRequests, "", 10, 1, 30 * time.Second},
		{"Retry-After em segundos ganha do backoff", http.StatusTooManyRequests, "5", 0, 1, 5 * time.Second},
		{"Retry-After em segundos vale também em 5xx", http.StatusServiceUnavailable, "3", 0, 0, 3 * time.Second},
		{"Retry-After zero espera zero", http.StatusTooManyRequests, "0", 0, 1, 0},
		{"Retry-After tem teto de 30 s", http.StatusTooManyRequests, "120", 0, 1, 30 * time.Second},
		{"Retry-After em data HTTP", http.StatusTooManyRequests, agora.Add(10 * time.Second).UTC().Format(http.TimeFormat), 0, 1, 10 * time.Second},
		{"Retry-After em data HTTP distante tem teto", http.StatusTooManyRequests, agora.Add(time.Hour).UTC().Format(http.TimeFormat), 0, 1, 30 * time.Second},
		{"Retry-After em data passada espera zero", http.StatusTooManyRequests, agora.Add(-time.Minute).UTC().Format(http.TimeFormat), 0, 1, 0},
		{"Retry-After ilegível cai no backoff", http.StatusTooManyRequests, "logo", 0, 1, 2 * time.Second},
		{"Retry-After negativo cai no backoff", http.StatusTooManyRequests, "-3", 0, 1, 2 * time.Second},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := esperaRetry(tc.status, tc.retryAfter, tc.retry, agora, fixo(tc.rnd))
			if got != tc.want {
				t.Errorf("esperaRetry = %v, quer %v", got, tc.want)
			}
		})
	}
}

func TestEspera_DormirRespeitaOCtx(t *testing.T) {
	t.Parallel()

	cancelado, cancela := context.WithCancel(context.Background())
	cancela()

	tests := []struct {
		name    string
		ctx     context.Context
		d       time.Duration
		wantErr error
	}{
		{"duração zero volta na hora", context.Background(), 0, nil},
		{"duração curta termina", context.Background(), time.Millisecond, nil},
		{"ctx já cancelado devolve o erro do ctx", cancelado, time.Hour, context.Canceled},
		{"ctx já cancelado vale também para duração zero", cancelado, 0, context.Canceled},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := dormirCtx(tc.ctx, tc.d); !errors.Is(err, tc.wantErr) {
				t.Errorf("dormirCtx = %v, quer %v", err, tc.wantErr)
			}
		})
	}
}

func TestEspera_InterrompidaPeloCtxDuranteAEspera(t *testing.T) {
	t.Parallel()

	ctx, cancela := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancela()

	inicio := time.Now()
	err := dormirCtx(ctx, time.Hour)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("dormirCtx = %v, quer DeadlineExceeded", err)
	}
	if time.Since(inicio) > 5*time.Second {
		t.Errorf("a espera não foi interrompida pelo ctx")
	}
}

// --- log ---

func novoLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

func registros(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("linha de log inválida: %v\n%s", err, buf.String())
		}
		out = append(out, m)
	}
	return out
}

func porNivel(rs []map[string]any, nivel string) []map[string]any {
	var out []map[string]any
	for _, r := range rs {
		if r["level"] == nivel {
			out = append(out, r)
		}
	}
	return out
}

// assertLogLimpo falha se o log traz algo que a arquitetura (§4.7) proíbe.
func assertLogLimpo(t *testing.T, buf *bytes.Buffer, extras ...string) {
	t.Helper()

	proibidos := []string{
		ucInventada, "codigoUnidadeConsumidora", "Bearer", "bearer", "token-inventado", segredoInventado, idInventado,
		"client_secret", "client_id", "mensagem inventada", "access_token", "127.0.0.1", "?",
	}
	for _, p := range append(proibidos, extras...) {
		if strings.Contains(buf.String(), p) {
			t.Errorf("o log contém %q:\n%s", p, buf.String())
		}
	}
}

func TestLog_DebugPorTentativaEWarnNoRetry(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t,
		resposta{status: 502, header: map[string]string{"x-amzn-RequestId": "req-inventado-1"}, corpo: erroJSON("ERR_X")},
		resposta{status: 200, header: map[string]string{"x-amzn-RequestId": "req-inventado-2"}, corpo: corpoOK},
	))
	logger, buf := novoLogger()
	c, _ := novoClienteTeste(t, srv, WithLogger(logger))

	if _, _, err := c.enviar(context.Background(), opLeitura(), callConfig{}); err != nil {
		t.Fatal(err)
	}

	rs := registros(t, buf)
	debug, warn := porNivel(rs, "DEBUG"), porNivel(rs, "WARN")
	if len(debug) != 2 || len(warn) != 1 {
		t.Fatalf("debug=%d warn=%d, quer 2 e 1:\n%s", len(debug), len(warn), buf.String())
	}
	for i, d := range debug {
		for _, k := range []string{"op", "metodo", "rota", "status", "duracao_ms", "tentativa", "request_id", "codigo"} {
			if _, ok := d[k]; !ok {
				t.Errorf("Debug %d sem o campo %q: %v", i+1, k, d)
			}
		}
	}
	if debug[0]["op"] != "listarMigracoes" || debug[0]["metodo"] != "GET" || debug[0]["rota"] != "/v1.1/varejista/migracoes" {
		t.Errorf("Debug 1 = %v", debug[0])
	}
	if debug[0]["status"] != float64(502) || debug[0]["tentativa"] != float64(1) || debug[0]["request_id"] != "req-inventado-1" || debug[0]["codigo"] != "ERR_X" {
		t.Errorf("Debug 1 = %v", debug[0])
	}
	if debug[1]["status"] != float64(200) || debug[1]["tentativa"] != float64(2) || debug[1]["request_id"] != "req-inventado-2" {
		t.Errorf("Debug 2 = %v", debug[1])
	}
	if warn[0]["msg"] != "retry agendado" || warn[0]["espera_ms"] != float64(500) {
		t.Errorf("Warn = %v", warn[0])
	}
	assertLogLimpo(t, buf)
}

func TestLog_WarnNaRenovacaoEmIncertoENaFalha(t *testing.T) {
	t.Parallel()

	autorizador := resposta{status: 403, header: gateway(map[string]string{"x-amzn-RequestId": "req-inventado-3"}), corpo: erroJSON("ERR_CREDENCIAL_INVALIDA")}
	ok := resposta{status: 200, corpo: corpoOK}
	tests := []struct {
		name      string
		respostas []resposta
		fluxo     func(c *Client) error
		wantMsgs  []string
	}{
		{
			name:      "token renovado pela regra do 403",
			respostas: []resposta{ok, autorizador, ok},
			fluxo: func(c *Client) error {
				if _, _, err := c.enviar(context.Background(), opLeitura(), callConfig{}); err != nil {
					return err
				}
				_, _, err := c.enviar(context.Background(), opLeitura(), callConfig{})
				return err
			},
			wantMsgs: []string{"token renovado"},
		},
		{
			name:      "resultado incerto em POST 502",
			respostas: []resposta{{status: 502, header: map[string]string{"x-amzn-RequestId": "req-inventado-4"}}},
			fluxo: func(c *Client) error {
				_, _, err := c.enviar(context.Background(), opEscrita(http.MethodPost), callConfig{})
				return err
			},
			wantMsgs: []string{"resultado incerto"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := novoServidor(t, sequencia(t, tc.respostas...))
			logger, buf := novoLogger()
			c, _ := novoClienteTeste(t, srv, WithLogger(logger))

			err := tc.fluxo(c)
			var incerto *UncertainResultError
			if err != nil && !errors.As(err, &incerto) {
				t.Fatal(err)
			}
			var msgs []string
			for _, w := range porNivel(registros(t, buf), "WARN") {
				msgs = append(msgs, fmt.Sprint(w["msg"]))
				for _, k := range []string{"op", "metodo", "rota", "status"} {
					if _, ok := w[k]; !ok {
						t.Errorf("Warn %v sem o campo %q", w["msg"], k)
					}
				}
			}
			if strings.Join(msgs, "|") != strings.Join(tc.wantMsgs, "|") {
				t.Errorf("Warn = %v, quer %v\n%s", msgs, tc.wantMsgs, buf.String())
			}
			assertLogLimpo(t, buf)
		})
	}
}

func TestLog_FalhaDeTransporteNaoVazaAURL(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	env := Environment{APIURL: srv.URL, TokenURL: srv.URL + "/sso/oauth/token"}
	srv.Close()
	logger, buf := novoLogger()
	c, _ := novoClienteComEnv(t, env, fonteFixa("token-inventado-1"), WithAgent(agenteInventado, perfilInventado),
		WithRateLimits(RateLimits{}), WithLogger(logger))

	if _, _, err := c.enviar(context.Background(), opLeitura(), callConfig{}); err == nil {
		t.Fatal("esperava erro")
	}
	debug := porNivel(registros(t, buf), "DEBUG")
	if len(debug) != 3 {
		t.Fatalf("Debug = %d, quer 3", len(debug))
	}
	if debug[0]["status"] != float64(0) || debug[0]["falha"] != "transporte" {
		t.Errorf("Debug = %v", debug[0])
	}
	assertLogLimpo(t, buf, srv.URL)
}

func TestLog_SemLoggerNadaEscrito(t *testing.T) {
	// Não paralelo: troca o logger padrão do processo.
	buf := &bytes.Buffer{}
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(anterior) })

	srv := novoServidor(t, sequencia(t, resposta{status: 502}, resposta{status: 200, corpo: corpoOK}))
	c, _ := novoClienteTeste(t, srv)
	if _, _, err := c.enviar(context.Background(), opLeitura(), callConfig{}); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("sem WithLogger o SDK escreveu no log padrão: %s", buf.String())
	}
}

func TestLog_NaoLogaOCorpoNemAQuery(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t, resposta{status: 400, corpo: erroJSON("ERR_VALIDACAO_CAMPOS")}))
	logger, buf := novoLogger()
	c, _ := novoClienteTeste(t, srv, WithLogger(logger))

	for _, op := range []operacao{opLeitura(), opEscrita(http.MethodPost)} {
		if _, _, err := c.enviar(context.Background(), op, callConfig{}); err == nil {
			t.Fatal("esperava erro")
		}
	}
	if len(porNivel(registros(t, buf), "DEBUG")) != 2 {
		t.Errorf("esperava 2 Debug:\n%s", buf.String())
	}
	assertLogLimpo(t, buf)
}
