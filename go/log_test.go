package abertura

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/MarcoBuarque/abertura-mercado-energia/go/internal/testserver"
)

// bufferLog é o buffer de log compartilhado pela suíte inteira.
type bufferLog struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *bufferLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *bufferLog) texto() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

var logDaSuite = &bufferLog{}

// loggerDaSuite devolve um logger no nível Debug que grava no buffer da
// suíte. Os testes dos métodos o usam, e TestLogSemDadoPessoal confere o
// buffer.
func loggerDaSuite() *slog.Logger {
	return slog.New(slog.NewTextHandler(logDaSuite, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// termosProibidos são os dados das fixtures e dos testes que nenhum log pode
// conter: UCs, documentos, e-mails, o segredo e o esquema do token.
var termosProibidos = []string{
	"000000000000001", "000000000000002", "000000000000003", "000000000000004", "000000000000005",
	"11111111111111", "22222222222", "33333333333",
	"@example.org", "contato@", "outro@", "novo@",
	segredoInventado, idInventado, "token-inventado", "Bearer",
}

// vazamentos devolve os termos proibidos que aparecem no texto.
func vazamentos(texto string) []string {
	var achados []string
	for _, termo := range termosProibidos {
		if strings.Contains(texto, termo) {
			achados = append(achados, termo)
		}
	}
	return achados
}

// TestMain confere, no fim da suíte inteira, que o log gravado pelos testes
// dos métodos não vazou dado pessoal.
func TestMain(m *testing.M) {
	codigo := m.Run()
	if codigo == 0 {
		if achados := vazamentos(logDaSuite.texto()); len(achados) > 0 {
			log.Fatalf("o log da suíte contém dado que não podia vazar: %q", achados)
		}
	}
	os.Exit(codigo)
}

// TestLogSemDadoPessoal roda os cenários dos métodos (sucesso, erro da API,
// resultado incerto, conferências) com um logger em Debug num buffer próprio e
// procura nele UCs, documentos, e-mails, o segredo e "Bearer". Nada pode
// aparecer, nem quando a entidade é logada por inteiro.
func TestLogSemDadoPessoal(t *testing.T) {
	buf := &bufferLog{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := context.Background()
	opt := WithLogger(logger)

	var ultima Migracao
	cenarios := []struct {
		name  string
		rotas []testserver.Rota
		rodar func(t *testing.T, c *Client)
	}{
		{
			name:  "listar duas páginas",
			rotas: []testserver.Rota{rota(http.MethodGet, modeloLista, resp(200, "migracoes_pagina1.json"), resp(200, "migracoes_pagina2.json"))},
			rodar: func(t *testing.T, c *Client) {
				itens, err := coletarSeq(t, c.Migracoes.Listar(ctx, FiltroMigracoes{De: deTst, Ate: ateTst, UC: ucInventada}))
				if err != nil || len(itens) == 0 {
					t.Fatalf("Listar: %d itens, %v", len(itens), err)
				}
				ultima = itens[0]
			},
		},
		{
			name:  "obter",
			rotas: []testserver.Rota{rota(http.MethodGet, modeloDetalhe, resp(200, "migracao_detalhe_array.json"))},
			rodar: func(t *testing.T, c *Client) {
				if _, err := c.Migracoes.Obter(ctx, idMigracaoTst); err != nil {
					t.Fatalf("Obter: %v", err)
				}
			},
		},
		{
			name:  "criar com sucesso",
			rotas: []testserver.Rota{rota(http.MethodPost, modeloLista, resp(200, "migracao_criada.json"))},
			rodar: func(t *testing.T, c *Client) {
				if _, err := c.Migracoes.Criar(ctx, novaTeste()); err != nil {
					t.Fatalf("Criar: %v", err)
				}
			},
		},
		{
			name:  "criar com migração já solicitada",
			rotas: []testserver.Rota{rota(http.MethodPost, modeloLista, resp(400, "erro_400_migracao_solicitada.json"))},
			rodar: func(t *testing.T, c *Client) {
				if _, err := c.Migracoes.Criar(ctx, novaTeste()); !errors.Is(err, ErrMigracaoSolicitada) {
					t.Fatalf("Criar: %v", err)
				}
			},
		},
		{
			name:  "criar com resultado incerto e conferir",
			rotas: []testserver.Rota{rota(http.MethodPost, modeloLista, resp(502, "erro_502.txt")), rota(http.MethodGet, modeloLista, resp(200, "migracoes_conferencia.json"))},
			rodar: func(t *testing.T, c *Client) {
				_, err := c.Migracoes.Criar(ctx, novaTeste())
				var inc *UncertainResultError
				if !errors.As(err, &inc) {
					t.Fatalf("Criar: %v", err)
				}
				chave, ok := inc.Key.(ChaveMigracao)
				if !ok {
					t.Fatalf("chave = %#v", inc.Key)
				}
				if _, err := c.Migracoes.ConferirCriacao(ctx, chave); err != nil {
					t.Fatalf("ConferirCriacao: %v", err)
				}
				logger.Warn("incerto", "chave", inc.Key.String(), "erro", inc)
			},
		},
		{
			name: "editar e conferir",
			rotas: []testserver.Rota{
				rota(http.MethodGet, modeloDetalhe, resp(200, "migracao_detalhe_array.json")),
				rota(http.MethodPut, modeloEdicao, resp(200, "migracao_detalhe_objeto.json")),
			},
			rodar: func(t *testing.T, c *Client) {
				mig, err := c.Migracoes.Editar(ctx, idMigracaoTst, func(e *EdicaoMigracao) { e.Email = "novo@example.org" })
				if err != nil {
					t.Fatalf("Editar: %v", err)
				}
				chave := ChaveEdicao{ID: idMigracaoTst, Desejado: EdicaoMigracao{Email: "novo@example.org", Documento: mig.Documento}}
				if _, _, err := c.Migracoes.ConferirEdicao(ctx, chave); err != nil {
					t.Fatalf("ConferirEdicao: %v", err)
				}
				logger.Info("edição", "migracao", mig, "chave", chave.String(), "documento", mig.Documento)
			},
		},
		{
			name: "editar com PUT incerto",
			rotas: []testserver.Rota{
				rota(http.MethodGet, modeloDetalhe, resp(200, "migracao_detalhe_array.json")),
				rota(http.MethodPut, modeloEdicao, resp(502, "erro_502.txt")),
			},
			rodar: func(t *testing.T, c *Client) {
				_, err := c.Migracoes.Editar(ctx, idMigracaoTst, func(e *EdicaoMigracao) { e.Status = StatusCancelada })
				var inc *UncertainResultError
				if !errors.As(err, &inc) {
					t.Fatalf("Editar: %v", err)
				}
			},
		},
		{
			name:  "medições",
			rotas: []testserver.Rota{rota(http.MethodGet, modeloMedicoes, resp(200, "medicoes_pagina1.json"), resp(200, "medicoes_pagina2.json"))},
			rodar: func(t *testing.T, c *Client) {
				itens, err := coletarSeq(t, c.Medicoes.Listar(ctx, filtroMedicoesTeste()))
				if err != nil || len(itens) != 2 {
					t.Fatalf("Listar: %d itens, %v", len(itens), err)
				}
				logger.Info("medição", "medicao", itens[0])
			},
		},
	}
	for _, tc := range cenarios {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := clienteSpec(t, tc.rotas, opt)
			tc.rodar(t, c)
		})
	}
	logger.Info("migração do primeiro cenário", "migracao", ultima)

	texto := buf.texto()
	for _, esperado := range []string{"op=listarMigracoes", "op=criarMigracao", "op=editarMigracao", "op=listarMedicoes", "resultado incerto"} {
		if !strings.Contains(texto, esperado) {
			t.Errorf("o log não tem %q: o teste não está exercitando o log", esperado)
		}
	}
	if achados := vazamentos(texto); len(achados) > 0 {
		t.Errorf("o log contém dado que não podia vazar: %q\n%s", achados, texto)
	}
}

// TestChavesEErrosSemDadoPessoal registra e formata as chaves naturais e os
// erros por todos os caminhos que um chamador usa (slog.JSONHandler,
// slog.TextHandler, %v, %+v, %#v e %s) e confere que nada vaza UC, documento,
// e-mail, consumo, token ou corpo.
func TestChavesEErrosSemDadoPessoal(t *testing.T) {
	t.Parallel()

	corpo := []byte(`{"error":"ERR_X","message":"m","uc":"000000000000001","email":"contato@example.org"}`)
	apiErr := &APIError{Operation: "criarMigracao", StatusCode: 502, rawBody: corpo, metodo: http.MethodPost}
	chaveMig := ChaveMigracao{UC: ucInventada, Concessionaria: 999001, Mes: Mes{}, Perfil: 1}
	chaveEd := ChaveEdicao{ID: idMigracaoTst, Desejado: EdicaoMigracao{Email: "novo@example.org", Perfil: 1}}
	incerto := &UncertainResultError{Operation: "criarMigracao", Key: chaveMig, Err: apiErr}
	incertoEd := &UncertainResultError{Operation: "editarMigracao", Key: chaveEd, Err: apiErr}

	valores := []struct {
		name string
		v    any
	}{
		{"ChaveMigracao", chaveMig},
		{"ChaveMigracao ponteiro", &chaveMig},
		{"ChaveEdicao", chaveEd},
		{"ChaveEdicao ponteiro", &chaveEd},
		{"APIError", apiErr},
		{"UncertainResultError", incerto},
		{"UncertainResultError de edição", incertoEd},
		{"ChaveNatural da incerteza", incerto.Key},
	}
	for _, tc := range valores {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			saidas := map[string]string{
				"%v": fmt.Sprintf("%v", tc.v), "%+v": fmt.Sprintf("%+v", tc.v),
				"%#v": fmt.Sprintf("%#v", tc.v), "%s": fmt.Sprintf("%s", tc.v),
			}
			var bj, bt bytes.Buffer
			slog.New(slog.NewJSONHandler(&bj, nil)).Info("registro", "valor", tc.v)
			slog.New(slog.NewTextHandler(&bt, nil)).Info("registro", "valor", tc.v)
			saidas["slog json"] = bj.String()
			saidas["slog texto"] = bt.String()
			for nome, saida := range saidas {
				if achados := vazamentos(saida); len(achados) > 0 {
					t.Errorf("%s vazou %q: %s", nome, achados, saida)
				}
				// O corpo cru, em claro ou em hexadecimal.
				if strings.Contains(saida, "ERR_X") || strings.Contains(saida, "7b226572726f72") || strings.Contains(strings.ToLower(saida), "0x7b") {
					t.Errorf("%s vazou o corpo: %s", nome, saida)
				}
			}
		})
	}

	t.Run("slog json da chave dentro do erro", func(t *testing.T) {
		t.Parallel()
		var b bytes.Buffer
		l := slog.New(slog.NewJSONHandler(&b, nil))
		l.Warn("incerto", "chave", incerto.Key, "erro", incerto, "edicao", incertoEd.Key)
		if achados := vazamentos(b.String()); len(achados) > 0 {
			t.Errorf("vazou %q: %s", achados, b.String())
		}
	})
}
