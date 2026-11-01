//go:build sandbox

// Package contrato tem o teste de contrato manual contra o sandbox. Roda só
// com a tag sandbox e com credenciais no ambiente; nada aqui entra no módulo
// que o usuário importa.
package contrato

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/MarcoBuarque/abertura-mercado-energia/go/internal/spec"
)

// limiteRequisicoes é o teto de requisições de uma rodada.
const limiteRequisicoes = 90

var (
	errOrcamento = errors.New("contrato: orçamento de requisições esgotado")
	errAbortada  = errors.New("contrato: rodada abortada pelo primeiro 403")
)

// orcamento é um http.RoundTripper que conta as requisições, para na 91ª e no
// primeiro 403, valida cada resposta contra a spec embutida e guarda uma linha
// por requisição com status e nomes de campo. Nunca guarda URL, query, corpo
// nem valores.
type orcamento struct {
	base   http.RoundTripper
	max    int64
	router routers.Router

	n        atomic.Int64
	abortada atomic.Bool

	mu        sync.Mutex
	linhas    []string
	violacoes []string
	raizes    map[string]string
}

func novoOrcamento(base http.RoundTripper, max int64) (*orcamento, error) {
	doc, err := openapi3.NewLoader().LoadFromData(spec.Abertura)
	if err != nil {
		return nil, fmt.Errorf("carregar a spec: %w", err)
	}
	doc.Servers = nil // casa só pelo caminho
	r, err := legacy.NewRouter(doc)
	if err != nil {
		return nil, fmt.Errorf("montar o roteador: %w", err)
	}
	return &orcamento{base: base, max: max, router: r}, nil
}

// RoundTrip implementa http.RoundTripper.
func (o *orcamento) RoundTrip(req *http.Request) (*http.Response, error) {
	if o.abortada.Load() {
		return nil, errAbortada
	}
	if o.n.Add(1) > o.max {
		o.abortada.Store(true)
		return nil, errOrcamento
	}
	resp, err := o.base.RoundTrip(req)
	if err != nil {
		o.anotar("requisição %d: falha de transporte", o.n.Load())
		return nil, err
	}
	corpo, err := io.ReadAll(resp.Body)
	if cerr := resp.Body.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, fmt.Errorf("ler a resposta: %w", err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(corpo))
	if resp.StatusCode == http.StatusForbidden {
		o.abortada.Store(true)
	}
	op, problema := o.validar(req, resp, corpo)
	linha := fmt.Sprintf("requisição %d: %s -> %d; campos: %s", o.n.Load(), op, resp.StatusCode, nomesDeCampos(corpo))
	if n := itensNaRaiz(corpo); n != "" {
		if op == "obterMigracao" {
			if c := comparaItens(corpo); c != "" {
				n += "; " + c
			}
		}
		linha += "; " + n
		o.mu.Lock()
		if o.raizes == nil {
			o.raizes = map[string]string{}
		}
		o.raizes[op] = n
		o.mu.Unlock()
	}
	if f := formatosDeCampos(corpo); f != "" {
		linha += "; formatos: " + f
	}
	if code := codigoDoErro(corpo); code != "" {
		linha += "; code: " + code
	}
	if dica := dicaDaDescricao(corpo); dica != "" {
		linha += "; " + dica
	}
	o.anotar("%s", linha)
	if problema != "" {
		o.mu.Lock()
		o.violacoes = append(o.violacoes, fmt.Sprintf("%s (%d): %s", op, resp.StatusCode, problema))
		o.mu.Unlock()
	}
	return resp, nil
}

func (o *orcamento) anotar(formato string, args ...any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.linhas = append(o.linhas, fmt.Sprintf(formato, args...))
}

// descarregar devolve e limpa as linhas guardadas.
func (o *orcamento) descarregar() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	l := o.linhas
	o.linhas = nil
	return l
}

// todasViolacoes devolve as divergências com a spec vistas até agora.
func (o *orcamento) todasViolacoes() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.violacoes...)
}

// raizDe devolve o "itens na raiz: N" da última resposta da operação cuja raiz
// era uma lista; vazio se não houve.
func (o *orcamento) raizDe(op string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.raizes[op]
}

// usadas devolve quantas requisições já passaram.
func (o *orcamento) usadas() int64 { return o.n.Load() }

// validar confere a resposta com a spec. Devolve o operationId (ou "fora da
// spec") e, se houver divergência, uma descrição sem valores.
func (o *orcamento) validar(req *http.Request, resp *http.Response, corpo []byte) (string, string) {
	route, params, err := o.router.FindRoute(req)
	if err != nil {
		return "fora da spec", "a rota não existe na spec"
	}
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route},
		Status:                 resp.StatusCode,
		Header:                 resp.Header,
		Options:                &openapi3filter.Options{IncludeResponseStatus: true},
	}
	in.SetBodyBytes(bytes.TrimSpace(corpo))
	if err := openapi3filter.ValidateResponse(context.Background(), in); err != nil {
		return route.Operation.OperationID, descreverViolacao(err)
	}
	return route.Operation.OperationID, ""
}

// descreverViolacao resume o erro do kin-openapi sem os valores: só o campo
// que divergiu, ou o motivo geral.
func descreverViolacao(err error) string {
	var se *openapi3.SchemaError
	if errors.As(err, &se) {
		return fmt.Sprintf("campo %q diverge do schema (%s)", "/"+strings.Join(se.JSONPointer(), "/"), se.SchemaField)
	}
	var re *openapi3filter.ResponseError
	if errors.As(err, &re) {
		return re.Reason
	}
	return "resposta fora da spec"
}

// nomesDeCampos lista os nomes de campo do JSON: os do objeto e, quando um
// valor é lista de objetos, os do primeiro item (com prefixo). Nunca valores.
func nomesDeCampos(corpo []byte) string {
	var v any
	if err := json.Unmarshal(bytes.TrimSpace(corpo), &v); err != nil {
		return fmt.Sprintf("corpo não JSON (%d bytes)", len(corpo))
	}
	var nomes []string
	var anda func(prefixo string, v any)
	anda = func(prefixo string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, filho := range x {
				nomes = append(nomes, prefixo+k)
				anda(prefixo+k+".", filho)
			}
		case []any:
			if len(x) > 0 {
				anda(prefixo, x[0])
			}
		}
	}
	anda("", v)
	if len(nomes) == 0 {
		return "(nenhum)"
	}
	sort.Strings(nomes)
	return strings.Join(nomes, ",")
}

// reCodigoDaAPI é a forma de um código de erro da API.
var reCodigoDaAPI = regexp.MustCompile(`^ERR_[A-Z0-9_]{1,80}$`)

// codigosOAuth2 são os seis códigos fixos de erro da RFC 6749, seção 5.2. O
// endpoint de token responde com eles; é por eles que se sabe qual falhou.
var codigosOAuth2 = map[string]bool{
	"invalid_request": true, "invalid_client": true, "invalid_grant": true,
	"unauthorized_client": true, "unsupported_grant_type": true, "invalid_scope": true,
}

// descricoesOAuth2 são as únicas descrições de erro do Keycloak que podem ser
// impressas, por serem texto fixo e conhecido. Qualquer outra é ignorada.
var descricoesOAuth2 = map[string]bool{
	"duplicated parameter":               true,
	"Missing form parameter: grant_type": true,
}

// dicaDaDescricao devolve "oauth2_error_description: <texto>" só quando
// error_description é exatamente uma das descrições fixas; senão, vazio, sem
// nada sobre o texto recebido (nem o tamanho).
func dicaDaDescricao(corpo []byte) string {
	var e struct {
		Descricao string `json:"error_description"`
	}
	if json.Unmarshal(bytes.TrimSpace(corpo), &e) != nil || !descricoesOAuth2[e.Descricao] {
		return ""
	}
	return "oauth2_error_description: " + e.Descricao
}

// codigoSeguro devolve o código se tiver a forma ERR_ seguida de maiúsculas,
// dígitos e sublinhados, ou se for um dos seis códigos fixos da RFC 6749;
// senão, "código fora do padrão". O campo vem do
// servidor e pode levar texto livre (com dado pessoal) ao log. Vazio continua
// vazio.
func codigoSeguro(code string) string {
	if code == "" || reCodigoDaAPI.MatchString(code) || codigosOAuth2[code] {
		return code
	}
	return "código fora do padrão"
}

// codigoDoErro devolve o campo "error" de um corpo de erro, se houver e se
// tiver a forma de um código da API.
func codigoDoErro(corpo []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(bytes.TrimSpace(corpo), &e) != nil {
		return ""
	}
	return codigoSeguro(e.Error)
}

// camposTemporais são os únicos campos de data e hora cujo formato pode ser
// descrito: nenhum é dado pessoal.
var camposTemporais = map[string]bool{
	"mesReferencia": true, "dataReferencia": true, "dataDenuncia": true, "dataSolicitacao": true,
	"dataValidacao": true, "diaReferencia": true, "dataReferenciaConsumo": true,
	"expires_in": true, "timestamp": true,
}

// tamanhoMaxMascara é o tamanho máximo, em caracteres, de uma máscara.
const tamanhoMaxMascara = 40

// mascarar troca cada dígito Unicode por 9 e cada letra Unicode por A, mantém os
// demais caracteres e corta em tamanhoMaxMascara com "…". O valor em si nunca
// sai daqui.
func mascarar(valor string) string {
	var b strings.Builder
	n := 0
	for _, r := range valor {
		if n == tamanhoMaxMascara {
			b.WriteString("…")
			break
		}
		switch {
		case unicode.IsDigit(r):
			r = '9'
		case unicode.IsLetter(r):
			r = 'A'
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// formatosDeCampos descreve o formato dos campos temporais da lista branca,
// "campo=máscara" separados por vírgula, em ordem de nome: a primeira
// ocorrência de cada um, procurada como em [nomesDeCampos]. Nulo vira "null";
// número vira uma sequência de noves. Nunca o valor.
func formatosDeCampos(corpo []byte) string {
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(corpo)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return ""
	}
	achados := map[string]string{}
	var anda func(v any)
	anda = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			chaves := make([]string, 0, len(x))
			for k := range x {
				chaves = append(chaves, k)
			}
			sort.Strings(chaves)
			for _, k := range chaves {
				if _, ja := achados[k]; camposTemporais[k] && !ja {
					achados[k] = mascaraDoValor(x[k])
				}
				anda(x[k])
			}
		case []any:
			if len(x) > 0 {
				anda(x[0])
			}
		}
	}
	anda(v)
	nomes := make([]string, 0, len(achados))
	for k := range achados {
		nomes = append(nomes, k)
	}
	sort.Strings(nomes)
	partes := make([]string, 0, len(nomes))
	for _, k := range nomes {
		partes = append(partes, k+"="+achados[k])
	}
	return strings.Join(partes, ", ")
}

// mascaraDoValor mascara um valor JSON de campo temporal.
func mascaraDoValor(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return mascarar(x)
	case json.Number:
		return mascarar(x.String())
	case bool:
		return "bool"
	case map[string]any:
		return "objeto"
	default:
		return "lista"
	}
}

// itensNaRaiz devolve "itens na raiz: N" quando a raiz do JSON é uma lista; N
// é uma contagem, nunca um valor. Para objeto ou corpo que não é JSON, vazio.
func itensNaRaiz(corpo []byte) string {
	var v any
	if json.Unmarshal(bytes.TrimSpace(corpo), &v) != nil {
		return ""
	}
	l, ok := v.([]any)
	if !ok {
		return ""
	}
	return fmt.Sprintf("itens na raiz: %d", len(l))
}

// comparaItens, quando a raiz é uma lista de mais de um objeto, diz se os itens
// são iguais depois de recodificados (a ordem dos campos não conta) e, se
// diferem, quais NOMES de campo de primeiro nível mudam entre o primeiro item
// e qualquer outro. Nunca imprime valores. Para os demais casos, vazio.
func comparaItens(corpo []byte) string {
	var v any
	if json.Unmarshal(bytes.TrimSpace(corpo), &v) != nil {
		return ""
	}
	l, ok := v.([]any)
	if !ok || len(l) < 2 {
		return ""
	}
	primeiro, ok := l[0].(map[string]any)
	if !ok {
		return ""
	}
	difere := map[string]bool{}
	for _, it := range l[1:] {
		outro, ok := it.(map[string]any)
		if !ok {
			return "itens diferentes"
		}
		for k, a := range primeiro {
			if !mesmoJSON(a, outro[k]) || !temChave(outro, k) {
				difere[k] = true
			}
		}
		for k := range outro {
			if !temChave(primeiro, k) {
				difere[k] = true
			}
		}
	}
	if len(difere) == 0 {
		return "itens iguais"
	}
	nomes := make([]string, 0, len(difere))
	for k := range difere {
		nomes = append(nomes, k)
	}
	sort.Strings(nomes)
	return "itens diferentes; campos que diferem: " + strings.Join(nomes, ",")
}

func temChave(m map[string]any, k string) bool {
	_, ok := m[k]
	return ok
}

// mesmoJSON compara dois valores decodificados pela forma canônica (chaves
// ordenadas pelo codificador).
func mesmoJSON(a, b any) bool {
	ja, erra := json.Marshal(a)
	jb, errb := json.Marshal(b)
	return erra == nil && errb == nil && bytes.Equal(ja, jb)
}
