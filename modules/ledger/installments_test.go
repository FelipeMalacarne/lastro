package ledger

import (
	"testing"
	"time"

	"lastro/money"
)

func TestGroupInstallments(t *testing.T) {
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	row := func(id, desc string, n, total int, cents int64, date time.Time) InstallmentRow {
		inst, err := NewInstallment(n, total)
		if err != nil {
			t.Fatal(err)
		}
		st := StatusPosted
		if date.After(day(2026, 10, 26)) {
			st = StatusPending
		}
		return InstallmentRow{ID: TransactionID(id), Description: desc, Installment: inst, Amount: money.BRLCents(cents), Date: date, Status: st}
	}
	rows := []InstallmentRow{
		// Nubank's suffix, and the rounding left on the first installment.
		row("a3", "Pag*Gatona 3/3", 3, 3, -5767, day(2026, 1, 29)),
		row("a1", "Pag*Gatona 1/3", 1, 3, -5770, day(2025, 12, 22)),
		row("a2", "PAG*GATONA 2/3", 2, 3, -5767, day(2025, 12, 29)), // the bank's capitals, once
		// Two purchases at the same shop with the same count: told apart by amount.
		row("b1", "Fast Shop", 1, 10, -31990, day(2026, 8, 22)),
		row("b2", "Fast Shop", 2, 10, -31990, day(2026, 9, 22)),
		row("b3", "Fast Shop", 3, 10, -31990, day(2026, 10, 22)),
		row("b4", "Fast Shop", 4, 10, -31990, day(2026, 11, 22)),
		row("c1", "Fast Shop", 1, 10, -8990, day(2026, 10, 2)),
		// Same shop, count and amount, but a repeated number: a second purchase.
		row("d1", "Uber One", 1, 2, -1990, day(2026, 3, 1)),
		row("e1", "Uber One", 1, 2, -1990, day(2026, 5, 1)),
		// Only the later installments are in the history.
		row("f5", "Cana Ropa 5/5", 5, 5, -10832, day(2025, 5, 29)),
		row("f4", "Cana Ropa 4/5", 4, 5, -10832, day(2025, 4, 28)),
	}
	got := GroupInstallments(rows)
	if len(got) != 6 {
		t.Fatalf("want 6 purchases, got %d: %+v", len(got), got)
	}
	byFirst := map[TransactionID]Purchase{}
	for _, p := range got {
		byFirst[p.Installments[0].ID] = p
	}
	today := day(2026, 10, 26)
	gatona := byFirst["a1"]
	if gatona.Description != "Pag*Gatona" || gatona.Total != 3 || len(gatona.Installments) != 3 || gatona.Amount != money.BRLCents(-5767) {
		t.Errorf("the suffix goes, the cents of rounding stay together: %+v", gatona)
	}
	fast := byFirst["b1"]
	if len(fast.Installments) != 4 || fast.Current(today) != 3 {
		t.Errorf("Fast Shop: %+v, current %d", fast, fast.Current(today))
	}
	if n, left := fast.Remaining(today); n != 7 || left != money.BRLCents(7*31990) {
		t.Errorf("7 installments still to pay, R$ 2.239,30: %d %s", n, left)
	}
	if last := fast.Last(); !last.Equal(day(2027, 5, 22)) {
		t.Errorf("the last installment, a month apart from the latest known: %v", last)
	}
	if c := byFirst["c1"]; len(c.Installments) != 1 || c.Amount != money.BRLCents(-8990) {
		t.Errorf("another amount is another purchase: %+v", c)
	}
	if _, ok := byFirst["e1"]; !ok || len(byFirst["d1"].Installments) != 1 {
		t.Errorf("a number seen twice starts a second purchase: %+v", got)
	}
	f := byFirst["f4"]
	if n, _ := f.Remaining(today); f.Current(today) != 5 || n != 0 || !f.Last().Equal(day(2025, 5, 29)) {
		t.Errorf("a finished purchase has nothing left: %+v", f)
	}
	if got[0].Installments[0].ID != "f4" || got[len(got)-1].Installments[0].ID != "c1" {
		t.Errorf("by first installment's date: %s … %s", got[0].Installments[0].ID, got[len(got)-1].Installments[0].ID)
	}
	if GroupInstallments(nil) != nil {
		t.Error("nothing in, nothing out")
	}
}
