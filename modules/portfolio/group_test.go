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

func TestReserve(t *testing.T) {
	r12k, _ := AmountReserve(money.BRLCents(1200000))
	tests := []struct {
		name                string
		r                   Reserve
		value               int64
		mode                string
		emergency, short    int64
		splitValue, splitIn int64 // the emergency fund's share of a group with half its value in gains
	}{
		{"none", NoReserve(), 2131000, ReserveNone, 0, 0, 0, 0},
		{"an amount under the group", r12k, 2400000, ReserveAmount, 1200000, 0, 1200000, 600000},
		{"an amount over the group", r12k, 1080000, ReserveAmount, 1080000, 120000, 1080000, 540000},
		{"all of it", WholeReserve(), 2400000, ReserveAll, 2400000, 0, 2400000, 1200000},
	}
	for _, tt := range tests {
		v := money.BRLCents(tt.value)
		if tt.r.Mode() != tt.mode || tt.r.Of(v).Cents() != tt.emergency || tt.r.Short(v).Cents() != tt.short {
			t.Errorf("%s: %s, %d, short %d", tt.name, tt.r.Mode(), tt.r.Of(v).Cents(), tt.r.Short(v).Cents())
		}
		whole := Amounts{Value: v, Basis: money.BRLCents(tt.value / 2)}
		e, rest := tt.r.split(whole)
		if e.Value.Cents() != tt.splitValue || e.Basis.Cents() != tt.splitIn {
			t.Errorf("%s: emergency fund %+v", tt.name, e)
		}
		if e.Value.Cents()+rest.Value.Cents() != whole.Value.Cents() || e.Basis.Cents()+rest.Basis.Cents() != whole.Basis.Cents() {
			t.Errorf("%s: the split loses money: %+v + %+v", tt.name, e, rest)
		}
	}

	for _, bad := range []struct {
		mode  string
		cents int64
	}{{"amount", 0}, {"amount", -100}, {"half", 100}} {
		if _, err := ParseReserve(bad.mode, bad.cents); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s %d: %v", bad.mode, bad.cents, err)
		}
	}
	if r, err := ParseReserve("amount", 1200000); err != nil || r != r12k {
		t.Errorf("parse an amount: %+v %v", r, err)
	}
}

// The set-apart amount earns what its group earns: R$ 21.000 that grows 1%
// in a month, R$ 12.000 of it the emergency fund, gives both parts 1%, to a
// basis point (the fund's gain moves to the class as a flow, which modified
// Dietz counts for half the month).
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
	fund, _ := AmountReserve(money.BRLCents(1200000))
	settings := map[string]GroupSetting{GroupID(a.Report): {Reserve: fund}}

	v0 := Value(GroupPositions([]*Position{a, b, ipca}, settings), nil, d0)
	if len(v0.Groups) != 2 || len(v0.Groups[0].Holdings) != 2 {
		t.Fatalf("groups %+v", v0.Groups)
	}
	if v0.Emergency.Value.Cents() != 1200000 || v0.Classes[ClassCDI].Value.Cents() != 900000 || v0.Classes[ClassIPCA].Value.Cents() != 500000 {
		t.Errorf("emergency %+v, CDI %+v", v0.Emergency, v0.Classes[ClassCDI])
	}

	_ = a.Update(lot("a", 1515000, 1500000, d1), d1)
	_ = b.Update(lot("b", 606000, 600000, d1), d1)
	v1 := Value(GroupPositions([]*Position{a, b, ipca}, settings), nil, d1)
	for name, pair := range map[string][2]Amounts{"emergency": {v0.Emergency, v1.Emergency}, "CDI": {v0.Classes[ClassCDI], v1.Classes[ClassCDI]}} {
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
