package budget

import (
	"fmt"

	"lastro/money"
)

// Adjustment moves money already invested into a fund (positive) or out of
// it, in a month and outside its monthly amount (ADR-0011): what was saved
// for it before it started, covering it while in the red, or moving money
// from one fund to another. It is old money, so neither Not assigned nor
// pay yourself first sees it. A fund has at most one a month.
type Adjustment struct {
	Month  Month
	Line   LineID
	Amount money.Money
}

// Adjustments are the adjustments by month and line.
type Adjustments map[Month]map[LineID]money.Money

// NewAdjustment is an adjustment to a line of b, the budget that applies to
// the month: in reais, never zero, and only for a line that is a fund that
// month.
func NewAdjustment(b *MonthlyBudget, id LineID, amt money.Money) (Adjustment, error) {
	l, ok := b.Line(id)
	switch {
	case !ok:
		return Adjustment{}, ErrNoLine
	case !l.IsFund():
		return Adjustment{}, fmt.Errorf("%w: only a fund takes an adjustment", ErrInvalid)
	case amt.Currency() != money.BRL:
		return Adjustment{}, fmt.Errorf("%w: an adjustment in %s, want BRL", ErrInvalid, amt.Currency())
	case amt.IsZero():
		return Adjustment{}, fmt.Errorf("%w: an adjustment of nothing", ErrInvalid)
	}
	return Adjustment{Month: b.month, Line: id, Amount: amt}, nil
}

// of is a line's adjustment in a month, in cents.
func (a Adjustments) of(m Month, id LineID) int64 { return a[m][id].Cents() }
