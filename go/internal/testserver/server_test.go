package testserver_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MarcoBuarque/abertura-mercado-energia/go/internal/testserver"
)

const modeloListagem = "/v1.1/varejista/migracoes"

// tbRegistrador captura as falhas que o servidor reporta, para provar que a
// validação reprova de fato sem derrubar o teste que a exercita.
type tbRegistrador struct {
	testing.TB
	mu    sync.Mutex
	erros []string
}

func (r *tbRegistrador) Errorf(formato string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.erros = append(r.erros, strings.TrimSpace(sprintf(formato, args...)))
}

func (r *tbRegistrador) falhas() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.erros...)
}

// recebida guarda o que os testes precisam da resposta, já com o corpo lido e
// fechado.
type recebida struct {
	StatusCode int
	Header     http.Header
}

func get(t testing.TB, url string, cabecalhos map[string]string) (recebida, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("criar requisição: %v", err)
	}
	for k, v := range cabecalhos {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("enviar requisição: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("fechar corpo: %v", err)
		}
	}()
	corpo, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ler corpo: %v", err)
	}
	return recebida{StatusCode: resp.StatusCode, Header: resp.Header}, string(corpo)
}

var cabecalhosValidos = map[string]string{"codigoAgente": "900002", "codigoPerfil": "900003"}

const queryValida = "?codigoPerfilVarejista=900003"

func TestServer_WhenSequencia502502200_RespondeNaOrdemERepeteAUltima(t *testing.T) {
	t.Parallel()
	srv := testserver.New(t, testserver.Rota{
		Metodo: http.MethodGet, Modelo: modeloListagem,
		Respostas: []testserver.Resposta{
			{Status: 502, Fixture: "erro_502.txt"},
			{Status: 502, Fixture: "erro_502.txt"},
			{Status: 200, Fixture: "migracoes_pagina1.json"},
		},
	})
	want := []int{502, 502, 200, 200}
	for i, status := range want {
		resp, corpo := get(t, srv.URL()+modeloListagem+queryValida, cabecalhosValidos)
		if resp.StatusCode != status {
			t.Fatalf("chamada %d: status = %d, quero %d", i+1, resp.StatusCode, status)
		}
		if resp.Header.Get("x-amzn-RequestId") == "" {
			t.Errorf("chamada %d: falta x-amzn-RequestId", i+1)
		}
		if status == 502 && !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
			t.Errorf("chamada %d: Content-Type = %q, quero texto puro", i+1, resp.Header.Get("Content-Type"))
		}
		if status == 200 && !strings.Contains(corpo, "cursor-inventado-2") {
			t.Errorf("chamada %d: corpo sem a fixture: %s", i+1, corpo)
		}
	}
	if got := srv.Contagem(http.MethodGet, modeloListagem); got != len(want) {
		t.Errorf("Contagem = %d, quero %d", got, len(want))
	}
}

func TestServer_WhenRespostaAdicionaCabecalho_EnviaCabecalho(t *testing.T) {
	t.Parallel()
	srv := testserver.New(t, testserver.Rota{
		Metodo: http.MethodGet, Modelo: modeloListagem,
		Respostas: []testserver.Resposta{
			{Status: 403, Fixture: "erro_403_autorizador.json"},
			{Status: 429, Fixture: "erro_400_validacao.json", Cabecalhos: map[string]string{"Retry-After": "2"}},
		},
	})
	resp, _ := get(t, srv.URL()+modeloListagem+queryValida, cabecalhosValidos)
	if got := resp.Header.Get("x-amzn-ErrorType"); got != "AccessDeniedException" {
		t.Errorf("403 sem x-amzn-ErrorType, veio %q", got)
	}
	resp, _ = get(t, srv.URL()+modeloListagem+queryValida, cabecalhosValidos)
	if got := resp.Header.Get("Retry-After"); got != "2" {
		t.Errorf("Retry-After = %q, quero 2", got)
	}
}

func TestServer_WhenAtraso_DemoraParaResponder(t *testing.T) {
	t.Parallel()
	srv := testserver.New(t, testserver.Rota{
		Metodo: http.MethodGet, Modelo: modeloListagem,
		Respostas: []testserver.Resposta{{Status: 200, Fixture: "migracoes_vazia.json", Atraso: 150 * time.Millisecond}},
	})
	inicio := time.Now()
	get(t, srv.URL()+modeloListagem+queryValida, cabecalhosValidos)
	if d := time.Since(inicio); d < 150*time.Millisecond {
		t.Errorf("respondeu em %v, quero ao menos 150ms", d)
	}
}

func TestServer_WhenDerrubar_FechaAConexaoSemResposta(t *testing.T) {
	t.Parallel()
	srv := testserver.New(t, testserver.Rota{
		Metodo: http.MethodGet, Modelo: modeloListagem,
		Respostas: []testserver.Resposta{{Derrubar: true}},
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL()+modeloListagem+queryValida, nil)
	if err != nil {
		t.Fatalf("criar requisição: %v", err)
	}
	for k, v := range cabecalhosValidos {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Errorf("fechar corpo: %v", cerr)
		}
		t.Fatalf("esperava erro de conexão, veio status %d", resp.StatusCode)
	}
	if got := srv.Contagem(http.MethodGet, modeloListagem); got != 1 {
		t.Errorf("Contagem = %d, quero 1 (a requisição foi lida antes de derrubar)", got)
	}
}

func TestServer_WhenRequisicaoForaDaSpec_ReprovaComMensagemClara(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		caminho    string
		cabecalhos map[string]string
		wantMsg    string
	}{
		{name: "sem o cabeçalho codigoAgente", caminho: modeloListagem + queryValida, cabecalhos: map[string]string{"codigoPerfil": "900003"}, wantMsg: "codigoAgente"},
		{name: "sem o parâmetro obrigatório codigoPerfilVarejista", caminho: modeloListagem, cabecalhos: cabecalhosValidos, wantMsg: "codigoPerfilVarejista"},
		{name: "parâmetro de consulta desconhecido", caminho: modeloListagem + queryValida + "&parametroInventado=1", cabecalhos: cabecalhosValidos, wantMsg: "parametroInventado"},
		{name: "caminho que a spec não tem", caminho: "/v1.1/varejista/inexistente", cabecalhos: cabecalhosValidos, wantMsg: "inexistente"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reg := &tbRegistrador{TB: t}
			srv := testserver.New(reg, testserver.Rota{
				Metodo: http.MethodGet, Modelo: modeloListagem,
				Respostas: []testserver.Resposta{{Status: 200, Fixture: "migracoes_vazia.json"}},
			})
			resp, _ := get(t, srv.URL()+tc.caminho, tc.cabecalhos)
			if resp.StatusCode < 400 {
				t.Errorf("status = %d, quero erro", resp.StatusCode)
			}
			falhas := reg.falhas()
			if len(falhas) != 1 || !strings.Contains(falhas[0], tc.wantMsg) || !strings.Contains(falhas[0], "spec") {
				t.Errorf("falhas = %q, quero uma mensagem sobre a spec citando %q", falhas, tc.wantMsg)
			}
			if got := srv.Violacoes(); len(got) != 1 {
				t.Errorf("Violacoes = %q, quero 1", got)
			}
		})
	}
}

func TestServer_WhenRequisicaoValida_NaoReportaViolacao(t *testing.T) {
	t.Parallel()
	reg := &tbRegistrador{TB: t}
	srv := testserver.New(reg, testserver.Rota{
		Metodo: http.MethodGet, Modelo: modeloListagem,
		Respostas: []testserver.Resposta{{Status: 200, Fixture: "migracoes_vazia.json"}},
	})
	get(t, srv.URL()+modeloListagem+queryValida+"&mesReferenciaInicial=2027-01", cabecalhosValidos)
	if f := reg.falhas(); len(f) != 0 {
		t.Errorf("falhas inesperadas: %q", f)
	}
}

func TestServer_WhenRotaNaoRegistrada_Devolve404ERegistraFalha(t *testing.T) {
	t.Parallel()
	reg := &tbRegistrador{TB: t}
	srv := testserver.New(reg)
	resp, _ := get(t, srv.URL()+modeloListagem+queryValida, cabecalhosValidos)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, quero 404", resp.StatusCode)
	}
	if f := reg.falhas(); len(f) != 1 || !strings.Contains(f[0], "sem rota registrada") {
		t.Errorf("falhas = %q", f)
	}
}

func TestServer_Requisicoes_GuardaCopiasComCorpo(t *testing.T) {
	t.Parallel()
	srv := testserver.New(t, testserver.Rota{
		Metodo: http.MethodPost, Modelo: "/sso/oauth/token",
		Respostas: []testserver.Resposta{{Status: 200, Fixture: "token.json"}},
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/sso/oauth/token",
		strings.NewReader("client_id=cliente-inventado&client_secret=segredo-inventado&grant_type=client_credentials"))
	if err != nil {
		t.Fatalf("criar requisição: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("enviar: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Errorf("fechar corpo: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, quero 200", resp.StatusCode)
	}
	reqs := srv.Requisicoes()
	if len(reqs) != 1 {
		t.Fatalf("requisições = %d, quero 1", len(reqs))
	}
	corpo, err := io.ReadAll(reqs[0].Body)
	if err != nil {
		t.Fatalf("ler corpo guardado: %v", err)
	}
	if !strings.Contains(string(corpo), "client_id=cliente-inventado") {
		t.Errorf("corpo guardado = %q", corpo)
	}
	if reqs[0].Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Errorf("cabeçalho não guardado: %v", reqs[0].Header)
	}
}

func sprintf(f string, a ...any) string { return fmt.Sprintf(f, a...) }
