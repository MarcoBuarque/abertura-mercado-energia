package abertura

import "net/http"

// acao é o que o laço de envio faz depois de uma tentativa.
type acao int

const (
	// acaoSucesso devolve a resposta ao chamador.
	acaoSucesso acao = iota + 1
	// acaoRepetir espera e envia de novo, gastando um retry.
	acaoRepetir
	// acaoRenovarRepetir descarta o token, busca outro e envia de novo, sem
	// gastar retry. Vale uma vez por chamada.
	acaoRenovarRepetir
	// acaoErro devolve o erro da tentativa.
	acaoErro
	// acaoIncerto devolve *UncertainResultError: a escrita pode ter sido
	// aplicada.
	acaoIncerto
)

func (a acao) String() string {
	switch a {
	case acaoSucesso:
		return "sucesso"
	case acaoRepetir:
		return "repetir"
	case acaoRenovarRepetir:
		return "renovar e repetir"
	case acaoErro:
		return "erro"
	case acaoIncerto:
		return "incerto"
	default:
		return "desconhecida"
	}
}

// tipoFalha classifica uma tentativa que não chegou a ter resposta completa.
type tipoFalha int

const (
	// semFalha: houve resposta.
	semFalha tipoFalha = iota
	// falhaTransporte: DNS, conexão, TLS, conexão derrubada, leitura.
	falhaTransporte
	// falhaTimeout: o prazo da tentativa venceu (o ctx do chamador não).
	falhaTimeout
	// falhaContexto: o ctx do chamador foi cancelado ou venceu.
	falhaContexto
)

// desfecho reúne o que decidir precisa saber de uma tentativa.
type desfecho struct {
	falha tipoFalha
	// escreveu: os cabeçalhos da requisição foram escritos na conexão
	// (httptrace.WroteHeaders). Falso, nada saiu e repetir é seguro.
	escreveu bool

	status int
	// gateway: a resposta trouxe x-amzn-ErrorType, ou seja, veio do API
	// Gateway e não do backend.
	gateway bool
	// credencialInvalida: o corpo traz o código ERR_CREDENCIAL_INVALIDA.
	credencialInvalida bool
	// tokenAceito: o token usado já foi aceito por alguma resposta sem a
	// marca do autorizador.
	tokenAceito bool
	// renovavel: o SDK sabe buscar outro token (falso com WithTokenSource).
	renovavel bool
	// jaRenovou: a renovação da regra do 403 já foi usada nesta chamada.
	jaRenovou bool
	// corpoIlegivel: resposta 2xx cujo corpo não pôde ser lido.
	corpoIlegivel bool

	// retries já feitos e máximo permitido (WithRetries).
	retries, maxRetries int
}

// ehLeitura diz se o método é seguro de repetir. Qualquer outro método
// (POST, PUT, PATCH, DELETE) é tratado como escrita.
func ehLeitura(metodo string) bool {
	return metodo == http.MethodGet || metodo == http.MethodHead
}

// decidir é a tabela de retries da arquitetura (§4.4) como função pura: o
// método e o que a tentativa trouxe determinam a ação. Não faz I/O nem lê
// relógio.
func decidir(metodo string, r desfecho) acao {
	leitura := ehLeitura(metodo)
	// repetir só enquanto sobra retry; esgotado, o erro da tentativa sai.
	repetir := acaoRepetir
	if r.retries >= r.maxRetries {
		repetir = acaoErro
	}

	if r.falha != semFalha {
		switch {
		case r.falha == falhaContexto:
			// O chamador desistiu: leitura para; escrita já enviada é incerta.
			if !leitura && r.escreveu {
				return acaoIncerto
			}
			return acaoErro
		case !r.escreveu:
			// Nada saiu da máquina: repetir é seguro em qualquer método.
			return repetir
		case leitura:
			return repetir
		default:
			return acaoIncerto
		}
	}

	switch {
	case r.status >= 200 && r.status < 300:
		switch {
		case !r.corpoIlegivel:
			return acaoSucesso
		case leitura:
			return repetir
		default:
			// A escrita foi aceita, mas falta o que ela devolveu.
			return acaoIncerto
		}
	case r.status == http.StatusForbidden:
		// Regra 4.2: autorizador, token que já foi aceito e renovação ainda
		// não usada. O API Gateway recusa antes do backend, então repetir é
		// seguro até em escrita.
		if !r.jaRenovou && r.renovavel && r.credencialInvalida && r.gateway && r.tokenAceito {
			return acaoRenovarRepetir
		}
		return acaoErro
	case r.status == http.StatusTooManyRequests:
		// Escrita só repete com a marca do autorizador: foi o gateway que
		// limitou, e o backend não viu a requisição.
		if leitura || r.gateway {
			return repetir
		}
		return acaoErro
	case r.status == http.StatusInternalServerError, r.status == http.StatusBadGateway,
		r.status == http.StatusServiceUnavailable, r.status == http.StatusGatewayTimeout:
		if leitura {
			return repetir
		}
		return acaoIncerto
	default:
		return acaoErro
	}
}
