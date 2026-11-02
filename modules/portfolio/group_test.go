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
