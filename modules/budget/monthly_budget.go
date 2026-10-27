package budget

import (
	"fmt"
	"sort"
	"time"

	"lastro/modules/ledger"
	"lastro/money"
)

// Line is one category's monthly limit and whether what is left of it rolls
// over to the next month.
type Line struct {
	Category ledger.CategoryID
	Limit    money.Money
	Rollover bool
}

// MonthlyBudget is the budget saved for one month: pay yourself first and a
// limit per category. A month without one inherits the latest saved before
// it (ADR-0006), so changing a month saves it whole from then on.
type MonthlyBudget struct {
	month     Month
	payday    Payday
	pyf       money.Money // pay yourself first
	lines     map[ledger.CategoryID]Line
	updatedAt time.Time
}

// defaultPayday is the 5th, when most Brazilian salaries land.
const defaultPayday = 5

// NewMonthlyBudget starts an empty budget: nothing set aside and no limits.
func NewMonthlyBudget(m Month, now time.Time) (*MonthlyBudget, error) {
	if m.IsZero() {
		return nil, fmt.Errorf("%w: budget without a month", ErrInvalid)
	}
	p, _ := NewPayday(defaultPayday)
	return &MonthlyBudget{month: m, payday: p, pyf: money.BRLCents(0), lines: map[ledger.CategoryID]Line{}, updatedAt: now}, nil
}

// RestoreMonthlyBudget rebuilds a saved budget, for the repository.
func RestoreMonthlyBudget(m Month, payday Payday, pyf money.Money, lines []Line, updatedAt time.Time) *MonthlyBudget {
	b := &MonthlyBudget{month: m, payday: payday, pyf: pyf, lines: make(map[ledger.CategoryID]Line, len(lines)), updatedAt: updatedAt}
	for _, l := range lines {
		b.lines[l.Category] = l
	}
	return b
}

// For is the budget a month starts from: a copy of this one, which is the
// latest saved at or before it. The copy is saved only once it changes.
func (b *MonthlyBudget) For(m Month) *MonthlyBudget {
	c := &MonthlyBudget{month: m, payday: b.payday, pyf: b.pyf, lines: make(map[ledger.CategoryID]Line, len(b.lines)), updatedAt: b.updatedAt}
	for k, v := range b.lines {
		c.lines[k] = v
	}
	return c
}

func (b *MonthlyBudget) Month() Month                  { return b.month }
func (b *MonthlyBudget) Payday() Payday                { return b.payday }
func (b *MonthlyBudget) PayYourselfFirst() money.Money { return b.pyf }
func (b *MonthlyBudget) UpdatedAt() time.Time          { return b.updatedAt }

// Lines are the category limits, in a stable order.
func (b *MonthlyBudget) Lines() []Line {
	out := make([]Line, 0, len(b.lines))
	for _, l := range b.lines {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Category.String() < out[j].Category.String() })
	return out
}

// Line is a category's limit, if it has one.
func (b *MonthlyBudget) Line(c ledger.CategoryID) (Line, bool) {
	l, ok := b.lines[c]
	return l, ok
}

// SetPayYourselfFirst sets aside an amount on payday, before any limit.
func (b *MonthlyBudget) SetPayYourselfFirst(amt money.Money, payday Payday, now time.Time) error {
	if err := amount(amt, "pay yourself first"); err != nil {
		return err
	}
	b.pyf, b.payday, b.updatedAt = amt, payday, now
	return nil
}

// SetLimit gives a category a monthly limit, or changes it. Zero is a limit
// too: the category should see no spending.
func (b *MonthlyBudget) SetLimit(c ledger.CategoryID, limit money.Money, rollover bool, now time.Time) error {
	if c.IsZero() {
		return fmt.Errorf("%w: limit without a category", ErrInvalid)
	}
	if err := amount(limit, "limit"); err != nil {
		return err
	}
	b.lines[c] = Line{Category: c, Limit: limit, Rollover: rollover}
	b.updatedAt = now
	return nil
}

// RemoveLimit leaves a category without a limit. It reports whether it had one.
func (b *MonthlyBudget) RemoveLimit(c ledger.CategoryID, now time.Time) bool {
	if _, ok := b.lines[c]; !ok {
		return false
	}
	delete(b.lines, c)
	b.updatedAt = now
	return true
}

// Limits is the sum of every category's limit, before rollover.
func (b *MonthlyBudget) Limits() money.Money {
	var cents int64
	for _, l := range b.lines {
		cents += l.Limit.Cents()
	}
	return money.BRLCents(cents)
}
