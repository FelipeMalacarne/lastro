package portfolio

import (
	"testing"
	"time"

	"lastro/money"
)

func TestValue(t *testing.T) {
	day := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	before := day.AddDate(0, 0, -1)
	q := func(s string) Quantity { x, _ := ParseQuantity(s); return x }
	tk := func(s string) Ticker { x, _ := NewTicker(s); return x }
	mk := func(r Report) *Position { r.Active, r.AsOf = true, before; p, _ := NewPosition(r, before); return p }

	cdb := mk(Report{ExternalID: "cdb", Kind: KindFixedIncome, Indexer: IndexerCDI, Value: money.BRLCents(100000)})
	reserve := mk(Report{ExternalID: "rdb", Kind: KindFixedIncome, Indexer: IndexerCDI, Value: money.BRLCents(50000)})
	reserve.MarkEmergency(true)
	wege := mk(Report{ExternalID: "wege", Kind: KindStock, Ticker: tk("WEGE3"), Quantity: q("10"), Value: money.BRLCents(40000)})
	ivvb := mk(Report{ExternalID: "ivvb", Kind: KindETF, Ticker: tk("IVVB11"), Quantity: q("2"), Value: money.BRLCents(70000)})
	closed := mk(Report{ExternalID: "old", Kind: KindFixedIncome, Value: money.BRLCents(99999)})
	closed.Close(day)

	v := Value([]*Position{cdb, reserve, wege, ivvb, closed}, map[string]Price{
		"WEGE3":  {Date: day, Price: money.BRLCents(4500)},
		"IVVB11": {Date: before.AddDate(0, 0, -3), Price: money.BRLCents(30000)}, // older than the bank's report
	}, day)
	if got := v.Classes[ClassBR]; got.Value.Cents() != 45000 || got.Basis.Cents() != 40000 {
		t.Errorf("WEGE3 at today's quote: %+v", got)
	}
	if got := v.Classes[ClassGlobal].Value.Cents(); got != 70000 {
		t.Errorf("IVVB11 keeps the bank's newer value: %d", got)
	}
	if v.Emergency.Value.Cents() != 50000 || v.Classes[ClassCDI].Value.Cents() != 100000 {
		t.Errorf("emergency %d, CDI %d", v.Emergency.Value.Cents(), v.Classes[ClassCDI].Value.Cents())
	}
	if v.Invested().Value.Cents() != 215000 || len(v.Holdings) != 4 {
		t.Errorf("invested %d, %d holdings", v.Invested().Value.Cents(), len(v.Holdings))
	}
}

func TestReturn(t *testing.T) {
	a := func(v, b int64) Amounts { return Amounts{Value: money.BRLCents(v), Basis: money.BRLCents(b)} }
	tests := []struct {
		name       string
		start, end Amounts
		bps        int64
		ok         bool
	}{
		{"10% with nothing added", a(100000, 100000), a(110000, 100000), 1000, true},
		// R$ 1.000 grew to R$ 1.150 with R$ 100 added halfway: 50 / 1.050.
		{"with a contribution", a(100000, 90000), a(115000, 100000), 476, true},
		{"from nothing, all put in", a(0, 0), a(100000, 100000), 0, true},
		{"nothing at all", a(0, 0), a(0, 0), 0, false},
	}
	for _, tt := range tests {
		got, ok := Return(tt.start, tt.end)
		if ok != tt.ok || (ok && got.Bps() != tt.bps) {
			t.Errorf("%s = %d (%v), want %d", tt.name, got.Bps(), ok, tt.bps)
		}
	}
}
