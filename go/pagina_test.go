package abertura

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// buscaFalsa devolve páginas por cursor e conta as chamadas.
type buscaFalsa struct {
	paginas  map[string]Pagina[int]
	errEm    string // cursor que dá erro
	err      error
	chamadas int
	cursores []string
	aoBuscar func(cursor string)
}

func (b *buscaFalsa) buscar(_ context.Context, cursor string) (Pagina[int], error) {
	b.chamadas++
	b.cursores = append(b.cursores, cursor)
	if b.aoBuscar != nil {
		b.aoBuscar(cursor)
	}
	if b.err != nil && cursor == b.errEm {
		return Pagina[int]{}, b.err
	}
	return b.paginas[cursor], nil
}

type resultado struct {
	item int
	err  error
}

func coletar(seq func(func(int, error) bool)) []resultado {
	var out []resultado
	for item, err := range seq {
		out = append(out, resultado{item, err})
	}
	return out
}

func TestPaginar_WhenDuasPaginas_EntregaTudoEmOrdem(t *testing.T) {
	b := &buscaFalsa{paginas: map[string]Pagina[int]{
		"":   {Itens: []int{1, 2}, Proxima: "p2"},
		"p2": {Itens: []int{3}},
	}}
	var itens []int
	for item, err := range paginar(context.Background(), b.buscar) {
		if err != nil {
			t.Fatal(err)
		}
		itens = append(itens, item)
	}
	if !slices.Equal(itens, []int{1, 2, 3}) {
		t.Errorf("itens = %v", itens)
	}
	if b.chamadas != 2 || !slices.Equal(b.cursores, []string{"", "p2"}) {
		t.Errorf("chamadas = %d, cursores = %v", b.chamadas, b.cursores)
	}
}

func TestPaginar_WhenSemRange_NaoBuscaNada(t *testing.T) {
	b := &buscaFalsa{paginas: map[string]Pagina[int]{"": {Itens: []int{1}}}}
	if seq := paginar(context.Background(), b.buscar); seq == nil {
		t.Fatal("paginar devolveu nil")
	}
	if b.chamadas != 0 {
		t.Errorf("chamadas antes do range = %d, queria 0", b.chamadas)
	}
}

func TestPaginar_WhenBreakNoPrimeiroItem_BuscaUmaVezSo(t *testing.T) {
	b := &buscaFalsa{paginas: map[string]Pagina[int]{
		"":   {Itens: []int{1, 2}, Proxima: "p2"},
		"p2": {Itens: []int{3}},
	}}
	for item, err := range paginar(context.Background(), b.buscar) {
		if err != nil || item != 1 {
			t.Fatalf("item = %d, err = %v", item, err)
		}
		break
	}
	if b.chamadas != 1 {
		t.Errorf("chamadas = %d, queria 1", b.chamadas)
	}
}

func TestPaginar_WhenBreakNoUltimoItemDaPrimeiraPagina_NaoBuscaAProxima(t *testing.T) {
	b := &buscaFalsa{paginas: map[string]Pagina[int]{
		"":   {Itens: []int{1, 2}, Proxima: "p2"},
		"p2": {Itens: []int{3}},
	}}
	for item, err := range paginar(context.Background(), b.buscar) {
		if err != nil {
			t.Fatal(err)
		}
		if item == 2 {
			break
		}
	}
	if b.chamadas != 1 {
		t.Errorf("chamadas = %d, queria 1", b.chamadas)
	}
}

func TestPaginar_WhenErroNaSegundaPagina_EntregaUmaVezEPara(t *testing.T) {
	falha := errors.New("falha de teste")
	b := &buscaFalsa{
		paginas: map[string]Pagina[int]{"": {Itens: []int{1}, Proxima: "p2"}},
		errEm:   "p2",
		err:     falha,
	}
	got := coletar(paginar(context.Background(), b.buscar))
	if len(got) != 2 {
		t.Fatalf("resultados = %v", got)
	}
	if got[0] != (resultado{1, nil}) {
		t.Errorf("primeiro = %v", got[0])
	}
	if got[1].item != 0 || !errors.Is(got[1].err, falha) {
		t.Errorf("segundo = %v", got[1])
	}
	if b.chamadas != 2 {
		t.Errorf("chamadas = %d, queria 2", b.chamadas)
	}
}

func TestPaginar_WhenCursorRepetido_DevolveErrCursorRepetido(t *testing.T) {
	tests := []struct {
		name    string
		paginas map[string]Pagina[int]
		want    int // chamadas
	}{
		{
			name: "mesmo cursor duas vezes seguidas",
			paginas: map[string]Pagina[int]{
				"":   {Itens: []int{1}, Proxima: "p2"},
				"p2": {Itens: []int{2}, Proxima: "p2"},
			},
			want: 2,
		},
		{
			name: "ciclo A, B, A",
			paginas: map[string]Pagina[int]{
				"":  {Itens: []int{1}, Proxima: "a"},
				"a": {Itens: []int{2}, Proxima: "b"},
				"b": {Itens: []int{3}, Proxima: "a"},
			},
			want: 3,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := &buscaFalsa{paginas: tc.paginas}
			got := coletar(paginar(context.Background(), b.buscar))
			ultimo := got[len(got)-1]
			if !errors.Is(ultimo.err, ErrCursorRepetido) {
				t.Errorf("último = %v, queria ErrCursorRepetido", ultimo)
			}
			if b.chamadas != tc.want {
				t.Errorf("chamadas = %d, queria %d", b.chamadas, tc.want)
			}
		})
	}
}

func TestPaginar_WhenContextoCanceladoEntrePaginas_DevolveErroDoContexto(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &buscaFalsa{paginas: map[string]Pagina[int]{
		"":   {Itens: []int{1}, Proxima: "p2"},
		"p2": {Itens: []int{2}},
	}}
	b.aoBuscar = func(string) { cancel() }
	got := coletar(paginar(ctx, b.buscar))
	if len(got) != 2 || got[0] != (resultado{1, nil}) || !errors.Is(got[1].err, context.Canceled) {
		t.Errorf("resultados = %v", got)
	}
	if b.chamadas != 1 {
		t.Errorf("chamadas = %d, queria 1", b.chamadas)
	}
}

func TestPaginar_WhenPaginaVazia_Termina(t *testing.T) {
	b := &buscaFalsa{paginas: map[string]Pagina[int]{}}
	if got := coletar(paginar(context.Background(), b.buscar)); len(got) != 0 || b.chamadas != 1 {
		t.Errorf("resultados = %v, chamadas = %d", got, b.chamadas)
	}
}
