package app

import (
	"strings"
	"testing"
	"time"

	"lastro/modules/budget"
	"lastro/money"
)

func TestBuildReport(t *testing.T) {
	sep, _ := budget.ParseMonth("2026-09")
	cents := func(c int64) *money.Money { m := money.BRLCents(c); return &m }
	in := reportInput{
		month: sep,
		today: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		view: budget.MonthView{
			Summary: budget.SummaryView{IncomeCents: 1840000, SpentCents: 828410, ContributionCents: 400000, SavingsRateBp: 3100},
			Lines: []budget.LineView{
				{ByCategory: []budget.LineCategoryView{{Category: "r"}}, SpentCents: 110430, AvailableCents: 90000},
				{ByCategory: []budget.LineCategoryView{{Category: "g"}}, SpentCents: 124110, AvailableCents: 140000},
			},
		},
		prev:  &budget.MonthView{Summary: budget.SummaryView{SpentCents: 881300}},
		names: map[string]string{"r": "Restaurants", "g": "Groceries"},
		recurring: budget.RecurringView{Charges: []budget.RecurringCharge{
			// As the recurring charges spell them; the notes read them as names, as the Dashboard does.
			{Name: "DUOLINGO", MonthlyCents: 2990, New: true, First: "2026-09-08"},
			{Name: "NETFLIX", MonthlyCents: 5590, Change: &budget.ChangeView{FromCents: 4490, ToCents: 5590, Since: "2026-09-12"}},
			{Name: "SPOTIFY", MonthlyCents: 2390, New: true, First: "2026-08-01"},
		}},
		waiting: 3,
		start:   cents(28019772), end: cents(28711012), putIn: cents(400000),
		dividends: 11240,
		returns:   cents(291240), offTarget: "IPCA+", offTargetBps: 300,
	}
	r := buildReport(in)
	if !r.Over || r.SpendingChangeBp == nil || *r.SpendingChangeBp != -600 {
		t.Fatalf("report %+v", r)
	}
	want := "You kept 31% of what came in. Spending was 6% below August, and Restaurants was the only category over its limit. Net worth grew R$ 6.912,40, with R$ 4.000,00 put in."
	if r.Sentence != want {
		t.Errorf("sentence:\n%s\nwant\n%s", r.Sentence, want)
	}
	if len(r.Notes) != 4 || !strings.HasPrefix(r.Notes[0], "Restaurants went R$ 204,30 over") || r.Notes[1] != "A new charge: Duolingo, R$ 29,90 a month." || !strings.HasPrefix(r.Notes[2], "Netflix went up") {
		t.Errorf("notes: %q (Spotify is not new in September)", r.Notes)
	}
	if r.ReturnsCents == nil || *r.ReturnsCents != 291240 || r.OffTarget != "IPCA+" || r.OffTargetBps != 300 {
		t.Errorf("investments: %+v", r)
	}
	if r.Categories[0].Name != "Restaurants" {
		t.Errorf("the most used limit first: %+v", r.Categories)
	}
	text := ReportText(r, "")
	for _, s := range []string{"lastro · September 2026", "Spending   R$ 8.284,10 (−6% vs Aug)", "Kept       31%", "• Duolingo"} {
		if !strings.Contains(text, s) && !strings.Contains(text, strings.TrimPrefix(s, "• ")) {
			t.Errorf("text misses %q:\n%s", s, text)
		}
	}
	if strings.Contains(text, "IFOOD") {
		t.Error("never a transaction")
	}
}

// The report's rows add up to the month's spending: the limits, the funds
// and what had no limit (#6: September left the Health fund out).
func TestReportRowsAddUpToSpending(t *testing.T) {
	sep, _ := budget.ParseMonth("2026-09")
	health, groceries, free := "h", "g", "Free to spend"
	r := buildReport(reportInput{
		month: sep,
		today: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC),
		view: budget.MonthView{
			Summary: budget.SummaryView{IncomeCents: 1288000, SpentCents: 704938},
			Lines: []budget.LineView{
				{ByCategory: []budget.LineCategoryView{{Category: "r"}}, SpentCents: 92040, AvailableCents: 90000},
				{Name: &free, ByCategory: []budget.LineCategoryView{{Category: "l"}, {Category: "s"}}, SpentCents: 51020, AvailableCents: 100000},
				{ByCategory: []budget.LineCategoryView{{Category: health}}, Fund: true, SpentCents: 11990, AvailableCents: 15000, LeftCents: 3010},
			},
			Unlimited: []budget.SpendView{{SpentCents: 549888}, {Category: &groceries, SpentCents: 0}},
		},
		names: map[string]string{"r": "Restaurants", "l": "Leisure", "s": "Shopping", "h": "Health", "g": "Groceries"},
	})
	if len(r.Funds) != 1 || r.Funds[0] != (ReportFund{Category: health, Name: "Health", SpentCents: 11990, BalanceCents: 3010}) {
		t.Fatalf("funds %+v", r.Funds)
	}
	var sum int64
	for _, c := range r.Categories {
		sum += c.SpentCents
	}
	for _, f := range r.Funds {
		sum += f.SpentCents
	}
	for _, u := range r.Unlimited {
		sum += u.SpentCents
	}
	if sum != r.SpentCents {
		t.Errorf("the rows add up to %d, spending is %d", sum, r.SpentCents)
	}
	if len(r.Categories) != 2 || len(r.Unlimited) != 1 || len(r.Notes) != 1 {
		t.Errorf("a fund is not a limit, and only a fund in the red is a note: %+v", r)
	}
}
