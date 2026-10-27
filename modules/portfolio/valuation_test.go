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

// Money taken out counts at what it was worth: redeeming a CDB that gained
// nothing since the start is no loss.
func TestReturnAfterARedemption(t *testing.T) {
	d0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d1 := d0.AddDate(0, 6, 0)
	in := func(c int64) *money.Money { m := money.BRLCents(c); return &m }
	a, _ := NewPosition(Report{ExternalID: "a", Kind: KindFixedIncome, Value: money.BRLCents(1100000), Invested: in(1000000), AsOf: d0, Active: true}, d0)
	b, _ := NewPosition(Report{ExternalID: "b", Kind: KindFixedIncome, Value: money.BRLCents(1000000), Invested: in(1000000), AsOf: d0, Active: true}, d0)
	start := Value([]*Position{a, b}, nil, d0).Classes[ClassCDI]

	a.Close(d1) // redeemed at R$ 11.000, prices unchanged
	end := Value([]*Position{a, b}, nil, d1).Classes[ClassCDI]
	if r, ok := Return(start, end); !ok || r.Bps() != 0 {
		t.Errorf("redeeming at value: %d bps, want 0", r.Bps())
	}
	if a.Realized().Cents() != 100000 || a.NetIn().Cents() != -100000 {
		t.Errorf("realized %d, net in %d", a.Realized().Cents(), a.NetIn().Cents())
	}

	// Half of 100 shares bought at R$ 50, sold at R$ 100: R$ 2.500 realized.
	q := func(s string) Quantity { x, _ := ParseQuantity(s); return x }
	s, _ := NewPosition(Report{ExternalID: "s", Kind: KindStock, Quantity: q("100"), Value: money.BRLCents(500000), AsOf: d0, Active: true}, d0)
	s0 := Value([]*Position{s}, nil, d0).Classes[ClassBR]
	_ = s.Update(Report{ExternalID: "s", Kind: KindStock, Quantity: q("100"), Value: money.BRLCents(1000000), AsOf: d1, Active: true}, d1)
	_ = s.Update(Report{ExternalID: "s", Kind: KindStock, Quantity: q("50"), Value: money.BRLCents(500000), AsOf: d1, Active: true}, d1)
	if s.Basis().Cents() != 250000 || s.Realized().Cents() != 250000 {
		t.Errorf("after selling half: basis %d, realized %d", s.Basis().Cents(), s.Realized().Cents())
	}
	// The R$ 5.000 earned is all there: value now plus cash out minus what went in.
	s1 := Value([]*Position{s}, nil, d1).Classes[ClassBR]
	if gain := s1.Value.Cents() - s0.Value.Cents() - (s1.Basis.Cents() - s0.Basis.Cents()); gain != 500000 {
		t.Errorf("gain %d, want 500000", gain)
	}
}
