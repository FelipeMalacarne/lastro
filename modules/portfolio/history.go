package portfolio

import (
	"fmt"
	"time"

	"lastro/money"
)

// MovementKind is what a movement did to an investment.
type MovementKind string

const (
	MoveBuy          MovementKind = "buy"          // money in: an application or a purchase
	MoveSell         MovementKind = "sell"         // money out: a withdrawal or a sale
	MoveTransferIn   MovementKind = "transfer_in"  // moved in from another broker, often without a quantity
	MoveTransferOut  MovementKind = "transfer_out" // moved out to another broker
	MoveTax          MovementKind = "tax"
	MoveIncome       MovementKind = "income" // dividends, coupons
	MoveAmortization MovementKind = "amortization"
)

func (k MovementKind) valid() bool {
	switch k {
	case MoveBuy, MoveSell, MoveTransferIn, MoveTransferOut, MoveTax, MoveIncome, MoveAmortization:
		return true
	}
	return false
}

// Movement is one thing that happened to an investment, as the bank
// reported it. The amount is gross and never negative: the kind says the
// direction.
type Movement struct {
	ExternalID string
	Date       time.Time
	Kind       MovementKind
	Quantity   *Quantity    // nil when the bank sends none
	Amount     money.Money  // gross
	Net        *money.Money // after taxes and fees, when the bank sends it
}

// History is an investment and its movements, kept also for one fully
// withdrawn before lastro knew it, which never becomes a Position. The days
// before the first sync are rebuilt from these. The Report says what the
// investment is and what the bank reported last.
type History struct {
	Report
	Issued    *time.Time // fixed income: the day it was bought
	Movements []Movement
}

func (h History) validate() error {
	if h.ExternalID == "" || h.Item == "" {
		return fmt.Errorf("%w: a history needs the investment and its item", ErrInvalid)
	}
	seen := map[string]bool{}
	for _, m := range h.Movements {
		switch {
		case m.ExternalID == "" || seen[m.ExternalID]:
			return fmt.Errorf("%w: movement of %s without an id of its own", ErrInvalid, h.ExternalID)
		case !m.Kind.valid():
			return fmt.Errorf("%w: movement %s of kind %q", ErrInvalid, m.ExternalID, m.Kind)
		case m.Date.IsZero():
			return fmt.Errorf("%w: movement %s without a date", ErrInvalid, m.ExternalID)
		case m.Amount.IsNegative() || (m.Net != nil && m.Net.IsNegative()):
			return fmt.Errorf("%w: movement %s with a negative amount", ErrInvalid, m.ExternalID)
		}
		seen[m.ExternalID] = true
	}
	return nil
}
