package portfolio

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"lastro/money"
)

// Reserve is how much of a group is set apart as the emergency fund: none,
// a fixed amount, or all of it. The zero value is none.
type Reserve struct {
	all    bool
	amount money.Money
}

// The reserve modes, as the API and the database write them.
const (
	ReserveNone   = "none"
	ReserveAmount = "amount"
	ReserveAll    = "all"
)

// NoReserve sets nothing apart.
func NoReserve() Reserve { return Reserve{} }

// WholeReserve sets the whole group apart.
func WholeReserve() Reserve { return Reserve{all: true} }

// AmountReserve sets a fixed amount of the group apart.
func AmountReserve(m money.Money) (Reserve, error) {
	if m.Currency() != money.BRL || !m.IsPositive() {
		return Reserve{}, fmt.Errorf("%w: an emergency fund of %s", ErrInvalid, m)
	}
	return Reserve{amount: m}, nil
}

// ParseReserve reads a mode and, for "amount", its cents.
func ParseReserve(mode string, cents int64) (Reserve, error) {
	switch mode {
	case ReserveNone, "":
		return NoReserve(), nil
	case ReserveAll:
		return WholeReserve(), nil
	case ReserveAmount:
		return AmountReserve(money.BRLCents(cents))
	}
	return Reserve{}, fmt.Errorf("%w: emergency fund %q", ErrInvalid, mode)
}

func (r Reserve) Mode() string {
	switch {
	case r.all:
		return ReserveAll
	case r.amount.IsPositive():
		return ReserveAmount
	}
	return ReserveNone
}

// Amount is the amount set apart, zero unless the mode is "amount".
func (r Reserve) Amount() money.Money {
	if r.amount.Currency() == "" {
		return money.BRLCents(0)
	}
	return r.amount
}

// Of is how much of a group worth value is the emergency fund: the amount
// set, or all of it when the group holds less.
func (r Reserve) Of(value money.Money) money.Money {
	switch {
	case r.all:
		return value
	case r.amount.IsPositive():
		return money.BRLCents(min(r.amount.Cents(), max(value.Cents(), 0)))
	}
	return money.BRLCents(0)
}

// Short is how much a group worth value lacks to cover the amount set.
func (r Reserve) Short(value money.Money) money.Money {
	if r.all || !r.amount.IsPositive() {
		return money.BRLCents(0)
	}
	return money.BRLCents(max(r.amount.Cents()-value.Cents(), 0))
}

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
// and its emergency fund.
type GroupSetting struct {
	Override Class
	Reserve  Reserve
}

// Group is every lot of one product, open or closed, and how I classify
// them. A new lot joins its group with the group's class and emergency
// fund.
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

// SetReserve changes how much of the group is the emergency fund.
func (g *Group) SetReserve(r Reserve) { g.Reserve = r }

// Open is whether any lot is still held.
func (g *Group) Open() bool {
	for _, p := range g.Lots {
		if p.Open() {
			return true
		}
	}
	return false
}

// split divides a group's value and net in between the emergency fund and
// its class. The net in goes in the same proportion as the value, so the
// set-apart amount earns what the group earns.
func (r Reserve) split(whole Amounts) (emergency, rest Amounts) {
	e := r.Of(whole.Value).Cents()
	v, b := whole.Value.Cents(), whole.Basis.Cents()
	var eb int64
	switch {
	case r.all:
		eb = b
	case v > 0:
		eb = b * e / v
	}
	emergency = Amounts{Value: money.BRLCents(e), Basis: money.BRLCents(eb)}
	rest = Amounts{Value: money.BRLCents(v - e), Basis: money.BRLCents(b - eb)}
	return emergency, rest
}
