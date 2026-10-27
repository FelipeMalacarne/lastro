package budget

import (
	"fmt"
	"testing"
	"time"

	"lastro/modules/ledger"
	"lastro/money"
)

// monthly makes one charge a month, the last one months before today's
// month, on day; amounts are in cents, oldest first.
func monthly(description string, day int, monthsAgo int, amounts ...int64) []Charge {
	var out []Charge
	for i, a := range amounts {
		back := monthsAgo + len(amounts) - 1 - i
		d := time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC).AddDate(0, -back, 0)
		out = append(out, Charge{ID: ledger.TransactionID(fmt.Sprintf("%s-%d", description, i)), Description: description, Date: d, Amount: money.BRLCents(a)})
	}
	return out
}

func TestFindRecurring(t *testing.T) {
	today := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	var charges []Charge
	charges = append(charges, monthly("NETFLIX.COM", 18, 0, 4490, 4490, 4490, 4490, 4490, 4490)...)
	charges = append(charges, monthly("SPOTIFY", 21, 0, 2190, 2190, 2190, 2190, 2390, 2390)...)
	charges = append(charges, monthly("ENEL ENERGIA SP", 20, 0, 14000, 26000, 19000, 21000, 17500)...)
	charges = append(charges, monthly("CLARO INTERNET", 12, 0, 9990, 9990, 9990, 11990)...) // up 20% this month
	charges = append(charges, monthly("DUOLINGO", 9, 0, 3490, 3490)...)                     // new
	charges = append(charges, monthly("OLD GYM", 15, 3, 9900, 9900, 9900, 9900)...)         // stopped
	charges = append(charges, monthly("AMAZON MARKETPLACE", 3, 0, 6000, 32000, 9000, 15000)...)
	charges = append(charges, monthly("AIRBNB", 2, 0, 50000, 0, 0, 80000)...)        // a gap: not monthly
	charges = append(charges, monthly("MERCADO BOM", 12, 0, 60000, 90000, 50000)...) // a habit, not a bill
	// The same amount every month, on any day: a habit too.
	for i, d := range []int{2, 25, 9, 18} {
		charges = append(charges, Charge{ID: ledger.TransactionID(fmt.Sprintf("bar-%d", i)), Description: "BAR DO ZE", Date: time.Date(2026, time.Month(7+i), d, 0, 0, 0, 0, time.UTC), Amount: money.BRLCents(5000)})
	}
	// iFood several times a month.
	for m := 0; m < 6; m++ {
		for _, d := range []int{3, 11, 19, 26} {
			charges = append(charges, Charge{ID: ledger.TransactionID(fmt.Sprintf("ifood-%d-%d", m, d)), Description: "PAG*IFOOD SP", Date: time.Date(2026, 10-time.Month(m), d, 0, 0, 0, 0, time.UTC), Amount: money.BRLCents(4500)})
		}
	}

	got := map[string]Recurring{}
	for _, r := range FindRecurring(charges, today) {
		got[r.Name] = r
	}
	for _, name := range []string{"IFOOD", "OLD GYM", "AMAZON MARKETPLACE", "AIRBNB", "MERCADO BOM", "BAR DO ZE"} {
		if _, ok := got[name]; ok {
			t.Errorf("%s should not be recurring", name)
		}
	}

	tests := []struct {
		name    string
		steady  Steadiness
		monthly int64
		paid    bool
		isNew   bool
		change  string // "from>to"
	}{
		{name: "NETFLIX", steady: Fixed, monthly: 4490, paid: true},
		{name: "SPOTIFY", steady: Fixed, monthly: 2390, paid: true, change: "2190>2390"},
		{name: "ENEL ENERGIA", steady: Varies, monthly: (19000 + 21000 + 17500) / 3, paid: true},
		{name: "CLARO INTERNET", steady: Fixed, monthly: 11990, paid: true, change: "9990>11990"},
		{name: "DUOLINGO", steady: Fixed, monthly: 3490, paid: true, isNew: true},
	}
	for _, tt := range tests {
		r, ok := got[tt.name]
		if !ok {
			t.Errorf("%s not found; got %v", tt.name, keys(got))
			continue
		}
		if r.Steadiness != tt.steady || r.Monthly.Cents() != tt.monthly || r.PaidThisMonth != tt.paid || r.New != tt.isNew {
			t.Errorf("%s: %s %d paid=%v new=%v", tt.name, r.Steadiness, r.Monthly.Cents(), r.PaidThisMonth, r.New)
		}
		change := ""
		if r.Change != nil {
			change = fmt.Sprintf("%d>%d", r.Change.From.Cents(), r.Change.To.Cents())
		}
		if change != tt.change {
			t.Errorf("%s: change %q, want %q", tt.name, change, tt.change)
		}
	}
	if r := got["SPOTIFY"]; r.Change != nil && r.Change.Since.Format(time.DateOnly) != "2026-09-21" {
		t.Errorf("Spotify changed since %s", r.Change.Since.Format(time.DateOnly))
	}
	if r := got["NETFLIX"]; r.Next.Format(time.DateOnly) != "2026-11-18" || len(r.Charges) != 6 {
		t.Errorf("Netflix next %s, %d charges", r.Next.Format(time.DateOnly), len(r.Charges))
	}
}

func keys(m map[string]Recurring) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestRecurringAroundTheMonthsEnd(t *testing.T) {
	d := func(s string) time.Time { x, _ := time.Parse(time.DateOnly, s); return x }
	c := func(id, day string) Charge {
		return Charge{ID: ledger.TransactionID(id), Description: "CONDOMINIO SOLAR", Date: d(day), Amount: money.BRLCents(64000)}
	}
	// Paid on the 1st, once early on 31 Jul for August.
	cond := []Charge{c("a", "2026-05-01"), c("b", "2026-06-01"), c("c", "2026-07-01"), c("d", "2026-07-31"), c("e", "2026-09-01"), c("f", "2026-10-01")}
	got := FindRecurring(cond, d("2026-10-27"))
	if len(got) != 1 {
		t.Fatalf("a charge paid a day early is still monthly: %+v", got)
	}

	// Rent on the 31st comes back on 30 Nov, not on 1 Dec.
	rent := []Charge{}
	for i, day := range []string{"2026-07-31", "2026-08-31", "2026-09-30", "2026-10-31"} {
		rent = append(rent, Charge{ID: ledger.TransactionID(fmt.Sprint(i)), Description: "ALUGUEL", Date: d(day), Amount: money.BRLCents(250000)})
	}
	r := FindRecurring(rent, d("2026-11-10"))
	if len(r) != 1 || r[0].Next.Format(time.DateOnly) != "2026-11-30" {
		t.Errorf("next rent: %+v", r)
	}
}
