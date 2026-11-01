package abertura

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

const (
	// limiteCorpoErro é o máximo de bytes do corpo que o APIError guarda.
	limiteCorpoErro = 4 << 10
	// limiteMensagemErro é o máximo de caracteres da mensagem em Error().
	limiteMensagemErro = 200
	// limiteCodigoErro é o máximo de caracteres do código em Error().
	limiteCodigoErro = 100
)

// APIError é o erro devolvido quando a API responde com status fora de 2xx.
//
// A API usa dois formatos de erro, ambos com os campos "error" e "message": o
// do backend (400) e o do autorizador do API Gateway (403, com o cabeçalho
// x-amzn-ErrorType). Um corpo que não seja JSON também vira APIError, com Code
// e Message vazios.
//
// Error() mostra a operação, o status, o código e a mensagem (cortada em 200
// caracteres). Nunca inclui o corpo nem cabeçalhos; o corpo só sai por RawBody.
type APIError struct {
	// Operation é o operationId da spec ("listarMigracoes"); "do" no Client.Do.
	Operation string
	// StatusCode é o status HTTP da resposta.
	StatusCode int
	// Code é o campo "error" do corpo, um código textual estável como
	// "ERR_MIGRACAO_SOLICITADA". Vazio se o corpo não o trouxer.
	Code string
	// Message é o campo "message" do corpo, escrito para humanos. É texto do
	// servidor e entra em Error(): nenhuma mensagem vista no sandbox trouxe
	// dado da unidade consumidora, mas nada impede que traga. Ao registrar o
	// erro onde dado pessoal não pode ir, prefira Operation, StatusCode, Code e
	// RequestID.
	Message string
	// RequestID é o cabeçalho x-amzn-RequestId, útil para suporte.
	RequestID string
	// GatewayErrorType é o cabeçalho x-amzn-ErrorType. Preenchido, indica que
	// o API Gateway recusou a requisição antes do backend; vazio, o erro veio
	// do backend.
	GatewayErrorType string

	// metodo é o método HTTP da chamada, registrado pelo SDK; alimenta
	// Problem().Retryable e Uncertain. Vazio num APIError montado à mão.
	metodo  string
	rawBody []byte
}

// decodificarErro monta o APIError de uma resposta com status de erro. O corpo
// já foi lido pelo chamador; resp só fornece status e cabeçalhos e pode ser nil.
func decodificarErro(op string, resp *http.Response, corpo []byte) *APIError {
	e := &APIError{Operation: op}
	if resp != nil {
		e.StatusCode = resp.StatusCode
		e.RequestID = resp.Header.Get("x-amzn-RequestId")
		e.GatewayErrorType = resp.Header.Get("x-amzn-ErrorType")
	}

	guardado := corpo
	if len(guardado) > limiteCorpoErro {
		guardado = guardado[:limiteCorpoErro]
	}
	e.rawBody = append([]byte(nil), guardado...)

	// Decodificação tolerante: campos a mais ou de outro tipo não derrubam.
	var campos map[string]json.RawMessage
	if err := json.Unmarshal(corpo, &campos); err != nil {
		return e
	}
	e.Code = textoDoCampo(campos["error"])
	e.Message = textoDoCampo(campos["message"])
	return e
}

// textoDoCampo devolve o texto de um campo JSON, ou "" se não for texto.
func textoDoCampo(raw json.RawMessage) string {
	var s string
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// Error implementa error, sem corpo nem cabeçalhos.
func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "abertura: %s: HTTP %d", e.Operation, e.StatusCode)
	if e.Code != "" {
		b.WriteString(" ")
		b.WriteString(limpar(e.Code, limiteCodigoErro))
	}
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(limpar(e.Message, limiteMensagemErro))
	}
	return b.String()
}

// GoString faz o %#v seguir o Error: sem isso, ele imprimiria o corpo cru em
// hexadecimal.
func (e *APIError) GoString() string { return e.Error() }

// limpar troca caracteres de controle por espaço e corta em max caracteres,
// para que um texto vindo do servidor não quebre nem forje linhas de log.
func limpar(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

// Is faz as sentinelas do pacote casarem com o erro: as de código ("ERR_*")
// pelo campo Code, ignorando o status; ErrNotFound e ErrRateLimited pelo
// StatusCode.
func (e *APIError) Is(target error) bool {
	s, ok := target.(*sentinela)
	if !ok {
		return false
	}
	switch {
	case s.codigo != "":
		return e.Code == s.codigo
	case s.status != 0:
		return e.StatusCode == s.status
	default:
		return false
	}
}

// RawBody devolve uma cópia dos primeiros 4 KiB do corpo da resposta. É o único
// caminho para o corpo: ele nunca aparece em Error() nem em logs do SDK.
func (e *APIError) RawBody() []byte {
	return append([]byte(nil), e.rawBody...)
}

// sentinela é o tipo dos erros-valor do pacote, comparáveis com errors.Is.
type sentinela struct {
	texto  string
	codigo string // casa com APIError.Code
	status int    // casa com APIError.StatusCode
}

func (s *sentinela) Error() string { return s.texto }

// Sentinelas por código de erro da API. Casam com errors.Is pelo campo Code do
// APIError, porque o mesmo status 400 tem significados diferentes.
var (
	// ErrCredencialInvalida: 403 do autorizador (ERR_CREDENCIAL_INVALIDA).
	ErrCredencialInvalida error = &sentinela{texto: "abertura: credencial inválida (ERR_CREDENCIAL_INVALIDA)", codigo: "ERR_CREDENCIAL_INVALIDA"}
	// ErrValidacaoCampos: campo inválido na requisição (ERR_VALIDACAO_CAMPOS).
	ErrValidacaoCampos error = &sentinela{texto: "abertura: validação de campos (ERR_VALIDACAO_CAMPOS)", codigo: "ERR_VALIDACAO_CAMPOS"}
	// ErrPerfilVarejistaObrigatorio: falta o perfil de varejista
	// (ERR_PERFIL_VAREJISTA_OBRIGATORIO).
	ErrPerfilVarejistaObrigatorio error = &sentinela{texto: "abertura: perfil de varejista obrigatório (ERR_PERFIL_VAREJISTA_OBRIGATORIO)", codigo: "ERR_PERFIL_VAREJISTA_OBRIGATORIO"}
	// ErrMigracaoSolicitada: já existe migração ativa para a mesma UC e
	// concessionária (ERR_MIGRACAO_SOLICITADA).
	ErrMigracaoSolicitada error = &sentinela{texto: "abertura: migração já solicitada (ERR_MIGRACAO_SOLICITADA)", codigo: "ERR_MIGRACAO_SOLICITADA"}
	// ErrFaixaDeMesesObrigatoria: a listagem de migrações foi pedida sem a faixa
	// de meses (ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO). O SDK valida a faixa
	// antes de enviar; só [Client.Do] chega a este código.
	ErrFaixaDeMesesObrigatoria error = &sentinela{texto: "abertura: faixa de meses obrigatória (ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO)", codigo: "ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO"}
	// ErrFaixaDeMesesLargaDemais: a faixa entre o mês inicial e o final passou
	// do limite da API (ERR_MES_REFERENCIA_INICIAL_DIFERENCA). O limite não é
	// publicado; no sandbox, em 01/11/2026, 12 meses passaram e 24 não (teto entre
	// 12 e 24, FONTES D22). Estreite a faixa ou divida a consulta.
	ErrFaixaDeMesesLargaDemais error = &sentinela{texto: "abertura: faixa de meses larga demais (ERR_MES_REFERENCIA_INICIAL_DIFERENCA)", codigo: "ERR_MES_REFERENCIA_INICIAL_DIFERENCA"}
)

// Sentinelas de mecânica HTTP. Casam com errors.Is pelo StatusCode do APIError.
var (
	// ErrNotFound: recurso inexistente (404). Também é devolvido, sem APIError,
	// quando uma consulta por id volta vazia.
	ErrNotFound error = &sentinela{texto: "abertura: não encontrado", status: http.StatusNotFound}
	// ErrRateLimited: limite de requisições excedido (429).
	ErrRateLimited error = &sentinela{texto: "abertura: limite de requisições excedido", status: http.StatusTooManyRequests}
)

// ChaveNatural identifica, no lado do servidor, o recurso que uma escrita de
// resultado incerto talvez tenha criado ou alterado. Serve de entrada para os
// métodos Conferir*. Só os tipos do pacote a implementam.
type ChaveNatural interface {
	String() string
	chaveNatural()
}

// UncertainResultError indica que uma escrita pode ou não ter sido aplicada:
// a requisição saiu, mas a resposta não permite saber (erro de transporte
// depois do envio, timeout, 5xx, ou 2xx com corpo ilegível). O SDK não repete
// a escrita sozinho; confira o estado com Key e o método Conferir* adequado.
//
// Error() não inclui Key, que pode conter dados da unidade consumidora.
type UncertainResultError struct {
	// Operation é o operationId da escrita.
	Operation string
	// Key localiza o recurso afetado; nil em Client.Do.
	Key ChaveNatural
	// Err é a causa: erro de transporte, timeout, *APIError 5xx ou falha de
	// decodificação.
	Err error
}

// Error implementa error.
func (e *UncertainResultError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("abertura: %s: resultado incerto", e.Operation)
	}
	return fmt.Sprintf("abertura: %s: resultado incerto: %v", e.Operation, e.Err)
}

// GoString faz o %#v seguir o Error: sem isso, ele imprimiria a Key e a causa
// por inteiras.
func (e *UncertainResultError) GoString() string { return e.Error() }

// Unwrap devolve a causa, para errors.Is e errors.As.
func (e *UncertainResultError) Unwrap() error { return e.Err }

// erroSemURL tira a URL de um *url.Error, que traz a query (UC e mês), mas
// mantém a causa acessível por errors.Is e errors.As.
func erroSemURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}

// erroTransporte é uma falha sem resposta utilizável (DNS, conexão, TLS,
// conexão derrubada, prazo da tentativa). Error() e Unwrap seguem a causa, já
// sem a URL; o tipo guarda o que decidir precisa para ProblemFrom dizer se
// repetir é seguro.
type erroTransporte struct {
	err      error
	metodo   string
	escreveu bool // os cabeçalhos da requisição foram escritos
	timeout  bool // o prazo da tentativa venceu
}

func (e *erroTransporte) Error() string { return e.err.Error() }

func (e *erroTransporte) Unwrap() error { return e.err }

// erroResposta é uma resposta 2xx que o SDK não conseguiu ler (corpo
// interrompido) ou entender (JSON inválido, campo ilegível). Error() e Unwrap
// seguem a causa. Fora de um *UncertainResultError, sempre vem de uma leitura:
// toda escrita com resposta ilegível é incerta.
type erroResposta struct{ err error }

func (e *erroResposta) Error() string { return e.err.Error() }

func (e *erroResposta) Unwrap() error { return e.err }

// respostaIlegivel marca err como resposta 2xx ilegível.
func respostaIlegivel(err error) error { return &erroResposta{err: err} }

// erroFalhaToken é uma falha ao obter o token que não é resposta de erro do
// endpoint nem falha de rede: resposta sem access_token, ou erro da fonte
// externa (WithTokenSource). Error() e Unwrap seguem a causa.
type erroFalhaToken struct{ err error }

func (e *erroFalhaToken) Error() string { return e.err.Error() }

func (e *erroFalhaToken) Unwrap() error { return e.err }
