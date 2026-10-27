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

// Valuation is the portfolio on a day: each class and the emergency fund,
// with their value and what was put into them.
type Valuation struct {
	Date      time.Time
	Holdings  []Holding
	Classes   map[Class]Amounts
	Emergency Amounts
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

// Invested is the allocation's value, the emergency fund left out.
func (v Valuation) Invested() Amounts {
	var val, basis int64
	for _, c := range Classes {
		val += v.Classes[c].Value.Cents()
		basis += v.Classes[c].Basis.Cents()
	}
	return Amounts{Value: money.BRLCents(val), Basis: money.BRLCents(basis)}
}

// Value values the open positions on a day. A stock or ETF takes the
// latest quote when it is newer than what its bank reported; everything
// else is what the bank said.
func Value(positions []*Position, prices map[string]Price, day time.Time) Valuation {
	v := Valuation{Date: day, Classes: make(map[Class]Amounts, len(Classes))}
	add := func(a Amounts, value int64, p *Position) Amounts {
		return Amounts{
			Value: money.BRLCents(a.Value.Cents() + value),
			Basis: money.BRLCents(a.Basis.Cents() + p.NetIn().Cents()),
		}
	}
	for _, c := range Classes {
		v.Classes[c] = Amounts{Value: money.BRLCents(0), Basis: money.BRLCents(0)}
	}
	v.Emergency = Amounts{Value: money.BRLCents(0), Basis: money.BRLCents(0)}
	for _, p := range positions {
		// A closed position is worth nothing, but what came out of it still
		// counts in its class's flows.
		var value int64
		if p.Open() {
			h := Holding{Position: p, Value: p.Value, At: p.AsOf}
			if q, ok := prices[p.Ticker.String()]; ok && !p.Ticker.IsZero() && !p.Quantity.IsZero() && q.Date.After(p.AsOf) && !q.Date.After(day) {
				h.Value, h.Priced, h.At = p.Quantity.Of(q.Price), true, q.Date
			}
			v.Holdings = append(v.Holdings, h)
			value = h.Value.Cents()
		}
		if p.Emergency() {
			v.Emergency = add(v.Emergency, value, p)
		} else {
			c := p.Class()
			v.Classes[c] = add(v.Classes[c], value, p)
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
// the allocation and the emergency fund, with what was put into them.
type NetWorth struct {
	Date      time.Time
	Cash      money.Money
	Cards     money.Money // negative
	Invested  money.Money
	Emergency money.Money
	Other     money.Money // assets outside Open Finance, typed in
	Basis     money.Money
}

// Total is the net worth.
func (n NetWorth) Total() money.Money {
	return money.BRLCents(n.Cash.Cents() + n.Cards.Cents() + n.Invested.Cents() + n.Emergency.Cents() + n.Other.Cents())
}
