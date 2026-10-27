package ledger

import (
	"testing"
	"time"
)

func TestMonthViews(t *testing.T) {
	m := func(mo time.Month, y int) time.Time { return time.Date(y, mo, 1, 0, 0, 0, 0, time.UTC) }
	itau := &AccountRef{ID: "itau", Name: "Checking", Institution: "Itaú", Type: "checking"}
	got := monthViews([]MonthFlowRow{
		{Month: m(9, 2026), Kind: KindExpense, Cents: -200000, Count: 30},
		{Month: m(10, 2026), Kind: KindBillPayment, Cents: -148237, Count: 1},
		{Month: m(10, 2026), Kind: KindExpense, Cents: -198640, Count: 21},
		{Month: m(10, 2026), Kind: KindExpense, Inflow: true, Cents: 5990, Count: 1},
		{Month: m(10, 2026), Kind: KindIncome, Inflow: true, Cents: 725000, Count: 1},
		{Month: m(10, 2026), Kind: KindTransfer, Counterpart: itau, Cents: -100000, Count: 1},
		{Month: m(11, 2026), Kind: KindExpense, Cents: -98200, Count: 5},
	}, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	if len(got) != 3 || got[0].Month != "2026-09" || got[2].Month != "2026-11" || got[1].Future || !got[2].Future {
		t.Fatalf("one view a month, the ones after today's marked future: %+v", got)
	}
	oct := got[1]
	if oct.InCents != 730990 || oct.OutCents != -446877 || oct.SpentCents != 198640 || oct.RefundsCents != 5990 || oct.Purchases != 21 || len(oct.Flows) != 5 {
		t.Errorf("October: %+v", oct)
	}
	if f := oct.Flows[4]; f.Kind != "transfer" || f.Direction != "out" || f.Counterpart != itau {
		t.Errorf("a transfer keeps the other side's account: %+v", f)
	}
	if len(monthViews(nil, time.Now())) != 0 {
		t.Error("no rows, no months")
	}
}

func TestDescriptionStats(t *testing.T) {
	got := descriptionStats([]MonthTotal{
		{Month: "2026-06", Cents: -9780, Count: 2},
		{Month: "2026-10", Cents: -4890, Count: 1},
		{Month: "2026-11", Cents: -100, Count: 1}, // outside the six months
	}, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	if len(got.Months) != 6 || got.Months[0].Month != "2026-05" || got.Months[5].Month != "2026-10" {
		t.Fatalf("six months, empty ones too: %+v", got.Months)
	}
	if got.Count != 3 || got.TotalCents != -14670 || got.Months[1].Cents != -9780 || got.Months[0].Count != 0 {
		t.Errorf("got %+v", got)
	}
}
