package abertura

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"strconv"

	"github.com/MarcoBuarque/abertura-mercado-energia/go/internal/gen"
)

// Medicoes agrupa as operações de medição de consumo. Pegue-o em
// [Client].Medicoes.
type Medicoes struct{ c *Client }

const rotaMedicoes = "/v1/varejista/consumo/medicoes"

// Listar percorre as medições do filtro, página a página, com as mesmas
// garantias de [Migracoes.Listar]. UC, Concessionaria, Inicio e Fim são
// obrigatórios; sem eles o erro sai antes de qualquer requisição.
func (m *Medicoes) Listar(ctx context.Context, f FiltroMedicoes, opts ...CallOption) iter.Seq2[Medicao, error] {
	return paginar(ctx, func(ctx context.Context, cursor string) (Pagina[Medicao], error) {
		return m.ListarPagina(ctx, f, cursor, opts...)
	})
}

// ListarPagina busca uma página de medições. cursor vazio pede a primeira.
// Inicio e Fim vão em -03:00. A consulta não leva o perfil; só o cabeçalho.
func (m *Medicoes) ListarPagina(ctx context.Context, f FiltroMedicoes, cursor string, opts ...CallOption) (Pagina[Medicao], error) {
	const id = "listarMedicoes"
	if err := validarFiltroMedicoes(f); err != nil {
		return Pagina[Medicao]{}, fmt.Errorf("%s: %w", id, err)
	}
	q := url.Values{
		"codigoUnidadeConsumidora":   {f.UC},
		"codigoAgenteConcessionaria": {strconv.Itoa(f.Concessionaria)},
		"dataInicio":                 {instanteDe(f.Inicio)},
		"dataFim":                    {instanteDe(f.Fim)},
	}
	setSe(q, "situacaoMedicao", string(f.Situacao))
	setSe(q, "indexProximaPagina", cursor)

	_, corpo, err := m.c.enviar(ctx, operacao{
		id: id, metodo: http.MethodGet, modelo: rotaMedicoes, caminho: rotaMedicoes,
		query: q, grupo: grupoMedicoes,
	}, novoCallConfig(opts...))
	if err != nil {
		return Pagina[Medicao]{}, err
	}
	var lista gen.ListaMedicoes
	if err := decodificarSaida(corpo, &lista); err != nil {
		return Pagina[Medicao]{}, fmt.Errorf("%s: %w", id, err)
	}
	pag := Pagina[Medicao]{Itens: make([]Medicao, 0, len(lista.Medicoes)), Proxima: lista.IndexProximaPagina}
	for _, g := range lista.Medicoes {
		med, err := medicaoDe(g)
		if err != nil {
			return Pagina[Medicao]{}, fmt.Errorf("%s: %w", id, err)
		}
		pag.Itens = append(pag.Itens, med)
	}
	return pag, nil
}

// validarFiltroMedicoes confere os quatro campos que a API exige. O erro não
// cita a UC.
func validarFiltroMedicoes(f FiltroMedicoes) error {
	switch {
	case f.UC == "":
		return argumentoInvalido("a UC é obrigatória")
	case f.Concessionaria <= 0:
		return argumentoInvalido("a concessionária é obrigatória")
	case f.Inicio.IsZero() || f.Fim.IsZero():
		return argumentoInvalido("o início e o fim são obrigatórios")
	}
	return nil
}
