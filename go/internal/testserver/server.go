// Package testserver oferece um servidor HTTP de teste que responde com as
// fixtures de testdata/ e valida cada requisição recebida contra a descrição
// OpenAPI do projeto.
//
// Ele existe para os testes do SDK: pega parâmetro com nome errado, cabeçalho
// ausente e caminho fora da spec. Não importa o pacote abertura, para não criar
// ciclo de importação, e usa kin-openapi, que fica restrita a testes.
package testserver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/MarcoBuarque/abertura-mercado-energia/go/internal/spec"
)

// Resposta descreve uma resposta do servidor.
type Resposta struct {
	// Status é o código HTTP.
	Status int
	// Fixture é o nome de um arquivo em testdata/. Vazio devolve corpo vazio.
	Fixture string
	// Cabecalhos são acrescentados aos cabeçalhos padrão e os sobrescrevem.
	Cabecalhos map[string]string
	// Atraso espera antes de responder (interrompido se o cliente desistir).
	Atraso time.Duration
	// Derrubar lê a requisição e fecha a conexão sem responder.
	Derrubar bool
}

// Rota liga um método e um caminho da spec a uma sequência de respostas.
type Rota struct {
	// Metodo é o método HTTP, por exemplo "GET".
	Metodo string
	// Modelo é o caminho como na spec, por exemplo "/v1.1/varejista/migracoes/{idMigracao}".
	Modelo string
	// Respostas são devolvidas em sequência; a última se repete.
	Respostas []Resposta
}

// requestIDInventado é o x-amzn-RequestId de todas as respostas.
const requestIDInventado = "00000000-0000-4000-8000-0000000000f1"

// cabecalhosPadrao vale para toda resposta.
var cabecalhosPadrao = map[string]string{
	"x-amzn-RequestId": requestIDInventado,
}

// cabecalhosPorStatus vale para as respostas com o status indicado: o
// autorizador do gateway marca o 403 com x-amzn-ErrorType.
var cabecalhosPorStatus = map[int]map[string]string{
	http.StatusForbidden: {"x-amzn-ErrorType": "AccessDeniedException"},
}

type registro struct {
	req  *http.Request
	body []byte
}

// Server é o servidor de teste. Use New.
type Server struct {
	t        testing.TB
	srv      *httptest.Server
	router   routers.Router
	rotas    []Rota
	dirTest  string
	validar  bool
	mu       sync.Mutex
	contagem map[string]int // chave "METODO modelo", chamadas atendidas
	posicao  map[string]int // próxima resposta da sequência
	registro []registro
	violacao []string
}

// New sobe o servidor e o encerra no fim do teste. Valida por padrão cada
// requisição contra a spec: a violação falha o teste via t.Errorf e a
// requisição recebe status 400.
func New(t testing.TB, rotas ...Rota) *Server {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(spec.Abertura)
	if err != nil {
		t.Fatalf("testserver: carregar spec: %v", err)
	}
	// Sem servidores, o roteador casa só pelo caminho.
	doc.Servers = nil
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatalf("testserver: montar roteador da spec: %v", err)
	}
	_, arquivo, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("testserver: não achei o caminho do código-fonte")
	}
	s := &Server{
		t:        t,
		router:   router,
		rotas:    rotas,
		dirTest:  filepath.Join(filepath.Dir(arquivo), "..", "..", "testdata"),
		validar:  true,
		contagem: map[string]int{},
		posicao:  map[string]int{},
	}
	s.srv = httptest.NewServer(http.HandlerFunc(s.atender))
	t.Cleanup(s.srv.Close)
	return s
}

// SemValidacao desliga a validação contra a spec, para testes que enviam de
// propósito uma requisição inválida.
func (s *Server) SemValidacao() *Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.validar = false
	return s
}

// URL devolve o endereço base do servidor, sem barra final.
func (s *Server) URL() string { return s.srv.URL }

// Contagem devolve quantas requisições o servidor atendeu para o método e o
// modelo de caminho informados.
func (s *Server) Contagem(metodo, modelo string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contagem[chave(metodo, modelo)]
}

// Requisicoes devolve cópias das requisições recebidas, em ordem, com o corpo
// legível. Serve para inspecionar cabeçalhos e corpo.
func (s *Server) Requisicoes() []*http.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*http.Request, 0, len(s.registro))
	for _, r := range s.registro {
		c := r.req.Clone(context.Background())
		c.Body = io.NopCloser(bytes.NewReader(r.body))
		out = append(out, c)
	}
	return out
}

// Violacoes devolve as mensagens das requisições que a spec não aceita.
func (s *Server) Violacoes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.violacao...)
}

func chave(metodo, modelo string) string { return strings.ToUpper(metodo) + " " + modelo }

func (s *Server) atender(w http.ResponseWriter, r *http.Request) {
	corpo, err := io.ReadAll(r.Body)
	if err != nil {
		s.t.Errorf("testserver: ler corpo da requisição: %v", err)
		http.Error(w, "erro ao ler corpo", http.StatusInternalServerError)
		return
	}
	if err := r.Body.Close(); err != nil {
		s.t.Errorf("testserver: fechar corpo da requisição: %v", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(corpo))

	s.mu.Lock()
	s.registro = append(s.registro, registro{req: r.Clone(context.Background()), body: corpo})
	validar := s.validar
	s.mu.Unlock()

	route, pathParams, err := s.router.FindRoute(r)
	if err != nil {
		if validar {
			s.violar(w, http.StatusNotFound, "%s %s não existe na spec: %v", r.Method, r.URL.Path, err)
		} else {
			http.Error(w, "rota fora da spec", http.StatusNotFound)
		}
		return
	}
	if validar {
		if msg := s.validarRequisicao(r, corpo, route, pathParams); msg != "" {
			s.violar(w, http.StatusBadRequest, "%s %s fora da spec: %s", r.Method, r.URL.Path, msg)
			return
		}
	}

	rota, ok := s.buscarRota(r.Method, route.Path)
	if !ok {
		s.t.Errorf("testserver: sem rota registrada para %s", chave(r.Method, route.Path))
		http.Error(w, "sem rota registrada", http.StatusNotFound)
		return
	}
	resp := s.proxima(rota)
	if resp.Atraso > 0 {
		select {
		case <-time.After(resp.Atraso):
		case <-r.Context().Done():
			return
		}
	}
	if resp.Derrubar {
		s.derrubar(w)
		return
	}
	s.escrever(w, resp)
}

// validarRequisicao devolve vazio se a spec aceita a requisição.
func (s *Server) validarRequisicao(r *http.Request, corpo []byte, route *routers.Route, pathParams map[string]string) string {
	// A validação consome o corpo; cada uso recebe um leitor novo.
	r.Body = io.NopCloser(bytes.NewReader(corpo))
	in := &openapi3filter.RequestValidationInput{
		Request:    r,
		PathParams: pathParams,
		Route:      route,
		Options:    &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
	}
	if err := openapi3filter.ValidateRequest(r.Context(), in); err != nil {
		return err.Error()
	}
	// O kin-openapi não reprova parâmetro de consulta que a spec não declara.
	declarados := map[string]bool{}
	if route.PathItem != nil {
		for _, p := range route.PathItem.Parameters {
			if p.Value != nil && p.Value.In == openapi3.ParameterInQuery {
				declarados[p.Value.Name] = true
			}
		}
	}
	for _, p := range route.Operation.Parameters {
		if p.Value != nil && p.Value.In == openapi3.ParameterInQuery {
			declarados[p.Value.Name] = true
		}
	}
	for nome := range r.URL.Query() {
		if !declarados[nome] {
			return fmt.Sprintf("parâmetro de consulta %q não é declarado pela operação %s", nome, route.Operation.OperationID)
		}
	}
	r.Body = io.NopCloser(bytes.NewReader(corpo))
	return ""
}

func (s *Server) violar(w http.ResponseWriter, status int, formato string, args ...any) {
	msg := fmt.Sprintf(formato, args...)
	s.mu.Lock()
	s.violacao = append(s.violacao, msg)
	s.mu.Unlock()
	s.t.Errorf("testserver: requisição do SDK viola a spec: %s", msg)
	http.Error(w, "violação da spec: "+msg, status)
}

func (s *Server) buscarRota(metodo, modelo string) (*Rota, bool) {
	for i := range s.rotas {
		if strings.EqualFold(s.rotas[i].Metodo, metodo) && s.rotas[i].Modelo == modelo {
			return &s.rotas[i], true
		}
	}
	return nil, false
}

// proxima conta a chamada e escolhe a resposta da sequência.
func (s *Server) proxima(rota *Rota) Resposta {
	k := chave(rota.Metodo, rota.Modelo)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.contagem[k]++
	i := s.posicao[k]
	if i >= len(rota.Respostas) {
		i = len(rota.Respostas) - 1
	}
	s.posicao[k]++
	if i < 0 {
		return Resposta{Status: http.StatusNoContent}
	}
	return rota.Respostas[i]
}

func (s *Server) derrubar(w http.ResponseWriter) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		s.t.Errorf("testserver: o ResponseWriter não permite derrubar a conexão")
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		s.t.Errorf("testserver: derrubar a conexão: %v", err)
		return
	}
	if err := conn.Close(); err != nil {
		s.t.Logf("testserver: fechar a conexão derrubada: %v", err)
	}
}

func (s *Server) escrever(w http.ResponseWriter, resp Resposta) {
	var corpo []byte
	if resp.Fixture != "" {
		b, err := os.ReadFile(filepath.Join(s.dirTest, resp.Fixture))
		if err != nil {
			s.t.Errorf("testserver: ler fixture %s: %v", resp.Fixture, err)
			http.Error(w, "fixture ausente", http.StatusInternalServerError)
			return
		}
		corpo = b
		if strings.HasSuffix(resp.Fixture, ".txt") {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
	}
	for k, v := range cabecalhosPadrao {
		w.Header().Set(k, v)
	}
	for k, v := range cabecalhosPorStatus[resp.Status] {
		w.Header().Set(k, v)
	}
	for k, v := range resp.Cabecalhos {
		w.Header().Set(k, v)
	}
	w.WriteHeader(resp.Status)
	if _, err := w.Write(corpo); err != nil {
		s.t.Logf("testserver: escrever resposta: %v", err)
	}
}
