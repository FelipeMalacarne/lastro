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
			Categories: []budget.CategoryView{
				{Category: "r", SpentCents: 110430, AvailableCents: 90000},
				{Category: "g", SpentCents: 124110, AvailableCents: 140000},
			},
		},
		prev:  &budget.MonthView{Summary: budget.SummaryView{SpentCents: 881300}},
		names: map[string]string{"r": "Restaurants", "g": "Groceries"},
		recurring: budget.RecurringView{Charges: []budget.RecurringCharge{
			{Name: "Duolingo", MonthlyCents: 2990, New: true, First: "2026-09-08"},
			{Name: "Netflix", MonthlyCents: 5590, Change: &budget.ChangeView{FromCents: 4490, ToCents: 5590, Since: "2026-09-12"}},
			{Name: "Spotify", MonthlyCents: 2390, New: true, First: "2026-08-01"},
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
	if len(r.Notes) != 4 || !strings.HasPrefix(r.Notes[0], "Restaurants went R$ 204,30 over") || !strings.Contains(r.Notes[1], "Duolingo") || !strings.Contains(r.Notes[2], "Netflix went up") {
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
