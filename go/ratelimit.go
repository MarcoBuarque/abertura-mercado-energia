package abertura

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// grupoLimite escolhe o balde do limitador local.
type grupoLimite int

const (
	// grupoGeral vale para tudo, menos medições.
	grupoGeral grupoLimite = iota
	// grupoMedicoes é o balde de medições (limite maior em produção).
	grupoMedicoes
)

// rajadaMaxima é o tamanho da rajada dos dois ambientes; um limite menor que
// isso (3 por 300 ms, nos testes) tem rajada do próprio tamanho.
const rajadaMaxima = 30

// limitadores guarda um balde por grupo. Balde nil não limita.
//
// O limite é por processo: vários processos com o mesmo IP ou agente somam
// requisições na API sem que este limitador saiba.
type limitadores struct {
	geral, medicoes *rate.Limiter
}

// novoBalde cria o balde de um RateLimit; o valor zero (ou incompleto) é um
// balde nil, que não limita.
func novoBalde(l RateLimit) *rate.Limiter {
	if l.Requests <= 0 || l.Per <= 0 {
		return nil
	}
	return rate.NewLimiter(rate.Every(l.Per/time.Duration(l.Requests)), min(rajadaMaxima, l.Requests))
}

// novosLimitadores monta os baldes. Medicoes zero usa o balde do Default
// (sandbox).
func novosLimitadores(rl RateLimits) limitadores {
	l := limitadores{geral: novoBalde(rl.Default)}
	l.medicoes = l.geral
	if rl.Medicoes != (RateLimit{}) {
		l.medicoes = novoBalde(rl.Medicoes)
	}
	return l
}

// esperar bloqueia até o grupo poder enviar uma requisição, ou até o ctx
// terminar. Um prazo mais curto que a espera devolve DeadlineExceeded na hora,
// sem esperar à toa.
func (l limitadores) esperar(ctx context.Context, g grupoLimite) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	balde := l.geral
	if g == grupoMedicoes {
		balde = l.medicoes
	}
	if balde == nil {
		return nil
	}
	if err := balde.Wait(ctx); err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		// Wait recusa de antemão uma espera que passaria do prazo do ctx.
		return fmt.Errorf("limitador local: %w", context.DeadlineExceeded)
	}
	return nil
}

// transporteLimitado faz cada requisição do cliente de token esperar o balde
// geral, porque o pedido de token também conta no limite da API.
type transporteLimitado struct {
	base    http.RoundTripper
	limites limitadores
}

func (t transporteLimitado) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.limites.esperar(req.Context(), grupoGeral); err != nil {
		// O contrato do http.RoundTripper manda fechar o corpo também no erro.
		if req.Body != nil {
			if cerr := req.Body.Close(); cerr != nil {
				err = errors.Join(err, cerr)
			}
		}
		return nil, err
	}
	return t.base.RoundTrip(req)
}

// clienteDeToken devolve uma cópia do cliente HTTP cujo transporte espera o
// limitador. A cópia nunca segue redirecionamento, também com um cliente de
// WithHTTPClient: o formulário do token leva o client_secret, e um 307 o
// reenviaria a outro host. O CheckRedirect do chamador vale só para as
// chamadas à API, não para o pedido de token.
func clienteDeToken(hc *http.Client, l limitadores) *http.Client {
	var cp http.Client
	if hc != nil {
		cp = *hc
	}
	cp.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	base := cp.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	cp.Transport = transporteLimitado{base: base, limites: l}
	return &cp
}
