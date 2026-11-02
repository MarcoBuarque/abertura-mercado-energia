package abertura

import (
	"context"
	"errors"
	"iter"
	"log/slog"
)

// ErrRepeatedCursor indicates that the API returned, as the next page, a
// cursor that was already used. The pagination mechanics have never been seen
// working in the sandbox; without this guard, a fixed cursor would make the
// iterator spin forever.
var ErrRepeatedCursor = errors.New("abertura: the API returned a repeated pagination cursor")

// Page is a page of results.
type Page[T any] struct {
	// Items are the items on the page.
	Items []T
	// NextCursor is the cursor of the following page; empty means there are no more.
	NextCursor string
}

// LogValue implements [slog.LogValuer] without the items: a page in a log shows
// how many items it has and whether a next cursor exists. The items may carry
// personal data and, through the slog JSON handler, would go through
// json.Marshal in clear text.
func (p Page[T]) LogValue() slog.Value {
	return slog.GroupValue(slog.Int("items", len(p.Items)), slog.Bool("has_next", p.NextCursor != ""))
}

// paginate walks all pages by calling fetch and yields the items in order.
// Nothing is fetched before the first range step, and a break stops the
// fetching. An error is yielded as (zero, err) exactly once and ends the
// iteration; a canceled context ends it with the context error.
//
// A repeated cursor ends the iteration with [ErrRepeatedCursor], in two cases.
// If a page returns as Next the same cursor that was just used to fetch it (a
// loop onto itself, which the mock does), its items repeat the previous page
// and are not yielded. If it returns a cursor seen before (cycle A, B, A), its
// items are new and are yielded; only then comes the error, without fetching
// again.
func paginate[T any](ctx context.Context, fetch func(ctx context.Context, cursor string) (Page[T], error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		used := map[string]struct{}{"": {}}
		cursor := ""
		for {
			if err := ctx.Err(); err != nil {
				yield(zero, err)
				return
			}
			page, err := fetch(ctx, cursor)
			if err != nil {
				yield(zero, err)
				return
			}
			_, seen := used[page.NextCursor]
			repeated := page.NextCursor != "" && seen
			if repeated && page.NextCursor == cursor {
				yield(zero, ErrRepeatedCursor)
				return
			}
			for _, item := range page.Items {
				if !yield(item, nil) {
					return
				}
			}
			if repeated {
				yield(zero, ErrRepeatedCursor)
				return
			}
			if page.NextCursor == "" {
				return
			}
			used[page.NextCursor] = struct{}{}
			cursor = page.NextCursor
		}
	}
}
