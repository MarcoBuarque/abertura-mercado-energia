package abertura

import (
	"net/http"
	"testing"
)

// base devolve um resultado "limpo" com orçamento de retries; cada caso da
// tabela altera só o que importa.
func base(alt func(*desfecho)) desfecho {
	r := desfecho{maxRetries: 2, renovavel: true}
	if alt != nil {
		alt(&r)
	}
	return r
}

// TestDecidir_TabelaDeRetries tem uma linha por linha da tabela 4.4 da
// arquitetura, para GET, POST e PUT. A coluna "linha" aponta a situação da
// tabela que o caso cobre.
func TestDecidir_TabelaDeRetries(t *testing.T) {
	t.Parallel()

	const (
		leitura  = "GET"
		escritaA = "POST"
		escritaB = "PUT"
	)
	tests := []struct {
		name   string
		linha  string
		metodo string
		r      desfecho
		want   acao
	}{
		// Erro antes de escrever: repete em todos os métodos.
		{"GET erro antes de escrever repete", "erro antes de escrever", leitura, base(func(r *desfecho) { r.falha = falhaTransporte }), acaoRepetir},
		{"POST erro antes de escrever repete", "erro antes de escrever", escritaA, base(func(r *desfecho) { r.falha = falhaTransporte }), acaoRepetir},
		{"PUT erro antes de escrever repete", "erro antes de escrever", escritaB, base(func(r *desfecho) { r.falha = falhaTransporte }), acaoRepetir},
		{"POST erro antes de escrever sem retries é erro", "erro antes de escrever", escritaA, base(func(r *desfecho) { r.falha = falhaTransporte; r.retries = 2 }), acaoErro},

		// Erro de transporte depois de escrever.
		{"GET erro depois de escrever repete", "erro depois de escrever", leitura, base(func(r *desfecho) { r.falha = falhaTransporte; r.escreveu = true }), acaoRepetir},
		{"POST erro depois de escrever é incerto", "erro depois de escrever", escritaA, base(func(r *desfecho) { r.falha = falhaTransporte; r.escreveu = true }), acaoIncerto},
		{"PUT erro depois de escrever é incerto", "erro depois de escrever", escritaB, base(func(r *desfecho) { r.falha = falhaTransporte; r.escreveu = true }), acaoIncerto},
		{"GET erro depois de escrever sem retries é erro", "fim dos retries", leitura, base(func(r *desfecho) { r.falha = falhaTransporte; r.escreveu = true; r.retries = 2 }), acaoErro},

		// Timeout da tentativa.
		{"GET timeout depois de escrever repete", "timeout da tentativa", leitura, base(func(r *desfecho) { r.falha = falhaTimeout; r.escreveu = true }), acaoRepetir},
		{"POST timeout depois de escrever é incerto", "timeout da tentativa", escritaA, base(func(r *desfecho) { r.falha = falhaTimeout; r.escreveu = true }), acaoIncerto},
		{"PUT timeout depois de escrever é incerto", "timeout da tentativa", escritaB, base(func(r *desfecho) { r.falha = falhaTimeout; r.escreveu = true }), acaoIncerto},
		{"POST timeout antes de escrever repete", "timeout da tentativa", escritaA, base(func(r *desfecho) { r.falha = falhaTimeout }), acaoRepetir},
		{"GET timeout sem retries é erro", "fim dos retries", leitura, base(func(r *desfecho) { r.falha = falhaTimeout; r.escreveu = true; r.retries = 2 }), acaoErro},

		// Contexto do chamador cancelado ou vencido.
		{"GET contexto antes de escrever para", "contexto do chamador", leitura, base(func(r *desfecho) { r.falha = falhaContexto }), acaoErro},
		{"GET contexto depois de escrever para", "contexto do chamador", leitura, base(func(r *desfecho) { r.falha = falhaContexto; r.escreveu = true }), acaoErro},
		{"POST contexto antes de escrever é erro", "contexto do chamador", escritaA, base(func(r *desfecho) { r.falha = falhaContexto }), acaoErro},
		{"POST contexto depois de escrever é incerto", "contexto do chamador", escritaA, base(func(r *desfecho) { r.falha = falhaContexto; r.escreveu = true }), acaoIncerto},
		{"PUT contexto depois de escrever é incerto", "contexto do chamador", escritaB, base(func(r *desfecho) { r.falha = falhaContexto; r.escreveu = true }), acaoIncerto},

		// 2xx.
		{"GET 200 é sucesso", "2xx", leitura, base(func(r *desfecho) { r.status = 200 }), acaoSucesso},
		{"POST 201 é sucesso", "2xx", escritaA, base(func(r *desfecho) { r.status = 201 }), acaoSucesso},
		{"PUT 204 é sucesso", "2xx", escritaB, base(func(r *desfecho) { r.status = 204 }), acaoSucesso},
		{"POST 200 ilegível é incerto", "2xx com corpo ilegível", escritaA, base(func(r *desfecho) { r.status = 200; r.corpoIlegivel = true }), acaoIncerto},
		{"PUT 200 ilegível é incerto", "2xx com corpo ilegível", escritaB, base(func(r *desfecho) { r.status = 200; r.corpoIlegivel = true }), acaoIncerto},
		{"GET 200 ilegível repete", "2xx com corpo ilegível", leitura, base(func(r *desfecho) { r.status = 200; r.corpoIlegivel = true }), acaoRepetir},
		{"GET 200 ilegível sem retries é erro", "fim dos retries", leitura, base(func(r *desfecho) { r.status = 200; r.corpoIlegivel = true; r.retries = 2 }), acaoErro},

		// Outros 4xx.
		{"GET 400 não repete", "400, 404, outros 4xx", leitura, base(func(r *desfecho) { r.status = 400 }), acaoErro},
		{"POST 400 não repete", "400, 404, outros 4xx", escritaA, base(func(r *desfecho) { r.status = 400 }), acaoErro},
		{"PUT 404 não repete", "400, 404, outros 4xx", escritaB, base(func(r *desfecho) { r.status = 404 }), acaoErro},
		{"GET 404 não repete", "400, 404, outros 4xx", leitura, base(func(r *desfecho) { r.status = 404 }), acaoErro},
		{"POST 422 não repete", "400, 404, outros 4xx", escritaA, base(func(r *desfecho) { r.status = 422 }), acaoErro},

		// 403 fora da regra 4.2.
		{"GET 403 do backend não repete", "403 fora da regra 4.2", leitura, base(func(r *desfecho) { r.status = 403; r.credencialInvalida = true; r.tokenAceito = true }), acaoErro},
		{"POST 403 do backend não repete", "403 fora da regra 4.2", escritaA, base(func(r *desfecho) { r.status = 403; r.credencialInvalida = true; r.tokenAceito = true }), acaoErro},
		{"GET 403 do autorizador com token nunca aceito não repete", "403 fora da regra 4.2", leitura, base(func(r *desfecho) { r.status = 403; r.credencialInvalida = true; r.gateway = true }), acaoErro},
		{"POST 403 do autorizador com token nunca aceito não repete", "403 fora da regra 4.2", escritaA, base(func(r *desfecho) { r.status = 403; r.credencialInvalida = true; r.gateway = true }), acaoErro},
		{"GET 403 do autorizador sem código ERR não repete", "403 fora da regra 4.2", leitura, base(func(r *desfecho) { r.status = 403; r.gateway = true; r.tokenAceito = true }), acaoErro},
		{"GET 403 sem código ERR do backend não repete", "403 fora da regra 4.2", leitura, base(func(r *desfecho) { r.status = 403 }), acaoErro},
		{"GET 403 com WithTokenSource não repete", "403 fora da regra 4.2", leitura, base(func(r *desfecho) {
			r.status = 403
			r.credencialInvalida = true
			r.gateway = true
			r.tokenAceito = true
			r.renovavel = false
		}), acaoErro},
		{"GET 403 já renovado uma vez não repete", "403 fora da regra 4.2", leitura, base(func(r *desfecho) {
			r.status = 403
			r.credencialInvalida = true
			r.gateway = true
			r.tokenAceito = true
			r.jaRenovou = true
		}), acaoErro},

		// 403 dentro da regra 4.2.
		{"GET 403 do autorizador com token aceito renova", "403 dentro da regra 4.2", leitura, base(func(r *desfecho) {
			r.status = 403
			r.credencialInvalida = true
			r.gateway = true
			r.tokenAceito = true
		}), acaoRenovarRepetir},
		{"POST 403 do autorizador com token aceito renova", "403 dentro da regra 4.2", escritaA, base(func(r *desfecho) {
			r.status = 403
			r.credencialInvalida = true
			r.gateway = true
			r.tokenAceito = true
		}), acaoRenovarRepetir},
		{"PUT 403 do autorizador com token aceito renova", "403 dentro da regra 4.2", escritaB, base(func(r *desfecho) {
			r.status = 403
			r.credencialInvalida = true
			r.gateway = true
			r.tokenAceito = true
		}), acaoRenovarRepetir},
		{"GET 403 renova mesmo com retries esgotados", "403 dentro da regra 4.2", leitura, base(func(r *desfecho) {
			r.status = 403
			r.credencialInvalida = true
			r.gateway = true
			r.tokenAceito = true
			r.retries = 2
		}), acaoRenovarRepetir},

		// 429.
		{"GET 429 sem marca repete", "429", leitura, base(func(r *desfecho) { r.status = 429 }), acaoRepetir},
		{"GET 429 com marca repete", "429", leitura, base(func(r *desfecho) { r.status = 429; r.gateway = true }), acaoRepetir},
		{"POST 429 com marca repete", "429", escritaA, base(func(r *desfecho) { r.status = 429; r.gateway = true }), acaoRepetir},
		{"PUT 429 com marca repete", "429", escritaB, base(func(r *desfecho) { r.status = 429; r.gateway = true }), acaoRepetir},
		{"POST 429 sem marca é erro", "429", escritaA, base(func(r *desfecho) { r.status = 429 }), acaoErro},
		{"PUT 429 sem marca é erro", "429", escritaB, base(func(r *desfecho) { r.status = 429 }), acaoErro},
		{"GET 429 sem retries é erro", "fim dos retries", leitura, base(func(r *desfecho) { r.status = 429; r.retries = 2 }), acaoErro},
		{"POST 429 com marca sem retries é erro", "fim dos retries", escritaA, base(func(r *desfecho) { r.status = 429; r.gateway = true; r.retries = 2 }), acaoErro},

		// 5xx.
		{"GET 500 repete", "500 a 599, menos 501", leitura, base(func(r *desfecho) { r.status = 500 }), acaoRepetir},
		{"GET 502 repete", "500 a 599, menos 501", leitura, base(func(r *desfecho) { r.status = 502 }), acaoRepetir},
		{"GET 503 repete", "500 a 599, menos 501", leitura, base(func(r *desfecho) { r.status = 503 }), acaoRepetir},
		{"GET 504 repete", "500 a 599, menos 501", leitura, base(func(r *desfecho) { r.status = 504 }), acaoRepetir},
		{"POST 500 é incerto", "500 a 599, menos 501", escritaA, base(func(r *desfecho) { r.status = 500 }), acaoIncerto},
		{"POST 502 é incerto", "500 a 599, menos 501", escritaA, base(func(r *desfecho) { r.status = 502 }), acaoIncerto},
		{"POST 503 é incerto", "500 a 599, menos 501", escritaA, base(func(r *desfecho) { r.status = 503 }), acaoIncerto},
		{"POST 504 é incerto", "500 a 599, menos 501", escritaA, base(func(r *desfecho) { r.status = 504 }), acaoIncerto},
		{"PUT 502 é incerto", "500 a 599, menos 501", escritaB, base(func(r *desfecho) { r.status = 502 }), acaoIncerto},
		{"PUT 504 é incerto", "500 a 599, menos 501", escritaB, base(func(r *desfecho) { r.status = 504 }), acaoIncerto},
		{"GET 502 sem retries é erro", "fim dos retries", leitura, base(func(r *desfecho) { r.status = 502; r.retries = 2 }), acaoErro},
		{"GET 500 com retries desligados é erro", "fim dos retries", leitura, base(func(r *desfecho) { r.status = 500; r.maxRetries = 0 }), acaoErro},
		{"GET 507 repete", "outros 5xx exceto 501", leitura, base(func(r *desfecho) { r.status = 507 }), acaoRepetir},
		{"GET 599 repete", "outros 5xx exceto 501", leitura, base(func(r *desfecho) { r.status = 599 }), acaoRepetir},
		{"POST 507 é incerto", "outros 5xx exceto 501", escritaA, base(func(r *desfecho) { r.status = 507 }), acaoIncerto},
		{"POST 520 é incerto", "outros 5xx exceto 501", escritaA, base(func(r *desfecho) { r.status = 520 }), acaoIncerto},
		{"PUT 599 é incerto", "outros 5xx exceto 501", escritaB, base(func(r *desfecho) { r.status = 599 }), acaoIncerto},
		{"GET 507 sem retries é erro", "fim dos retries", leitura, base(func(r *desfecho) { r.status = 507; r.retries = 2 }), acaoErro},
		{"GET 600 não repete", "fora de 5xx", leitura, base(func(r *desfecho) { r.status = 600 }), acaoErro},
		{"POST 600 não é incerto", "fora de 5xx", escritaA, base(func(r *desfecho) { r.status = 600 }), acaoErro},
		{"GET 501 não repete", "501 é definitivo", leitura, base(func(r *desfecho) { r.status = 501 }), acaoErro},
		{"POST 501 não repete", "501 é definitivo", escritaA, base(func(r *desfecho) { r.status = 501 }), acaoErro},
		{"GET 302 não repete", "outros status (interpretação)", leitura, base(func(r *desfecho) { r.status = http.StatusFound }), acaoErro},
		{"PATCH 502 é incerto", "escrita genérica (interpretação)", "PATCH", base(func(r *desfecho) { r.status = 502 }), acaoIncerto},
		{"HEAD 502 repete", "leitura genérica (interpretação)", "HEAD", base(func(r *desfecho) { r.status = 502 }), acaoRepetir},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := decidir(tc.metodo, tc.r); got != tc.want {
				t.Errorf("decidir(%s, %+v) [%s] = %v, quer %v", tc.metodo, tc.r, tc.linha, got, tc.want)
			}
		})
	}
}

func TestDecidir_AcaoTemNome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a    acao
		want string
	}{
		{acaoSucesso, "sucesso"},
		{acaoRepetir, "repetir"},
		{acaoRenovarRepetir, "renovar e repetir"},
		{acaoErro, "erro"},
		{acaoIncerto, "incerto"},
		{acao(0), "desconhecida"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			if got := tc.a.String(); got != tc.want {
				t.Errorf("String() = %q, quer %q", got, tc.want)
			}
		})
	}
}

func TestLog_NomeDaFalha(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		d    desfecho
		want string
	}{
		{"sem falha", desfecho{}, ""},
		{"transporte", desfecho{falha: falhaTransporte}, "transporte"},
		{"timeout", desfecho{falha: falhaTimeout}, "timeout"},
		{"contexto", desfecho{falha: falhaContexto}, "contexto"},
		{"corpo ilegível", desfecho{corpoIlegivel: true}, "corpo"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := nomeFalha(tc.d); got != tc.want {
				t.Errorf("nomeFalha = %q, quer %q", got, tc.want)
			}
		})
	}
}
