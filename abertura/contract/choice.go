//go:build sandbox

package contract

import (
	"errors"
	"fmt"
	"time"

	"github.com/MarcoBuarque/abertura-mercado-energia/abertura"
)

// isActive says whether the migration still counts as active. Canceled,
// closed, expired and rejected ones do not count; a new or empty status counts
// (the API may create values).
func isActive(m abertura.Migration) bool {
	switch m.Status {
	case abertura.MigrationStatusCanceled, abertura.MigrationStatusClosed, abertura.MigrationStatusExpired, abertura.MigrationStatusRejected:
		return false
	}
	return true
}

// pickActive returns the first active migration in the list.
func pickActive(found []abertura.Migration) (abertura.Migration, bool) {
	for _, m := range found {
		if isActive(m) {
			return m, true
		}
	}
	return abertura.Migration{}, false
}

// ofConsumerUnit filters the migrations of the consumer unit and the
// distributor (0 does not filter by distributor). A list over a wide span may
// bring other consumer units if the server-side filter does not apply.
func ofConsumerUnit(list []abertura.Migration, consumerUnit string, distributor int) []abertura.Migration {
	var out []abertura.Migration
	for _, m := range list {
		if m.ConsumerUnit == consumerUnit && (distributor == 0 || m.Distributor == distributor) {
			out = append(out, m)
		}
	}
	return out
}

// initialSpanMonths is the width of the first span tried. On 01/11/2026 the
// sandbox accepted 12 months and refused 24 (cap between 12 and 24, see
// SOURCES D22).
const initialSpanMonths = 12

// initialSpan returns the first span to try: initialSpanMonths months ending
// today.
func initialSpan(today abertura.YearMonth) (from, to abertura.YearMonth) {
	return addMonths(today, -(initialSpanMonths - 1)), today
}

// maxSpanAttempts is the cap on requests of the step that looks for the widest
// month span the API accepts: the initial span and up to six halvings (from 12
// months four are enough: 12, 6, 3 and 1).
const maxSpanAttempts = 7

// halfSpan returns half of an n-month span, with a minimum of one month.
func halfSpan(n int) int {
	return max(1, n/2)
}

// monthsInSpan counts the months from from to to, both included.
func monthsInSpan(from, to abertura.YearMonth) int {
	return (to.Year*12 + int(to.Month)) - (from.Year*12 + int(from.Month)) + 1
}

// addMonths adds n months (n may be negative) to m.
func addMonths(m abertura.YearMonth, n int) abertura.YearMonth {
	total := m.Year*12 + int(m.Month) - 1 + n
	return abertura.NewYearMonth(total/12, time.Month(total%12+1))
}

// centeredSpan returns the n-month span centered on center: with even n, one
// more month before than after. With n = 1 it is the month itself.
func centeredSpan(center abertura.YearMonth, n int) (from, to abertura.YearMonth) {
	from = addMonths(center, -(n / 2))
	return from, addMonths(from, n-1)
}

// acceptedSpan is the result of [findAcceptedSpan].
type acceptedSpan struct {
	// From, To and Months describe the span the API accepted.
	From, To abertura.YearMonth
	Months   int
	// Refused is the size, in months, of the last span refused as too wide; 0
	// if the first one was accepted.
	Refused int
	// Page is the first page returned for the accepted span.
	Page abertura.Page[abertura.Migration]
}

// findAcceptedSpan asks for the first page from from0 to to0 and, while the API
// answers ErrMonthRangeTooWide, halves the span, always centered on center,
// until a page comes back or the span reaches one month. At most
// [maxSpanAttempts] requests. Any other error ends the search.
func findAcceptedSpan(from0, to0, center abertura.YearMonth, try func(from, to abertura.YearMonth) (abertura.Page[abertura.Migration], error)) (acceptedSpan, error) {
	from, to := from0, to0
	refused := 0
	for attempt := 1; ; attempt++ {
		n := monthsInSpan(from, to)
		page, err := try(from, to)
		if err == nil {
			return acceptedSpan{From: from, To: to, Months: n, Refused: refused, Page: page}, nil
		}
		if !errors.Is(err, abertura.ErrMonthRangeTooWide) || n == 1 || attempt >= maxSpanAttempts {
			return acceptedSpan{}, fmt.Errorf("span of %d months: %w", n, err)
		}
		refused = n
		from, to = centeredSpan(center, halfSpan(n))
	}
}
