package abertura

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// cenarios são situações representativas de uma entrada do catálogo, com o
// que decidir responde a cada uma. O rótulo "" é o caso sem condição; os
// outros rótulos ("nada saiu", "saiu", "com marca", "sem marca") são os dois
// lados de uma regra condicional escrita na tabela.
type cenarios map[string]desfecho

// especRepetir diz, para cada entrada, como montar o desfecho nas duas
// colunas "Repetir" da tabela-resumo de docs/erros.md. Uma coluna ausente não
// tem decisão do transporte a comparar (o texto não vem de decidir).
type especRepetir struct{ get, escrita cenarios }

func umCenario(d desfecho) cenarios { return cenarios{"": d} }

func comStatus(status int) cenarios { return umCenario(desfecho{status: status}) }

func faltaEscrever(falha tipoFalha) cenarios {
	return cenarios{
		"nada saiu": {falha: falha, escreveu: false},
		"saiu":      {falha: falha, escreveu: true},
	}
}

var especDeRepetir = map[string]especRepetir{
	"credencial-invalida-autorizador": {
		// Sem token já aceito, o SDK não renova: o erro sai. A renovação
		// automática está no texto e é coberta pelo teste da regra do 403.
		get:     umCenario(desfecho{status: 403, gateway: true, credencialInvalida: true, renovavel: true}),
		escrita: umCenario(desfecho{status: 403, gateway: true, credencialInvalida: true, renovavel: true}),
	},
	"credencial-invalida-backend":  {get: comStatus(403), escrita: comStatus(403)},
	"validacao-campos":             {get: comStatus(400), escrita: comStatus(400)},
	"perfil-varejista-obrigatorio": {get: comStatus(400), escrita: comStatus(400)},
	"migracao-solicitada":          {get: comStatus(400), escrita: comStatus(400)},
	"faixa-de-meses-obrigatoria":   {get: comStatus(400), escrita: comStatus(400)},
	"faixa-de-meses-larga-demais":  {get: comStatus(400), escrita: comStatus(400)},
	"codigo-desconhecido":          {get: comStatus(400), escrita: comStatus(400)},
	"recusa-sem-codigo":            {get: comStatus(403), escrita: comStatus(403)},
	"nao-encontrado":               {get: comStatus(404), escrita: comStatus(404)},
	"limite-de-requisicoes": {
		get: comStatus(429),
		escrita: cenarios{
			"com marca": {status: 429, gateway: true},
			"sem marca": {status: 429},
		},
	},
	"erro-interno":     {get: comStatus(500), escrita: comStatus(500)},
	"falha-do-gateway": {get: comStatus(502), escrita: comStatus(502)},
	"erro-de-transporte": {
		get:     umCenario(desfecho{falha: falhaTransporte, escreveu: true}),
		escrita: faltaEscrever(falhaTransporte),
	},
	"tempo-esgotado": {
		get:     umCenario(desfecho{falha: falhaTimeout, escreveu: true}),
		escrita: faltaEscrever(falhaTimeout),
	},
	"chamada-cancelada": {
		get:     umCenario(desfecho{falha: falhaContexto, escreveu: true}),
		escrita: faltaEscrever(falhaContexto),
	},
	// A coluna de GET diz "não se aplica": só a escrita vem de decidir.
	"resultado-incerto": {escrita: comStatus(502)},
	"resposta-ilegivel": {
		get:     umCenario(desfecho{status: 200, corpoIlegivel: true}),
		escrita: umCenario(desfecho{status: 200, corpoIlegivel: true}),
	},
	// Sem decidir: erro local do SDK, ou texto de outra fonte.
	"cursor-repetido":      {},
	"falha-ao-obter-token": {},
	"argumento-invalido":   {},
	"fora-do-catalogo":     {},
}

// reLinhaResumo casa as linhas de dados da tabela-resumo: "| [Título](#slug) | …".
var reLinhaResumo = regexp.MustCompile(`^\|\s*\[[^\]]+\]\(#([a-z0-9-]+)\)\s*\|(.*)\|\s*$`)

// colunasRepetir devolve, por slug, o texto das colunas "Repetir um GET" e
// "Repetir uma escrita" (as duas últimas de cada linha).
func colunasRepetir(t *testing.T, doc string) map[string][2]string {
	t.Helper()
	out := map[string][2]string{}
	for _, linha := range strings.Split(doc, "\n") {
		m := reLinhaResumo.FindStringSubmatch(linha)
		if m == nil {
			continue
		}
		celulas := strings.Split(m[2], "|")
		if len(celulas) != 4 {
			t.Fatalf("linha do resumo com %d colunas depois da entrada, quero 4: %s", len(celulas), linha)
		}
		out[m[1]] = [2]string{strings.TrimSpace(celulas[2]), strings.TrimSpace(celulas[3])}
	}
	return out
}

// respostas aplica decidir a cada cenário.
func respostas(metodo string, cs cenarios) map[string]acao {
	out := make(map[string]acao, len(cs))
	for rotulo, d := range cs {
		d.maxRetries = 1 // só para a decisão ser "repetir", não "esgotou"
		out[rotulo] = decidir(metodo, d)
	}
	return out
}

// confereCelula compara o texto de uma célula com o que decidir respondeu. O
// texto sem regra conhecida falha: quem muda o catálogo escolhe a regra aqui.
func confereCelula(texto string, r map[string]acao) string {
	t := strings.ToLower(texto)
	tem := func(rotulo string, quer acao) bool { a, ok := r[rotulo]; return ok && a == quer }
	switch {
	case strings.HasPrefix(t, "sim se nada saiu"):
		if !tem("nada saiu", acaoRepetir) || !tem("saiu", acaoIncerto) {
			return "decidir não repete quando nada saiu e é incerto quando saiu: " + fmtRespostas(r)
		}
	case strings.HasPrefix(t, "só com a marca do gateway"):
		if !tem("com marca", acaoRepetir) || !tem("sem marca", acaoErro) {
			return "decidir só repete com a marca do gateway: " + fmtRespostas(r)
		}
	case strings.HasPrefix(t, "incerto se já saiu"):
		if !tem("saiu", acaoIncerto) || !tem("nada saiu", acaoErro) {
			return "decidir não dá incerto só quando já saiu: " + fmtRespostas(r)
		}
	case strings.HasPrefix(t, "só você decide"):
		if !tem("", acaoErro) {
			return "decidir não devolve o erro sem repetir: " + fmtRespostas(r)
		}
	case strings.HasPrefix(t, "sim"):
		if !tem("", acaoRepetir) {
			return "decidir não manda repetir: " + fmtRespostas(r)
		}
	case strings.HasPrefix(t, "incerto"):
		if !tem("", acaoIncerto) {
			return "decidir não dá incerto: " + fmtRespostas(r)
		}
	case strings.HasPrefix(t, "não"):
		if tem("", acaoRepetir) {
			return "decidir manda repetir, e o catálogo diz não: " + fmtRespostas(r)
		}
	default:
		return "texto sem regra de paridade; acrescente uma regra em confereCelula"
	}
	return ""
}

func fmtRespostas(r map[string]acao) string {
	var partes []string
	for rotulo, a := range r {
		if rotulo == "" {
			rotulo = "único"
		}
		partes = append(partes, rotulo+"="+a.String())
	}
	return strings.Join(partes, ", ")
}

// TestCatalogo_WhenRepetirNaTabela_BateComDecidir prova que as colunas
// "Repetir um GET" e "Repetir uma escrita" de docs/erros.md dizem o mesmo que
// decidir responde para uma situação representativa de cada entrada.
func TestCatalogo_WhenRepetirNaTabela_BateComDecidir(t *testing.T) {
	t.Parallel()

	colunas := colunasRepetir(t, lerArquivo(t, "../docs/erros.md"))
	for slug := range catalogo {
		if _, ok := colunas[slug]; !ok {
			t.Errorf("entrada %q sem linha na tabela-resumo de docs/erros.md", slug)
		}
		if _, ok := especDeRepetir[slug]; !ok {
			t.Errorf("entrada %q sem cenário em especDeRepetir", slug)
		}
	}
	for slug, cols := range colunas {
		spec, ok := especDeRepetir[slug]
		if !ok {
			continue
		}
		t.Run(slug, func(t *testing.T) {
			t.Parallel()
			get := cols[0]
			if spec.get != nil {
				if msg := confereCelula(get, respostas(http.MethodGet, spec.get)); msg != "" {
					t.Errorf("coluna GET %q: %s", get, msg)
				}
			}
			if spec.escrita != nil {
				escrita := cols[1]
				if strings.EqualFold(escrita, "idem") {
					escrita = get
				}
				if msg := confereCelula(escrita, respostas(http.MethodPost, spec.escrita)); msg != "" {
					t.Errorf("coluna escrita %q: %s", escrita, msg)
				}
			}
		})
	}
}

// TestConfereCelula_WhenTextoDivergeDeDecidir_Reprova prova que o teste de
// paridade pega a divergência que motivou a checagem: o catálogo diz
// "repetir" onde o código não repete.
func TestConfereCelula_WhenTextoDivergeDeDecidir_Reprova(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		texto   string
		metodo  string
		cs      cenarios
		wantMsg bool
	}{
		{name: "sim onde decidir repete", texto: "sim", metodo: http.MethodGet, cs: comStatus(500)},
		{name: "sim onde decidir dá incerto", texto: "sim", metodo: http.MethodPost, cs: comStatus(500), wantMsg: true},
		{name: "não onde decidir repete", texto: "não", metodo: http.MethodGet, cs: comStatus(500), wantMsg: true},
		{name: "não onde decidir devolve o erro", texto: "não", metodo: http.MethodGet, cs: comStatus(400)},
		{name: "incerto onde decidir repete", texto: "incerto: confira", metodo: http.MethodGet, cs: comStatus(500), wantMsg: true},
		{name: "texto novo sem regra", texto: "talvez", metodo: http.MethodGet, cs: comStatus(500), wantMsg: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			msg := confereCelula(tc.texto, respostas(tc.metodo, tc.cs))
			if (msg != "") != tc.wantMsg {
				t.Errorf("confereCelula(%q) = %q, quer divergência = %v", tc.texto, msg, tc.wantMsg)
			}
		})
	}
}
