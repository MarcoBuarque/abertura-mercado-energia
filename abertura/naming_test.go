package abertura

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// These tests keep the package in one language and free of the small
// mistakes that reached v0.1.0: Portuguese words in identifiers, a field named
// like its own type, accented letters in sources and exported names without a
// doc comment. They parse the sources instead of relying on reviewers.

// portugueseWords are identifier segments (CamelCase parts, lower-cased) that
// mark a Portuguese name. English words that also exist in Portuguese (data,
// final, status, decimal, token) are deliberately absent.
//
//nolint:misspell // Portuguese words on purpose
var portugueseWords = func() map[string]bool {
	list := strings.Fields(`
		migracao migracoes medicao medicoes solicitacao solicitacoes suspensao representante
		mes meses ano anos dia dias hora horas prazo tempo esgotado inicio inicial fim
		obter listar criar criacao editar edicao conferir conferencia repetir tentativa
		enviar decidir escolha orcamento rodada buscar mudar mudanca escrita leitura
		chave chaves filtro filtros novo nova nome senha lista pacote arquivo caminho
		documento numero tipo tipos situacao perfil perfis agente varejista concessionaria
		supridora unidade consumidora erro erros faixa pagina paginas itens proximo proxima
		anterior atual sim nao conhecido conhecida desconhecido argumento invalido invalida
		repetido credencial credenciais validacao campo campos obrigatorio obrigatoria
		solicitada catalogo referencia consumo valor valores justificativa comentario
		indicador denuncia submercado multa pagamento celebracao processo codigo codigos
		cancelada cancelado expirada encerrada reprovada concluida criada aprovada pendencia
		consistida rejeitada texto textos ambiente producao homologacao cliente clientes
		chamada resposta respostas requisicao requisicoes pedido corpo cabecalho incerto
		resultado limite limites fora falha falhas segredo autorizador fonte divergencia
		confianca verificado exemplo descricao titulo detalhe resumo mascara oculto vazio
		vazia igual iguais diferente entrada saida ilegivel transporte interno recusa
		desejado troca fornecimento ate apos com para por uma umas
		uc de em ja tem atualizada atualizado metodo rota modelo sentinela limpar mensagem
		operacao decisao busca grupo geral duracao dormir aceito agora vence sufixo
		renovavel dados larga demais contrato teste testes
	`)
	m := make(map[string]bool, len(list))
	for _, w := range list {
		m[w] = true
	}
	return m
}()

// allowedNonASCII is the one accented phrase a source may contain: the legal
// name in the non-affiliation sentence.
const allowedNonASCII = "Câmara de Comercialização de Energia Elétrica"

// sourceFiles lists the Go files of this module, skipping testdata and the
// generated code (its names come from the spec and the API). Test files are
// included only when asked: their identifiers must be English too, but their
// fixtures quote the API's Portuguese messages.
func sourceFiles(t *testing.T, includeTests bool) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "testdata", "gen":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if includeTests || !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk sources: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no source files found")
	}
	return files
}

func parseSources(t *testing.T, includeTests bool) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	parsed := make(map[string]*ast.File)
	for _, path := range sourceFiles(t, includeTests) {
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		parsed[path] = f
	}
	return fset, parsed
}

// portugueseWord reports whether a CamelCase segment is a listed Portuguese
// word, also in its plural forms (agentes, varejistas, solicitacoes).
func portugueseWord(seg string) (string, bool) {
	w := strings.ToLower(seg)
	candidates := []string{w}
	if strings.HasSuffix(w, "oes") {
		candidates = append(candidates, strings.TrimSuffix(w, "oes")+"ao")
	}
	if strings.HasSuffix(w, "es") {
		candidates = append(candidates, strings.TrimSuffix(w, "es"))
	}
	if strings.HasSuffix(w, "s") {
		candidates = append(candidates, strings.TrimSuffix(w, "s"))
	}
	for _, c := range candidates {
		if portugueseWords[c] {
			return c, true
		}
	}
	return "", false
}

// portugueseFunctionWords are words that only occur in Portuguese prose; one
// of them in a comment means the comment was not written in English.
//
//nolint:misspell // Portuguese words on purpose
var portugueseFunctionWords = map[string]bool{
	"não": true, "para": true, "uma": true, "são": true, "também": true, "então": true,
	"porque": true, "até": true, "após": true, "ainda": true, "mês": true, "meses": true,
	"isso": true, "isto": true, "está": true, "estão": true, "pelo": true, "pela": true,
	"dos": true, "das": true, "aos": true, "às": true, "você": true, "quando": true,
	"sempre": true, "nunca": true, "apenas": true, "cada": true, "entre": true, "sobre": true,
	"que": true, "por": true, "mais": true, "como": true, "devolve": true, "campo": true,
}

// segments splits an identifier into its CamelCase words: "ParseYearMonth"
// gives [Parse Year Month], "CERSigningID" gives [CER Signing ID].
func segments(ident string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	runes := []rune(ident)
	for i, r := range runes {
		if r == '_' || unicode.IsDigit(r) {
			flush()
			continue
		}
		if i > 0 && unicode.IsUpper(r) {
			prevUpper := unicode.IsUpper(runes[i-1])
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if !prevUpper || nextLower {
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return out
}

// declaredIdents collects every identifier the file declares: types, funcs,
// methods, receivers, parameters, results, fields, interface methods,
// constants, variables, short declarations, range variables and labels.
func declaredIdents(f *ast.File) []*ast.Ident {
	var idents []*ast.Ident
	add := func(names ...*ast.Ident) {
		for _, n := range names {
			if n != nil && n.Name != "_" {
				idents = append(idents, n)
			}
		}
	}
	fieldNames := func(fl *ast.FieldList) {
		if fl == nil {
			return
		}
		for _, field := range fl.List {
			add(field.Names...)
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			add(x.Name)
			fieldNames(x.Recv)
		case *ast.FuncType:
			fieldNames(x.Params)
			fieldNames(x.Results)
		case *ast.TypeSpec:
			add(x.Name)
			fieldNames(x.TypeParams)
		case *ast.ValueSpec:
			add(x.Names...)
		case *ast.StructType:
			fieldNames(x.Fields)
		case *ast.InterfaceType:
			fieldNames(x.Methods)
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				for _, lhs := range x.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						add(id)
					}
				}
			}
		case *ast.RangeStmt:
			if x.Tok == token.DEFINE {
				if id, ok := x.Key.(*ast.Ident); ok {
					add(id)
				}
				if id, ok := x.Value.(*ast.Ident); ok {
					add(id)
				}
			}
		case *ast.LabeledStmt:
			add(x.Label)
		}
		return true
	})
	return idents
}

func TestNaming_WhenIdentifierHasPortugueseWord_Fails(t *testing.T) {
	fset, files := parseSources(t, true)
	for _, f := range files {
		for _, id := range declaredIdents(f) {
			for _, seg := range segments(id.Name) {
				if w, ok := portugueseWord(seg); ok {
					t.Errorf("%s: identifier %q contains the Portuguese word %q", fset.Position(id.Pos()), id.Name, w)
				}
			}
		}
	}
}

func TestNaming_WhenFieldRepeatsTypeName_Fails(t *testing.T) {
	fset, files := parseSources(t, true)
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := spec.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					if name.Name == spec.Name.Name {
						t.Errorf("%s: field %s.%s repeats the type name", fset.Position(name.Pos()), spec.Name.Name, name.Name)
					}
				}
			}
			return true
		})
	}
}

func TestNaming_WhenSourceHasAccentedLetter_Fails(t *testing.T) {
	for _, path := range sourceFiles(t, false) {
		file, err := os.Open(path) //nolint:gosec // paths come from WalkDir over this module
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
		line := 0
		for scanner.Scan() {
			line++
			text := strings.ReplaceAll(scanner.Text(), allowedNonASCII, "")
			for _, r := range text {
				if r > unicode.MaxASCII && unicode.IsLetter(r) {
					t.Errorf("%s:%d: non-ASCII letter %q; sources are English", path, line, r)
					break
				}
			}
		}
		if err := scanner.Err(); err != nil {
			t.Errorf("scan %s: %v", path, err)
		}
		if err := file.Close(); err != nil {
			t.Errorf("close %s: %v", path, err)
		}
	}
}

// receiverExported reports whether a function is a plain function or a method
// of an exported type. Methods of unexported types are not part of the API and
// godoc does not show them, so they need no doc comment.
func receiverExported(d *ast.FuncDecl) bool {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return true
	}
	expr := d.Recv.List[0].Type
	for {
		switch x := expr.(type) {
		case *ast.StarExpr:
			expr = x.X
		case *ast.IndexExpr:
			expr = x.X
		case *ast.IndexListExpr:
			expr = x.X
		case *ast.Ident:
			return x.IsExported()
		default:
			return true
		}
	}
}

// docStartsWith reports whether a doc comment exists and its first word is
// the identifier name, as godoc expects.
func docStartsWith(doc *ast.CommentGroup, name string) bool {
	if doc == nil {
		return false
	}
	text := strings.TrimSpace(doc.Text())
	return strings.HasPrefix(text, name+" ") || strings.HasPrefix(text, name+"\n") || strings.HasPrefix(text, "A "+name+" ") || strings.HasPrefix(text, "An "+name+" ") || strings.HasPrefix(text, "The "+name+" ")
}

func TestNaming_WhenExportedNameLacksDoc_Fails(t *testing.T) {
	fset, files := parseSources(t, false)
	for _, f := range files {
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Name.IsExported() && receiverExported(d) && !docStartsWith(d.Doc, d.Name.Name) {
					t.Errorf("%s: exported %s has no doc comment starting with its name", fset.Position(d.Pos()), d.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						documented := docStartsWith(s.Doc, s.Name.Name) || (len(d.Specs) == 1 && docStartsWith(d.Doc, s.Name.Name))
						if s.Name.IsExported() && !documented {
							t.Errorf("%s: exported type %s has no doc comment starting with its name", fset.Position(s.Pos()), s.Name.Name)
						}
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if name.IsExported() && s.Doc == nil && d.Doc == nil {
								t.Errorf("%s: exported %s has no doc comment", fset.Position(name.Pos()), name.Name)
							}
						}
					}
				}
			}
		}
	}
}

func TestNaming_WhenCommentIsPortuguese_Fails(t *testing.T) {
	fset, files := parseSources(t, true)
	for _, f := range files {
		for _, group := range f.Comments {
			text := strings.ReplaceAll(group.Text(), allowedNonASCII, "")
			for _, word := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) }) {
				if portugueseFunctionWords[strings.ToLower(word)] {
					t.Errorf("%s: comment contains the Portuguese word %q; comments are English", fset.Position(group.Pos()), word)
					break
				}
			}
		}
	}
}

// TestModulePath_WhenRead_EndsWithThePackageName guards the import path: the
// last element must be the package name, so the import needs no alias and no
// /go suffix ever comes back.
func TestModulePath_WhenRead_EndsWithThePackageName(t *testing.T) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	const want = "module github.com/MarcoBuarque/abertura-mercado-energia/abertura\n"
	if !strings.HasPrefix(string(data), want) {
		t.Fatalf("go.mod must start with %q", strings.TrimSpace(want))
	}
	if filepath.Base(strings.TrimSpace(strings.TrimPrefix(want, "module "))) != "abertura" {
		t.Fatal("the module path must end with the package name")
	}
}
