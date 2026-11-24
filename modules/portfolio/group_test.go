package portfolio

import (
	"errors"
	"testing"
	"time"

	"lastro/money"
)

func TestGroupID(t *testing.T) {
	jan, may := time.Date(2028, 1, 4, 0, 0, 0, 0, time.UTC), time.Date(2028, 5, 4, 0, 0, 0, 0, time.UTC)
	lot := func(id string, rate int64, maturity *time.Time) Report {
		return Report{ExternalID: id, Institution: "Nubank", Kind: KindFixedIncome, Name: "CDB - NU FINANCEIRA S.A.", Indexer: IndexerCDI, Rate: money.FromBps(rate), Maturity: maturity}
	}
	tests := []struct {
		name string
		a, b Report
		same bool
	}{
		{"two contributions to a Caixinha", lot("a", 10000, &jan), lot("b", 10000, &may), true},
		{"another rate", lot("a", 10000, &jan), lot("b", 12000, &jan), false},
		{"another bank", lot("a", 10000, &jan), func() Report { r := lot("b", 10000, &jan); r.Institution = "Inter"; return r }(), false},
	}
	for _, tt := range tests {
		if got := GroupID(tt.a) == GroupID(tt.b); got != tt.same {
			t.Errorf("%s: same group %v, want %v", tt.name, got, tt.same)
		}
	}
	// The migration computes the same id in SQL; this pins the format.
	if id := GroupID(lot("a", 10000, nil)); len(id) != 16 {
		t.Errorf("id %q", id)
	}
}

// The set-apart amount earns what its group earns: R$ 21.000 that grows 1%
// in a month, R$ 12.000 of it claimed by the pots, gives both parts 1%, to
// a basis point (the pots' gain moves to the class as a flow, which
// modified Dietz counts for half the month).
func TestValueSplitsAGroup(t *testing.T) {
	d0 := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	d1 := d0.AddDate(0, 1, 0)
	in := func(c int64) *money.Money { m := money.BRLCents(c); return &m }
	lot := func(id string, value, invested int64, at time.Time) Report {
		return Report{ExternalID: id, Institution: "Nubank", Kind: KindFixedIncome, Name: "CDB", Indexer: IndexerCDI, Rate: money.FromBps(10000), Value: money.BRLCents(value), Invested: in(invested), AsOf: at, Active: true}
	}
	a, _ := NewPosition(lot("a", 1500000, 1500000, d0), d0)
	b, _ := NewPosition(lot("b", 600000, 600000, d0), d0)
	ipca, _ := NewPosition(Report{ExternalID: "ipca", Kind: KindTreasury, Name: "Tesouro IPCA+ 2035", Indexer: IndexerIPCA, Value: money.BRLCents(500000), AsOf: d0, Active: true}, d0)
	settings := map[string]GroupSetting{GroupID(a.Report): {HoldsPots: true}}
	claim := money.BRLCents(1200000)

	v0 := Value(GroupPositions([]*Position{a, b, ipca}, settings), nil, d0, claim)
	if len(v0.Groups) != 2 || len(v0.Groups[0].Holdings) != 2 {
		t.Fatalf("groups %+v", v0.Groups)
	}
	if v0.Pots.Value.Cents() != 1200000 || v0.Classes[ClassCDI].Value.Cents() != 900000 || v0.Classes[ClassIPCA].Value.Cents() != 500000 {
		t.Errorf("pots %+v, CDI %+v", v0.Pots, v0.Classes[ClassCDI])
	}

	_ = a.Update(lot("a", 1515000, 1500000, d1), d1)
	_ = b.Update(lot("b", 606000, 600000, d1), d1)
	v1 := Value(GroupPositions([]*Position{a, b, ipca}, settings), nil, d1, claim)
	for name, pair := range map[string][2]Amounts{"pots": {v0.Pots, v1.Pots}, "CDI": {v0.Classes[ClassCDI], v1.Classes[ClassCDI]}} {
		if r, ok := Return(pair[0], pair[1]); !ok || r.Bps() < 99 || r.Bps() > 101 {
			t.Errorf("%s returned %d bps, want 1%%", name, r.Bps())
		}
	}
	if c := v1.Groups[0].Cost().Cents(); c != 2100000 {
		t.Errorf("cost %d", c)
	}
}

func TestGroupReclassify(t *testing.T) {
	p, _ := NewPosition(Report{ExternalID: "a", Kind: KindFixedIncome, Indexer: IndexerCDI, Value: money.BRLCents(100), Active: true}, time.Now())
	g := GroupPositions([]*Position{p}, nil)[0]
	if err := g.Reclassify(ClassIPCA); err != nil || g.Class() != ClassIPCA {
		t.Errorf("to IPCA+: %v, %s", err, g.Class())
	}
	if err := g.Reclassify(ClassCDI); err != nil || g.Override != "" {
		t.Errorf("back to its automatic class keeps no override: %q", g.Override)
	}
	if err := g.Reclassify("crypto"); !errors.Is(err, ErrInvalid) {
		t.Errorf("an unknown class: %v", err)
	}
}

// WRLD11 was first seen at R$ 153,61 a unit, its bank sending no cost; the
// broker's average is R$ 137,00. Typing it moves the basis and says by how
// much, so the stored history can move with it; clearing it puts the
// estimate back.
func TestAveragePrice(t *testing.T) {
	seen := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	typed := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	q := func(s string) Quantity { x, _ := ParseQuantity(s); return x }
	tk, _ := NewTicker("WRLD11")
	etf := func(id string, units string, value int64) *Position {
		p, err := NewPosition(Report{ExternalID: id, Institution: "Nubank", Kind: KindETF, Name: "WRLD11", Ticker: tk, Quantity: q(units), Value: money.BRLCents(value), AsOf: seen, Active: true}, seen)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	price, err := NewAveragePrice(money.BRLCents(13700))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("one lot", func(t *testing.T) {
		p := etf("a", "70", 1075270)
		g := GroupPositions([]*Position{p}, nil)[0]
		if !g.Priceable() || !g.CostEstimated() || g.TypedAt() != nil {
			t.Fatalf("before: priceable %v, estimated %v", g.Priceable(), g.CostEstimated())
		}
		shifts, err := g.SetAveragePrice(price, typed)
		if err != nil {
			t.Fatal(err)
		}
		if p.Basis().Cents() != 959000 || len(shifts) != 1 || shifts[0].Shift.Cents() != -116270 || !shifts[0].From.Equal(seen) {
			t.Fatalf("basis %d, shifts %+v", p.Basis().Cents(), shifts)
		}
		if g.CostEstimated() || g.TypedAt() == nil || !g.TypedAt().Equal(typed) {
			t.Errorf("after: estimated %v, typed %v", g.CostEstimated(), g.TypedAt())
		}
		// Typed again: the shift from the estimate adds up, the move is the
		// difference.
		again, _ := NewAveragePrice(money.BRLCents(14000))
		shifts, _ = g.SetAveragePrice(again, typed)
		if p.Basis().Cents() != 980000 || shifts[0].Shift.Cents() != 21000 || p.Typed().Shift.Cents() != -95270 {
			t.Errorf("again: basis %d, shifts %+v, typed %+v", p.Basis().Cents(), shifts, p.Typed())
		}
		shifts = g.ClearAveragePrice()
		if p.Basis().Cents() != 1075270 || len(shifts) != 1 || shifts[0].Shift.Cents() != 95270 || !g.CostEstimated() {
			t.Errorf("cleared: basis %d, shifts %+v", p.Basis().Cents(), shifts)
		}
		if shifts := g.ClearAveragePrice(); len(shifts) != 0 {
			t.Errorf("cleared twice: %+v", shifts)
		}
	})

	t.Run("lots share the price by units", func(t *testing.T) {
		a, b := etf("a", "50", 768050), etf("b", "20", 307220)
		g := GroupPositions([]*Position{a, b}, nil)[0]
		if _, err := g.SetAveragePrice(price, typed); err != nil {
			t.Fatal(err)
		}
		if a.Basis().Cents() != 685000 || b.Basis().Cents() != 274000 {
			t.Errorf("bases %d and %d", a.Basis().Cents(), b.Basis().Cents())
		}
	})

	t.Run("a sale takes its share of the shift", func(t *testing.T) {
		p := etf("a", "70", 1075270)
		g := GroupPositions([]*Position{p}, nil)[0]
		_, _ = g.SetAveragePrice(price, typed)
		r := p.Report
		r.Quantity, r.Value = q("35"), money.BRLCents(542885)
		if err := p.Update(r, typed); err != nil {
			t.Fatal(err)
		}
		if p.Basis().Cents() != 479500 || p.Typed().Shift.Cents() != -58135 {
			t.Errorf("basis %d, typed %+v", p.Basis().Cents(), p.Typed())
		}
		g.ClearAveragePrice()
		if p.Basis().Cents() != 537635 {
			t.Errorf("cleared: basis %d, want half the estimate", p.Basis().Cents())
		}
	})

	t.Run("the bank's figure wins", func(t *testing.T) {
		p := etf("a", "70", 1075270)
		g := GroupPositions([]*Position{p}, nil)[0]
		_, _ = g.SetAveragePrice(price, typed)
		r := p.Report
		in := money.BRLCents(950000)
		r.Invested = &in
		if err := p.Update(r, typed); err != nil {
			t.Fatal(err)
		}
		if p.Typed() != nil || p.Basis().Cents() != 950000 || g.Priceable() || g.CostEstimated() {
			t.Errorf("typed %+v, basis %d", p.Typed(), p.Basis().Cents())
		}
		if _, err := g.SetAveragePrice(price, typed); !errors.Is(err, ErrInvalid) {
			t.Errorf("priced a lot with a bank figure: %v", err)
		}
	})

	t.Run("refused", func(t *testing.T) {
		p := etf("a", "70", 1075270)
		g := GroupPositions([]*Position{p}, map[string]GroupSetting{GroupID(p.Report): {HoldsPots: true}})[0]
		if _, err := g.SetAveragePrice(price, typed); !errors.Is(err, ErrInvalid) {
			t.Errorf("a group holding the pots: %v", err)
		}
		cdb, _ := NewPosition(Report{ExternalID: "cdb", Kind: KindFixedIncome, Name: "CDB", Indexer: IndexerCDI, Value: money.BRLCents(100000), AsOf: seen, Active: true}, seen)
		if g := GroupPositions([]*Position{cdb}, nil)[0]; g.Priceable() || g.CostEstimated() {
			t.Errorf("a CDB is priceable")
		}
		for _, c := range []int64{0, -100} {
			if _, err := NewAveragePrice(money.BRLCents(c)); !errors.Is(err, ErrInvalid) {
				t.Errorf("price %d: %v", c, err)
			}
		}
	})
}
