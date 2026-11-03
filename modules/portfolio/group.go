package portfolio

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"lastro/money"
)

// GroupID names the product a position is a lot of: the same bank, kind,
// ticker, name, indexer and rate. Each contribution to a Nubank Caixinha is
// a CDB of its own, with its own maturity, so the maturity does not split
// a group. The migration that created the groups computes the same id in
// SQL, so the parts and their order are fixed.
func GroupID(r Report) string {
	key := strings.Join([]string{r.Institution, string(r.Kind), r.Ticker.String(), r.Name, string(r.Indexer), strconv.FormatInt(r.Rate.Bps(), 10)}, "|")
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:16]
}

// GroupSetting is how I classify a group: its class (empty: Classify's)
// and whether it holds the pots (ADR-0014).
type GroupSetting struct {
	Override  Class
	HoldsPots bool
}

// Group is every lot of one product, open or closed, and how I classify
// them. A new lot joins its group with the group's class, and in the pool
// when the group holds the pots.
type Group struct {
	ID   string
	Lots []*Position
	GroupSetting
}

// GroupPositions puts each position in its group, in the order they come,
// with the settings stored for each group.
func GroupPositions(ps []*Position, settings map[string]GroupSetting) []*Group {
	byID := map[string]*Group{}
	var out []*Group
	for _, p := range ps {
		id := GroupID(p.Report)
		g, ok := byID[id]
		if !ok {
			g = &Group{ID: id, GroupSetting: settings[id]}
			byID[id] = g
			out = append(out, g)
		}
		g.Lots = append(g.Lots, p)
	}
	return out
}

// AutoClass is the class Classify gives the group's product.
func (g *Group) AutoClass() Class {
	p := g.Lots[0]
	return Classify(p.Kind, p.Indexer, p.Ticker, p.Name)
}

// Class is the allocation class the group counts in: mine when I chose
// one, Classify's otherwise.
func (g *Group) Class() Class {
	if g.Override != "" {
		return g.Override
	}
	return g.AutoClass()
}

// Reclassify puts every lot in another class, or back to Classify's with "".
func (g *Group) Reclassify(c Class) error {
	if c != "" {
		if _, err := ParseClass(string(c)); err != nil {
			return err
		}
	}
	if c == g.AutoClass() {
		c = ""
	}
	g.Override = c
	return nil
}

// SetHoldsPots puts the group in the pool that holds the pots, or takes it
// out. Other groups stay as they are: the pool may be several.
func (g *Group) SetHoldsPots(on bool) { g.HoldsPots = on }

// Open is whether any lot is still held.
func (g *Group) Open() bool {
	for _, p := range g.Lots {
		if p.Open() {
			return true
		}
	}
	return false
}

// CostShift is how far a lot's basis moved when its cost was typed or went
// back to the estimate. The stored history moves by the same amount from
// the day lastro first saw the lot: the difference was there all along, so
// no period's return jumps.
type CostShift struct {
	From  time.Time
	Shift money.Money
}

// priceable are the open lots whose cost can be typed.
func (g *Group) priceable() []*Position {
	var out []*Position
	for _, p := range g.Lots {
		if p.Open() && p.Priceable() {
			out = append(out, p)
		}
	}
	return out
}

// Priceable is whether the group has an open lot whose cost can be typed.
func (g *Group) Priceable() bool { return len(g.priceable()) > 0 }

// CostEstimated is whether an open lot's cost is still lastro's estimate.
func (g *Group) CostEstimated() bool {
	for _, p := range g.priceable() {
		if p.CostEstimated() {
			return true
		}
	}
	return false
}

// TypedAt is when its average price was last typed, or nil.
func (g *Group) TypedAt() *time.Time {
	var at *time.Time
	for _, p := range g.priceable() {
		if t := p.Typed(); t != nil && (at == nil || t.At.After(*at)) {
			at = &t.At
		}
	}
	return at
}

// SetAveragePrice makes each priceable lot cost its units at the price, and
// returns how far each basis moved. The pots' pool keeps what its bank sent.
func (g *Group) SetAveragePrice(price AveragePrice, at time.Time) ([]CostShift, error) {
	if g.HoldsPots {
		return nil, fmt.Errorf("%w: a group that holds the pots keeps its bank's cost", ErrInvalid)
	}
	lots := g.priceable()
	if len(lots) == 0 {
		return nil, fmt.Errorf("%w: its bank sends what it cost", ErrInvalid)
	}
	var out []CostShift
	for _, p := range lots {
		if d := p.setCost(p.Quantity.Of(price.Money()), at); !d.IsZero() {
			out = append(out, CostShift{From: p.FirstSeen(), Shift: d})
		}
	}
	return out, nil
}

// ClearAveragePrice puts back lastro's estimate on every lot with a typed
// price, and returns how far each basis moved.
func (g *Group) ClearAveragePrice() []CostShift {
	var out []CostShift
	for _, p := range g.priceable() {
		if d := p.clearCost(); !d.IsZero() {
			out = append(out, CostShift{From: p.FirstSeen(), Shift: d})
		}
	}
	return out
}
