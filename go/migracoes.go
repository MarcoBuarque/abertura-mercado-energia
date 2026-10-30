package abertura

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/MarcoBuarque/abertura-mercado-energia/go/internal/gen"
)

// Migracoes agrupa as operações de migração de unidade consumidora. Pegue-o em
// [Client].Migracoes.
type Migracoes struct{ c *Client }

const (
	rotaListaMigracoes = "/v1.1/varejista/migracoes"
	rotaMigracaoV11    = "/v1.1/varejista/migracoes/{idMigracao}"
	rotaMigracaoV1     = "/v1/varejista/migracoes/{idMigracao}"
)

// Listar percorre as migrações que casam com o filtro, página a página. Nada
// é buscado antes do primeiro range, e um break interrompe a busca. Um erro
// sai como (zero, err) uma vez e encerra; cursor repetido da API dá
// [ErrCursorRepetido].
//
// O perfil da consulta é o do cliente (ou o da chamada, com [WithAgent]);
// FiltroMigracoes.Perfil o troca.
func (m *Migracoes) Listar(ctx context.Context, f FiltroMigracoes, opts ...CallOption) iter.Seq2[Migracao, error] {
	return paginar(ctx, func(ctx context.Context, cursor string) (Pagina[Migracao], error) {
		return m.ListarPagina(ctx, f, cursor, opts...)
	})
}

// ListarPagina busca uma página. cursor vazio pede a primeira; o
// [Pagina].Proxima devolvido serve de cursor para retomar depois.
func (m *Migracoes) ListarPagina(ctx context.Context, f FiltroMigracoes, cursor string, opts ...CallOption) (Pagina[Migracao], error) {
	const id = "listarMigracoes"
	p := m.c.parametros(novoCallConfig(opts...))
	perfil := p.perfil
	if f.Perfil != 0 {
		perfil = f.Perfil
	}
	q := url.Values{"codigoPerfilVarejista": {strconv.Itoa(perfil)}}
	setSe(q, "mesReferenciaInicial", f.De.String())
	setSe(q, "mesReferenciaFinal", f.Ate.String())
	setSe(q, "codigoUnidadeConsumidora", f.UC)
	setSe(q, "statusMigracao", string(f.Status))
	setSe(q, "indexProximaPagina", cursor)

	_, corpo, err := m.c.enviar(ctx, operacao{
		id: id, metodo: http.MethodGet, modelo: rotaListaMigracoes, caminho: rotaListaMigracoes,
		query: q, grupo: grupoGeral,
	}, novoCallConfig(opts...))
	if err != nil {
		return Pagina[Migracao]{}, err
	}
	var lista gen.ListaMigracoes
	if err := decodificarSaida(corpo, &lista); err != nil {
		return Pagina[Migracao]{}, fmt.Errorf("%s: %w", id, err)
	}
	pag := Pagina[Migracao]{Itens: make([]Migracao, 0, len(lista.Migracao)), Proxima: lista.IndexProximaPagina}
	for _, r := range lista.Migracao {
		mig, err := migracaoDeResumo(r)
		if err != nil {
			return Pagina[Migracao]{}, fmt.Errorf("%s: %w", id, err)
		}
		pag.Itens = append(pag.Itens, mig)
	}
	return pag, nil
}

// Obter devolve uma migração pelo id. A API responde com uma lista de um item
// ou com um objeto; o SDK aceita os dois. Lista vazia dá [ErrNotFound]; mais
// de um item, erro.
func (m *Migracoes) Obter(ctx context.Context, id string, opts ...CallOption) (Migracao, error) {
	const op = "obterMigracao"
	id, err := validarID(op, id)
	if err != nil {
		return Migracao{}, err
	}
	_, corpo, err := m.c.enviar(ctx, operacao{
		id: op, metodo: http.MethodGet, modelo: rotaMigracaoV11,
		caminho: strings.Replace(rotaMigracaoV11, "{idMigracao}", url.PathEscape(id), 1), grupo: grupoGeral,
	}, novoCallConfig(opts...))
	if err != nil {
		return Migracao{}, err
	}
	itens, err := detalhesDe(corpo)
	if err != nil {
		return Migracao{}, fmt.Errorf("%s: %w", op, err)
	}
	switch len(itens) {
	case 0:
		return Migracao{}, fmt.Errorf("%s: %w", op, ErrNotFound)
	case 1:
		mig, err := migracaoDeDetalhe(itens[0])
		if err != nil {
			return Migracao{}, fmt.Errorf("%s: %w", op, err)
		}
		return mig, nil
	default:
		return Migracao{}, fmt.Errorf("%s: a API devolveu mais de uma migração para o id", op)
	}
}

// Criar registra uma nova migração. O código do varejista e o perfil vêm do
// cliente (ou da chamada, com [WithAgent]).
//
// A escrita não é repetida. Se o resultado for incerto, o erro é um
// [*UncertainResultError] com uma [ChaveMigracao]: confira com
// [Migracoes.ConferirCriacao]. Repetir a criação com migração ativa dá
// [ErrMigracaoSolicitada].
func (m *Migracoes) Criar(ctx context.Context, n NovaMigracao, opts ...CallOption) (Migracao, error) {
	const op = "criarMigracao"
	p := m.c.parametros(novoCallConfig(opts...))
	chave := ChaveMigracao{UC: n.UC, Concessionaria: n.Concessionaria, Mes: n.Mes, Perfil: p.perfil}
	_, corpo, err := m.c.enviar(ctx, operacao{
		id: op, metodo: http.MethodPost, modelo: rotaListaMigracoes, caminho: rotaListaMigracoes,
		corpo: novaMigracaoGen(n, p.agente, p.perfil), grupo: grupoGeral, chave: chave,
	}, novoCallConfig(opts...))
	if err != nil {
		return Migracao{}, err
	}
	mig, err := migracaoEscrita(corpo)
	if err != nil {
		// O servidor aceitou, mas não sabemos o que foi criado.
		return Migracao{}, &UncertainResultError{Operation: op, Key: chave, Err: fmt.Errorf("%s: %w", op, err)}
	}
	return mig, nil
}

// Editar lê a migração, aplica mudar sobre os campos editáveis e envia o
// objeto inteiro. Como não se sabe se o PUT da API é parcial ou total, enviar
// tudo dá o mesmo resultado nos dois casos, ao custo de um GET antes.
//
// Não há ETag: outra escrita entre o GET e o PUT pode ser sobrescrita. Se o
// resultado do PUT for incerto, o erro é um [*UncertainResultError] com uma
// [ChaveEdicao]; confira com [Migracoes.ConferirEdicao].
func (m *Migracoes) Editar(ctx context.Context, id string, mudar func(*EdicaoMigracao), opts ...CallOption) (Migracao, error) {
	const op = "editarMigracao"
	if mudar == nil {
		return Migracao{}, argumentoInvalido("%s: a função de mudança não pode ser nula", op)
	}
	atual, err := m.Obter(ctx, id, opts...)
	if err != nil {
		return Migracao{}, err
	}
	e := EdicaoMigracao{
		Mes: atual.Mes, Perfil: atual.Perfil, Documento: atual.Documento,
		Email: atual.Email, Status: atual.Status,
	}
	mudar(&e)
	id = strings.TrimSpace(id)
	chave := ChaveEdicao{ID: id, Desejado: e}
	_, corpo, err := m.c.enviar(ctx, operacao{
		id: op, metodo: http.MethodPut, modelo: rotaMigracaoV1,
		caminho: strings.Replace(rotaMigracaoV1, "{idMigracao}", url.PathEscape(id), 1),
		corpo:   edicaoGen(e), grupo: grupoGeral, chave: chave,
	}, novoCallConfig(opts...))
	if err != nil {
		return Migracao{}, err
	}
	mig, err := migracaoEscrita(corpo)
	if err != nil {
		return Migracao{}, &UncertainResultError{Operation: op, Key: chave, Err: fmt.Errorf("%s: %w", op, err)}
	}
	return mig, nil
}

// ConferirCriacao diz o que existe para a chave depois de um resultado
// incerto em [Migracoes.Criar]. Lista pela UC e pelo mês, filtra a
// concessionária e devolve o que achar, da mais recente para a mais antiga.
// Resultado vazio quer dizer "não criada".
func (m *Migracoes) ConferirCriacao(ctx context.Context, k ChaveMigracao, opts ...CallOption) ([]Migracao, error) {
	filtro := FiltroMigracoes{De: k.Mes, Ate: k.Mes, UC: k.UC, Perfil: k.Perfil}
	achadas := make([]Migracao, 0)
	for mig, err := range m.Listar(ctx, filtro, opts...) {
		if err != nil {
			return nil, err
		}
		if k.Concessionaria == 0 || mig.Concessionaria == k.Concessionaria {
			achadas = append(achadas, mig)
		}
	}
	slices.SortStableFunc(achadas, func(a, b Migracao) int {
		if c := b.DataSolicitacao.Compare(a.DataSolicitacao); c != 0 {
			return c
		}
		if c := b.Mes.Ano - a.Mes.Ano; c != 0 {
			return c
		}
		return int(b.Mes.Mes) - int(a.Mes.Mes)
	})
	return achadas, nil
}

// ConferirEdicao lê a migração e diz se ela já tem os campos desejados da
// chave. Campos zero em ChaveEdicao.Desejado não entram na comparação. Devolve
// a migração como está agora.
func (m *Migracoes) ConferirEdicao(ctx context.Context, k ChaveEdicao, opts ...CallOption) (Migracao, bool, error) {
	atual, err := m.Obter(ctx, k.ID, opts...)
	if err != nil {
		return Migracao{}, false, err
	}
	d := k.Desejado
	bate := (d.Mes.IsZero() || d.Mes == atual.Mes) &&
		(d.Perfil == 0 || d.Perfil == atual.Perfil) &&
		(d.Documento.IsZero() || (d.Documento.Tipo() == atual.Documento.Tipo() && d.Documento.Numero() == atual.Documento.Numero())) &&
		(d.Email == "" || d.Email == atual.Email) &&
		(d.Status == "" || d.Status == atual.Status)
	return atual, bate, nil
}

// validarID confere o id de uma migração, sem repeti-lo na mensagem.
func validarID(op, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", argumentoInvalido("%s: o id da migração não pode ser vazio", op)
	}
	return id, nil
}

// detalhesDe lê o corpo de um detalhe, que a API manda como lista ou como
// objeto.
func detalhesDe(corpo []byte) ([]gen.MigracaoDetalhe, error) {
	t := bytes.TrimSpace(corpo)
	var itens []gen.MigracaoDetalhe
	if len(t) > 0 && t[0] == '[' {
		if err := decodificarSaida(t, &itens); err != nil {
			return nil, err
		}
		return itens, nil
	}
	if len(t) == 0 || bytes.Equal(t, []byte("null")) {
		return nil, errors.New("resposta vazia")
	}
	var um gen.MigracaoDetalhe
	if err := decodificarSaida(t, &um); err != nil {
		return nil, err
	}
	return []gen.MigracaoDetalhe{um}, nil
}

// migracaoEscrita lê a resposta de um POST ou PUT: um único detalhe.
func migracaoEscrita(corpo []byte) (Migracao, error) {
	itens, err := detalhesDe(corpo)
	if err != nil {
		return Migracao{}, err
	}
	if len(itens) != 1 || itens[0].IdMigracao == "" {
		return Migracao{}, errors.New("a resposta não traz uma migração identificável")
	}
	return migracaoDeDetalhe(itens[0])
}

// setSe acrescenta o parâmetro se o valor não for vazio.
func setSe(q url.Values, nome, valor string) {
	if valor != "" {
		q.Set(nome, valor)
	}
}
