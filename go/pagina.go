package abertura

import (
	"context"
	"errors"
	"iter"
)

// ErrCursorRepetido indica que a API devolveu, como próxima página, um cursor
// já usado. A mecânica de paginação nunca foi vista funcionando no sandbox;
// sem esta guarda, um cursor fixo faria o iterador girar para sempre.
var ErrCursorRepetido = errors.New("abertura: a API devolveu um cursor de paginação repetido")

// Pagina é uma página de resultados.
type Pagina[T any] struct {
	// Itens são os itens da página.
	Itens []T
	// Proxima é o cursor da página seguinte; vazio quer dizer que acabou.
	Proxima string
}

// paginar percorre todas as páginas chamando buscar e entrega os itens em
// ordem. Nada é buscado antes do primeiro range, e um break interrompe a
// busca. Um erro sai como (zero, err) uma única vez e encerra a iteração. Um
// cursor repetido encerra com [ErrCursorRepetido]; o contexto cancelado,
// com o erro do contexto.
func paginar[T any](ctx context.Context, buscar func(ctx context.Context, cursor string) (Pagina[T], error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		usados := map[string]struct{}{"": {}}
		cursor := ""
		for {
			if err := ctx.Err(); err != nil {
				yield(zero, err)
				return
			}
			pag, err := buscar(ctx, cursor)
			if err != nil {
				yield(zero, err)
				return
			}
			for _, item := range pag.Itens {
				if !yield(item, nil) {
					return
				}
			}
			if pag.Proxima == "" {
				return
			}
			if _, visto := usados[pag.Proxima]; visto {
				yield(zero, ErrCursorRepetido)
				return
			}
			usados[pag.Proxima] = struct{}{}
			cursor = pag.Proxima
		}
	}
}
