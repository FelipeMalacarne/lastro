package portfolio

import (
	"lastro/money"
)

// Pot is a pot on Budget as the portfolio sees it (ADR-0014): money set
// apart for something, kept invested in the pool.
type Pot struct {
	Name    string
	Balance money.Money // negative when it is in the red
	Target  money.Money // zero for none
}

// Claim is what the pots claim of the pool: their positive balances. A pot
// in the red claims nothing and takes nothing from the others.
func Claim(pots []Pot) money.Money {
	var cents int64
	for _, p := range pots {
		cents += max(p.Balance.Cents(), 0)
	}
	return money.BRLCents(cents)
}

// Pool is the groups that hold the pots, together, against what the pots
// claim of them. When it holds less, the shortfall belongs to the pots
// together: no pot comes first.
type Pool struct {
	Value money.Money // what the groups that hold the pots are worth
	Claim money.Money
}

// SetApart is what the pool sets apart for the pots: their claim, or all
// of it when it holds less.
func (p Pool) SetApart() money.Money {
	return money.BRLCents(min(p.Claim.Cents(), max(p.Value.Cents(), 0)))
}

// Left is what the pool holds beyond the claim, which counts in its
// groups' classes.
func (p Pool) Left() money.Money {
	return money.BRLCents(max(p.Value.Cents()-p.Claim.Cents(), 0))
}

// Short is what the pool lacks to cover the claim.
func (p Pool) Short() money.Money {
	return money.BRLCents(max(p.Claim.Cents()-max(p.Value.Cents(), 0), 0))
}

// split divides what the pool sets apart among its groups in proportion
// to their value, to the cent, and each group's part takes the same share
// of its net in, so the set-apart money earns what its group earns.
// wholes are the groups' amounts, in the pool's order.
func (p Pool) split(wholes []Amounts) (pots, rest []Amounts) {
	pots, rest = make([]Amounts, len(wholes)), make([]Amounts, len(wholes))
	weights := make([]int64, len(wholes))
	var any bool
	for i, w := range wholes {
		weights[i] = max(w.Value.Cents(), 0)
		any = any || weights[i] > 0
	}
	parts := make([]money.Money, len(wholes))
	if apart := p.SetApart(); any && apart.IsPositive() {
		parts, _ = apart.Allocate(weights...) // never fails: the weights are positive and the currency is one
	}
	for i, w := range wholes {
		e := parts[i].Cents()
		v, b := w.Value.Cents(), w.Basis.Cents()
		var eb int64
		switch {
		case e == v:
			eb = b // all of it, its net in too
		case v > 0:
			eb = b * e / v
		}
		pots[i] = Amounts{Value: money.BRLCents(e), Basis: money.BRLCents(eb)}
		rest[i] = Amounts{Value: money.BRLCents(v - e), Basis: money.BRLCents(b - eb)}
	}
	return pots, rest
}
