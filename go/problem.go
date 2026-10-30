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

// ProblemContentType é o tipo de mídia da RFC 9457 (Problem Details for HTTP
// APIs). Use-o no cabeçalho Content-Type ao devolver um [Problem] como JSON.
const ProblemContentType = "application/problem+json"

// CatalogoErrosURL é a página do catálogo de erros (docs/erros.md). O campo
// Type de um [Problem] é essa URL mais "#" e o slug da entrada.
//
// A página vale a partir da publicação do repositório: até lá o endereço ainda
// não responde, e o catálogo se lê em docs/erros.md.
const CatalogoErrosURL = "https://marcobuarque.github.io/abertura-mercado-energia/erros"

// Problem é um erro do SDK no formato da RFC 9457 (Problem Details for HTTP
// APIs). A API devolve só quatro campos (timestamp, status, error, message);
// o SDK entrega este formato por cima, com título e explicação escritos pelo
// projeto e ligados ao catálogo em [CatalogoErrosURL].
//
// Um Problem nunca carrega a mensagem crua da API, que pode citar unidade
// consumidora, documento ou e-mail: Detail é texto do catálogo. A mensagem
// original continua em [APIError].Message, para quem decidir usá-la. Também
// não carrega token, segredo, URL com consulta nem corpo.
//
// Para devolver um Problem num servidor HTTP, grave-o com Content-Type
// [ProblemContentType]. Status é o da resposta da API; se o seu servidor
// responde com outro status, ajuste Status para o mesmo valor antes de
// codificar, como a RFC pede.
type Problem struct {
	// Type é a URI da entrada do catálogo ("…/erros#migracao-solicitada"), ou
	// "about:blank" para erro sem entrada (por exemplo, um erro que não é do
	// SDK).
	Type string
	// Title é o título curto da entrada, em português.
	Title string
	// Status é o status HTTP da resposta da API. Vale 0 quando não houve
	// resposta (falha de rede, validação local); nesse caso sai omitido do JSON.
	Status int
	// Detail explica esta ocorrência com texto do catálogo e o nome da
	// operação. Nunca traz dado pessoal.
	Detail string
	// Instance é o identificador desta ocorrência. Fica vazio na v0.1 e sai
	// omitido do JSON.
	Instance string

	// Code é o código da API ("ERR_MIGRACAO_SOLICITADA"), se houver e se tiver
	// a forma ERR_ seguida de maiúsculas, dígitos e sublinhados.
	Code string
	// Operation é o operationId da chamada ("criarMigracao"), se o SDK souber.
	Operation string
	// RequestID é o x-amzn-RequestId da resposta, útil para suporte.
	RequestID string
	// Retryable diz se é seguro repetir a mesma chamada agora, sem conferir
	// nada antes. Segue a tabela de retries do SDK: leituras repetem em 429 e
	// 5xx; escritas só quando nada foi enviado ou o gateway limitou.
	Retryable bool
	// Uncertain marca uma escrita de resultado incerto: confira o estado
	// (ConferirCriacao, ConferirEdicao) antes de repetir.
	Uncertain bool
}

// problemaJSON é a forma serializada de um [Problem].
type problemaJSON struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Instance  string `json:"instance,omitempty"`
	Code      string `json:"code,omitempty"`
	Operation string `json:"operation,omitempty"`
	RequestID string `json:"requestId,omitempty"`
	Retryable bool   `json:"retryable"`
	Uncertain bool   `json:"uncertain"`
}

// MarshalJSON escreve o Problem como na RFC 9457: type, title, status, detail
// e instance, mais as extensões code, operation, requestId, retryable e
// uncertain (camelCase). Campos de texto vazios e status 0 são omitidos;
// retryable e uncertain saem sempre, porque "falso" também informa.
func (p Problem) MarshalJSON() ([]byte, error) {
	return json.Marshal(problemaJSON(p))
}

// ErrArgumentoInvalido casa, com errors.Is, com os erros de validação local:
// o SDK recusou a chamada antes de enviá-la (filtro incompleto, id vazio,
// caminho de Do sem "/"). Nada foi enviado e repetir sem corrigir não adianta.
var ErrArgumentoInvalido error = &sentinela{texto: "abertura: argumento inválido"}

// erroArgumento é a validação local. Error() mantém o texto original; Is faz
// o erro casar com [ErrArgumentoInvalido].
type erroArgumento struct{ msg string }

func (e *erroArgumento) Error() string { return e.msg }

func (e *erroArgumento) Is(target error) bool { return target == ErrArgumentoInvalido }

// argumentoInvalido monta um erro de validação local.
func argumentoInvalido(formato string, a ...any) error {
	return &erroArgumento{msg: fmt.Sprintf(formato, a...)}
}

// entradaCatalogo é uma entrada da tabela; a âncora de docs/erros.md é o slug.
type entradaCatalogo struct {
	titulo  string
	detalhe string
	// puro: o Type é "about:blank" em vez da URL do catálogo.
	puro bool
}

// catalogo é a única fonte, no código, de slugs e títulos. O teste de
// paridade confere cada slug com as âncoras de docs/erros.md.
var catalogo = map[string]entradaCatalogo{
	"credencial-invalida-autorizador": {
		titulo:  "Credencial recusada pelo autorizador",
		detalhe: "O autorizador da API recusou o token, ou o agente e o perfil, antes de a requisição chegar ao backend. Nada foi processado.",
	},
	"credencial-invalida-backend": {
		titulo:  "Credencial recusada pelo backend",
		detalhe: "O backend da API respondeu 403 com ERR_CREDENCIAL_INVALIDA, sem a marca do autorizador. Confira credencial, agente e perfil; o SDK não repete.",
	},
	"validacao-campos": {
		titulo:  "Campo inválido na requisição",
		detalhe: "A API rejeitou um campo da requisição. Ela valida um campo por vez e para no primeiro problema.",
	},
	"perfil-varejista-obrigatorio": {
		titulo:  "Perfil de varejista obrigatório",
		detalhe: "A API exige o perfil de varejista nesta consulta e não o recebeu.",
	},
	"migracao-solicitada": {
		titulo:  "Migração já solicitada",
		detalhe: "Já existe migração ativa para a mesma unidade consumidora e concessionária. Se a criação anterior ficou incerta, ela provavelmente valeu: confira.",
	},
	"codigo-desconhecido": {
		titulo:  "Código de erro desconhecido do SDK",
		detalhe: "A API devolveu um código ERR_* que o SDK ainda não conhece. O código está em code; o texto da API continua em APIError.Message.",
	},
	"recusa-sem-codigo": {
		titulo:  "Recusa sem código da API",
		detalhe: "A API recusou a requisição sem um código ERR_* reconhecível. Use o status e o request id para investigar.",
	},
	"nao-encontrado": {
		titulo:  "Recurso não encontrado",
		detalhe: "A API não tem o recurso pedido, ou a consulta por id voltou vazia.",
	},
	"limite-de-requisicoes": {
		titulo:  "Limite de requisições excedido",
		detalhe: "A API ou o gateway limitou a taxa de requisições. Espere antes de tentar de novo.",
	},
	"erro-interno": {
		titulo:  "Erro interno da API",
		detalhe: "O backend da API falhou ao processar a requisição.",
	},
	"falha-do-gateway": {
		titulo:  "Falha no gateway da API",
		detalhe: "O gateway da API devolveu 502 ou 504, em geral com texto puro em vez de JSON.",
	},
	"erro-de-transporte": {
		titulo:  "Falha de rede",
		detalhe: "A requisição não completou por falha de rede (DNS, conexão ou TLS).",
	},
	"tempo-esgotado": {
		titulo:  "Tempo esgotado",
		detalhe: "O prazo da tentativa, ou do contexto da chamada, venceu antes da resposta.",
	},
	"chamada-cancelada": {
		titulo:  "Chamada cancelada",
		detalhe: "O contexto da chamada foi cancelado antes de ela terminar.",
	},
	"resultado-incerto": {
		titulo:  "Resultado incerto da escrita",
		detalhe: "A escrita saiu, mas a resposta não permite saber se foi aplicada. O SDK não repete: confira o estado antes.",
	},
	"cursor-repetido": {
		titulo:  "Cursor de paginação repetido",
		detalhe: "A API devolveu como próxima página um cursor já usado; o SDK parou para não girar para sempre.",
	},
	"falha-ao-obter-token": {
		titulo:  "Falha ao obter o token",
		detalhe: "O endpoint de token recusou o pedido ou falhou. Confira client_id, client_secret e o ambiente.",
	},
	"argumento-invalido": {
		titulo:  "Argumento inválido",
		detalhe: "O SDK recusou a chamada antes de enviá-la: falta ou sobra algum argumento. Nada foi enviado.",
	},
	"fora-do-catalogo": {
		titulo:  "Erro fora do catálogo",
		detalhe: "O erro não vem do SDK, ou não tem entrada no catálogo.",
		puro:    true,
	},
}

// codigoValido devolve code se tiver a forma de um código da API (ERR_ e
// maiúsculas, dígitos e sublinhados, até 80 caracteres depois do prefixo);
// senão vazio. O campo vem do servidor e não pode levar texto livre ao JSON.
func codigoValido(code string) string {
	resto, ok := strings.CutPrefix(code, "ERR_")
	if !ok || resto == "" || len(resto) > 80 {
		return ""
	}
	for _, r := range resto {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return ""
		}
	}
	return code
}

// soIdentificador devolve s se for só letras, dígitos e sublinhado (um
// operationId), senão vazio.
func soIdentificador(s string) string {
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

// novoProblem monta o Problem de uma entrada do catálogo.
func novoProblem(slug string, status int, op string) Problem {
	e := catalogo[slug]
	p := Problem{Type: "about:blank", Title: e.titulo, Status: status, Detail: e.detalhe}
	if !e.puro {
		p.Type = CatalogoErrosURL + "#" + slug
	}
	if op = soIdentificador(op); op != "" {
		p.Operation = op
		p.Detail = "Operação " + op + ": " + e.detalhe
	}
	return p
}

// ProblemFrom traduz um erro do SDK em [Problem]. Aceita o erro direto ou
// embrulhado com %w. Um erro que não é do SDK vira o Problem "about:blank"
// (entrada "fora-do-catalogo"), sem o texto do erro. ProblemFrom(nil) devolve
// o valor zero.
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
	var ne net.Error
	switch {
	case errors.Is(err, ErrNotFound):
		return novoProblem("nao-encontrado", http.StatusNotFound, "")
	case errors.Is(err, ErrCursorRepetido):
		return novoProblem("cursor-repetido", 0, "")
	case errors.Is(err, ErrArgumentoInvalido):
		return novoProblem("argumento-invalido", 0, "")
	case errors.Is(err, context.DeadlineExceeded):
		p := novoProblem("tempo-esgotado", 0, "")
		p.Retryable = true
		return p
	case errors.Is(err, context.Canceled):
		return novoProblem("chamada-cancelada", 0, "")
	case errors.As(err, &ne) && ne.Timeout():
		p := novoProblem("tempo-esgotado", 0, "")
		p.Retryable = true
		return p
	case errors.As(err, &ne):
		p := novoProblem("erro-de-transporte", 0, "")
		p.Retryable = true
		return p
	}
	return novoProblem("fora-do-catalogo", 0, "")
}

// slug diz qual entrada do catálogo descreve o erro.
func (e *APIError) slug(code string) string {
	switch {
	case e.Operation == operacaoToken:
		return "falha-ao-obter-token"
	case code == "ERR_CREDENCIAL_INVALIDA" && e.GatewayErrorType != "":
		return "credencial-invalida-autorizador"
	case code == "ERR_CREDENCIAL_INVALIDA":
		return "credencial-invalida-backend"
	case code == "ERR_VALIDACAO_CAMPOS":
		return "validacao-campos"
	case code == "ERR_PERFIL_VAREJISTA_OBRIGATORIO":
		return "perfil-varejista-obrigatorio"
	case code == "ERR_MIGRACAO_SOLICITADA":
		return "migracao-solicitada"
	case code != "":
		return "codigo-desconhecido"
	case e.StatusCode == http.StatusNotFound:
		return "nao-encontrado"
	case e.StatusCode == http.StatusTooManyRequests:
		return "limite-de-requisicoes"
	case e.StatusCode == http.StatusBadGateway, e.StatusCode == http.StatusGatewayTimeout:
		return "falha-do-gateway"
	case e.StatusCode >= 500:
		return "erro-interno"
	default:
		return "recusa-sem-codigo"
	}
}

// Problem traduz o erro em [Problem]. Detail vem do catálogo; a mensagem da
// API fica de fora (use e.Message). Retryable e Uncertain seguem a mesma
// função de decisão do transporte, a partir do método da chamada que o SDK
// registrou; um APIError montado à mão não tem método e não afirma nenhum dos
// dois.
func (e *APIError) Problem() Problem {
	code := codigoValido(e.Code)
	p := novoProblem(e.slug(code), e.StatusCode, e.Operation)
	p.Code = code
	p.RequestID = limpar(e.RequestID, 100)
	if e.metodo != "" {
		switch decidir(e.metodo, desfecho{
			status:             e.StatusCode,
			gateway:            e.GatewayErrorType != "",
			credencialInvalida: code == codigoCredencialInvalida,
			maxRetries:         1, // só para a decisão ser "repetir", não "esgotou"
		}) {
		case acaoRepetir:
			p.Retryable = true
		case acaoIncerto:
			p.Uncertain = true
		}
	}
	return p
}

// Problem traduz o erro em [Problem] com a entrada "resultado-incerto".
// Status, Code e RequestID vêm da causa, quando ela é uma resposta da API;
// Detail cita só o título da causa, nunca a Key (que pode ter a UC).
func (e *UncertainResultError) Problem() Problem {
	p := novoProblem("resultado-incerto", 0, e.Operation)
	p.Uncertain = true
	if e.Err == nil {
		return p
	}
	causa := ProblemFrom(e.Err)
	p.Status, p.Code, p.RequestID = causa.Status, causa.Code, causa.RequestID
	p.Detail += " Causa: " + causa.Title + "."
	return p
}
