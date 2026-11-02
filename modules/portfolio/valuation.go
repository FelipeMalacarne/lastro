package portfolio

import (
	"time"

	"lastro/money"
)

// Price is a ticker's latest known price and its day.
type Price struct {
	Date  time.Time
	Price money.Money
}

// Holding is one open position valued on a day.
type Holding struct {
	Position *Position
	Value    money.Money
	// Priced reports whether the value comes from a market quote newer than
	// the bank's report, rather than from the bank.
	Priced bool
	At     time.Time // the day the value is from
}

// Valuation is the portfolio on a day: each group, each class and the
// pots, with their value and what was put into them, and the pool that
// holds the pots against what they claim (ADR-0014).
type Valuation struct {
	Date     time.Time
	Holdings []Holding
	Groups   []GroupValue
	Classes  map[Class]Amounts
	Pots     Amounts
	Pool     Pool
	// Claims are the pots behind the pool's claim, as the service read
	// them; Value leaves it empty.
	Claims []Pot
}

// GroupValue is a group on a day: its open lots, the whole group (the open
// lots' value; every lot's net in, the closed ones' too) and the part of it
// set apart for the pots.
type GroupValue struct {
	Group    *Group
	Holdings []Holding
	Amounts  Amounts
	Pots     Amounts
}

// Cost is what is still in the open lots, at cost: what "since bought" is
// measured against.
func (g GroupValue) Cost() money.Money {
	var c int64
	for _, h := range g.Holdings {
		c += h.Position.Basis().Cents()
	}
	return money.BRLCents(c)
}

// Amounts is a value and its basis: what went in minus what came out, at
// value, so returns count cashed-out gains.
type Amounts struct {
	Value money.Money
	Basis money.Money
}

// Values is each class's value, for the planner.
func (v Valuation) Values() map[Class]money.Money {
	out := make(map[Class]money.Money, len(Classes))
	for _, c := range Classes {
		out[c] = v.Classes[c].Value
	}
	return out
}

// Invested is the allocation's value, the pots left out.
func (v Valuation) Invested() Amounts {
	var val, basis int64
	for _, c := range Classes {
		val += v.Classes[c].Value.Cents()
		basis += v.Classes[c].Basis.Cents()
	}
	return Amounts{Value: money.BRLCents(val), Basis: money.BRLCents(basis)}
}

// Value values the open lots on a day. A stock or ETF takes the latest
// quote when it is newer than what its bank reported; everything else is
// what the bank said. Each group goes to its class, but for the part of it
// set apart for the pots: claim, what the pots claim, is set apart from
// the groups that hold them in proportion to their value.
func Value(groups []*Group, prices map[string]Price, day time.Time, claim money.Money) Valuation {
	v := Valuation{Date: day, Classes: make(map[Class]Amounts, len(Classes))}
	zero := Amounts{Value: money.BRLCents(0), Basis: money.BRLCents(0)}
	add := func(a, b Amounts) Amounts {
		return Amounts{Value: money.BRLCents(a.Value.Cents() + b.Value.Cents()), Basis: money.BRLCents(a.Basis.Cents() + b.Basis.Cents())}
	}
	for _, c := range Classes {
		v.Classes[c] = zero
	}
	v.Pots = zero
	var all []GroupValue
	for _, g := range groups {
		gv := GroupValue{Group: g, Amounts: zero}
		for _, p := range g.Lots {
			// A closed lot is worth nothing, but what came out of it still
			// counts in its group's flows.
			var value int64
			if p.Open() {
				h := Holding{Position: p, Value: p.Value, At: p.AsOf}
				if q, ok := prices[p.Ticker.String()]; ok && !p.Ticker.IsZero() && !p.Quantity.IsZero() && q.Date.After(p.AsOf) && !q.Date.After(day) {
					h.Value, h.Priced, h.At = p.Quantity.Of(q.Price), true, q.Date
				}
				gv.Holdings = append(gv.Holdings, h)
				value = h.Value.Cents()
			}
			gv.Amounts = add(gv.Amounts, Amounts{Value: money.BRLCents(value), Basis: p.NetIn()})
		}
		gv.Pots = zero
		all = append(all, gv)
	}
	var pool []int
	var wholes []Amounts
	var held int64
	for i, gv := range all {
		if gv.Group.HoldsPots && len(gv.Holdings) > 0 {
			pool, wholes = append(pool, i), append(wholes, gv.Amounts)
			held += gv.Amounts.Value.Cents()
		}
	}
	v.Pool = Pool{Value: money.BRLCents(held), Claim: money.BRLCents(max(claim.Cents(), 0))}
	if len(pool) == 0 {
		v.Pool.Claim = money.BRLCents(0) // nothing holds the pots: their money counts in its class
	}
	rests := map[int]Amounts{}
	pots, rest := v.Pool.split(wholes)
	for k, i := range pool {
		all[i].Pots, rests[i] = pots[k], rest[k]
	}
	for i, gv := range all {
		r, ok := rests[i]
		if !ok {
			r = gv.Amounts
		}
		v.Pots = add(v.Pots, gv.Pots)
		v.Classes[gv.Group.Class()] = add(v.Classes[gv.Group.Class()], r)
		v.Holdings = append(v.Holdings, gv.Holdings...)
		if len(gv.Holdings) > 0 {
			v.Groups = append(v.Groups, gv)
		}
	}
	return v
}

// Return is the return between two valuations of the same money, with what
// was put in or taken out between them counted half the time (modified
// Dietz). It is false when there was nothing to return on.
func Return(start, end Amounts) (money.Percent, bool) {
	flow := end.Basis.Cents() - start.Basis.Cents()
	gain := end.Value.Cents() - start.Value.Cents() - flow
	base := start.Value.Cents() + flow/2
	if base <= 0 {
		return money.Percent{}, false
	}
	return money.FromBps(gain * 10000 / base), true
}

// NetWorth is everything on a day: cash in accounts, what the cards owe,
// the allocation and the pots, with what was put into them.
type NetWorth struct {
	Date     time.Time
	Cash     money.Money
	Cards    money.Money // negative
	Invested money.Money
	Pots     money.Money
	Other    money.Money // assets outside Open Finance, typed in
	Basis    money.Money
}

// Total is the net worth.
func (n NetWorth) Total() money.Money {
	return money.BRLCents(n.Cash.Cents() + n.Cards.Cents() + n.Invested.Cents() + n.Pots.Cents() + n.Other.Cents())
}
