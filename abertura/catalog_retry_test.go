package abertura

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// scenarios are representative situations of a catalog entry, with what
// decide answers for each one. The "" label is the unconditional case; the
// other labels ("nothing left", "left", "with mark", "without mark") are the
// two sides of a conditional rule written in the table.
type scenarios map[string]outcome

// retrySpec says, for each entry, how to build the outcome for the two "Retry"
// columns of the summary table in docs/errors.md. A missing column has no
// transport decision to compare (the text does not come from decide).
type retrySpec struct{ get, write scenarios }

func oneScenario(d outcome) scenarios { return scenarios{"": d} }

func withStatus(status int) scenarios { return oneScenario(outcome{status: status}) }

func missingWrite(failure failureKind) scenarios {
	return scenarios{
		"nothing left": {failure: failure, wrote: false},
		"left":         {failure: failure, wrote: true},
	}
}

var retrySpecs = map[string]retrySpec{
	"invalid-credential-authorizer": {
		// Without an already accepted token, the SDK does not renew: the error is
		// returned. The automatic renewal is in the text and is covered by the
		// test of the 403 rule.
		get:   oneScenario(outcome{status: 403, gateway: true, invalidCredential: true, renewable: true}),
		write: oneScenario(outcome{status: 403, gateway: true, invalidCredential: true, renewable: true}),
	},
	"invalid-credential-backend":  {get: withStatus(403), write: withStatus(403)},
	"field-validation":            {get: withStatus(400), write: withStatus(400)},
	"retailer-profile-required":   {get: withStatus(400), write: withStatus(400)},
	"migration-already-requested": {get: withStatus(400), write: withStatus(400)},
	"month-range-required":        {get: withStatus(400), write: withStatus(400)},
	"month-range-too-wide":        {get: withStatus(400), write: withStatus(400)},
	"unknown-code":                {get: withStatus(400), write: withStatus(400)},
	"refusal-without-code":        {get: withStatus(403), write: withStatus(403)},
	"not-found":                   {get: withStatus(404), write: withStatus(404)},
	"rate-limited": {
		get: withStatus(429),
		write: scenarios{
			"with mark":    {status: 429, gateway: true},
			"without mark": {status: 429},
		},
	},
	"internal-error":  {get: withStatus(500), write: withStatus(500)},
	"gateway-failure": {get: withStatus(502), write: withStatus(502)},
	"transport-error": {
		get:   oneScenario(outcome{failure: failureTransport, wrote: true}),
		write: missingWrite(failureTransport),
	},
	"timeout": {
		get:   oneScenario(outcome{failure: failureTimeout, wrote: true}),
		write: missingWrite(failureTimeout),
	},
	"call-canceled": {
		get:   oneScenario(outcome{failure: failureContext, wrote: true}),
		write: missingWrite(failureContext),
	},
	// The GET column says "not applicable": only the write comes from decide.
	"uncertain-result": {write: withStatus(502)},
	"unreadable-response": {
		get:   oneScenario(outcome{status: 200, unreadableBody: true}),
		write: oneScenario(outcome{status: 200, unreadableBody: true}),
	},
	// Without decide: local SDK error, or text from another source.
	"repeated-cursor":      {},
	"token-request-failed": {},
	"invalid-argument":     {},
	"not-in-catalog":       {},
}

// reSummaryRow matches the data rows of the summary table: "| [Title](#slug) | …".
var reSummaryRow = regexp.MustCompile(`^\|\s*\[[^\]]+\]\(#([a-z0-9-]+)\)\s*\|(.*)\|\s*$`)

// retryColumns returns, by slug, the text of the "Retry a GET" and "Retry a
// write" columns (the last two of each row).
func retryColumns(t *testing.T, doc string) map[string][2]string {
	t.Helper()
	out := map[string][2]string{}
	for _, row := range strings.Split(doc, "\n") {
		m := reSummaryRow.FindStringSubmatch(row)
		if m == nil {
			continue
		}
		cells := strings.Split(m[2], "|")
		if len(cells) != 4 {
			t.Fatalf("summary row com %d columns depois da entrada, quero 4: %s", len(cells), row)
		}
		out[m[1]] = [2]string{strings.TrimSpace(cells[2]), strings.TrimSpace(cells[3])}
	}
	return out
}

// responses applies decide to each scenario.
func responses(httpMethod string, cs scenarios) map[string]action {
	out := make(map[string]action, len(cs))
	for label, d := range cs {
		d.maxRetries = 1 // only so the decision is "retry", not "exhausted"
		out[label] = decide(httpMethod, d)
	}
	return out
}

// checkCell compares the text of a cell with what decide answered. Text with
// no known rule fails: whoever changes the catalog picks the rule here.
func checkCell(text string, r map[string]action) string {
	t := strings.ToLower(text)
	has := func(label string, want action) bool { a, ok := r[label]; return ok && a == want }
	switch {
	case strings.HasPrefix(t, "yes if nothing left"):
		if !has("nothing left", actionRetry) || !has("left", actionUncertain) {
			return "decide does not retry when nothing left and is uncertain when it left: " + formatResponses(r)
		}
	case strings.HasPrefix(t, "only with the gateway mark"):
		if !has("with mark", actionRetry) || !has("without mark", actionError) {
			return "decide retries only with the gateway mark: " + formatResponses(r)
		}
	case strings.HasPrefix(t, "uncertain if it already left"):
		if !has("left", actionUncertain) || !has("nothing left", actionError) {
			return "decide is not uncertain only when it already left: " + formatResponses(r)
		}
	case strings.HasPrefix(t, "your decision"):
		if !has("", actionError) {
			return "decide does not return the error without a retry: " + formatResponses(r)
		}
	case strings.HasPrefix(t, "yes"):
		if !has("", actionRetry) {
			return "decide does not order a retry: " + formatResponses(r)
		}
	case strings.HasPrefix(t, "uncertain"):
		if !has("", actionUncertain) {
			return "decide is not uncertain: " + formatResponses(r)
		}
	case strings.HasPrefix(t, "no"):
		if has("", actionRetry) {
			return "decide orders a retry, and the catalog says no: " + formatResponses(r)
		}
	default:
		return "text without a parity rule; add a rule in checkCell"
	}
	return ""
}

func formatResponses(r map[string]action) string {
	var parts []string
	for label, a := range r {
		if label == "" {
			label = "single"
		}
		parts = append(parts, label+"="+a.String())
	}
	return strings.Join(parts, ", ")
}

// TestCatalog_WhenRetryInTable_MatchesDecide proves that the "Retry a GET" and
// "Retry a write" columns of docs/errors.md say the same as decide answers for a
// representative situation of each entry.
func TestCatalog_WhenRetryInTable_MatchesDecide(t *testing.T) {
	t.Parallel()

	columns := retryColumns(t, readFile(t, "../docs/errors.md"))
	for slug := range catalog {
		if _, ok := columns[slug]; !ok {
			t.Errorf("entry %q has no row in the summary table of docs/errors.md", slug)
		}
		if _, ok := retrySpecs[slug]; !ok {
			t.Errorf("entry %q has no scenario in retrySpecs", slug)
		}
	}
	for slug, cols := range columns {
		spec, ok := retrySpecs[slug]
		if !ok {
			continue
		}
		t.Run(slug, func(t *testing.T) {
			t.Parallel()
			get := cols[0]
			if spec.get != nil {
				if msg := checkCell(get, responses(http.MethodGet, spec.get)); msg != "" {
					t.Errorf("GET column %q: %s", get, msg)
				}
			}
			if spec.write != nil {
				write := cols[1]
				if strings.EqualFold(write, "same") {
					write = get
				}
				if msg := checkCell(write, responses(http.MethodPost, spec.write)); msg != "" {
					t.Errorf("write column %q: %s", write, msg)
				}
			}
		})
	}
}

// TestCheckCell_WhenTextDivergesFromDecide_Fails proves that the parity test
// catches the divergence that motivated the check: the catalog says "retry"
// where the code does not retry.
func TestCheckCell_WhenTextDivergesFromDecide_Fails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		text       string
		httpMethod string
		cs         scenarios
		wantMsg    bool
	}{
		{name: "yes where decide retries", text: "yes", httpMethod: http.MethodGet, cs: withStatus(500)},
		{name: "yes where decide is uncertain", text: "yes", httpMethod: http.MethodPost, cs: withStatus(500), wantMsg: true},
		{name: "no where decide retries", text: "no", httpMethod: http.MethodGet, cs: withStatus(500), wantMsg: true},
		{name: "no where decide returns the error", text: "no", httpMethod: http.MethodGet, cs: withStatus(400)},
		{name: "uncertain where decide retries", text: "uncertain: check", httpMethod: http.MethodGet, cs: withStatus(500), wantMsg: true},
		{name: "new text without a rule", text: "maybe", httpMethod: http.MethodGet, cs: withStatus(500), wantMsg: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			msg := checkCell(tc.text, responses(tc.httpMethod, tc.cs))
			if (msg != "") != tc.wantMsg {
				t.Errorf("checkCell(%q) = %q, want divergence = %v", tc.text, msg, tc.wantMsg)
			}
		})
	}
}
