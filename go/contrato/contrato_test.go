//go:build sandbox

package contrato

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	abertura "github.com/MarcoBuarque/abertura-mercado-energia/go"
)

// Roteiro (go test -tags sandbox ./contrato/... -count=1 -v):
//
//	ABERTURA_CLIENT_ID, ABERTURA_CLIENT_SECRET, ABERTURA_AGENTE, ABERTURA_PERFIL,
//	ABERTURA_CONCESSIONARIA   leitura
//	ABERTURA_CONTRATO_ESCRITA=1 e ABERTURA_UC   liga a escrita (opt-in)
//
// Só o sandbox é alvo. Imprime status, Code e nomes de campo; nunca valores.
// Para na 91ª requisição e no primeiro 403.

type config struct {
	id, segredo                string
	agente, perfil, concession int
	escrita                    bool
	uc                         string
}

func lerConfig(t *testing.T) config {
	t.Helper()
	var c config
	c.id, c.segredo = os.Getenv("ABERTURA_CLIENT_ID"), os.Getenv("ABERTURA_CLIENT_SECRET")
	nums := map[string]*int{"ABERTURA_AGENTE": &c.agente, "ABERTURA_PERFIL": &c.perfil, "ABERTURA_CONCESSIONARIA": &c.concession}
	faltam := c.id == "" || c.segredo == ""
	for nome, dest := range nums {
		n, err := strconv.Atoi(os.Getenv(nome))
		if err != nil || n <= 0 {
			faltam = true
			continue
		}
		*dest = n
	}
	if faltam {
		t.Skip("contrato: defina ABERTURA_CLIENT_ID, ABERTURA_CLIENT_SECRET, ABERTURA_AGENTE, ABERTURA_PERFIL e ABERTURA_CONCESSIONARIA")
	}
	c.uc = os.Getenv("ABERTURA_UC")
	c.escrita = os.Getenv("ABERTURA_CONTRATO_ESCRITA") == "1" && c.uc != ""
	return c
}

var brasilia = time.FixedZone("-03:00", -3*60*60)

// descreve um erro sem URL nem corpo: só status e Code.
func descreve(err error) string {
	var api *abertura.APIError
	if errors.As(err, &api) {
		return fmt.Sprintf("APIError op=%s status=%d code=%q", api.Operation, api.StatusCode, api.Code)
	}
	var inc *abertura.UncertainResultError
	if errors.As(err, &inc) {
		return "resultado incerto em " + inc.Operation
	}
	return fmt.Sprintf("erro do tipo %T", err)
}

func TestContrato(t *testing.T) {
	cfg := lerConfig(t)
	o, err := novoOrcamento(http.DefaultTransport, limiteRequisicoes)
	if err != nil {
		t.Fatalf("orçamento: %v", err)
	}
	hc := &http.Client{Transport: o, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	c, err := abertura.NewClient(abertura.Sandbox,
		abertura.WithCredentials(cfg.id, cfg.segredo),
		abertura.WithAgent(cfg.agente, cfg.perfil),
		abertura.WithHTTPClient(hc),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()
	var achados []string
	achado := func(t *testing.T, formato string, args ...any) {
		t.Helper()
		l := fmt.Sprintf(formato, args...)
		achados = append(achados, l)
		t.Log("ACHADO: " + l)
	}
	// passo roda um subteste, pula se a rodada abortou e despeja as linhas.
	passo := func(nome string, f func(t *testing.T)) {
		t.Run(nome, func(t *testing.T) {
			if o.abortada.Load() {
				t.Skip("rodada abortada (primeiro 403 ou orçamento esgotado)")
			}
			t.Cleanup(func() {
				for _, l := range o.descarregar() {
					t.Log(l)
				}
			})
			f(t)
		})
	}

	hoje := time.Now().In(brasilia)
	mes := abertura.MesDe(hoje.Year(), hoje.Month())
	faixaDe, faixaAte := abertura.MesDe(2020, time.January), abertura.MesDe(hoje.Year()+1, time.December)
	var primeiro abertura.Migracao

	passo("leitura/token e lista de um mês", func(t *testing.T) {
		pag, err := c.Migracoes.ListarPagina(ctx, abertura.FiltroMigracoes{De: mes, Ate: mes}, "")
		var api *abertura.APIError
		if errors.As(err, &api) && api.Operation == "obterToken" {
			achado(t, "token: o endpoint recusou o pedido do SDK (grant_type extra provavelmente); %s. Plano B: TokenSource próprio", descreve(err))
			t.Fatalf("sem token não há contrato")
		}
		if err != nil {
			t.Fatalf("ListarPagina: %s", descreve(err))
		}
		achado(t, "token: aceito com o grant_type que o SDK envia (client_credentials)")
		achado(t, "lista de um mês: %d itens, cursor presente: %v", len(pag.Itens), pag.Proxima != "")
		if len(pag.Itens) > 0 {
			primeiro = pag.Itens[0]
		}
	})

	passo("leitura/lista sem perfil pelo Do", func(t *testing.T) {
		q := url.Values{"mesReferenciaInicial": {mes.String()}, "mesReferenciaFinal": {mes.String()}}
		var raw json.RawMessage
		err := c.Do(ctx, http.MethodGet, "/v1.1/varejista/migracoes?"+q.Encode(), nil, &raw)
		switch {
		case errors.Is(err, abertura.ErrPerfilVarejistaObrigatorio):
			achado(t, "lista sem perfil na query: ErrPerfilVarejistaObrigatorio, como a spec diz")
		case err == nil:
			achado(t, "lista sem perfil na query: DIVERGE, a API respondeu 200")
			t.Error("esperava ErrPerfilVarejistaObrigatorio")
		default:
			achado(t, "lista sem perfil na query: erro diferente do esperado: %s", descreve(err))
			t.Error("esperava ErrPerfilVarejistaObrigatorio")
		}
	})

	passo("leitura/lista sem faixa de meses pelo Do", func(t *testing.T) {
		q := url.Values{"codigoPerfilVarejista": {strconv.Itoa(cfg.perfil)}}
		var raw json.RawMessage
		err := c.Do(ctx, http.MethodGet, "/v1.1/varejista/migracoes?"+q.Encode(), nil, &raw)
		if err != nil {
			achado(t, "faixa de meses: parece obrigatória (%s)", descreve(err))
			return
		}
		achado(t, "faixa de meses: NÃO é obrigatória (200)")
	})

	passo("leitura/Obter do primeiro item", func(t *testing.T) {
		if primeiro.ID == "" {
			pag, err := c.Migracoes.ListarPagina(ctx, abertura.FiltroMigracoes{De: faixaDe, Ate: faixaAte}, "")
			if err != nil {
				t.Fatalf("lista ampla: %s", descreve(err))
			}
			if len(pag.Itens) == 0 {
				t.Skip("o sandbox não tem migrações visíveis para este perfil")
			}
			primeiro = pag.Itens[0]
		}
		m, err := c.Migracoes.Obter(ctx, primeiro.ID)
		if err != nil {
			t.Fatalf("Obter: %s", descreve(err))
		}
		achado(t, "Obter: ok; mesmo status da lista: %v; mesmo mês da lista: %v", m.Status == primeiro.Status, m.Mes == primeiro.Mes)
	})

	passo("leitura/medições", func(t *testing.T) {
		if primeiro.UC == "" {
			t.Skip("sem UC de uma migração para consultar medições")
		}
		pag, err := c.Medicoes.ListarPagina(ctx, abertura.FiltroMedicoes{
			UC: primeiro.UC, Concessionaria: cfg.concession,
			Inicio: time.Date(hoje.Year(), hoje.Month(), 1, 0, 0, 0, 0, brasilia).AddDate(0, -1, 0),
			Fim:    time.Date(hoje.Year(), hoje.Month(), 1, 0, 0, 0, 0, brasilia).Add(-15 * time.Minute),
		}, "")
		if err != nil {
			achado(t, "medições: %s", descreve(err))
			t.Fatalf("ListarPagina: %s", descreve(err))
		}
		achado(t, "medições: %d itens, cursor presente: %v", len(pag.Itens), pag.Proxima != "")
	})

	passo("leitura/até 3 páginas numa faixa ampla", func(t *testing.T) {
		cursor, vistos := "", 0
		for pagina := 1; pagina <= 3; pagina++ {
			pag, err := c.Migracoes.ListarPagina(ctx, abertura.FiltroMigracoes{De: faixaDe, Ate: faixaAte}, cursor)
			if err != nil {
				t.Fatalf("página %d: %s", pagina, descreve(err))
			}
			vistos += len(pag.Itens)
			if pag.Proxima == "" {
				achado(t, "paginação: %d página(s), %d itens, cursor NÃO apareceu na última", pagina, vistos)
				return
			}
			if pag.Proxima == cursor {
				achado(t, "paginação: cursor repetido na página %d (ErrCursorRepetido protege o Listar)", pagina)
				return
			}
			cursor = pag.Proxima
		}
		achado(t, "paginação: o cursor apareceu e levou a 3 páginas, %d itens", vistos)
	})

	if !cfg.escrita {
		t.Run("escrita", func(t *testing.T) {
			t.Skip("escrita desligada: defina ABERTURA_CONTRATO_ESCRITA=1 e ABERTURA_UC (UC válida da doc oficial)")
		})
	} else {
		var id string
		passo("escrita/Criar e ConferirCriacao", func(t *testing.T) {
			chave := abertura.ChaveMigracao{UC: cfg.uc, Concessionaria: cfg.concession, Mes: proximoMes(mes)}
			_, err := c.Migracoes.Criar(ctx, abertura.NovaMigracao{
				UC: cfg.uc, Concessionaria: cfg.concession,
				Documento: abertura.DocumentoCNPJ("11111111111111"), Email: "contrato@example.org",
				Mes: chave.Mes, DataDenuncia: hoje, Comentario: "contrato automatizado",
			})
			switch {
			case err == nil:
				achado(t, "Criar: 200")
			case errors.Is(err, abertura.ErrMigracaoSolicitada):
				achado(t, "Criar: ErrMigracaoSolicitada (já havia migração ativa)")
			default:
				achado(t, "Criar: %s", descreve(err))
				t.Fatalf("Criar: %s", descreve(err))
			}
			achadas, err := c.Migracoes.ConferirCriacao(ctx, chave)
			if err != nil {
				t.Fatalf("ConferirCriacao: %s", descreve(err))
			}
			achado(t, "ConferirCriacao: %d migração(ões) para a chave", len(achadas))
			if len(achadas) == 0 {
				t.Fatal("ConferirCriacao devolveu vazio depois do Criar")
			}
			id = achadas[0].ID
		})
		passo("escrita/Editar e ConferirEdicao", func(t *testing.T) {
			if id == "" {
				t.Skip("sem migração")
			}
			const email = "contrato-editado@example.org"
			if _, err := c.Migracoes.Editar(ctx, id, func(e *abertura.EdicaoMigracao) { e.Email = email }); err != nil {
				achado(t, "Editar (GET + PUT do objeto inteiro): %s", descreve(err))
				t.Fatalf("Editar: %s", descreve(err))
			}
			achado(t, "Editar (GET + PUT do objeto inteiro): ok")
			_, bate, err := c.Migracoes.ConferirEdicao(ctx, abertura.ChaveEdicao{ID: id, Desejado: abertura.EdicaoMigracao{Email: email}})
			if err != nil {
				t.Fatalf("ConferirEdicao: %s", descreve(err))
			}
			achado(t, "ConferirEdicao: bate = %v", bate)
			if !bate {
				t.Error("o e-mail devia ter mudado")
			}
		})
		passo("escrita/PUT parcial pelo Do", func(t *testing.T) {
			if id == "" {
				t.Skip("sem migração")
			}
			antes, err := c.Migracoes.Obter(ctx, id)
			if err != nil {
				t.Fatalf("Obter antes: %s", descreve(err))
			}
			const email = "contrato-parcial@example.org"
			err = c.Do(ctx, http.MethodPut, "/v1/varejista/migracoes/"+url.PathEscape(id),
				map[string]string{"emailUnidadeConsumidora": email}, nil)
			if err != nil {
				achado(t, "PUT parcial: recusado (%s): o PUT parece ser total", descreve(err))
				return
			}
			depois, err := c.Migracoes.Obter(ctx, id)
			if err != nil {
				t.Fatalf("Obter depois: %s", descreve(err))
			}
			preservou := depois.Mes == antes.Mes && depois.Perfil == antes.Perfil &&
				depois.Documento.Numero() == antes.Documento.Numero() && depois.Status == antes.Status
			achado(t, "PUT parcial: aceito; e-mail mudou: %v; demais campos preservados: %v (se preservou, o PUT é parcial)",
				depois.Email == email, preservou)
		})
	}

	t.Run("resumo", func(t *testing.T) {
		t.Logf("requisições usadas: %d de %d", o.usadas(), limiteRequisicoes)
		for _, v := range o.todasViolacoes() {
			t.Errorf("resposta diverge da spec: %s", v)
		}
		t.Log("Para openapi/FONTES.md (D?, x-confianca, x-verificado-em):")
		for _, a := range achados {
			t.Log("  - " + a)
		}
	})
}

// proximoMes devolve o mês seguinte.
func proximoMes(m abertura.Mes) abertura.Mes {
	if m.Mes == time.December {
		return abertura.MesDe(m.Ano+1, time.January)
	}
	return abertura.MesDe(m.Ano, m.Mes+1)
}
