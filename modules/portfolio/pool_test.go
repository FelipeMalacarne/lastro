package portfolio

import (
	"testing"
	"time"

	"lastro/money"
)

func TestClaim(t *testing.T) {
	pots := []Pot{
		{Name: "Emergency fund", Balance: money.BRLCents(1100000)},
		{Name: "Car", Balance: money.BRLCents(-40000)}, // in the red: claims nothing, takes nothing
		{Name: "Dates", Balance: money.BRLCents(157000)},
	}
	if c := Claim(pots).Cents(); c != 1257000 {
		t.Errorf("claim %d", c)
	}
	if c := Claim(nil).Cents(); c != 0 {
		t.Errorf("no pots claim %d", c)
	}
}

func TestPool(t *testing.T) {
	tests := []struct {
		name                       string
		value, claim               int64
		groups                     []int64 // each group's value; half of it is gains
		apart, left, short         int64
		pots                       []int64 // what each group sets apart
	}{
		{"left over", 2131000, 1257000, []int64{2131000}, 1257000, 874000, 0, []int64{1257000}},
		{"short", 1100000, 1257000, []int64{1100000}, 1100000, 0, 157000, []int64{1100000}},
		{"nothing claimed", 2131000, 0, []int64{2131000}, 0, 2131000, 0, []int64{0}},
		{"two groups, by value", 2531000, 1257000, []int64{2131000, 400000}, 1257000, 1274000, 0, []int64{1058343, 198657}},
		{"an empty group in the pool", 2131000, 1257000, []int64{2131000, 0}, 1257000, 874000, 0, []int64{1257000, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Pool{Value: money.BRLCents(tt.value), Claim: money.BRLCents(tt.claim)}
			if p.SetApart().Cents() != tt.apart || p.Left().Cents() != tt.left || p.Short().Cents() != tt.short {
				t.Errorf("apart %d, left %d, short %d", p.SetApart().Cents(), p.Left().Cents(), p.Short().Cents())
			}
			wholes := make([]Amounts, len(tt.groups))
			for i, v := range tt.groups {
				wholes[i] = Amounts{Value: money.BRLCents(v), Basis: money.BRLCents(v / 2)}
			}
			pots, rest := p.split(wholes)
			var sum int64
			for i, w := range wholes {
				if pots[i].Value.Cents() != tt.pots[i] {
					t.Errorf("group %d sets apart %d, want %d", i, pots[i].Value.Cents(), tt.pots[i])
				}
				if pots[i].Value.Cents()+rest[i].Value.Cents() != w.Value.Cents() || pots[i].Basis.Cents()+rest[i].Basis.Cents() != w.Basis.Cents() {
					t.Errorf("group %d: the split loses money: %+v + %+v", i, pots[i], rest[i])
				}
				sum += pots[i].Value.Cents()
			}
			if sum != tt.apart {
				t.Errorf("the groups set apart %d, the pool %d", sum, tt.apart)
			}
		})
	}
}

// Nothing in the pool: the pots' claim sets nothing apart and their money
// counts in its class.
func TestValueWithoutPool(t *testing.T) {
	day := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	p, _ := NewPosition(Report{ExternalID: "rdb", Kind: KindFixedIncome, Indexer: IndexerCDI, Value: money.BRLCents(50000), AsOf: day, Active: true}, day)
	v := Value(GroupPositions([]*Position{p}, nil), nil, day, money.BRLCents(30000))
	if v.Pots.Value.Cents() != 0 || v.Classes[ClassCDI].Value.Cents() != 50000 || !v.Pool.Claim.IsZero() || !v.Pool.Short().IsZero() {
		t.Errorf("pots %+v, CDI %+v, pool %+v", v.Pots, v.Classes[ClassCDI], v.Pool)
	}
}
