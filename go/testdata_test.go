package abertura_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/MarcoBuarque/abertura-mercado-energia/go/internal/spec"
)

// carregarRoteador lê a spec embutida e monta um roteador sem servidores, que
// casa só pelo caminho.
func carregarRoteador(t *testing.T) routers.Router {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(spec.Abertura)
	if err != nil {
		t.Fatalf("carregar spec: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("spec inválida: %v", err)
	}
	doc.Servers = nil
	r, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatalf("montar roteador: %v", err)
	}
	return r
}

// TestFixturesValidamContraSpec garante a cadeia sandbox, spec, fixture, SDK:
// cada fixture JSON precisa ser uma resposta válida da operação a que serve.
func TestFixturesValidamContraSpec(t *testing.T) {
	t.Parallel()
	const id = "00000000-0000-4000-8000-000000000001"
	tests := []struct {
		name    string
		fixture string
		method  string
		path    string
		status  int
	}{
		{name: "token", fixture: "token.json", method: http.MethodPost, path: "/sso/oauth/token", status: 200},
		{name: "migrações página 1", fixture: "migracoes_pagina1.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes", status: 200},
		{name: "migrações página 2", fixture: "migracoes_pagina2.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes", status: 200},
		{name: "migrações vazia", fixture: "migracoes_vazia.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes", status: 200},
		{name: "migrações com campos novos, status novo e mês antigo", fixture: "migracoes_tolerancia.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes", status: 200},
		{name: "detalhe em array", fixture: "migracao_detalhe_array.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes/" + id, status: 200},
		{name: "detalhe com dois itens", fixture: "migracao_detalhe_dois.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes/" + id, status: 200},
		{name: "migrações de concessionárias e datas diferentes", fixture: "migracoes_conferencia.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes", status: 200},
		{name: "detalhe vazio", fixture: "migracao_detalhe_vazio.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes/" + id, status: 200},
		{name: "migração criada", fixture: "migracao_criada.json", method: http.MethodPost, path: "/v1.1/varejista/migracoes", status: 200},
		{name: "migração editada devolve o detalhe", fixture: "migracao_detalhe_objeto.json", method: http.MethodPut, path: "/v1/varejista/migracoes/" + id, status: 200},
		{name: "medições com consumo string", fixture: "medicoes_pagina1.json", method: http.MethodGet, path: "/v1/varejista/consumo/medicoes", status: 200},
		{name: "medições com consumo número", fixture: "medicoes_pagina2.json", method: http.MethodGet, path: "/v1/varejista/consumo/medicoes", status: 200},
		{name: "medições vazia", fixture: "medicoes_vazia.json", method: http.MethodGet, path: "/v1/varejista/consumo/medicoes", status: 200},
		{name: "400 validação", fixture: "erro_400_validacao.json", method: http.MethodPost, path: "/v1.1/varejista/migracoes", status: 400},
		{name: "400 perfil obrigatório", fixture: "erro_400_perfil.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes", status: 400},
		{name: "400 migração solicitada", fixture: "erro_400_migracao_solicitada.json", method: http.MethodPost, path: "/v1.1/varejista/migracoes", status: 400},
		{name: "403 autorizador", fixture: "erro_403_autorizador.json", method: http.MethodGet, path: "/v1.1/varejista/migracoes", status: 403},
	}
	router := carregarRoteador(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			corpo, err := os.ReadFile(filepath.Join("testdata", tc.fixture))
			if err != nil {
				t.Fatalf("ler fixture: %v", err)
			}
			req := httptest.NewRequest(tc.method, tc.path, nil)
			route, pathParams, err := router.FindRoute(req)
			if err != nil {
				t.Fatalf("rota %s %s fora da spec: %v", tc.method, tc.path, err)
			}
			header := http.Header{"Content-Type": {"application/json"}, "X-Amzn-Requestid": {"00000000-0000-4000-8000-0000000000f1"}}
			if tc.status == 403 {
				header.Set("X-Amzn-Errortype", "AccessDeniedException")
			}
			in := &openapi3filter.ResponseValidationInput{
				RequestValidationInput: &openapi3filter.RequestValidationInput{
					Request: req, PathParams: pathParams, Route: route,
				},
				Status: tc.status,
				Header: header,
				Options: &openapi3filter.Options{
					IncludeResponseStatus: true,
				},
			}
			in.SetBodyBytes(bytes.TrimSpace(corpo))
			if err := openapi3filter.ValidateResponse(context.Background(), in); err != nil {
				t.Errorf("fixture %s inválida para %s %s (%d): %v", tc.fixture, tc.method, tc.path, tc.status, err)
			}
		})
	}
}

// TestFixturesInventadas barra valores que lembrem dados reais: UC com zeros
// à frente, agentes na faixa 900000 e e-mail em example.org.
func TestFixturesInventadas(t *testing.T) {
	t.Parallel()
	raiz, err := os.OpenRoot("testdata")
	if err != nil {
		t.Fatalf("abrir testdata: %v", err)
	}
	t.Cleanup(func() {
		if err := raiz.Close(); err != nil {
			t.Errorf("fechar testdata: %v", err)
		}
	})
	arquivos, err := filepath.Glob(filepath.Join("testdata", "*"))
	if err != nil {
		t.Fatalf("listar fixtures: %v", err)
	}
	if len(arquivos) == 0 {
		t.Fatal("nenhuma fixture em testdata")
	}
	for _, a := range arquivos {
		t.Run(filepath.Base(a), func(t *testing.T) {
			t.Parallel()
			b, err := raiz.ReadFile(filepath.Base(a))
			if err != nil {
				t.Fatalf("ler: %v", err)
			}
			for _, linha := range strings.Split(string(b), "\n") {
				l := strings.TrimSpace(linha)
				if strings.Contains(l, "@") && !strings.Contains(l, "@example.org") {
					t.Errorf("e-mail fora de example.org: %s", l)
				}
				if strings.Contains(l, "\"codigoUnidadeConsumidora\"") && !strings.Contains(l, "\"00000") {
					t.Errorf("UC sem zeros à frente: %s", l)
				}
			}
		})
	}
}
