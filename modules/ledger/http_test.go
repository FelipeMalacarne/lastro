package ledger

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTransactionsEndpointValidatesTheFilter(t *testing.T) {
	mux := http.NewServeMux()
	New(nil, nil).RegisterRoutes(mux, slog.New(slog.NewTextHandler(io.Discard, nil)))
	tests := []struct {
		query string
		want  string
	}{
		{"", "month must look like"},
		{"month=10-2026", "month must look like"},
		{"month=2026-10&account=1", "account must be an account id"},
		{"month=2026-10&flow=sideways", "flow must be"},
		{"month=2026-10&q=" + strings.Repeat("a", 101), "limited to 100"},
		{"month=2026-10&day=2026-11-02", "day must be a day of the month"},
		{"month=2026-10&day=22", "day must be a day of the month"},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/transactions?"+tt.query, nil))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tt.want) {
			t.Errorf("%q: got %d %s", tt.query, rec.Code, rec.Body.String())
		}
	}
}

func TestParseFilter(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/transactions?month=2026-10&account=0b7f2c58-1d3e-4a4b-9c55-0e6f1f2a3b4c&flow=out&q=%20pão%20", nil)
	f, err := parseFilter(r)
	if err != nil {
		t.Fatal(err)
	}
	from, to := f.MonthRange()
	if from.Format("2006-01-02") != "2026-10-01" || to.Format("2006-01-02") != "2026-11-01" || f.Flow != FlowOut || f.Search != "pão" || f.Account == "" {
		t.Fatalf("unexpected filter %+v", f)
	}
	if got := likePattern(`50% off_now\`); got != `50\% off\_now\\` {
		t.Errorf("LIKE wildcards must be escaped, got %q", got)
	}
}

func TestSummarize(t *testing.T) {
	v := func(cents int64, kind, status string) TransactionView {
		return TransactionView{AmountCents: cents, CountedCents: cents, Kind: kind, Status: status}
	}
	s := summarize([]TransactionView{
		v(-4890, "expense", "posted"),
		v(-2310, "expense", "pending"),
		v(5990, "expense", "posted"), // a refund
		v(850000, "income", "posted"),
		v(-100000, "transfer", "posted"),
		v(148237, "bill_payment", "posted"),
		{AmountCents: -50000, CountedCents: -30000, Kind: "expense", Status: "posted"}, // a split's part under a filter
	})
	if s != (ListSummary{Count: 7, Pending: 1, InCents: 855990, OutCents: -37200}) {
		t.Fatalf("transfers and bill payments never count, a split counts its part: %+v", s)
	}
}

func TestReadEndpointsValidateIDs(t *testing.T) {
	mux := http.NewServeMux()
	New(nil, nil).RegisterRoutes(mux, slog.New(slog.NewTextHandler(io.Discard, nil)))
	tests := []struct{ path, want string }{
		{"/api/v1/accounts/nubank", "id must be an account id"},
		{"/api/v1/accounts/42/balance", "id must be an account id"},
		{"/api/v1/accounts/x/months", "id must be an account id"},
		{"/api/v1/accounts/x/installments", "id must be an account id"},
		{"/api/v1/transactions/ifood", "id must be a transaction id"},
		{"/api/v1/transactions/days", "month must look like"},
		{"/api/v1/transactions/days?month=October", "month must look like"},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tt.want) {
			t.Errorf("%s: got %d %s", tt.path, rec.Code, rec.Body.String())
		}
	}
}

func TestFilterRange(t *testing.T) {
	f := TransactionFilter{Month: time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)}
	if from, to := f.Range(); from.Format(time.DateOnly) != "2026-10-01" || to.Format(time.DateOnly) != "2026-11-01" {
		t.Errorf("the month: %v %v", from, to)
	}
	f.Day = time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	if from, to := f.Range(); from.Format(time.DateOnly) != "2026-10-22" || to.Format(time.DateOnly) != "2026-10-23" {
		t.Errorf("one day: %v %v", from, to)
	}
}
