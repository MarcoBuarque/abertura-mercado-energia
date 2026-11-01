//go:build sandbox

package contract

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	abertura "github.com/MarcoBuarque/abertura-mercado-energia/abertura"
)

func TestPickActive_WhenAmongFound_PicksAnActiveOne(t *testing.T) {
	m := func(id string, s abertura.MigrationStatus) abertura.Migration {
		return abertura.Migration{ID: id, Status: s, ConsumerUnit: "000000000000001", Distributor: 900003}
	}
	tests := []struct {
		name   string
		found  []abertura.Migration
		wantID string
		wantOK bool
	}{
		{name: "none found", found: nil},
		{name: "only inactive", found: []abertura.Migration{m("a", abertura.MigrationStatusCanceled), m("b", abertura.MigrationStatusClosed), m("c", abertura.MigrationStatusExpired), m("d", abertura.MigrationStatusRejected)}},
		{name: "skips the inactive and picks the first active", found: []abertura.Migration{m("a", abertura.MigrationStatusCanceled), m("b", abertura.MigrationStatusCreated), m("c", abertura.MigrationStatusCompleted)}, wantID: "b", wantOK: true},
		{name: "a new, unknown status counts as active", found: []abertura.Migration{m("a", abertura.MigrationStatusRejected), m("b", "EM_ANALISE")}, wantID: "b", wantOK: true},
		{name: "an empty status counts as active", found: []abertura.Migration{m("a", "")}, wantID: "a", wantOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := pickActive(tc.found)
			if ok != tc.wantOK || got.ID != tc.wantID {
				t.Errorf("pickActive = (%q, %v), want (%q, %v)", got.ID, ok, tc.wantID, tc.wantOK)
			}
		})
	}
}

func TestOfConsumerUnit_WhenWideList_FiltersByConsumerUnitAndDistributor(t *testing.T) {
	m := func(id, consumerUnit string, conc int) abertura.Migration {
		return abertura.Migration{ID: id, ConsumerUnit: consumerUnit, Distributor: conc, Month: abertura.NewYearMonth(2027, time.January)}
	}
	list := []abertura.Migration{
		m("a", "000000000000001", 900003), m("b", "000000000000002", 900003),
		m("c", "000000000000001", 900004), m("d", "000000000000001", 900003),
	}
	got := ofConsumerUnit(list, "000000000000001", 900003)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "d" {
		t.Errorf("ofConsumerUnit = %v, want a and d", got)
	}
	if got := ofConsumerUnit(list, "000000000000001", 0); len(got) != 3 {
		t.Errorf("distributor 0 does not filter: %d found, want 3", len(got))
	}
}

func TestErrorCode_WhenNonStandard_DoesNotPrintTheText(t *testing.T) {
	const text = "contact@example.org breaks the pattern"
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "standard code", body: `{"error":"ERR_MIGRACAO_SOLICITADA"}`, want: "ERR_MIGRACAO_SOLICITADA"},
		{name: "without the field", body: `{"message":"x"}`, want: ""},
		{name: "free text", body: `{"error":"` + text + `"}`, want: "non-standard code"},
		{name: "lowercase", body: `{"error":"err_x"}`, want: "non-standard code"},
		{name: "fixed OAuth2 code from RFC 6749", body: `{"error":"invalid_request"}`, want: "invalid_request"},
		{name: "OAuth2 code with a suffix", body: `{"error":"invalid_requestX"}`, want: "non-standard code"},
		{name: "OAuth2 code in uppercase", body: `{"error":"INVALID_REQUEST"}`, want: "non-standard code"},
		{name: "too long", body: `{"error":"ERR_` + strings.Repeat("A", 81) + `"}`, want: "non-standard code"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := errorCode([]byte(tc.body)); got != tc.want {
				t.Errorf("errorCode = %q, want %q", got, tc.want)
			}
		})
	}
	api := &abertura.APIError{Operation: "createMigration", StatusCode: 400, Code: text}
	if d := describe(api); strings.Contains(d, "contact@") {
		t.Errorf("describe printed the free text of the error field: %s", d)
	} else if !strings.Contains(d, "non-standard code") {
		t.Errorf("describe devia marcar o non-standard code: %s", d)
	}
}

// fakeBase always returns the same response, without a network.
type fakeBase struct {
	status int
	body   string
}

func (b fakeBase) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: b.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(b.body))}, nil
}

func TestBudget_WhenOAuth2Error_HintOnlyForFixedDescriptions(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantHint  string
		forbidden string
	}{
		{name: "fixed description duplicated parameter", body: `{"error":"invalid_request","error_description":"duplicated parameter"}`, wantHint: "oauth2_error_description: duplicated parameter"},
		{name: "fixed description missing grant_type", body: `{"error":"invalid_request","error_description":"Missing form parameter: grant_type"}`, wantHint: "oauth2_error_description: Missing form parameter: grant_type"},
		{name: "free description does not appear", body: `{"error":"invalid_request","error_description":"contact@example.org livre"}`, forbidden: "contact@"},
		{name: "variation of the fixed description does not appear", body: `{"error":"invalid_request","error_description":"duplicated parameter!"}`, forbidden: "duplicated"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o, err := newBudget(fakeBase{status: 400, body: tc.body}, 5)
			if err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequest(http.MethodPost, "https://example.invalid/sso/oauth/token", strings.NewReader(""))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := o.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			lines := strings.Join(o.drain(), "\n")
			if !strings.Contains(lines, "code: invalid_request") {
				t.Errorf("line without the OAuth2 code: %q", lines)
			}
			if tc.wantHint != "" && !strings.Contains(lines, tc.wantHint) {
				t.Errorf("line without the hint %q: %q", tc.wantHint, lines)
			}
			if strings.Contains(lines, "oauth2_error_description") && tc.wantHint == "" {
				t.Errorf("line mentions a description that is not fixed: %q", lines)
			}
			if tc.forbidden != "" && strings.Contains(lines, tc.forbidden) {
				t.Errorf("line leaks %q: %q", tc.forbidden, lines)
			}
		})
	}
}

func TestHalfSpan_WhenWideSpan_HalvesDownToOneMonth(t *testing.T) {
	got := []int{96}
	for n := 96; n > 1; {
		n = halfSpan(n)
		got = append(got, n)
	}
	want := []int{96, 48, 24, 12, 6, 3, 1}
	if len(got) != len(want) {
		t.Fatalf("sequence = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sequence = %v, want %v", got, want)
		}
	}
	if halfSpan(1) != 1 {
		t.Errorf("halfSpan(1) = %d, want 1", halfSpan(1))
	}
}

func TestCenteredSpan_WhenCalled_HasTheRequestedSize(t *testing.T) {
	center := abertura.NewYearMonth(2026, time.November)
	tests := []struct {
		name           string
		n              int
		wantFrom, want abertura.YearMonth
	}{
		{name: "one month is the month itself", n: 1, wantFrom: center, want: center},
		{name: "three months", n: 3, wantFrom: abertura.NewYearMonth(2026, time.October), want: abertura.NewYearMonth(2026, time.December)},
		{name: "twelve months cross the year", n: 12, wantFrom: abertura.NewYearMonth(2026, time.May), want: abertura.NewYearMonth(2027, time.April)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			from, to := centeredSpan(center, tc.n)
			if from != tc.wantFrom || to != tc.want {
				t.Errorf("centeredSpan = %v to %v, want %v to %v", from, to, tc.wantFrom, tc.want)
			}
			if got := monthsInSpan(from, to); got != tc.n {
				t.Errorf("monthsInSpan = %d, want %d", got, tc.n)
			}
		})
	}
}

func TestFindAcceptedSpan_WhenTooWide_NarrowsUntilTheAPIAccepts(t *testing.T) {
	center := abertura.NewYearMonth(2026, time.November)
	from0, to0 := abertura.NewYearMonth(2020, time.January), abertura.NewYearMonth(2027, time.December)
	tests := []struct {
		name         string
		cap          int // widest span the server accepts
		wantN        int
		wantRefused  int
		wantAttempts int
		wantErr      bool
	}{
		{name: "accepted at the first try", cap: 200, wantN: 96, wantRefused: 0, wantAttempts: 1},
		{name: "cap of 12 months", cap: 12, wantN: 12, wantRefused: 24, wantAttempts: 4},
		{name: "cap of 1 month", cap: 1, wantN: 1, wantRefused: 3, wantAttempts: 7},
		{name: "not even one month passes", cap: 0, wantErr: true, wantAttempts: 7},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			attempts := 0
			try := func(from, to abertura.YearMonth) (abertura.Page[abertura.Migration], error) {
				attempts++
				if monthsInSpan(from, to) > tc.cap {
					return abertura.Page[abertura.Migration]{}, abertura.ErrMonthRangeTooWide
				}
				return abertura.Page[abertura.Migration]{Next: "c"}, nil
			}
			r, err := findAcceptedSpan(from0, to0, center, try)
			if attempts != tc.wantAttempts {
				t.Errorf("attempts = %d, want %d", attempts, tc.wantAttempts)
			}
			if tc.wantErr {
				if !errors.Is(err, abertura.ErrMonthRangeTooWide) {
					t.Fatalf("err = %v, want ErrMonthRangeTooWide", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if r.Months != tc.wantN || r.Refused != tc.wantRefused || r.Page.Next != "c" {
				t.Errorf("result = %d months, refused %d, page %q; want %d, %d", r.Months, r.Refused, r.Page.Next, tc.wantN, tc.wantRefused)
			}
		})
	}

	t.Run("another error does not narrow", func(t *testing.T) {
		attempts := 0
		boom := errors.New("boom")
		_, err := findAcceptedSpan(from0, to0, center, func(_, _ abertura.YearMonth) (abertura.Page[abertura.Migration], error) {
			attempts++
			return abertura.Page[abertura.Migration]{}, boom
		})
		if !errors.Is(err, boom) || attempts != 1 {
			t.Errorf("err = %v, attempts = %d; want boom and 1", err, attempts)
		}
	})
}

func TestInitialSpan(t *testing.T) {
	tests := []struct {
		name             string
		today            abertura.YearMonth
		wantFrom, wantTo abertura.YearMonth
	}{
		{name: "middle of the year", today: abertura.NewYearMonth(2026, time.November), wantFrom: abertura.NewYearMonth(2025, time.December), wantTo: abertura.NewYearMonth(2026, time.November)},
		{name: "December", today: abertura.NewYearMonth(2026, time.December), wantFrom: abertura.NewYearMonth(2026, time.January), wantTo: abertura.NewYearMonth(2026, time.December)},
		{name: "January", today: abertura.NewYearMonth(2027, time.January), wantFrom: abertura.NewYearMonth(2026, time.February), wantTo: abertura.NewYearMonth(2027, time.January)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			from, to := initialSpan(tc.today)
			if from != tc.wantFrom || to != tc.wantTo || monthsInSpan(from, to) != initialSpanMonths {
				t.Errorf("initialSpan = %v to %v (%d months); want %v to %v (%d)", from, to, monthsInSpan(from, to), tc.wantFrom, tc.wantTo, initialSpanMonths)
			}
		})
	}
}
