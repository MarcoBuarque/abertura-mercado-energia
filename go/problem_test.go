package abertura

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const (
	pontoErroPuro = "about:blank"
	urlCatalogo   = "https://marcobuarque.github.io/abertura-mercado-energia/erros"
)

func apiErro(metodo string, status int, codigo, gateway string) *APIError {
	return &APIError{
		Operation: "criarMigracao", StatusCode: status, Code: codigo,
		Message: "texto da API", RequestID: "req-inventado-1",
		GatewayErrorType: gateway, metodo: metodo,
	}
}

func TestProblemFrom_WhenErroDoSDK_ClassificaNoCatalogo(t *testing.T) {
	t.Parallel()

	causaTransporte := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	tests := []struct {
		name          string
		err           error
		wantType      string
		wantStatus    int
		wantCode      string
		wantRetryable bool
		wantUncertain bool
	}{
		{name: "migração já solicitada", err: apiErro("POST", 400, "ERR_MIGRACAO_SOLICITADA", ""), wantType: "migracao-solicitada", wantStatus: 400, wantCode: "ERR_MIGRACAO_SOLICITADA"},
		{name: "validação de campos", err: apiErro("POST", 400, "ERR_VALIDACAO_CAMPOS", ""), wantType: "validacao-campos", wantStatus: 400, wantCode: "ERR_VALIDACAO_CAMPOS"},
		{name: "perfil obrigatório", err: apiErro("GET", 400, "ERR_PERFIL_VAREJISTA_OBRIGATORIO", ""), wantType: "perfil-varejista-obrigatorio", wantStatus: 400, wantCode: "ERR_PERFIL_VAREJISTA_OBRIGATORIO"},
		{name: "faixa de meses obrigatória", err: apiErro("GET", 400, "ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO", ""), wantType: "faixa-de-meses-obrigatoria", wantStatus: 400, wantCode: "ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO"},
		{name: "faixa de meses larga demais", err: apiErro("GET", 400, "ERR_MES_REFERENCIA_INICIAL_DIFERENCA", ""), wantType: "faixa-de-meses-larga-demais", wantStatus: 400, wantCode: "ERR_MES_REFERENCIA_INICIAL_DIFERENCA"},
		{name: "credencial recusada pelo autorizador", err: apiErro("GET", 403, "ERR_CREDENCIAL_INVALIDA", "AccessDeniedException"), wantType: "credencial-invalida-autorizador", wantStatus: 403, wantCode: "ERR_CREDENCIAL_INVALIDA"},
		{name: "credencial recusada pelo backend", err: apiErro("GET", 403, "ERR_CREDENCIAL_INVALIDA", ""), wantType: "credencial-invalida-backend", wantStatus: 403, wantCode: "ERR_CREDENCIAL_INVALIDA"},
		{name: "403 sem código", err: apiErro("GET", 403, "", ""), wantType: "recusa-sem-codigo", wantStatus: 403},
		{name: "404 de API", err: apiErro("GET", 404, "", ""), wantType: "nao-encontrado", wantStatus: 404},
		{name: "ErrNotFound embrulhado", err: fmt.Errorf("obterMigracao: %w", ErrNotFound), wantType: "nao-encontrado", wantStatus: 404},
		{name: "429 em leitura repete", err: apiErro("GET", 429, "", ""), wantType: "limite-de-requisicoes", wantStatus: 429, wantRetryable: true},
		{name: "429 em escrita sem marca do gateway não repete", err: apiErro("POST", 429, "", ""), wantType: "limite-de-requisicoes", wantStatus: 429},
		{name: "429 em escrita com marca do gateway repete", err: apiErro("POST", 429, "", "ThrottlingException"), wantType: "limite-de-requisicoes", wantStatus: 429, wantRetryable: true},
		{name: "500 em leitura repete", err: apiErro("GET", 500, "", ""), wantType: "erro-interno", wantStatus: 500, wantRetryable: true},
		{name: "500 em escrita é incerto", err: apiErro("POST", 500, "", ""), wantType: "erro-interno", wantStatus: 500, wantUncertain: true},
		{name: "502 do gateway", err: apiErro("GET", 502, "", ""), wantType: "falha-do-gateway", wantStatus: 502, wantRetryable: true},
		{name: "504 do gateway em escrita", err: apiErro("PUT", 504, "", ""), wantType: "falha-do-gateway", wantStatus: 504, wantUncertain: true},
		{name: "método desconhecido não afirma nada", err: apiErro("", 503, "", ""), wantType: "erro-interno", wantStatus: 503},
		{name: "código novo da API", err: apiErro("POST", 400, "ERR_NOVO_DA_API", ""), wantType: "codigo-desconhecido", wantStatus: 400, wantCode: "ERR_NOVO_DA_API"},
		{name: "campo error que não é código", err: apiErro("POST", 400, "texto livre", ""), wantType: "recusa-sem-codigo", wantStatus: 400},
		{name: "falha ao obter token", err: &APIError{Operation: operacaoToken, StatusCode: 503, metodo: http.MethodGet}, wantType: "falha-ao-obter-token", wantStatus: 503, wantRetryable: true},
		{name: "APIError embrulhado com %w", err: fmt.Errorf("camada: %w", apiErro("POST", 400, "ERR_MIGRACAO_SOLICITADA", "")), wantType: "migracao-solicitada", wantStatus: 400, wantCode: "ERR_MIGRACAO_SOLICITADA"},
		{
			name:     "incerto com 502 de causa",
			err:      &UncertainResultError{Operation: "criarMigracao", Key: ChaveMigracao{UC: "000000000000001"}, Err: apiErro("POST", 502, "", "")},
			wantType: "resultado-incerto", wantStatus: 502, wantUncertain: true,
		},
		{
			name:     "incerto com erro de transporte",
			err:      fmt.Errorf("camada: %w", &UncertainResultError{Operation: "editarMigracao", Err: causaTransporte}),
			wantType: "resultado-incerto", wantUncertain: true,
		},
		{name: "cursor repetido", err: fmt.Errorf("listarMigracoes: %w", ErrCursorRepetido), wantType: "cursor-repetido", wantRetryable: false},
		{name: "validação local", err: fmt.Errorf("listarMedicoes: %w", validarFiltroMedicoes(FiltroMedicoes{})), wantType: "argumento-invalido"},
		{name: "prazo vencido", err: fmt.Errorf("x: %w", context.DeadlineExceeded), wantType: "tempo-esgotado", wantRetryable: true},
		{name: "contexto cancelado", err: context.Canceled, wantType: "chamada-cancelada"},
		{name: "erro de rede", err: fmt.Errorf("listarMigracoes: %w", causaTransporte), wantType: "erro-de-transporte", wantRetryable: true},
		{name: "erro alheio ao SDK", err: errors.New("outra coisa"), wantType: "fora-do-catalogo"},
		{name: "APIError dentro de errors.Join", err: errors.Join(errors.New("outra coisa"), apiErro("POST", 400, "ERR_MIGRACAO_SOLICITADA", "")), wantType: "migracao-solicitada", wantStatus: 400, wantCode: "ERR_MIGRACAO_SOLICITADA"},
		{
			name:     "incerto ganha do APIError no errors.Join",
			err:      errors.Join(apiErro("GET", 404, "", ""), &UncertainResultError{Operation: "criarMigracao", Err: apiErro("POST", 502, "", "")}),
			wantType: "resultado-incerto", wantStatus: 502, wantUncertain: true,
		},
		{name: "prazo dentro de errors.Join", err: errors.Join(errors.New("fechar corpo"), context.DeadlineExceeded), wantType: "tempo-esgotado", wantRetryable: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := ProblemFrom(tc.err)
			wantType := CatalogoErrosURL + "#" + tc.wantType
			if p.Type != wantType {
				t.Errorf("Type = %q, quero %q", p.Type, wantType)
			}
			if p.Title == "" {
				t.Error("Title vazio")
			}
			if p.Status != tc.wantStatus {
				t.Errorf("Status = %d, quero %d", p.Status, tc.wantStatus)
			}
			if p.Code != tc.wantCode {
				t.Errorf("Code = %q, quero %q", p.Code, tc.wantCode)
			}
			if p.Retryable != tc.wantRetryable {
				t.Errorf("Retryable = %v, quero %v", p.Retryable, tc.wantRetryable)
			}
			if p.Uncertain != tc.wantUncertain {
				t.Errorf("Uncertain = %v, quero %v", p.Uncertain, tc.wantUncertain)
			}
			if p.Instance != "" {
				t.Errorf("Instance = %q, na v0.1 fica vazio", p.Instance)
			}
		})
	}
}

func TestProblemFrom_WhenNil_DevolveZero(t *testing.T) {
	t.Parallel()
	if p := ProblemFrom(nil); p != (Problem{}) {
		t.Errorf("ProblemFrom(nil) = %+v, quero o valor zero", p)
	}
}

func TestProblem_WhenAPIError_TrazOperacaoEIDDaRequisicao(t *testing.T) {
	t.Parallel()
	p := apiErro("POST", 400, "ERR_MIGRACAO_SOLICITADA", "").Problem()
	if p.Operation != "criarMigracao" || p.RequestID != "req-inventado-1" {
		t.Errorf("Operation=%q RequestID=%q", p.Operation, p.RequestID)
	}
	if !strings.Contains(p.Detail, "criarMigracao") {
		t.Errorf("Detail = %q, deveria citar a operação", p.Detail)
	}
	u := (&UncertainResultError{Operation: "criarMigracao", Err: apiErro("POST", 502, "", "")}).Problem()
	if u.Operation != "criarMigracao" || u.RequestID != "req-inventado-1" {
		t.Errorf("incerto: Operation=%q RequestID=%q", u.Operation, u.RequestID)
	}
}

// A decisão de repetir vem de decidir: este teste confere a tabela da
// arquitetura (§4.4) de forma independente, para o Problem não divergir dela.
func TestProblem_RetryableEUncertain_WhenMetodoEStatus_BateComTabelaDeRetries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		metodo        string
		status        int
		gateway       string
		wantRetryable bool
		wantUncertain bool
	}{
		{name: "GET 400", metodo: "GET", status: 400},
		{name: "GET 403", metodo: "GET", status: 403, gateway: "AccessDeniedException"},
		{name: "GET 404", metodo: "GET", status: 404},
		{name: "GET 429", metodo: "GET", status: 429, wantRetryable: true},
		{name: "GET 500", metodo: "GET", status: 500, wantRetryable: true},
		{name: "GET 502", metodo: "GET", status: 502, wantRetryable: true},
		{name: "GET 503", metodo: "GET", status: 503, wantRetryable: true},
		{name: "GET 504", metodo: "GET", status: 504, wantRetryable: true},
		{name: "POST 400", metodo: "POST", status: 400},
		{name: "POST 403", metodo: "POST", status: 403, gateway: "AccessDeniedException"},
		{name: "POST 429 sem gateway", metodo: "POST", status: 429},
		{name: "POST 429 com gateway", metodo: "POST", status: 429, gateway: "ThrottlingException", wantRetryable: true},
		{name: "POST 500", metodo: "POST", status: 500, wantUncertain: true},
		{name: "PUT 502", metodo: "PUT", status: 502, wantUncertain: true},
		{name: "PUT 503", metodo: "PUT", status: 503, wantUncertain: true},
		{name: "POST 504", metodo: "POST", status: 504, wantUncertain: true},
		{name: "GET 507", metodo: "GET", status: 507, wantRetryable: true},
		{name: "POST 520", metodo: "POST", status: 520, wantUncertain: true},
		{name: "GET 501 é definitivo", metodo: "GET", status: 501},
		{name: "POST 501 é definitivo", metodo: "POST", status: 501},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := apiErro(tc.metodo, tc.status, "", tc.gateway).Problem()
			if p.Retryable != tc.wantRetryable || p.Uncertain != tc.wantUncertain {
				t.Errorf("Retryable=%v Uncertain=%v, quero %v e %v", p.Retryable, p.Uncertain, tc.wantRetryable, tc.wantUncertain)
			}
		})
	}
}

func TestProblem_MarshalJSON_WhenCompleto_SegueARFC9457(t *testing.T) {
	t.Parallel()

	p := apiErro("POST", 400, "ERR_MIGRACAO_SOLICITADA", "").Problem()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	want := map[string]any{
		"type":      CatalogoErrosURL + "#migracao-solicitada",
		"status":    float64(400),
		"code":      "ERR_MIGRACAO_SOLICITADA",
		"operation": "criarMigracao",
		"requestId": "req-inventado-1",
		"retryable": false,
		"uncertain": false,
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %v, quero %v", k, m[k], v)
		}
	}
	for _, k := range []string{"title", "detail"} {
		if s, _ := m[k].(string); s == "" {
			t.Errorf("%s ausente ou vazio", k)
		}
	}
	if _, ok := m["instance"]; ok {
		t.Error("instance deveria ser omitido quando vazio")
	}
}

func TestProblem_MarshalJSON_WhenSemResposta_OmiteVazios(t *testing.T) {
	t.Parallel()

	b, err := json.Marshal(ProblemFrom(errors.New("x")))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, k := range []string{"status", "instance", "code", "operation", "requestId"} {
		if _, ok := m[k]; ok {
			t.Errorf("%s deveria ser omitido: %s", k, b)
		}
	}
	if want := CatalogoErrosURL + "#fora-do-catalogo"; m["type"] != want {
		t.Errorf("type = %v, quero %s", m["type"], want)
	}
}

func TestProblemFrom_WhenMensagemTemDadoPessoal_NaoVaza(t *testing.T) {
	t.Parallel()

	sensiveis := []string{"000000000000001", "123.456.789-09", "nome.inventado@example.org", "Fulano Inventado"}
	mensagem := "UC 000000000000001 do documento 123.456.789-09, contato nome.inventado@example.org, titular Fulano Inventado"
	api := &APIError{
		Operation: "criarMigracao", StatusCode: 400, Code: "ERR_VALIDACAO_CAMPOS",
		Message: mensagem, RequestID: "req-inventado-1", metodo: "POST", rawBody: []byte(mensagem),
	}
	codigoSujo := &APIError{Operation: "criarMigracao", StatusCode: 400, Code: "ERR_" + mensagem, Message: mensagem, metodo: "POST"}
	causaSuja := fmt.Errorf("listarMigracoes: %s: %w", mensagem, context.DeadlineExceeded)
	erros := map[string]error{
		"api":         api,
		"codigo sujo": codigoSujo,
		"incerto":     &UncertainResultError{Operation: "criarMigracao", Key: ChaveMigracao{UC: "000000000000001", Concessionaria: 900001}, Err: api},
		"causa suja":  causaSuja,
		"alheio":      errors.New(mensagem),
	}
	for nome, err := range erros {
		t.Run(nome, func(t *testing.T) {
			t.Parallel()
			b, jerr := json.Marshal(ProblemFrom(err))
			if jerr != nil {
				t.Fatalf("Marshal: %v", jerr)
			}
			for _, s := range sensiveis {
				if strings.Contains(string(b), s) {
					t.Errorf("JSON vazou %q: %s", s, b)
				}
			}
		})
	}
	if !strings.Contains(api.Message, "000000000000001") {
		t.Error("a mensagem da API deve continuar acessível em APIError.Message")
	}
}

// --- paridade entre a tabela, o código e docs/erros.md ---

var reAncora = regexp.MustCompile(`<a id="([a-z0-9-]+)"></a>`)

func lerArquivo(t *testing.T, caminho string) string {
	t.Helper()
	raiz, err := os.OpenRoot("..")
	if err != nil {
		t.Fatalf("abrir a raiz do repositório: %v", err)
	}
	defer func() {
		if err := raiz.Close(); err != nil {
			t.Errorf("fechar a raiz: %v", err)
		}
	}()
	b, err := raiz.ReadFile(strings.TrimPrefix(caminho, "../"))
	if err != nil {
		t.Fatalf("ler %s: %v", caminho, err)
	}
	return string(b)
}

func TestCatalogo_WhenComparadoComDocs_AncorasBatemComATabela(t *testing.T) {
	t.Parallel()

	doc := lerArquivo(t, "../docs/erros.md")
	ancoras := map[string]bool{}
	for _, m := range reAncora.FindAllStringSubmatch(doc, -1) {
		if ancoras[m[1]] {
			t.Errorf("âncora duplicada em docs/erros.md: %s", m[1])
		}
		ancoras[m[1]] = true
	}
	for slug, e := range catalogo {
		if !ancoras[slug] {
			t.Errorf("slug %q da tabela não tem âncora em docs/erros.md", slug)
		}
		if e.titulo == "" || e.detalhe == "" {
			t.Errorf("entrada %q sem título ou detalhe", slug)
		}
		if !regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`).MatchString(slug) {
			t.Errorf("slug %q fora do padrão", slug)
		}
	}
	for a := range ancoras {
		if _, ok := catalogo[a]; !ok {
			t.Errorf("âncora %q de docs/erros.md não existe na tabela", a)
		}
	}
	if CatalogoErrosURL != urlCatalogo {
		t.Errorf("CatalogoErrosURL = %q, quero %q", CatalogoErrosURL, urlCatalogo)
	}
}

// TestCatalogo_WhenComparadoComDocs_TitulosBatemComATabela prova que o título
// de cada entrada da tabela é o mesmo do cabeçalho logo abaixo da âncora e do
// link no índice de docs/erros.md: o Problem e o catálogo dizem o mesmo.
func TestCatalogo_WhenComparadoComDocs_TitulosBatemComATabela(t *testing.T) {
	t.Parallel()

	doc := lerArquivo(t, "../docs/erros.md")
	for slug, e := range catalogo {
		cabecalho := `<a id="` + slug + `"></a>` + "\n## " + e.titulo + "\n"
		if !strings.Contains(doc, cabecalho) {
			t.Errorf("entrada %q: o cabeçalho depois da âncora não é %q", slug, "## "+e.titulo)
		}
		if indice := "[" + e.titulo + "](#" + slug + ")"; !strings.Contains(doc, indice) {
			t.Errorf("entrada %q: o índice não traz %q", slug, indice)
		}
	}
}

func TestCatalogo_WhenSentinelaExportada_TemEntrada(t *testing.T) {
	t.Parallel()

	// Cada sentinela exportada com a forma de erro que o SDK realmente devolve.
	cobertas := map[string]error{
		"ErrCredencialInvalida":         apiErro("GET", 403, "ERR_CREDENCIAL_INVALIDA", "x"),
		"ErrValidacaoCampos":            apiErro("POST", 400, "ERR_VALIDACAO_CAMPOS", ""),
		"ErrPerfilVarejistaObrigatorio": apiErro("GET", 400, "ERR_PERFIL_VAREJISTA_OBRIGATORIO", ""),
		"ErrMigracaoSolicitada":         apiErro("POST", 400, "ERR_MIGRACAO_SOLICITADA", ""),
		"ErrFaixaDeMesesObrigatoria":    apiErro("GET", 400, "ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO", ""),
		"ErrFaixaDeMesesLargaDemais":    apiErro("GET", 400, "ERR_MES_REFERENCIA_INICIAL_DIFERENCA", ""),
		"ErrNotFound":                   ErrNotFound,
		"ErrRateLimited":                apiErro("GET", 429, "", ""),
		"ErrCursorRepetido":             ErrCursorRepetido,
		"ErrArgumentoInvalido":          ErrArgumentoInvalido,
	}
	sentinelas := map[string]error{
		"ErrCredencialInvalida": ErrCredencialInvalida, "ErrValidacaoCampos": ErrValidacaoCampos,
		"ErrPerfilVarejistaObrigatorio": ErrPerfilVarejistaObrigatorio, "ErrMigracaoSolicitada": ErrMigracaoSolicitada,
		"ErrFaixaDeMesesObrigatoria": ErrFaixaDeMesesObrigatoria, "ErrFaixaDeMesesLargaDemais": ErrFaixaDeMesesLargaDemais,
		"ErrNotFound": ErrNotFound, "ErrRateLimited": ErrRateLimited,
		"ErrCursorRepetido": ErrCursorRepetido, "ErrArgumentoInvalido": ErrArgumentoInvalido,
	}

	// Garante que a lista acima não ficou para trás: lê as declarações do pacote.
	declaradas := erroExportadosDoPacote(t)
	var nomes []string
	for n := range cobertas {
		nomes = append(nomes, n)
	}
	sort.Strings(nomes)
	if got := strings.Join(declaradas, ","); got != strings.Join(nomes, ",") {
		t.Errorf("sentinelas exportadas = %s; o teste cobre %s", got, strings.Join(nomes, ","))
	}

	for nome, err := range cobertas {
		p := ProblemFrom(fmt.Errorf("camada: %w", err))
		if !strings.HasPrefix(p.Type, CatalogoErrosURL+"#") {
			t.Errorf("%s: Type = %q, esperava uma entrada do catálogo", nome, p.Type)
			continue
		}
		slug := strings.TrimPrefix(p.Type, CatalogoErrosURL+"#")
		if _, ok := catalogo[slug]; !ok {
			t.Errorf("%s: slug %q não está na tabela", nome, slug)
		}
		// A própria sentinela também precisa casar com o erro classificado.
		if !errors.Is(err, sentinelas[nome]) {
			t.Errorf("%s: errors.Is falhou", nome)
		}
	}
}

// erroExportadosDoPacote lista, em ordem, as variáveis exportadas Err* dos
// arquivos do pacote (sem os de teste).
func erroExportadosDoPacote(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	arquivos, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	var out []string
	for _, nome := range arquivos {
		if strings.HasSuffix(nome, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, nome, nil, 0)
		if err != nil {
			t.Fatalf("ParseFile %s: %v", nome, err)
		}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, s := range gd.Specs {
				vs, ok := s.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, n := range vs.Names {
					if n.IsExported() && strings.HasPrefix(n.Name, "Err") {
						out = append(out, n.Name)
					}
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

var reCodigoSpec = regexp.MustCompile(`(?m)^\s+- (ERR_[A-Z_]+)\s*$`)

func TestCatalogo_WhenCodigoDaSpec_TemEntradaEMencaoNosDocs(t *testing.T) {
	t.Parallel()

	spec := lerArquivo(t, "../openapi/abertura.yaml")
	doc := lerArquivo(t, "../docs/erros.md")
	codigos := reCodigoSpec.FindAllStringSubmatch(spec, -1)
	if len(codigos) < 4 {
		t.Fatalf("achei %d códigos na spec, esperava ao menos 4", len(codigos))
	}
	for _, m := range codigos {
		cod := m[1]
		p := apiErro("POST", 400, cod, "x").Problem()
		if strings.HasSuffix(p.Type, "#codigo-desconhecido") || strings.HasSuffix(p.Type, "#recusa-sem-codigo") {
			t.Errorf("código %s da spec sem entrada própria: %s", cod, p.Type)
		}
		if !strings.Contains(doc, cod) {
			t.Errorf("código %s da spec não aparece em docs/erros.md", cod)
		}
	}
}

func TestCatalogo_WhenSituacaoSemCodigo_TemEntradaNosDocs(t *testing.T) {
	t.Parallel()

	doc := lerArquivo(t, "../docs/erros.md")
	for _, nome := range []string{"ErrNotFound", "ErrRateLimited", "ErrCursorRepetido", "ErrArgumentoInvalido", "UncertainResultError", "ConferirCriacao", "ConferirEdicao"} {
		if !strings.Contains(doc, nome) {
			t.Errorf("docs/erros.md não menciona %s", nome)
		}
	}
}
