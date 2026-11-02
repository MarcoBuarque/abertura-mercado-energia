package abertura

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// fakeFetcher returns pages by cursor and counts the calls.
type fakeFetcher struct {
	pages   map[string]Page[int]
	errAt   string // cursor that fails
	err     error
	calls   int
	cursors []string
	onFetch func(cursor string)
}

func (b *fakeFetcher) fetch(_ context.Context, cursor string) (Page[int], error) {
	b.calls++
	b.cursors = append(b.cursors, cursor)
	if b.onFetch != nil {
		b.onFetch(cursor)
	}
	if b.err != nil && cursor == b.errAt {
		return Page[int]{}, b.err
	}
	return b.pages[cursor], nil
}

type result struct {
	item int
	err  error
}

func collect(seq func(func(int, error) bool)) []result {
	var out []result
	for item, err := range seq {
		out = append(out, result{item, err})
	}
	return out
}

func TestPaginate_WhenTwoPages_YieldsEverythingInOrder(t *testing.T) {
	b := &fakeFetcher{pages: map[string]Page[int]{
		"":   {Items: []int{1, 2}, NextCursor: "p2"},
		"p2": {Items: []int{3}},
	}}
	var items []int
	for item, err := range paginate(context.Background(), b.fetch) {
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, item)
	}
	if !slices.Equal(items, []int{1, 2, 3}) {
		t.Errorf("items = %v", items)
	}
	if b.calls != 2 || !slices.Equal(b.cursors, []string{"", "p2"}) {
		t.Errorf("calls = %d, cursors = %v", b.calls, b.cursors)
	}
}

func TestPaginate_WhenNoRange_FetchesNothing(t *testing.T) {
	b := &fakeFetcher{pages: map[string]Page[int]{"": {Items: []int{1}}}}
	if seq := paginate(context.Background(), b.fetch); seq == nil {
		t.Fatal("paginate returned nil")
	}
	if b.calls != 0 {
		t.Errorf("calls before the range = %d, want 0", b.calls)
	}
}

func TestPaginate_WhenBreakOnFirstItem_FetchesOnlyOnce(t *testing.T) {
	b := &fakeFetcher{pages: map[string]Page[int]{
		"":   {Items: []int{1, 2}, NextCursor: "p2"},
		"p2": {Items: []int{3}},
	}}
	for item, err := range paginate(context.Background(), b.fetch) {
		if err != nil || item != 1 {
			t.Fatalf("item = %d, err = %v", item, err)
		}
		break
	}
	if b.calls != 1 {
		t.Errorf("calls = %d, want 1", b.calls)
	}
}

func TestPaginate_WhenBreakOnLastItemOfFirstPage_DoesNotFetchNext(t *testing.T) {
	b := &fakeFetcher{pages: map[string]Page[int]{
		"":   {Items: []int{1, 2}, NextCursor: "p2"},
		"p2": {Items: []int{3}},
	}}
	for item, err := range paginate(context.Background(), b.fetch) {
		if err != nil {
			t.Fatal(err)
		}
		if item == 2 {
			break
		}
	}
	if b.calls != 1 {
		t.Errorf("calls = %d, want 1", b.calls)
	}
}

func TestPaginate_WhenErrorOnSecondPage_YieldsOnceAndStops(t *testing.T) {
	failure := errors.New("test failure")
	b := &fakeFetcher{
		pages: map[string]Page[int]{"": {Items: []int{1}, NextCursor: "p2"}},
		errAt: "p2",
		err:   failure,
	}
	got := collect(paginate(context.Background(), b.fetch))
	if len(got) != 2 {
		t.Fatalf("results = %v", got)
	}
	if got[0] != (result{1, nil}) {
		t.Errorf("first = %v", got[0])
	}
	if got[1].item != 0 || !errors.Is(got[1].err, failure) {
		t.Errorf("second = %v", got[1])
	}
	if b.calls != 2 {
		t.Errorf("calls = %d, want 2", b.calls)
	}
}

func TestPaginate_WhenRepeatedCursor_ReturnsErrRepeatedCursor(t *testing.T) {
	tests := []struct {
		name  string
		pages map[string]Page[int]
		want  int   // calls
		items []int // items yielded: those of the page that repeats the cursor are left out
	}{
		{
			name: "page pointing to itself yields no items",
			pages: map[string]Page[int]{
				"":   {Items: []int{1}, NextCursor: "p2"},
				"p2": {Items: []int{2}, NextCursor: "p2"},
			},
			want:  2,
			items: []int{1},
		},
		{
			name: "cycle A, B, A yields the new items of B and does not fetch again",
			pages: map[string]Page[int]{
				"":  {Items: []int{1}, NextCursor: "a"},
				"a": {Items: []int{2}, NextCursor: "b"},
				"b": {Items: []int{3}, NextCursor: "a"},
			},
			want:  3,
			items: []int{1, 2, 3},
		},
		{
			name: "longer cycle C, D, E, C yields the items of E",
			pages: map[string]Page[int]{
				"":  {Items: []int{1}, NextCursor: "c"},
				"c": {Items: []int{2}, NextCursor: "d"},
				"d": {Items: []int{3}, NextCursor: "e"},
				"e": {Items: []int{4}, NextCursor: "c"},
			},
			want:  4,
			items: []int{1, 2, 3, 4},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := &fakeFetcher{pages: tc.pages}
			got := collect(paginate(context.Background(), b.fetch))
			last := got[len(got)-1]
			if !errors.Is(last.err, ErrRepeatedCursor) {
				t.Errorf("last = %v, want ErrRepeatedCursor", last)
			}
			if b.calls != tc.want {
				t.Errorf("calls = %d, want %d", b.calls, tc.want)
			}
			var items []int
			for _, r := range got {
				if r.err == nil {
					items = append(items, r.item)
				}
			}
			if !slices.Equal(items, tc.items) {
				t.Errorf("items yielded = %v, want %v", items, tc.items)
			}
		})
	}
}

func TestPaginate_WhenContextCanceledBetweenPages_ReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &fakeFetcher{pages: map[string]Page[int]{
		"":   {Items: []int{1}, NextCursor: "p2"},
		"p2": {Items: []int{2}},
	}}
	b.onFetch = func(string) { cancel() }
	got := collect(paginate(ctx, b.fetch))
	if len(got) != 2 || got[0] != (result{1, nil}) || !errors.Is(got[1].err, context.Canceled) {
		t.Errorf("results = %v", got)
	}
	if b.calls != 1 {
		t.Errorf("calls = %d, want 1", b.calls)
	}
}

func TestPaginate_WhenEmptyPage_Ends(t *testing.T) {
	b := &fakeFetcher{pages: map[string]Page[int]{}}
	if got := collect(paginate(context.Background(), b.fetch)); len(got) != 0 || b.calls != 1 {
		t.Errorf("results = %v, calls = %d", got, b.calls)
	}
}
