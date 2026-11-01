package abertura

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MarcoBuarque/abertura-mercado-energia/go/internal/testserver"
)

// deTst e ateTst são a faixa de meses que a listagem exige.
var deTst, ateTst = MesDe(2027, time.January), MesDe(2027, time.March)

const (
	modeloLista   = "/v1.1/varejista/migracoes"
	modeloDetalhe = "/v1.1/varejista/migracoes/{idMigracao}"
	modeloEdicao  = "/v1/varejista/migracoes/{idMigracao}"
	idMigracaoTst = "00000000-0000-4000-8000-000000000001"
)

// rotaToken serve o token das fixtures. O testserver valida o pedido de token
// contra a spec.
var rotaToken = testserver.Rota{
	Metodo: http.MethodPost, Modelo: "/sso/oauth/token",
	Respostas: []testserver.Resposta{{Status: http.StatusOK, Fixture: "token.json"}},
}

func rota(metodo, modelo string, respostas ...testserver.Resposta) testserver.Rota {
	return testserver.Rota{Metodo: metodo, Modelo: modelo, Respostas: respostas}
}

func resp(status int, fixture string) testserver.Resposta {
	return testserver.Resposta{Status: status, Fixture: fixture}
}

// clienteSpec monta um cliente contra o testserver, que valida cada
// requisição contra a spec. O log da suíte (log_test.go) fica ligado.
func clienteSpec(t *testing.T, rotas []testserver.Rota, opts ...Option) (*Client, *testserver.Server) {
	t.Helper()
	srv := testserver.New(t, append([]testserver.Rota{rotaToken}, rotas...)...)
	todas := append([]Option{
		WithCredentials(idInventado, segredoInventado),
		WithAgent(agenteInventado, perfilInventado),
		WithRateLimits(RateLimits{}),
		WithLogger(loggerDaSuite()),
	}, opts...)
	c, _ := novoClienteComEnv(t, Environment{
		Name: "teste", APIURL: srv.URL(), TokenURL: srv.URL() + "/sso/oauth/token",
	}, todas...)
	return c, srv
}

// chamadas devolve as requisições à API (sem o token) para o caminho.
func chamadas(srv *testserver.Server, metodo, caminho string) []*http.Request {
	var out []*http.Request
	for _, r := range srv.Requisicoes() {
		if r.Method == metodo && r.URL.Path == caminho {
			out = append(out, r)
		}
	}
	return out
}

func corpoJSON(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("ler corpo: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("corpo não é JSON: %v", err)
	}
	return m
}

func coletarSeq[T any](t *testing.T, seq func(func(T, error) bool)) ([]T, error) {
	t.Helper()
	var itens []T
	for item, err := range seq {
		if err != nil {
			return itens, err
		}
		itens = append(itens, item)
	}
	return itens, nil
}

func TestMigracoesListar_WhenDuasPaginas_PercorreTudo(t *testing.T) {
	t.Parallel()
	c, srv := clienteSpec(t, []testserver.Rota{
		rota(http.MethodGet, modeloLista, resp(200, "migracoes_pagina1.json"), resp(200, "migracoes_pagina2.json")),
	})
	itens, err := coletarSeq(t, c.Migracoes.Listar(context.Background(), FiltroMigracoes{De: deTst, Ate: ateTst}))
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if len(itens) != 3 {
		t.Fatalf("itens = %d, quero 3", len(itens))
	}
	if itens[0].ID != idMigracaoTst || itens[0].Mes != MesDe(2027, time.March) || itens[0].Documento.Numero() != "11111111111111" {
		t.Errorf("primeiro item mal convertido: %v", itens[0])
	}
	reqs := chamadas(srv, http.MethodGet, modeloLista)
	if len(reqs) != 2 {
		t.Fatalf("requisições = %d, quero 2", len(reqs))
	}
	if reqs[0].URL.Query().Has("indexProximaPagina") {
		t.Errorf("a primeira página não leva cursor: %s", reqs[0].URL.RawQuery)
	}
	if got := reqs[1].URL.Query().Get("indexProximaPagina"); got != "cursor-inventado-2" {
		t.Errorf("cursor da segunda página = %q", got)
	}
}

func TestMigracoesListar_WhenFiltros_VaoNaQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		filtro FiltroMigracoes
		opts   []CallOption
		want   url.Values
	}{
		{
			name:   "só a faixa leva o perfil do cliente e os meses",
			filtro: FiltroMigracoes{De: deTst, Ate: ateTst},
			want:   url.Values{"codigoPerfilVarejista": {"900002"}, "mesReferenciaInicial": {"2027-01"}, "mesReferenciaFinal": {"2027-03"}},
		},
		{
			name:   "Perfil do filtro troca o do cliente",
			filtro: FiltroMigracoes{De: deTst, Ate: ateTst, Perfil: 900077},
			want:   url.Values{"codigoPerfilVarejista": {"900077"}, "mesReferenciaInicial": {"2027-01"}, "mesReferenciaFinal": {"2027-03"}},
		},
		{
			name:   "WithAgent na chamada troca o perfil da query",
			filtro: FiltroMigracoes{De: deTst, Ate: ateTst},
			opts:   []CallOption{WithAgent(900005, 900006)},
			want:   url.Values{"codigoPerfilVarejista": {"900006"}, "mesReferenciaInicial": {"2027-01"}, "mesReferenciaFinal": {"2027-03"}},
		},
		{
			name: "meses, UC e status",
			filtro: FiltroMigracoes{
				De: MesDe(2027, time.January), Ate: MesDe(2027, time.December),
				UC: ucInventada, Status: StatusConcluida,
			},
			want: url.Values{
				"codigoPerfilVarejista": {"900002"}, "mesReferenciaInicial": {"2027-01"}, "mesReferenciaFinal": {"2027-12"},
				"codigoUnidadeConsumidora": {ucInventada}, "statusMigracao": {"CONCLUIDA"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, srv := clienteSpec(t, []testserver.Rota{rota(http.MethodGet, modeloLista, resp(200, "migracoes_vazia.json"))})
			if _, err := coletarSeq(t, c.Migracoes.Listar(context.Background(), tc.filtro, tc.opts...)); err != nil {
				t.Fatalf("Listar: %v", err)
			}
			reqs := chamadas(srv, http.MethodGet, modeloLista)
			if len(reqs) != 1 {
				t.Fatalf("requisições = %d, quero 1", len(reqs))
			}
			if got := reqs[0].URL.Query(); !equalValues(got, tc.want) {
				t.Errorf("query = %v, quero %v", got, tc.want)
			}
		})
	}
}

func equalValues(a, b url.Values) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if !slices.Equal(v, b[k]) {
			return false
		}
	}
	return true
}

func TestMigracoesListar_WhenBreak_NaoBuscaAMais(t *testing.T) {
	t.Parallel()
	c, srv := clienteSpec(t, []testserver.Rota{
		rota(http.MethodGet, modeloLista, resp(200, "migracoes_pagina1.json"), resp(200, "migracoes_pagina2.json")),
	})
	for _, err := range c.Migracoes.Listar(context.Background(), FiltroMigracoes{De: deTst, Ate: ateTst}) {
		if err != nil {
			t.Fatalf("Listar: %v", err)
		}
		break
	}
	if n := srv.Contagem(http.MethodGet, modeloLista); n != 1 {
		t.Errorf("requisições = %d, quero 1", n)
	}
}

func TestMigracoesListar_WhenErro_SaiUmaVez(t *testing.T) {
	t.Parallel()
	c, _ := clienteSpec(t, []testserver.Rota{rota(http.MethodGet, modeloLista, resp(400, "erro_400_perfil.json"))})
	itens, err := coletarSeq(t, c.Migracoes.Listar(context.Background(), FiltroMigracoes{De: deTst, Ate: ateTst}))
	if !errors.Is(err, ErrPerfilVarejistaObrigatorio) {
		t.Fatalf("err = %v, quero ErrPerfilVarejistaObrigatorio", err)
	}
	if len(itens) != 0 {
		t.Errorf("itens = %d, quero 0", len(itens))
	}
}

func TestMigracoesListarPagina_WhenCursor_DevolveOProximo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		fixture     string
		cursor      string
		wantItens   int
		wantProxima string
	}{
		{name: "primeira página traz o cursor", fixture: "migracoes_pagina1.json", wantItens: 2, wantProxima: "cursor-inventado-2"},
		{name: "retomar com cursor e ser a última", fixture: "migracoes_pagina2.json", cursor: "cursor-inventado-2", wantItens: 1},
		{name: "vazia", fixture: "migracoes_vazia.json", wantItens: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, srv := clienteSpec(t, []testserver.Rota{rota(http.MethodGet, modeloLista, resp(200, tc.fixture))})
			pag, err := c.Migracoes.ListarPagina(context.Background(), FiltroMigracoes{De: deTst, Ate: ateTst}, tc.cursor)
			if err != nil {
				t.Fatalf("ListarPagina: %v", err)
			}
			if len(pag.Itens) != tc.wantItens || pag.Proxima != tc.wantProxima {
				t.Errorf("página = %d itens, próxima %q; quero %d e %q", len(pag.Itens), pag.Proxima, tc.wantItens, tc.wantProxima)
			}
			reqs := chamadas(srv, http.MethodGet, modeloLista)
			if got := reqs[0].URL.Query().Get("indexProximaPagina"); got != tc.cursor {
				t.Errorf("cursor enviado = %q, quero %q", got, tc.cursor)
			}
		})
	}
}

func TestMigracoesListar_WhenToleranciaDaListagem_Converte(t *testing.T) {
	t.Parallel()
	c, _ := clienteSpec(t, []testserver.Rota{rota(http.MethodGet, modeloLista, resp(200, "migracoes_tolerancia.json"))})
	itens, err := coletarSeq(t, c.Migracoes.Listar(context.Background(), FiltroMigracoes{De: deTst, Ate: ateTst}))
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if len(itens) != 1 || itens[0].Status != "STATUS_NOVO" || itens[0].Status.Conhecido() || itens[0].Mes != MesDe(2026, time.December) {
		t.Errorf("item = %+v", itens)
	}
}

func TestMigracoesObter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		resposta testserver.Resposta
		wantID   string
		wantErr  func(error) bool
	}{
		{name: "array de um item", resposta: resp(200, "migracao_detalhe_array.json"), wantID: idMigracaoTst},
		{name: "objeto", resposta: resp(200, "migracao_detalhe_objeto.json"), wantID: idMigracaoTst},
		{
			name: "array vazio é ErrNotFound", resposta: resp(200, "migracao_detalhe_vazio.json"),
			wantErr: func(err error) bool { return errors.Is(err, ErrNotFound) },
		},
		{name: "três itens com o id no meio devolve o certo", resposta: resp(200, "migracao_detalhe_tres_com_id.json"), wantID: idMigracaoTst},
		{
			name: "três itens sem o id é ErrNotFound", resposta: resp(200, "migracao_detalhe_tres_sem_id.json"),
			wantErr: func(err error) bool { return errors.Is(err, ErrNotFound) },
		},
		{
			name: "id repetido é resposta ilegível", resposta: resp(200, "migracao_detalhe_id_repetido.json"),
			wantErr: func(err error) bool {
				var er *erroResposta
				return errors.As(err, &er) && !errors.Is(err, ErrNotFound) && strings.Contains(err.Error(), "mais de uma")
			},
		},
		{
			name: "404 do servidor é ErrNotFound", resposta: testserver.Resposta{Status: 404},
			wantErr: func(err error) bool { return errors.Is(err, ErrNotFound) },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, srv := clienteSpec(t, []testserver.Rota{rota(http.MethodGet, modeloDetalhe, tc.resposta)})
			m, err := c.Migracoes.Obter(context.Background(), idMigracaoTst)
			if tc.wantErr != nil {
				if !tc.wantErr(err) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Obter: %v", err)
			}
			if m.ID != tc.wantID || m.Mes != MesDe(2027, time.March) {
				t.Errorf("migração = %v", m)
			}
			if got := chamadas(srv, http.MethodGet, "/v1.1/varejista/migracoes/"+idMigracaoTst); len(got) != 1 {
				t.Errorf("requisições ao id = %d, quero 1", len(got))
			}
		})
	}
}

func TestMigracoesObter_WhenIDVazio_NaoFazRequisicao(t *testing.T) {
	t.Parallel()
	c, srv := clienteSpec(t, []testserver.Rota{rota(http.MethodGet, modeloDetalhe, resp(200, "migracao_detalhe_array.json"))})
	if _, err := c.Migracoes.Obter(context.Background(), " "); err == nil {
		t.Fatal("esperava erro")
	}
	if n := srv.Contagem(http.MethodGet, modeloDetalhe); n != 0 {
		t.Errorf("requisições = %d, quero 0", n)
	}
}

func novaTeste() NovaMigracao {
	return NovaMigracao{
		UC: ucInventada, Concessionaria: 900001,
		Documento: DocumentoCNPJ("11.111.111/1111-11"), Email: "contato@example.org",
		Mes: MesDe(2027, time.March), DataDenuncia: time.Date(2027, 1, 15, 0, 0, 0, 0, fusoBrasilia),
		Comentario: "Inventado: exemplo de comentário",
	}
}

func TestMigracoesCriar_WhenSucesso_EnviaDocumentoEmClaro(t *testing.T) {
	t.Parallel()
	c, srv := clienteSpec(t, []testserver.Rota{rota(http.MethodPost, modeloLista, resp(200, "migracao_criada.json"))})
	m, err := c.Migracoes.Criar(context.Background(), novaTeste())
	if err != nil {
		t.Fatalf("Criar: %v", err)
	}
	if m.ID != "00000000-0000-4000-8000-000000000005" || m.Status != StatusCriada {
		t.Errorf("migração = %v", m)
	}
	reqs := chamadas(srv, http.MethodPost, modeloLista)
	if len(reqs) != 1 {
		t.Fatalf("requisições = %d, quero 1", len(reqs))
	}
	corpo := corpoJSON(t, reqs[0])
	want := map[string]any{
		"numeroDocumento": "11111111111111", "tipoDocumento": "CNPJ", "mesReferencia": "2027-03",
		"dataDenuncia": "2027-01-15", "codigoUnidadeConsumidora": ucInventada,
		"codigoAgenteConcessionaria": float64(900001), "codigoAgenteVarejista": float64(agenteInventado),
		"codigoPerfilVarejista": float64(perfilInventado), "emailUnidadeConsumidora": "contato@example.org",
	}
	for k, v := range want {
		if corpo[k] != v {
			t.Errorf("corpo[%s] = %v, quero %v", k, corpo[k], v)
		}
	}
}

func TestMigracoesCriar_WhenFalha(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		resposta     testserver.Resposta
		wantSentinel error
		wantIncerto  bool
	}{
		{name: "400 de migração solicitada", resposta: resp(400, "erro_400_migracao_solicitada.json"), wantSentinel: ErrMigracaoSolicitada},
		{name: "400 de validação", resposta: resp(400, "erro_400_validacao.json"), wantSentinel: ErrValidacaoCampos},
		{name: "502 é resultado incerto", resposta: resp(502, "erro_502.txt"), wantIncerto: true},
		{
			name: "2xx com corpo ilegível é resultado incerto", resposta: resp(200, "erro_502.txt"),
			wantIncerto: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, srv := clienteSpec(t, []testserver.Rota{rota(http.MethodPost, modeloLista, tc.resposta, resp(200, "migracao_criada.json"))})
			_, err := c.Migracoes.Criar(context.Background(), novaTeste())
			if n := srv.Contagem(http.MethodPost, modeloLista); n != 1 {
				t.Errorf("POSTs = %d, quero 1 (escrita não repete)", n)
			}
			if tc.wantSentinel != nil {
				if !errors.Is(err, tc.wantSentinel) {
					t.Fatalf("err = %v, quero %v", err, tc.wantSentinel)
				}
				var inc *UncertainResultError
				if errors.As(err, &inc) {
					t.Errorf("não devia ser incerto: %v", err)
				}
				return
			}
			var inc *UncertainResultError
			if !errors.As(err, &inc) {
				t.Fatalf("err = %v, quero *UncertainResultError", err)
			}
			want := ChaveMigracao{UC: ucInventada, Concessionaria: 900001, Mes: MesDe(2027, time.March), Perfil: perfilInventado}
			if inc.Operation != "criarMigracao" || inc.Key != want {
				t.Errorf("incerto = %q %v, quero criarMigracao %v", inc.Operation, inc.Key, want)
			}
		})
	}
}

func TestMigracoesConferirCriacao(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		fixture string
		chave   ChaveMigracao
		wantIDs []string // mais recente primeiro
	}{
		{
			name: "filtra a concessionária e ordena da mais recente", fixture: "migracoes_conferencia.json",
			chave:   ChaveMigracao{UC: ucInventada, Concessionaria: 900001, Mes: MesDe(2027, time.March)},
			wantIDs: []string{"00000000-0000-4000-8000-000000000013", "00000000-0000-4000-8000-000000000011"},
		},
		{
			name: "concessionária sem migração dá vazio", fixture: "migracoes_conferencia.json",
			chave: ChaveMigracao{UC: ucInventada, Concessionaria: 900042, Mes: MesDe(2027, time.March)},
		},
		{
			name: "lista vazia dá vazio", fixture: "migracoes_vazia.json",
			chave: ChaveMigracao{UC: ucInventada, Concessionaria: 900001, Mes: MesDe(2027, time.March)},
		},
		{
			name: "Perfil da chave troca o do cliente", fixture: "migracoes_vazia.json",
			chave: ChaveMigracao{UC: ucInventada, Concessionaria: 900001, Mes: MesDe(2027, time.March), Perfil: 900077},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, srv := clienteSpec(t, []testserver.Rota{rota(http.MethodGet, modeloLista, resp(200, tc.fixture))})
			got, err := c.Migracoes.ConferirCriacao(context.Background(), tc.chave)
			if err != nil {
				t.Fatalf("ConferirCriacao: %v", err)
			}
			if got == nil {
				t.Error("o resultado vazio deve ser uma fatia não nula")
			}
			var ids []string
			for _, m := range got {
				ids = append(ids, m.ID)
			}
			if !slices.Equal(ids, tc.wantIDs) {
				t.Errorf("ids = %v, quero %v", ids, tc.wantIDs)
			}
			q := chamadas(srv, http.MethodGet, modeloLista)[0].URL.Query()
			wantPerfil := "900002"
			if tc.chave.Perfil != 0 {
				wantPerfil = "900077"
			}
			if q.Get("codigoUnidadeConsumidora") != ucInventada || q.Get("mesReferenciaInicial") != "2027-03" ||
				q.Get("mesReferenciaFinal") != "2027-03" || q.Get("codigoPerfilVarejista") != wantPerfil {
				t.Errorf("query = %v", q)
			}
		})
	}
}

func TestMigracoesEditar_WhenSucesso_FazGetEDepoisPutComOObjetoInteiro(t *testing.T) {
	t.Parallel()
	c, srv := clienteSpec(t, []testserver.Rota{
		rota(http.MethodGet, modeloDetalhe, resp(200, "migracao_detalhe_array.json")),
		rota(http.MethodPut, modeloEdicao, resp(200, "migracao_detalhe_objeto.json")),
	})
	m, err := c.Migracoes.Editar(context.Background(), idMigracaoTst, func(e *EdicaoMigracao) { e.Email = "novo@example.org" })
	if err != nil {
		t.Fatalf("Editar: %v", err)
	}
	if m.ID == "" {
		t.Errorf("migração vazia: %v", m)
	}
	var ordem []string
	for _, r := range srv.Requisicoes() {
		if r.URL.Path != "/sso/oauth/token" {
			ordem = append(ordem, r.Method)
		}
	}
	if !slices.Equal(ordem, []string{http.MethodGet, http.MethodPut}) {
		t.Fatalf("ordem = %v, quero GET e PUT", ordem)
	}
	puts := chamadas(srv, http.MethodPut, "/v1/varejista/migracoes/"+idMigracaoTst)
	corpo := corpoJSON(t, puts[0])
	want := map[string]any{
		"mesReferencia": "2027-03", "codigoPerfilVarejista": float64(900003), "tipoDocumento": "CNPJ",
		"numeroDocumento": "11111111111111", "emailUnidadeConsumidora": "novo@example.org", "statusMigracao": "CRIADA",
	}
	if len(corpo) != len(want) {
		t.Errorf("corpo = %v, quero exatamente %v", corpo, want)
	}
	for k, v := range want {
		if corpo[k] != v {
			t.Errorf("corpo[%s] = %v, quero %v", k, corpo[k], v)
		}
	}
}

func TestMigracoesEditar_WhenFalha(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		get      testserver.Resposta
		put      testserver.Resposta
		mudar    func(*EdicaoMigracao)
		wantPuts int
		check    func(*testing.T, error)
	}{
		{
			name: "PUT 502 é incerto com ChaveEdicao", get: resp(200, "migracao_detalhe_array.json"), put: resp(502, "erro_502.txt"),
			mudar: func(e *EdicaoMigracao) { e.Email = "novo@example.org" }, wantPuts: 1,
			check: func(t *testing.T, err error) {
				var inc *UncertainResultError
				if !errors.As(err, &inc) {
					t.Fatalf("err = %v, quero incerto", err)
				}
				k, ok := inc.Key.(ChaveEdicao)
				if !ok || k.ID != idMigracaoTst || k.Desejado.Email != "novo@example.org" || k.Desejado.Perfil != 900003 {
					t.Errorf("chave = %#v", inc.Key)
				}
			},
		},
		{
			name: "migração inexistente não chega ao PUT", get: resp(200, "migracao_detalhe_vazio.json"), put: resp(200, "migracao_detalhe_objeto.json"),
			mudar: func(*EdicaoMigracao) {}, wantPuts: 0,
			check: func(t *testing.T, err error) {
				if !errors.Is(err, ErrNotFound) {
					t.Errorf("err = %v, quero ErrNotFound", err)
				}
			},
		},
		{
			name: "função de mudança nula", get: resp(200, "migracao_detalhe_array.json"), put: resp(200, "migracao_detalhe_objeto.json"),
			mudar: nil, wantPuts: 0,
			check: func(t *testing.T, err error) {
				if err == nil {
					t.Error("esperava erro")
				}
			},
		},
		{
			name: "PUT 400 é erro da API", get: resp(200, "migracao_detalhe_array.json"), put: resp(400, "erro_400_validacao.json"),
			mudar: func(e *EdicaoMigracao) { e.Status = StatusCancelada }, wantPuts: 1,
			check: func(t *testing.T, err error) {
				if !errors.Is(err, ErrValidacaoCampos) {
					t.Errorf("err = %v, quero ErrValidacaoCampos", err)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, srv := clienteSpec(t, []testserver.Rota{
				rota(http.MethodGet, modeloDetalhe, tc.get),
				rota(http.MethodPut, modeloEdicao, tc.put),
			})
			_, err := c.Migracoes.Editar(context.Background(), idMigracaoTst, tc.mudar)
			tc.check(t, err)
			if n := srv.Contagem(http.MethodPut, modeloEdicao); n != tc.wantPuts {
				t.Errorf("PUTs = %d, quero %d", n, tc.wantPuts)
			}
		})
	}
}

func TestMigracoesConferirEdicao(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		desejado EdicaoMigracao
		wantBate bool
	}{
		{name: "campos que batem", desejado: EdicaoMigracao{Email: "contato@example.org", Perfil: 900003, Status: StatusCriada}, wantBate: true},
		{name: "mês e documento que batem", desejado: EdicaoMigracao{Mes: MesDe(2027, time.March), Documento: DocumentoCNPJ("11111111111111")}, wantBate: true},
		{name: "campo zero é ignorado", desejado: EdicaoMigracao{}, wantBate: true},
		{name: "e-mail diferente", desejado: EdicaoMigracao{Email: "novo@example.org"}},
		{name: "status diferente", desejado: EdicaoMigracao{Status: StatusCancelada}},
		{name: "mês diferente", desejado: EdicaoMigracao{Mes: MesDe(2027, time.April)}},
		{name: "perfil diferente", desejado: EdicaoMigracao{Perfil: 900099}},
		{name: "documento diferente", desejado: EdicaoMigracao{Documento: DocumentoCPF("22222222222")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := clienteSpec(t, []testserver.Rota{rota(http.MethodGet, modeloDetalhe, resp(200, "migracao_detalhe_array.json"))})
			m, bate, err := c.Migracoes.ConferirEdicao(context.Background(), ChaveEdicao{ID: idMigracaoTst, Desejado: tc.desejado})
			if err != nil {
				t.Fatalf("ConferirEdicao: %v", err)
			}
			if bate != tc.wantBate {
				t.Errorf("bate = %v, quero %v", bate, tc.wantBate)
			}
			if m.ID != idMigracaoTst {
				t.Errorf("migração atual = %v", m)
			}
		})
	}
}

func TestMigracoesConferirEdicao_WhenNaoExiste_DaErrNotFound(t *testing.T) {
	t.Parallel()
	c, _ := clienteSpec(t, []testserver.Rota{rota(http.MethodGet, modeloDetalhe, resp(200, "migracao_detalhe_vazio.json"))})
	_, bate, err := c.Migracoes.ConferirEdicao(context.Background(), ChaveEdicao{ID: idMigracaoTst})
	if !errors.Is(err, ErrNotFound) || bate {
		t.Errorf("bate = %v, err = %v", bate, err)
	}
}

// As chaves naturais precisam satisfazer a interface; o compilador confere.
var (
	_ ChaveNatural = ChaveMigracao{}
	_ ChaveNatural = ChaveEdicao{}
)

func TestMigracoesListar_WhenFaixaDeMesesInvalida_FalhaSemRequisicao(t *testing.T) {
	t.Parallel()
	jan, fev := MesDe(2027, time.January), MesDe(2027, time.February)
	tests := []struct {
		name      string
		filtro    FiltroMigracoes
		wantErr   bool
		wantCampo string
	}{
		{name: "sem De nem Ate", filtro: FiltroMigracoes{}, wantErr: true, wantCampo: "De"},
		{name: "só De", filtro: FiltroMigracoes{De: jan}, wantErr: true, wantCampo: "Ate"},
		{name: "só Ate", filtro: FiltroMigracoes{Ate: jan}, wantErr: true, wantCampo: "De"},
		{name: "Ate antes de De", filtro: FiltroMigracoes{De: fev, Ate: jan}, wantErr: true, wantCampo: "Ate"},
		{name: "mês 0 em De", filtro: FiltroMigracoes{De: Mes{Ano: 2027, Mes: 0}, Ate: jan}, wantErr: true, wantCampo: "De"},
		{name: "mês 13 em De", filtro: FiltroMigracoes{De: Mes{Ano: 2027, Mes: 13}, Ate: Mes{Ano: 2028, Mes: time.January}}, wantErr: true, wantCampo: "De"},
		{name: "mês 0 em Ate", filtro: FiltroMigracoes{De: jan, Ate: Mes{Ano: 2027, Mes: 0}}, wantErr: true, wantCampo: "Ate"},
		{name: "mês 13 em Ate", filtro: FiltroMigracoes{De: jan, Ate: Mes{Ano: 2027, Mes: 13}}, wantErr: true, wantCampo: "Ate"},
		{name: "ano 0 em De", filtro: FiltroMigracoes{De: Mes{Ano: 0, Mes: time.March}, Ate: jan}, wantErr: true, wantCampo: "De"},
		{name: "faixa de um mês", filtro: FiltroMigracoes{De: jan, Ate: jan}},
		{name: "faixa válida", filtro: FiltroMigracoes{De: jan, Ate: fev}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, via := range []string{"ListarPagina", "Listar"} {
				c, srv := clienteSpec(t, []testserver.Rota{
					rota(http.MethodGet, modeloLista, resp(200, "migracoes_vazia.json")),
				})
				var err error
				if via == "ListarPagina" {
					_, err = c.Migracoes.ListarPagina(context.Background(), tc.filtro, "")
				} else {
					_, err = coletarSeq(t, c.Migracoes.Listar(context.Background(), tc.filtro))
				}
				n := srv.Contagem(http.MethodGet, modeloLista)
				if !tc.wantErr {
					if err != nil || n != 1 {
						t.Errorf("%s: err = %v, requisições = %d; quero nil e 1", via, err, n)
					}
					continue
				}
				if !errors.Is(err, ErrArgumentoInvalido) {
					t.Errorf("%s: err = %v, quero ErrArgumentoInvalido", via, err)
				} else if !strings.Contains(err.Error(), tc.wantCampo) {
					t.Errorf("%s: a mensagem %q não cita o campo %s", via, err.Error(), tc.wantCampo)
				}
				if n != 0 || len(srv.Requisicoes()) != 0 {
					t.Errorf("%s: houve %d requisição(ões) antes da validação local", via, len(srv.Requisicoes()))
				}
			}
		})
	}
}
