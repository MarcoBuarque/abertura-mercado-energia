package abertura

import (
	"context"
	"errors"
	"net/http"
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
