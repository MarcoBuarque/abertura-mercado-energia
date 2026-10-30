package abertura

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	"golang.org/x/oauth2"
)

const (
	// codigoCredencialInvalida é o código do 403 do autorizador.
	codigoCredencialInvalida = "ERR_CREDENCIAL_INVALIDA" //nolint:gosec // G101: nome de código de erro da API, não é credencial
	// cabecalhoGateway marca respostas do API Gateway (autorizador, limite).
	cabecalhoGateway = "x-amzn-ErrorType"
)

// Tempos de espera entre tentativas (arquitetura §4.4).
const (
	baseBackoff    = 500 * time.Millisecond
	tetoBackoff    = 8 * time.Second
	baseBackoff429 = 2 * time.Second
	tetoBackoff429 = 30 * time.Second
	tetoRetryAfter = 30 * time.Second
)

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

// esperaRetry calcula quanto esperar antes do próximo retry. retry conta os
// retries já feitos (0 no primeiro). Um Retry-After legível, em segundos ou
// data HTTP, ganha do backoff, com teto de 30 s. Sem ele, o backoff é
// exponencial com jitter cheio: rnd() em [0,1) multiplica a faixa
// base*2^retry, limitada ao teto. Em 429 a base é 2 s e o teto 30 s; nos
// demais casos, 500 ms e 8 s.
func esperaRetry(status int, retryAfter string, retry int, agora time.Time, rnd func() float64) time.Duration {
	if d, ok := lerRetryAfter(retryAfter, agora); ok {
		return min(d, tetoRetryAfter)
	}
	faixa, teto := baseBackoff, tetoBackoff
	if status == http.StatusTooManyRequests {
		faixa, teto = baseBackoff429, tetoBackoff429
	}
	for range retry {
		if faixa >= teto {
			break
		}
		faixa *= 2
	}
	faixa = min(faixa, teto)
	return time.Duration(rnd() * float64(faixa))
}

// lerRetryAfter entende o cabeçalho em segundos inteiros ou em data HTTP.
func lerRetryAfter(valor string, agora time.Time) (time.Duration, bool) {
	if valor == "" {
		return 0, false
	}
	if s, err := strconv.Atoi(valor); err == nil {
		if s < 0 {
			return 0, false
		}
		// Limita antes de multiplicar, para não estourar time.Duration.
		return time.Duration(min(s, int(tetoRetryAfter/time.Second))) * time.Second, true
	}
	t, err := http.ParseTime(valor)
	if err != nil {
		return 0, false
	}
	return max(t.Sub(agora), 0), true
}

// dormirCtx espera d ou até o ctx terminar; nesse caso devolve o erro do ctx.
func dormirCtx(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// operacao descreve uma chamada à API para o laço de envio.
type operacao struct {
	id      string       // operationId: "listarMigracoes"
	metodo  string       // GET, POST, PUT
	modelo  string       // "/v1.1/varejista/migracoes/{idMigracao}", para o log
	caminho string       // já preenchido e escapado
	query   url.Values   // nunca vai para o log nem para os erros
	corpo   any          // nil em GET; vai como JSON
	grupo   grupoLimite  // balde do limitador
	chave   ChaveNatural // para o UncertainResultError
}

// limiteResposta é o máximo de bytes lidos de uma resposta.
const limiteResposta = 32 << 20

// parametros são os valores de uma chamada depois de somar cliente e chamada.
type parametros struct {
	agente, perfil int
	timeout        time.Duration
}

func (c *Client) parametros(cc callConfig) parametros {
	p := parametros{agente: c.agente, perfil: c.perfil, timeout: c.timeout}
	if cc.temAgente {
		p.agente, p.perfil = cc.agente, cc.perfil
	}
	if cc.timeout > 0 {
		p.timeout = cc.timeout
	}
	return p
}

// tentativa é o que uma única requisição trouxe.
type tentativa struct {
	status     int
	header     http.Header
	corpo      []byte
	err        error // erro de transporte, sem resposta
	errLeitura error // erro ao ler o corpo de uma resposta
	escreveu   bool  // os cabeçalhos foram escritos (httptrace.WroteHeaders)
	duracao    time.Duration
}

// enviar é o laço único de envio: limitador, token, cabeçalhos, tentativa,
// decisão (decidir) e espera. Devolve o status e o corpo de uma resposta 2xx.
// Qualquer outra saída é erro: *APIError, *UncertainResultError, o erro do
// ctx, ou um erro de transporte sem a URL.
func (c *Client) enviar(ctx context.Context, op operacao, cc callConfig) (int, []byte, error) {
	var corpo []byte
	if op.corpo != nil {
		var err error
		if corpo, err = json.Marshal(op.corpo); err != nil {
			return 0, nil, fmt.Errorf("%s: codificar o corpo: %w", op.id, err)
		}
	}
	alvo := c.apiURL + op.caminho
	if len(op.query) > 0 {
		alvo += "?" + op.query.Encode()
	}
	// O erro de montagem do net/http cita a URL, e a URL leva a query; o
	// protótipo valida o que sobra sem repetir o texto.
	proto, err := http.NewRequestWithContext(ctx, op.metodo, alvo, nil)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			return 0, nil, fmt.Errorf("%s: caminho inválido", op.id)
		}
		return 0, nil, fmt.Errorf("%s: requisição inválida: %w", op.id, err)
	}
	p := c.parametros(cc)

	retries, jaRenovou := 0, false
	for n := 1; ; n++ {
		if err := c.limites.esperar(ctx, op.grupo); err != nil {
			return 0, nil, err
		}
		tok, err := c.fonte.token(ctx)
		if err != nil {
			return 0, nil, err
		}

		t := c.tentar(ctx, proto, corpo, tok, p)
		d, apiErr := c.avaliar(ctx, op, t, tok, retries, jaRenovou)
		c.logTentativa(ctx, op, t, d, apiErr, n)

		switch decidir(op.metodo, d) {
		case acaoSucesso:
			return t.status, t.corpo, nil
		case acaoRepetir:
			espera := esperaRetry(t.status, t.header.Get("Retry-After"), retries, c.agora(), c.rnd)
			retries++
			c.aviso(ctx, "retry agendado", op, t, d, slog.Int("tentativa", n+1), slog.Int64("espera_ms", espera.Milliseconds()))
			if err := c.dormir(ctx, espera); err != nil {
				return 0, nil, err
			}
		case acaoRenovarRepetir:
			c.fonte.invalidar(tok)
			jaRenovou = true
			c.aviso(ctx, "token renovado", op, t, d, slog.Int("tentativa", n+1))
		case acaoIncerto:
			c.aviso(ctx, "resultado incerto", op, t, d, slog.Int("tentativa", n))
			return 0, nil, &UncertainResultError{Operation: op.id, Key: op.chave, Err: c.causa(ctx, op, d, t, apiErr)}
		default:
			if d.falha == falhaContexto {
				return 0, nil, ctx.Err()
			}
			return 0, nil, c.causa(ctx, op, d, t, apiErr)
		}
	}
}

// tentar faz uma requisição, com o prazo da tentativa, e lê a resposta inteira.
func (c *Client) tentar(ctx context.Context, proto *http.Request, corpo []byte, tok *oauth2.Token, p parametros) tentativa {
	var escreveu atomic.Bool
	actx, cancela := context.WithTimeout(ctx, p.timeout)
	defer cancela()
	actx = httptrace.WithClientTrace(actx, &httptrace.ClientTrace{
		WroteHeaders: func() { escreveu.Store(true) },
	})

	req := proto.Clone(actx)
	if corpo != nil {
		req.Body = io.NopCloser(bytes.NewReader(corpo))
		req.ContentLength = int64(len(corpo))
		req.Header["Content-Type"] = []string{"application/json"}
	}
	tok.SetAuthHeader(req)
	// Os nomes seguem a documentação da API; o mapa evita a canonicalização.
	req.Header["codigoAgente"] = []string{strconv.Itoa(p.agente)}
	req.Header["codigoPerfil"] = []string{strconv.Itoa(p.perfil)}
	req.Header["Accept"] = []string{"application/json"}
	req.Header["User-Agent"] = []string{c.userAgnt}

	inicio := time.Now()
	t := tentativa{}
	resp, err := c.http.Do(req)
	if err != nil {
		t.err = err
	} else {
		t.status, t.header = resp.StatusCode, resp.Header
		var errFechar error
		t.corpo, t.errLeitura = io.ReadAll(io.LimitReader(resp.Body, limiteResposta))
		if errFechar = resp.Body.Close(); errFechar != nil && t.errLeitura == nil {
			t.errLeitura = errFechar
		}
	}
	t.escreveu = escreveu.Load()
	t.duracao = time.Since(inicio)
	return t
}

// avaliar traduz a tentativa no desfecho que decidir entende. Devolve também
// o *APIError de uma resposta fora de 2xx. Marca o token como aceito quando a
// resposta não veio do autorizador.
func (c *Client) avaliar(ctx context.Context, op operacao, t tentativa, tok *oauth2.Token, retries int, jaRenovou bool) (desfecho, *APIError) {
	d := desfecho{
		escreveu:    t.escreveu,
		status:      t.status,
		renovavel:   c.fonte.renovavel(),
		jaRenovou:   jaRenovou,
		tokenAceito: c.fonte.foiAceito(tok),
		retries:     retries,
		maxRetries:  c.retries,
	}

	falha := t.err
	if falha == nil && t.status >= 200 && t.status <= 299 {
		falha = t.errLeitura
	}
	switch {
	case falha == nil:
	case ctx.Err() != nil:
		d.falha = falhaContexto
	case errors.Is(falha, context.DeadlineExceeded):
		d.falha = falhaTimeout
	case t.err != nil:
		d.falha = falhaTransporte
	default:
		d.corpoIlegivel = true
	}
	var apiErr *APIError
	if t.err == nil {
		d.gateway = t.header.Get(cabecalhoGateway) != ""
		if !d.gateway {
			c.fonte.marcarAceito(tok)
		}
		if t.status < 200 || t.status > 299 {
			apiErr = decodificarErro(op.id, &http.Response{StatusCode: t.status, Header: t.header}, t.corpo)
			apiErr.metodo = op.metodo
			d.credencialInvalida = apiErr.Code == codigoCredencialInvalida
		}
	}
	return d, apiErr
}

// causa é o erro que explica por que a tentativa não deu certo, sem a URL.
func (c *Client) causa(ctx context.Context, op operacao, d desfecho, t tentativa, apiErr *APIError) error {
	switch {
	case d.falha == falhaContexto:
		return ctx.Err()
	case t.err != nil:
		return fmt.Errorf("%s: %w", op.id, erroSemURL(t.err))
	case d.corpoIlegivel || apiErr == nil:
		return fmt.Errorf("%s: corpo da resposta ilegível: %w", op.id, erroSemURL(t.errLeitura))
	default:
		return apiErr
	}
}

// atributosBase são os campos de toda linha de log do transporte. Nunca
// entram query, corpo, cabeçalhos (salvo o request id), token nem credenciais.
func atributosBase(op operacao, t tentativa) []slog.Attr {
	return []slog.Attr{
		slog.String("op", op.id),
		slog.String("metodo", op.metodo),
		slog.String("rota", op.modelo),
		slog.Int("status", t.status),
		slog.String("request_id", limpar(t.header.Get("x-amzn-RequestId"), 100)),
	}
}

// nomeFalha descreve, sem detalhes, por que a tentativa não teve resposta
// utilizável.
func nomeFalha(d desfecho) string {
	switch {
	case d.falha == falhaTransporte:
		return "transporte"
	case d.falha == falhaTimeout:
		return "timeout"
	case d.falha == falhaContexto:
		return "contexto"
	case d.corpoIlegivel:
		return "corpo"
	default:
		return ""
	}
}

// logTentativa escreve o Debug de cada tentativa.
func (c *Client) logTentativa(ctx context.Context, op operacao, t tentativa, d desfecho, apiErr *APIError, n int) {
	if c.logger == nil || !c.logger.Enabled(ctx, slog.LevelDebug) {
		return
	}
	codigo := ""
	if apiErr != nil {
		codigo = limpar(apiErr.Code, limiteCodigoErro)
	}
	attrs := append(atributosBase(op, t),
		slog.Int64("duracao_ms", t.duracao.Milliseconds()),
		slog.Int("tentativa", n),
		slog.String("codigo", codigo),
	)
	if f := nomeFalha(d); f != "" {
		attrs = append(attrs, slog.String("falha", f))
	}
	c.logger.LogAttrs(ctx, slog.LevelDebug, "tentativa", attrs...)
}

// aviso escreve um Warn do transporte (retry, renovação, incerto).
func (c *Client) aviso(ctx context.Context, msg string, op operacao, t tentativa, d desfecho, extras ...slog.Attr) {
	if c.logger == nil {
		return
	}
	attrs := append(atributosBase(op, t), extras...)
	if f := nomeFalha(d); f != "" {
		attrs = append(attrs, slog.String("falha", f))
	}
	c.logger.LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
}
