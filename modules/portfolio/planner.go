package portfolio

import (
	"fmt"

	"lastro/money"
)

// Why says why a class gets what it gets in a contribution.
type Why string

const (
	WhyBelowBand Why = "below_band" // outside its band: comes first, up to the band's edge, then its share of the gap
	WhyGap       Why = "gap"        // under its target: its share of the gap
	WhyTarget    Why = "target"     // every class at its target: split by the targets
	WhyAboveBand Why = "above_band" // over its band: nothing this time
	WhyAtTarget  Why = "at_target"  // at or over its target, inside the band: nothing
)

// Split is one class's part of a contribution.
type Split struct {
	Class  Class
	Amount money.Money
	Why    Why
	Value  money.Money   // the class's value before
	Before money.Percent // its weight before
	After  money.Percent // its weight after
	Target money.Percent
}

// Plan is a contribution split by class, in display order. Its amounts add
// up to the contribution exactly and are never negative: lastro never
// suggests selling.
type Plan struct {
	Amount money.Money
	Total  money.Money // invested value after the contribution
	Splits []Split
}

// PlanContribution splits new money so the classes move toward their
// targets (F7). Classes below their band come first, up to the band's lower
// edge; what is left goes to every class under its target in proportion to
// how far under it is; past every target, the rest follows the targets.
// values are the classes' invested value now, the emergency fund left out.
func PlanContribution(values map[Class]money.Money, target Allocation, band Band, amount money.Money) (Plan, error) {
	if target.IsZero() {
		return Plan{}, fmt.Errorf("%w: no target allocation", ErrInvalid)
	}
	if amount.IsNegative() || amount.Currency() != money.BRL {
		return Plan{}, fmt.Errorf("%w: contribution of %s", ErrInvalid, amount)
	}
	var now int64
	for _, c := range Classes {
		if values[c].IsNegative() {
			return Plan{}, fmt.Errorf("%w: negative value for %s", ErrInvalid, c)
		}
		now += values[c].Cents()
	}
	total := now + amount.Cents()
	cur := map[Class]int64{}
	weight := map[Class]int64{} // before, in bps
	for _, c := range Classes {
		cur[c] = values[c].Cents()
		if now > 0 {
			weight[c] = cur[c] * 10000 / now
		}
	}
	give := map[Class]int64{}
	why := map[Class]Why{}
	left := amount.Cents()

	// spread hands out up to left, in proportion to need, never more than
	// each need. It returns what it gave.
	spread := func(need map[Class]int64) int64 {
		var sum int64
		ws := make([]int64, len(Classes))
		for i, c := range Classes {
			ws[i] = need[c]
			sum += need[c]
		}
		if sum == 0 || left == 0 {
			return 0
		}
		if sum <= left {
			for _, c := range Classes {
				give[c] += need[c]
			}
			return sum
		}
		parts, _ := money.BRLCents(left).Allocate(ws...)
		for i, c := range Classes {
			give[c] += parts[i].Cents()
		}
		return left
	}

	// 1. Below the band: up to its lower edge, after the contribution.
	edge := map[Class]int64{}
	if now > 0 {
		for _, c := range Classes {
			t := target.Of(c).Bps()
			if t > 0 && weight[c] < t-band.Bps() {
				why[c] = WhyBelowBand
				edge[c] = max(0, total*(t-band.Bps())/10000-cur[c])
			}
		}
	}
	left -= spread(edge)

	// 2. Under the target: the gap that remains, after step 1.
	gap := map[Class]int64{}
	for _, c := range Classes {
		g := total*target.Of(c).Bps()/10000 - cur[c] - give[c]
		if g > 0 {
			gap[c] = g
			if why[c] == "" {
				why[c] = WhyGap
			}
		}
	}
	left -= spread(gap)

	// 3. Every gap closed: the rest by the targets.
	if left > 0 {
		ws := make([]int64, len(Classes))
		for i, c := range Classes {
			ws[i] = target.Of(c).Bps()
		}
		parts, err := money.BRLCents(left).Allocate(ws...)
		if err != nil {
			return Plan{}, err
		}
		for i, c := range Classes {
			if parts[i].Cents() > 0 {
				give[c] += parts[i].Cents()
				if why[c] == "" {
					why[c] = WhyTarget
				}
			}
		}
	}

	p := Plan{Amount: amount, Total: money.BRLCents(total)}
	for _, c := range Classes {
		t := target.Of(c).Bps()
		w := why[c]
		if w == "" {
			w = WhyAtTarget
			if now > 0 && weight[c] > t+band.Bps() {
				w = WhyAboveBand
			}
		}
		s := Split{Class: c, Amount: money.BRLCents(give[c]), Why: w, Value: values[c], Before: money.FromBps(weight[c]), Target: money.FromBps(t)}
		if total > 0 {
			s.After = money.FromBps((cur[c] + give[c]) * 10000 / total)
		}
		if s.Value.Currency() == "" {
			s.Value = money.BRLCents(0)
		}
		p.Splits = append(p.Splits, s)
	}
	return p, nil
}
