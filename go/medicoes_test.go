package abertura

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/MarcoBuarque/abertura-mercado-energia/go/internal/testserver"
)

const modeloMedicoes = "/v1/varejista/consumo/medicoes"

func filtroMedicoesTeste() FiltroMedicoes {
	return FiltroMedicoes{
		UC: ucInventada, Concessionaria: 900001,
		Inicio: time.Date(2027, 2, 1, 3, 0, 0, 0, time.UTC), // meia-noite em -03:00
		Fim:    time.Date(2027, 2, 28, 23, 45, 0, 0, fusoBrasilia),
	}
}

func TestMedicoesListar_WhenDuasPaginas_ConsumoStringENumero(t *testing.T) {
	t.Parallel()
	c, srv := clienteSpec(t, []testserver.Rota{
		rota(http.MethodGet, modeloMedicoes, resp(200, "medicoes_pagina1.json"), resp(200, "medicoes_pagina2.json")),
	})
	itens, err := coletarSeq(t, c.Medicoes.Listar(context.Background(), filtroMedicoesTeste()))
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if len(itens) != 2 {
		t.Fatalf("itens = %d, quero 2", len(itens))
	}
	if itens[0].Consumo.String() != "12.345" || itens[1].Consumo.String() != "0.5" {
		t.Errorf("consumos = %v e %v", itens[0].Consumo, itens[1].Consumo)
	}
	if itens[0].Concessionaria != 900001 || itens[1].Concessionaria != 900001 {
		t.Errorf("concessionárias = %d e %d", itens[0].Concessionaria, itens[1].Concessionaria)
	}
	if itens[0].Situacao != SituacaoConsistida || !itens[0].Dia.Equal(time.Date(2027, 2, 1, 0, 0, 0, 0, fusoBrasilia)) {
		t.Errorf("primeira medição = %v", itens[0])
	}
	reqs := chamadas(srv, http.MethodGet, modeloMedicoes)
	if len(reqs) != 2 {
		t.Fatalf("requisições = %d, quero 2", len(reqs))
	}
	want := url.Values{
		"codigoUnidadeConsumidora": {ucInventada}, "codigoAgenteConcessionaria": {"900001"},
		"dataInicio": {"2027-02-01T00:00:00-03:00"}, "dataFim": {"2027-02-28T23:45:00-03:00"},
	}
	if got := reqs[0].URL.Query(); !equalValues(got, want) {
		t.Errorf("query da primeira página = %v, quero %v", got, want)
	}
	want.Set("indexProximaPagina", "cursor-inventado-2")
	if got := reqs[1].URL.Query(); !equalValues(got, want) {
		t.Errorf("query da segunda página = %v, quero %v", got, want)
	}
	if reqs[0].URL.Query().Has("codigoPerfilVarejista") {
		t.Error("medições não levam o perfil na query")
	}
}

func TestMedicoesListar_WhenSituacao_VaiNaQuery(t *testing.T) {
	t.Parallel()
	c, srv := clienteSpec(t, []testserver.Rota{rota(http.MethodGet, modeloMedicoes, resp(200, "medicoes_vazia.json"))})
	f := filtroMedicoesTeste()
	f.Situacao = SituacaoConsistida
	itens, err := coletarSeq(t, c.Medicoes.Listar(context.Background(), f))
	if err != nil || len(itens) != 0 {
		t.Fatalf("itens = %d, err = %v", len(itens), err)
	}
	if got := chamadas(srv, http.MethodGet, modeloMedicoes)[0].URL.Query().Get("situacaoMedicao"); got != "CONSISTIDA" {
		t.Errorf("situacaoMedicao = %q", got)
	}
}

func TestMedicoesListarPagina_WhenCursor_DevolveOProximo(t *testing.T) {
	t.Parallel()
	c, _ := clienteSpec(t, []testserver.Rota{rota(http.MethodGet, modeloMedicoes, resp(200, "medicoes_pagina1.json"))})
	pag, err := c.Medicoes.ListarPagina(context.Background(), filtroMedicoesTeste(), "")
	if err != nil {
		t.Fatalf("ListarPagina: %v", err)
	}
	if len(pag.Itens) != 1 || pag.Proxima != "cursor-inventado-2" {
		t.Errorf("página = %d itens, próxima %q", len(pag.Itens), pag.Proxima)
	}
}

func TestMedicoesListar_WhenFiltroIncompleto_DaErroSemRequisicao(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		mudar func(*FiltroMedicoes)
	}{
		{name: "sem UC", mudar: func(f *FiltroMedicoes) { f.UC = "" }},
		{name: "sem concessionária", mudar: func(f *FiltroMedicoes) { f.Concessionaria = 0 }},
		{name: "sem início", mudar: func(f *FiltroMedicoes) { f.Inicio = time.Time{} }},
		{name: "sem fim", mudar: func(f *FiltroMedicoes) { f.Fim = time.Time{} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, srv := clienteSpec(t, []testserver.Rota{rota(http.MethodGet, modeloMedicoes, resp(200, "medicoes_vazia.json"))})
			f := filtroMedicoesTeste()
			tc.mudar(&f)
			if _, err := coletarSeq(t, c.Medicoes.Listar(context.Background(), f)); err == nil {
				t.Fatal("esperava erro")
			}
			if _, err := c.Medicoes.ListarPagina(context.Background(), f, ""); err == nil {
				t.Fatal("ListarPagina: esperava erro")
			}
			if n := srv.Contagem(http.MethodGet, modeloMedicoes); n != 0 {
				t.Errorf("requisições = %d, quero 0", n)
			}
		})
	}
}
