package abertura

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLimitador_QuartaRequisicaoEspera(t *testing.T) {
	t.Parallel()

	l := novosLimitadores(RateLimits{Default: RateLimit{Requests: 3, Per: 300 * time.Millisecond}})
	ctx := context.Background()

	inicio := time.Now()
	for i := range 3 {
		if err := l.esperar(ctx, grupoGeral); err != nil {
			t.Fatalf("requisição %d: %v", i+1, err)
		}
	}
	if d := time.Since(inicio); d > 50*time.Millisecond {
		t.Errorf("a rajada de 3 esperou %v", d)
	}

	inicio = time.Now()
	if err := l.esperar(ctx, grupoGeral); err != nil {
		t.Fatalf("quarta requisição: %v", err)
	}
	if d := time.Since(inicio); d < 60*time.Millisecond {
		t.Errorf("a quarta requisição esperou só %v, devia esperar perto de 100 ms", d)
	}
}

func TestLimitador_CtxDuranteAEspera(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     func() (context.Context, context.CancelFunc)
		wantErr error
	}{
		{
			name: "prazo mais curto que a espera devolve DeadlineExceeded sem esperar",
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 30*time.Millisecond)
			},
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "cancelamento durante a espera devolve Canceled",
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancela := context.WithCancel(context.Background())
				time.AfterFunc(30*time.Millisecond, cancela)
				return ctx, cancela
			},
			wantErr: context.Canceled,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := novosLimitadores(RateLimits{Default: RateLimit{Requests: 1, Per: time.Hour}})
			if err := l.esperar(context.Background(), grupoGeral); err != nil {
				t.Fatal(err)
			}
			ctx, cancela := tc.ctx()
			defer cancela()

			inicio := time.Now()
			err := l.esperar(ctx, grupoGeral)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("erro = %v, quer %v", err, tc.wantErr)
			}
			if d := time.Since(inicio); d > 2*time.Second {
				t.Errorf("devolveu só depois de %v", d)
			}
		})
	}
}

func TestLimitador_BaldeDeMedicoes(t *testing.T) {
	t.Parallel()

	curto := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 30*time.Millisecond)
	}
	umaPorHora := RateLimit{Requests: 1, Per: time.Hour}
	tests := []struct {
		name         string
		limites      RateLimits
		wantMedicoes bool // a segunda medição passa depois de esgotar o geral
	}{
		{"produção: medições têm balde próprio", RateLimits{Default: umaPorHora, Medicoes: RateLimit{Requests: 5, Per: time.Hour}}, true},
		{"sandbox: Medicoes zero usa o balde geral", RateLimits{Default: umaPorHora}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := novosLimitadores(tc.limites)
			if err := l.esperar(context.Background(), grupoGeral); err != nil {
				t.Fatal(err)
			}
			ctx, cancela := curto()
			defer cancela()

			err := l.esperar(ctx, grupoMedicoes)
			if got := err == nil; got != tc.wantMedicoes {
				t.Errorf("medições passou = %v (%v), quer %v", got, err, tc.wantMedicoes)
			}
		})
	}
}

func TestLimitador_BaldeDeMedicoesSemGeral(t *testing.T) {
	t.Parallel()

	l := novosLimitadores(RateLimits{Medicoes: RateLimit{Requests: 1, Per: time.Hour}})
	ctx, cancela := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancela()

	if err := l.esperar(ctx, grupoGeral); err != nil {
		t.Errorf("o geral é ilimitado: %v", err)
	}
	if err := l.esperar(ctx, grupoMedicoes); err != nil {
		t.Fatalf("primeira medição: %v", err)
	}
	if err := l.esperar(ctx, grupoMedicoes); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("segunda medição = %v, quer DeadlineExceeded", err)
	}
}

func TestLimitador_ValorZeroDesliga(t *testing.T) {
	t.Parallel()

	l := novosLimitadores(RateLimits{})
	inicio := time.Now()
	for range 1000 {
		if err := l.esperar(context.Background(), grupoMedicoes); err != nil {
			t.Fatal(err)
		}
		if err := l.esperar(context.Background(), grupoGeral); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(inicio); d > time.Second {
		t.Errorf("limitador desligado esperou %v", d)
	}
}

func TestLimitador_RajadaNuncaPassaDe30(t *testing.T) {
	t.Parallel()

	l := novosLimitadores(RateLimits{Default: RateLimit{Requests: 900, Per: 5 * time.Minute}})
	ctx, cancela := context.WithTimeout(context.Background(), time.Second)
	defer cancela()

	for i := range 30 {
		if err := l.esperar(ctx, grupoGeral); err != nil {
			t.Fatalf("requisição %d da rajada: %v", i+1, err)
		}
	}
	// A 31ª espera ~333 ms (5 min / 900), mais que o prazo de 50 ms.
	curto, cancela2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancela2()
	if err := l.esperar(curto, grupoGeral); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("31ª requisição = %v, quer DeadlineExceeded", err)
	}
}

// O pedido de token passa pelo balde geral: com 2 por hora, a primeira
// chamada gasta 1 no token e 1 na requisição, e a segunda já não cabe.
func TestLimitador_TokenGastaDoBaldeGeral(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t, resposta{status: 200, corpo: corpoOK}))
	c, _ := novoClienteTeste(t, srv, WithRateLimits(RateLimits{Default: RateLimit{Requests: 2, Per: time.Hour}}))

	if _, _, err := c.enviar(context.Background(), opLeitura(), callConfig{}); err != nil {
		t.Fatalf("primeira chamada: %v", err)
	}
	ctx, cancela := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancela()
	_, _, err := c.enviar(ctx, opLeitura(), callConfig{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("segunda chamada = %v, quer DeadlineExceeded", err)
	}
	if srv.tokens.Load() != 1 || srv.reqs.Load() != 1 {
		t.Errorf("tokens=%d reqs=%d, quer 1 e 1", srv.tokens.Load(), srv.reqs.Load())
	}
}

// Cada tentativa, inclusive retry, espera o limitador.
func TestLimitador_RetriesGastamDoBalde(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t, resposta{status: 502}))
	c, _ := novoClienteComEnv(t, srv.env(), fonteFixa("token-inventado-1"), WithAgent(agenteInventado, perfilInventado),
		WithRateLimits(RateLimits{Default: RateLimit{Requests: 2, Per: time.Hour}}))

	ctx, cancela := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancela()
	_, _, err := c.enviar(ctx, opLeitura(), callConfig{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("erro = %v, quer DeadlineExceeded na terceira tentativa", err)
	}
	if srv.reqs.Load() != 2 {
		t.Errorf("requisições = %d, quer 2", srv.reqs.Load())
	}
}
