package abertura

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type saidaTeste struct {
	OK bool `json:"ok"`
}

func TestDo_LeituraDecodificaNaSaida(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t, resposta{status: 200, corpo: corpoOK}))
	c, _ := novoClienteTeste(t, srv)

	t.Run("struct tipada", func(t *testing.T) {
		var out saidaTeste
		if err := c.Do(context.Background(), "GET", "/v1/x", nil, &out); err != nil {
			t.Fatal(err)
		}
		if !out.OK {
			t.Errorf("out = %+v", out)
		}
	})
	t.Run("json.RawMessage", func(t *testing.T) {
		var raw json.RawMessage
		if err := c.Do(context.Background(), "get", "/v1/x", nil, &raw); err != nil {
			t.Fatal(err)
		}
		if string(raw) != corpoOK {
			t.Errorf("raw = %s", raw)
		}
	})
	t.Run("out nil ignora o corpo", func(t *testing.T) {
		if err := c.Do(context.Background(), "GET", "/v1/x", nil, nil); err != nil {
			t.Fatal(err)
		}
	})
}

func TestDo_CorpoVazioDeixaOutIntocado(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t, resposta{status: 204}))
	c, _ := novoClienteTeste(t, srv)

	out := saidaTeste{OK: true}
	if err := c.Do(context.Background(), "PUT", "/v1/x", map[string]int{"a": 1}, &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Errorf("out foi alterado: %+v", out)
	}
}

func TestDo_EnviaCorpoQueryEOpcoesDaChamada(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var visto struct {
		metodo, caminho, query, corpo, agente, perfil string
	}
	srv := novoServidor(t, func(w http.ResponseWriter, r *http.Request) {
		lido, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("lendo o corpo: %v", err)
		}
		mu.Lock()
		visto.metodo, visto.caminho, visto.query, visto.corpo = r.Method, r.URL.Path, r.URL.RawQuery, string(lido)
		visto.agente, visto.perfil = r.Header.Get("codigoAgente"), r.Header.Get("codigoPerfil")
		mu.Unlock()
		escrever(t, w, corpoOK)
	})
	c, _ := novoClienteTeste(t, srv)

	err := c.Do(context.Background(), "POST", "/v1.1/varejista/migracoes?codigoUnidadeConsumidora="+ucInventada+"&b=2",
		map[string]string{"k": "v"}, nil, WithAgent(900011, 900012))
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if visto.metodo != "POST" || visto.caminho != "/v1.1/varejista/migracoes" {
		t.Errorf("visto = %+v", visto)
	}
	if visto.query != "b=2&codigoUnidadeConsumidora="+ucInventada {
		t.Errorf("query = %q", visto.query)
	}
	if visto.corpo != `{"k":"v"}` {
		t.Errorf("corpo = %q", visto.corpo)
	}
	if visto.agente != "900011" || visto.perfil != "900012" {
		t.Errorf("agente/perfil = %q/%q", visto.agente, visto.perfil)
	}
}

func TestDo_Erros(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		metodo       string
		resp         resposta
		out          any
		wantIncerto  bool
		wantStatus   int // *APIError esperado
		wantIs       error
		wantQualquer bool // qualquer erro, sem tipo definido
	}{
		{name: "POST 502 vira incerto com Key nil", metodo: "POST", resp: resposta{status: 502}, wantIncerto: true},
		{name: "PUT 503 vira incerto com Key nil", metodo: "PUT", resp: resposta{status: 503}, wantIncerto: true},
		{name: "POST 200 ilegível para out vira incerto", metodo: "POST", resp: resposta{status: 200, corpo: "<html>"}, out: &saidaTeste{}, wantIncerto: true},
		{name: "POST 200 com tipo errado vira incerto", metodo: "POST", resp: resposta{status: 200, corpo: `{"ok":"sim"}`}, out: &saidaTeste{}, wantIncerto: true},
		{name: "GET 200 ilegível é erro comum", metodo: "GET", resp: resposta{status: 200, corpo: "<html>"}, out: &saidaTeste{}, wantQualquer: true},
		{name: "GET 400 traz APIError de do", metodo: "GET", resp: resposta{status: 400, corpo: erroJSON("ERR_VALIDACAO_CAMPOS")}, wantStatus: 400, wantIs: ErrValidacaoCampos},
		{name: "GET 404 casa com ErrNotFound", metodo: "GET", resp: resposta{status: 404}, wantStatus: 404, wantIs: ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := novoServidor(t, sequencia(t, tc.resp))
			c, _ := novoClienteTeste(t, srv, WithRetries(0))

			err := c.Do(context.Background(), tc.metodo, "/v1/x?codigoUnidadeConsumidora="+ucInventada, map[string]string{"a": "b"}, tc.out)
			if err == nil {
				t.Fatal("esperava erro")
			}
			var incerto *UncertainResultError
			if got := errors.As(err, &incerto); got != tc.wantIncerto {
				t.Fatalf("incerto = %v, quer %v (%v)", got, tc.wantIncerto, err)
			}
			if tc.wantIncerto {
				if incerto.Key != nil || incerto.Operation != "do" {
					t.Errorf("Key=%v Operation=%q, quer nil e \"do\"", incerto.Key, incerto.Operation)
				}
			}
			if tc.wantStatus != 0 {
				var api *APIError
				if !errors.As(err, &api) || api.StatusCode != tc.wantStatus || api.Operation != "do" {
					t.Fatalf("erro = %v, quer APIError %d de do", err, tc.wantStatus)
				}
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("errors.Is(%v) = false", tc.wantIs)
			}
			assertSemVazamento(t, err.Error(), srv.URL, "<html>", "sim")
		})
	}
}

func TestDo_EntradasInvalidas(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t, resposta{status: 200, corpo: corpoOK}))
	c, _ := novoClienteTeste(t, srv)

	tests := []struct {
		name   string
		metodo string
		path   string
		body   any
	}{
		{"método vazio", "", "/v1/x", nil},
		{"caminho sem barra inicial", "GET", "v1/x", nil},
		{"caminho vazio", "GET", "", nil},
		{"query inválida não vaza", "GET", "/v1/x?codigoUnidadeConsumidora=%zz" + ucInventada, nil},
		{"corpo que não vira JSON", "POST", "/v1/x", make(chan int)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := c.Do(context.Background(), tc.metodo, tc.path, tc.body, nil)
			if err == nil {
				t.Fatal("esperava erro")
			}
			assertSemVazamento(t, err.Error())
		})
	}
	if srv.reqs.Load() != 0 {
		t.Errorf("nada devia ser enviado: %d", srv.reqs.Load())
	}
}

func TestDo_LogComRotaSemQuery(t *testing.T) {
	t.Parallel()

	srv := novoServidor(t, sequencia(t, resposta{status: 400, corpo: erroJSON("ERR_VALIDACAO_CAMPOS")}))
	logger, buf := novoLogger()
	c, _ := novoClienteTeste(t, srv, WithLogger(logger))

	err := c.Do(context.Background(), "GET", "/v1.1/varejista/migracoes?codigoUnidadeConsumidora="+ucInventada, nil, nil)
	if err == nil {
		t.Fatal("esperava erro")
	}
	debug := porNivel(registros(t, buf), "DEBUG")
	if len(debug) != 1 {
		t.Fatalf("Debug = %d\n%s", len(debug), buf.String())
	}
	if debug[0]["op"] != "do" || debug[0]["rota"] != "/v1.1/varejista/migracoes" || debug[0]["codigo"] != "ERR_VALIDACAO_CAMPOS" {
		t.Errorf("Debug = %v", debug[0])
	}
	assertLogLimpo(t, buf)
}

func TestDo_GrupoDeLimiteDaRota(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want grupoLimite
	}{
		{"migrações usam o geral", "/v1.1/varejista/migracoes", grupoGeral},
		{"medições usam o próprio", "/v1/varejista/consumo/medicoes", grupoMedicoes},
		{"medições com id", "/v1/varejista/consumo/medicoes/abc", grupoMedicoes},
		{"nome parecido não conta", "/v1/medicoesx/outra", grupoGeral},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := grupoDaRota(tc.path); got != tc.want {
				t.Errorf("grupoDaRota(%q) = %v, quer %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestDecodificarSaida_WhenValorNaoCombina_NaoVazaOValor(t *testing.T) {
	t.Parallel()
	type saida struct {
		Consumo json.Number `json:"consumo"`
		Nome    string      `json:"nome"`
	}
	tests := []struct {
		name      string
		corpo     string
		want      string
		wantNunca []string
	}{
		{name: "número com vírgula decimal", corpo: `{"consumo":"1.234,56"}`, want: "decodificar a resposta: JSON inesperado", wantNunca: []string{"1.234", "56"}},
		{name: "texto onde se espera número", corpo: `{"consumo":true}`, want: `campo "consumo" com tipo inesperado (esperado json.Number)`},
		{name: "número onde se espera texto", corpo: `{"nome":1.5}`, want: `campo "nome" com tipo inesperado (esperado string)`, wantNunca: []string{"1.5"}},
		{name: "JSON truncado cita só o byte", corpo: `{"consumo":`, want: "a resposta não é JSON válido (byte "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out saida
			err := decodificarSaida([]byte(tc.corpo), &out)
			if err == nil {
				t.Fatal("quero erro")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("erro %q não contém %q", err, tc.want)
			}
			for _, n := range tc.wantNunca {
				if strings.Contains(err.Error(), n) {
					t.Errorf("erro %q vazou %q", err, n)
				}
			}
		})
	}
}
