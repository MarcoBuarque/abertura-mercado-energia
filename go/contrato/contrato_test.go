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
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	abertura "github.com/MarcoBuarque/abertura-mercado-energia/go"
)

// Roteiro (go test -tags sandbox ./contrato/... -count=1 -v):
//
//	ABERTURA_CLIENT_ID, ABERTURA_CLIENT_SECRET, ABERTURA_AGENTE, ABERTURA_PERFIL,
//	ABERTURA_CONCESSIONARIA   leitura
//	ABERTURA_CONTRATO_ESCRITA=1, ABERTURA_UC e ABERTURA_DOCUMENTO   ligam a
//	    escrita (opt-in). A UC e o documento (CPF de 11 ou CNPJ de 14
//	    caracteres, com dígito verificador válido) vêm da documentação oficial
//	    do sandbox e só do ambiente, nunca do repositório. Faltando qualquer
//	    um, os passos de escrita dão t.Skip.
//
// ATENÇÃO: a rodada de escrita pode deixar uma migração ativa numa UC fictícia
// do sandbox compartilhado (o Criar e as edições seguintes não têm como ser
// desfeitos por este teste). Rode a escrita com consciência disso.
//
// Só o sandbox é alvo. Imprime status, Code e nomes de campo; nunca valores.
// Para na 91ª requisição e no primeiro 403.
//
// Leitura (sem efeito no servidor): token, lista de um mês, lista sem perfil,
// lista sem faixa, Obter, medições e até 3 páginas. A faixa ampla é procurada
// uma vez: começa em 2020-01 até dezembro do ano que vem e, a cada
// ErrFaixaDeMesesLargaDemais, cai pela metade (centrada no mês corrente) até
// a API aceitar; os passos seguintes reaproveitam a faixa aceita. Cada
// resposta ganha, além dos nomes de campo, a máscara de formato dos campos
// temporais (dígito = 9, letra = A), nunca o valor. Escrita (cria e altera
// registro no sandbox): Criar, ConferirCriacao, Editar, ConferirEdicao e o PUT
// parcial pelo Do. Na escrita, ErrValidacaoCampos no Criar, uma conferência
// que não acha migração (depois de procurar a UC numa faixa ampla de meses
// quando o Criar deu ErrMigracaoSolicitada) e a falta de uma migração ativa
// (não CANCELADA, ENCERRADA, EXPIRADA nem REPROVADA) viram achado + t.Skip
// (divergência a registrar, não falha do SDK). Um resultado incerto no Criar
// é conferido com ConferirCriacao e, se a migração existe, a rodada segue;
// qualquer outro erro falha.

type config struct {
	id, segredo                string
	agente, perfil, concession int
	escrita                    bool
	uc                         string
	documento                  abertura.Documento
	// semEscrita explica, na mensagem do t.Skip, por que a escrita está desligada.
	semEscrita string
}

// documentoDoAmbiente monta o documento de ABERTURA_DOCUMENTO: 11 caracteres
// são CPF, 14 são CNPJ (que passou a aceitar letras). Outro tamanho é recusado.
func documentoDoAmbiente(valor string) (abertura.Documento, bool) {
	var d abertura.Documento
	switch n := len(abertura.DocumentoCPF(valor).Numero()); n {
	case 11:
		d = abertura.DocumentoCPF(valor)
	case 14:
		d = abertura.DocumentoCNPJ(valor)
	default:
		return abertura.Documento{}, false
	}
	return d, true
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
	doc, docOK := documentoDoAmbiente(os.Getenv("ABERTURA_DOCUMENTO"))
	c.documento = doc
	switch {
	case os.Getenv("ABERTURA_CONTRATO_ESCRITA") != "1":
		c.semEscrita = "escrita desligada: defina ABERTURA_CONTRATO_ESCRITA=1, ABERTURA_UC e ABERTURA_DOCUMENTO"
	case c.uc == "":
		c.semEscrita = "escrita desligada: falta ABERTURA_UC (UC válida da doc oficial)"
	case !docOK:
		c.semEscrita = "escrita desligada: falta ABERTURA_DOCUMENTO (CPF de 11 ou CNPJ de 14 caracteres, com dígito verificador válido)"
	default:
		c.escrita = true
	}
	return c
}

// limiteBuscaAmpla limita os itens lidos na busca da UC por faixa ampla, para
// não gastar o orçamento de requisições.
const limiteBuscaAmpla = 200

var brasilia = time.FixedZone("-03:00", -3*60*60)

// descreve um erro sem URL nem corpo: só status e Code.
func descreve(err error) string {
	var api *abertura.APIError
	if errors.As(err, &api) {
		return fmt.Sprintf("APIError op=%s status=%d code=%q", api.Operation, api.StatusCode, codigoSeguro(api.Code))
	}
	var inc *abertura.UncertainResultError
	if errors.As(err, &inc) {
		return "resultado incerto em " + inc.Operation
	}
	var tipo *json.UnmarshalTypeError
	if errors.As(err, &tipo) {
		// Field é o caminho JSON, Value o tipo JSON ("string", "number") e
		// Type o tipo Go; nenhum é um valor da resposta.
		return fmt.Sprintf("campo com tipo inesperado na resposta: %s (%s → %s)", tipo.Field, tipo.Value, tipo.Type)
	}
	var sintaxe *json.SyntaxError
	if errors.As(err, &sintaxe) {
		return fmt.Sprintf("resposta não é JSON válido (offset %d)", sintaxe.Offset)
	}
	if m := reCampoIlegivel.FindStringSubmatch(err.Error()); m != nil {
		return "campo ilegível na resposta: " + m[1]
	}
	if errors.Is(err, abertura.ErrNotFound) {
		return "ErrNotFound"
	}
	if t := textoFixoDoSDK(err.Error()); t != "" {
		return t
	}
	return fmt.Sprintf("erro do tipo %T", err)
}

// reCampoIlegivel casa o texto fixo do erro de conversão do SDK
// (erroCampo, go/converter.go). O nome do campo não é um valor.
var reCampoIlegivel = regexp.MustCompile(`o campo "([A-Za-z0-9_.]+)" da resposta tem um valor ilegível`)

// textosFixosDoSDK são os textos de resposta ilegível que o próprio SDK
// escreve (go/migracoes.go, go/do.go). O tipo que os carrega não é exportado,
// então se casa o texto; só o texto fixo casado é impresso, nada além dele.
var textosFixosDoSDK = []string{
	"a API devolveu mais de uma migração para o id",
	"resposta vazia",
	"a resposta não traz uma migração identificável",
	"decodificar a resposta: ",
}

// reJSONInvalido casa o texto fixo do erro de sintaxe do SDK (go/do.go).
var reJSONInvalido = regexp.MustCompile(`a resposta não é JSON válido \(byte \d+\)`)

// textoFixoDoSDK devolve o texto fixo do SDK contido na mensagem, ou vazio.
func textoFixoDoSDK(msg string) string {
	if m := reJSONInvalido.FindString(msg); m != "" {
		return m
	}
	for _, t := range textosFixosDoSDK {
		if strings.Contains(msg, t) {
			return t
		}
	}
	return ""
}

// raizOu devolve a contagem de itens da raiz, ou "raiz: objeto" quando a raiz
// não era uma lista.
func raizOu(raiz string) string {
	if raiz == "" {
		return "raiz: objeto"
	}
	return raiz
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
	faixaDe, faixaAte := faixaInicial(mes)
	var primeiro abertura.Migracao

	// faixaLarga procura, na primeira vez, a maior faixa de meses que a API
	// aceita: começa em 12 meses terminando no mês corrente
	// (faixaDe a faixaAte; teto medido em 01/11/2026 entre 12 e 24) e, a cada
	// ErrFaixaDeMesesLargaDemais, reduz pela metade, centrada no mês corrente
	// (12, 6, 3, 1), com no máximo maxTentativasFaixa requisições.
	// O resultado fica guardado: os passos que listam numa faixa ampla usam a
	// faixa aceita e a primeira página já lida.
	var (
		faixaFeita bool
		faixa      faixaAceita
		faixaErr   error
	)
	faixaLarga := func(t *testing.T) (faixaAceita, error) {
		t.Helper()
		if faixaFeita {
			return faixa, faixaErr
		}
		faixaFeita = true
		faixa, faixaErr = buscarFaixaAceita(faixaDe, faixaAte, mes, func(de, ate abertura.Mes) (abertura.Pagina[abertura.Migracao], error) {
			return c.Migracoes.ListarPagina(ctx, abertura.FiltroMigracoes{De: de, Ate: ate}, "")
		})
		switch {
		case faixaErr != nil:
			achado(t, "faixa de meses: nenhuma faixa testada foi aceita (%s)", descreve(faixaErr))
		case faixa.Recusada == 0:
			achado(t, "faixa de %d meses aceita (teto entre 12 e 24, medido em 01/11/2026)", faixa.Meses)
		default:
			achado(t, "faixa de %d meses aceita; de %d recusada (teto entre %d e %d)", faixa.Meses, faixa.Recusada, faixa.Meses, faixa.Recusada)
		}
		return faixa, faixaErr
	}

	passo("leitura/token e lista de um mês", func(t *testing.T) {
		pag, err := c.Migracoes.ListarPagina(ctx, abertura.FiltroMigracoes{De: mes, Ate: mes}, "")
		var api *abertura.APIError
		if errors.As(err, &api) && api.Operation == "obterToken" {
			achado(t, "token: o endpoint recusou o pedido do SDK; %s", descreve(err))
			t.Fatalf("sem token não há contrato")
		}
		if err != nil {
			t.Fatalf("ListarPagina: %s", descreve(err))
		}
		achado(t, "token: aceito só com client_id e client_secret no formulário (sem grant_type)")
		fora := 0
		for _, m := range pag.Itens {
			if m.Mes != mes {
				fora++
			}
		}
		achado(t, "lista de um mês: %d itens, %d com mês de referência fora do filtro AAAA-MM (0 com itens = o filtro vale; D2), cursor presente: %v",
			len(pag.Itens), fora, pag.Proxima != "")
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
		if errors.Is(err, abertura.ErrFaixaDeMesesObrigatoria) {
			achado(t, "faixa de meses: obrigatória, ErrFaixaDeMesesObrigatoria (%s)", descreve(err))
			return
		}
		if err != nil {
			achado(t, "faixa de meses: parece obrigatória, mas com outro erro (%s)", descreve(err))
			return
		}
		achado(t, "faixa de meses: NÃO é obrigatória (200)")
	})

	passo("leitura/Obter do primeiro item", func(t *testing.T) {
		if primeiro.ID == "" {
			fx, err := faixaLarga(t)
			if err != nil {
				t.Fatalf("lista ampla: %s", descreve(err))
			}
			pag := fx.Pagina
			if len(pag.Itens) == 0 {
				t.Skip("o sandbox não tem migrações visíveis para este perfil")
			}
			primeiro = pag.Itens[0]
		}
		m, err := c.Migracoes.Obter(ctx, primeiro.ID)
		if err != nil {
			achado(t, "Obter: %s; %s (o nome do campo ilegível, se for o caso, é o dado a registrar em FONTES.md)", descreve(err), raizOu(o.raizDe("obterMigracao")))
			t.Fatalf("Obter: %s", descreve(err))
		}
		achado(t, "Obter: ok; %s; mesmo status da lista: %v; mesmo mês da lista: %v", raizOu(o.raizDe("obterMigracao")), m.Status == primeiro.Status, m.Mes == primeiro.Mes)
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
		fx, err := faixaLarga(t)
		if err != nil {
			t.Fatalf("página 1: %s", descreve(err))
		}
		pag := fx.Pagina
		cursor, vistos := "", 0
		for pagina := 1; pagina <= 3; pagina++ {
			if pagina > 1 {
				pag, err = c.Migracoes.ListarPagina(ctx, abertura.FiltroMigracoes{De: fx.De, Ate: fx.Ate}, cursor)
				if err != nil {
					t.Fatalf("página %d: %s", pagina, descreve(err))
				}
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
			t.Skip(cfg.semEscrita)
		})
	} else {
		var id string
		passo("escrita/Criar e ConferirCriacao", func(t *testing.T) {
			chave := abertura.ChaveMigracao{UC: cfg.uc, Concessionaria: cfg.concession, Mes: proximoMes(mes)}
			_, err := c.Migracoes.Criar(ctx, abertura.NovaMigracao{
				UC: cfg.uc, Concessionaria: cfg.concession,
				Documento: cfg.documento, Email: "contrato@example.org",
				Mes: chave.Mes, DataDenuncia: hoje, Comentario: "contrato automatizado",
			})
			var incerto *abertura.UncertainResultError
			solicitada := false
			switch {
			case err == nil:
				achado(t, "Criar: 200")
			case errors.Is(err, abertura.ErrMigracaoSolicitada):
				solicitada = true
				achado(t, "Criar: ErrMigracaoSolicitada (já havia migração ativa)")
			case errors.Is(err, abertura.ErrValidacaoCampos):
				// A API recusou um campo (UC, documento ou mês do ambiente): é
				// achado sobre os dados de teste, não defeito do SDK.
				achado(t, "Criar: ErrValidacaoCampos com a UC e o documento do ambiente; confira os valores e o mês. O passo de escrita foi pulado")
				t.Skip("Criar recusado por validação de campos (achado registrado)")
			case errors.As(err, &incerto):
				// O resultado é incerto: confere abaixo, como o catálogo manda.
				achado(t, "Criar: resultado incerto (%s); conferindo com ConferirCriacao", descreve(err))
			default:
				achado(t, "Criar: %s", descreve(err))
				t.Fatalf("Criar: %s", descreve(err))
			}
			achadas, err := c.Migracoes.ConferirCriacao(ctx, chave)
			if err != nil {
				t.Fatalf("ConferirCriacao: %s", descreve(err))
			}
			achado(t, "ConferirCriacao: %d migração(ões) para a chave", len(achadas))
			if len(achadas) == 0 && solicitada {
				// Com ErrMigracaoSolicitada a migração ativa pode estar em outro
				// mês ou perfil: procura a UC numa faixa ampla de meses.
				achado(t, "ConferirCriacao: vazio no mês da chave; procurando a UC numa faixa ampla de meses")
				fx, err := faixaLarga(t)
				if err != nil {
					t.Fatalf("faixa ampla: %s", descreve(err))
				}
				n := 0
				for m, err := range c.Migracoes.Listar(ctx, abertura.FiltroMigracoes{De: fx.De, Ate: fx.Ate, UC: cfg.uc}) {
					if err != nil {
						t.Fatalf("lista ampla pela UC: %s", descreve(err))
					}
					if n++; n > limiteBuscaAmpla {
						achado(t, "lista ampla pela UC: parei em %d itens", limiteBuscaAmpla)
						break
					}
					achadas = append(achadas, m)
				}
				achadas = daUC(achadas, cfg.uc, cfg.concession)
				achado(t, "faixa ampla: %d migração(ões) da UC e da concessionária", len(achadas))
			}
			if len(achadas) == 0 {
				achado(t, "Criar sem migração localizável depois da conferência; os passos seguintes foram pulados")
				t.Skip("nenhuma migração localizada depois do Criar (achado registrado)")
			}
			ativa, ok := escolherAtiva(achadas)
			if !ok {
				achado(t, "nenhuma das %d migração(ões) da UC está ativa (todas canceladas, encerradas, expiradas ou reprovadas); os passos seguintes foram pulados", len(achadas))
				t.Skip("nenhuma migração ativa para editar (achado registrado)")
			}
			id = ativa.ID
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
