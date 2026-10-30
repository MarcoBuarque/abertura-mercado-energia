package abertura_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	abertura "github.com/MarcoBuarque/abertura-mercado-energia/go"
)

// exemploAmbiente sobe um servidor local que imita a API com as fixtures de
// testdata/, para os Examples rodarem sem rede nem credenciais. Devolve o
// ambiente e a função que encerra o servidor.
//
// Rotas: o token; a listagem de migrações (páginas 1 e 2 pelo cursor, e a
// resposta de conferência quando a consulta traz faixa de meses); o POST de
// migração, que responde 502; as medições; e as solicitações.
func exemploAmbiente() (abertura.Environment, func()) {
	raiz, err := os.OpenRoot("testdata")
	if err != nil {
		log.Fatalf("exemplo: abrir testdata: %v", err)
	}
	servir := func(w http.ResponseWriter, status int, fixture string) {
		corpo, err := raiz.ReadFile(fixture)
		if err != nil {
			log.Printf("exemplo: ler fixture: %v", err)
			http.Error(w, "fixture ausente", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write(corpo); err != nil {
			log.Printf("exemplo: escrever resposta: %v", err)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sso/oauth/token", func(w http.ResponseWriter, _ *http.Request) { servir(w, 200, "token.json") })
	mux.HandleFunc("GET /v1.1/varejista/migracoes", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("mesReferenciaInicial") != "":
			servir(w, 200, "migracoes_conferencia.json")
		case q.Get("indexProximaPagina") != "":
			servir(w, 200, "migracoes_pagina2.json")
		default:
			servir(w, 200, "migracoes_pagina1.json")
		}
	})
	mux.HandleFunc("POST /v1.1/varejista/migracoes", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	})
	mux.HandleFunc("GET /v1/varejista/consumo/medicoes", func(w http.ResponseWriter, _ *http.Request) {
		servir(w, 200, "medicoes_pagina1.json")
	})
	mux.HandleFunc("GET /v1/solicitacoes", func(w http.ResponseWriter, _ *http.Request) {
		servir(w, 200, "solicitacoes_pagina1.json")
	})
	srv := httptest.NewServer(mux)
	return abertura.Environment{
			Name: "exemplo", APIURL: srv.URL, TokenURL: srv.URL + "/sso/oauth/token",
		}, func() {
			srv.Close()
			if err := raiz.Close(); err != nil {
				log.Printf("exemplo: fechar testdata: %v", err)
			}
		}
}

// clienteExemplo cria o cliente dos Examples. Num programa de verdade, o
// ambiente é abertura.Sandbox ou abertura.Production e as credenciais vêm do
// seu cofre de segredos ou do ambiente do processo.
func clienteExemplo(env abertura.Environment) *abertura.Client {
	c, err := abertura.NewClient(env,
		abertura.WithCredentials("id-inventado", "segredo-inventado"),
		abertura.WithAgent(900002, 900003),
		abertura.WithRateLimits(abertura.RateLimits{}),
	)
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}
	return c
}

// Percorre todas as migrações do perfil, página a página. O documento sai
// mascarado em qualquer impressão.
func Example_listarMigracoes() {
	env, fim := exemploAmbiente()
	defer fim()
	c := clienteExemplo(env)

	for m, err := range c.Migracoes.Listar(context.Background(), abertura.FiltroMigracoes{}) {
		if err != nil {
			log.Fatalf("Listar: %v", err)
		}
		fmt.Println(m.ID, m.Status, m.Mes, m.Documento)
	}
	// Output:
	// 00000000-0000-4000-8000-000000000001 CRIADA 2027-03 CNPJ ************11
	// 00000000-0000-4000-8000-000000000002 CONCLUIDA 2027-03 CPF *********22
	// 00000000-0000-4000-8000-000000000003 CANCELADA 2027-03 CPF *********33
}

// Criar não repete a escrita. Se a resposta se perde, o erro é um
// *UncertainResultError com a chave natural da migração, e ConferirCriacao diz
// se ela chegou a ser criada.
func ExampleMigracoes_Criar_resultadoIncerto() {
	env, fim := exemploAmbiente()
	defer fim()
	c := clienteExemplo(env)
	ctx := context.Background()

	_, err := c.Migracoes.Criar(ctx, abertura.NovaMigracao{
		UC:             "000000000000001",
		Concessionaria: 900001,
		Documento:      abertura.DocumentoCNPJ("11.111.111/1111-11"),
		Email:          "contato@example.org",
		Mes:            abertura.MesDe(2027, time.March),
		DataDenuncia:   time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC),
	})
	var incerto *abertura.UncertainResultError
	if !errors.As(err, &incerto) {
		log.Fatalf("esperava resultado incerto, veio %v", err)
	}
	fmt.Println("resultado incerto em", incerto.Operation)

	chave, ok := incerto.Key.(abertura.ChaveMigracao)
	if !ok {
		log.Fatalf("chave inesperada: %T", incerto.Key)
	}
	achadas, err := c.Migracoes.ConferirCriacao(ctx, chave)
	if err != nil {
		log.Fatalf("ConferirCriacao: %v", err)
	}
	fmt.Println("migrações encontradas:", len(achadas))
	// Output:
	// resultado incerto em criarMigracao
	// migrações encontradas: 2
}

// ListarPagina devolve o cursor da próxima página, para retomar a leitura
// depois. O consumo é um Decimal exato; a API não declara a unidade.
func ExampleMedicoes_ListarPagina() {
	env, fim := exemploAmbiente()
	defer fim()
	c := clienteExemplo(env)

	pag, err := c.Medicoes.ListarPagina(context.Background(), abertura.FiltroMedicoes{
		UC:             "000000000000001",
		Concessionaria: 900001,
		Inicio:         time.Date(2027, 2, 1, 0, 0, 0, 0, time.FixedZone("", -3*3600)),
		Fim:            time.Date(2027, 2, 28, 23, 45, 0, 0, time.FixedZone("", -3*3600)),
	}, "")
	if err != nil {
		log.Fatalf("ListarPagina: %v", err)
	}
	for _, m := range pag.Itens {
		fmt.Println(m.Dia.Format("2006-01-02"), m.Consumo, m.Situacao)
	}
	fmt.Println("próxima página:", pag.Proxima)
	// Output:
	// 2027-02-01 12.345 CONSISTIDA
	// próxima página: cursor-inventado-2
}

// O Documento nunca se imprime em claro: fmt, slog e JSON mostram só o texto
// mascarado. Numero devolve o número, por chamada explícita.
func ExampleDocumento() {
	d := abertura.DocumentoCNPJ("11.111.111/1111-11")

	fmt.Println(d)
	fmt.Printf("%+v\n", d)
	texto, err := d.MarshalText()
	if err != nil {
		log.Fatalf("MarshalText: %v", err)
	}
	fmt.Println(string(texto))
	fmt.Println(d.Numero())
	// Output:
	// CNPJ ************11
	// CNPJ ************11
	// CNPJ ************11
	// 11111111111111
}

// Do chama uma rota que o SDK ainda não cobre, como a listagem de
// solicitações. Passa pelo mesmo laço de token, limites, retries e erros.
func ExampleClient_Do() {
	env, fim := exemploAmbiente()
	defer fim()
	c := clienteExemplo(env)

	var lista struct {
		Solicitacao []struct {
			ID       string `json:"idSolicitacaoAlteracao"`
			Situacao string `json:"situacaoAlteracao"`
			Tipo     string `json:"tipoSolicitacao"`
		} `json:"solicitacao"`
	}
	err := c.Do(context.Background(), http.MethodGet,
		"/v1/solicitacoes?tipoSolicitacao=SUSPENSAO_FORNECIMENTO_RESILICAO", nil, &lista)
	if err != nil {
		log.Fatalf("Do: %v", err)
	}
	for _, s := range lista.Solicitacao {
		fmt.Println(s.ID, s.Situacao, s.Tipo)
	}
	// Output:
	// 00000000-0000-4000-8000-0000000000b1 CRIADA SUSPENSAO_FORNECIMENTO_RESILICAO
}
