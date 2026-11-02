package app

import (
	"context"
	"time"

	"lastro/modules/budget"
	"lastro/modules/portfolio"
)

// budgetPots hands Budget's pots to the portfolio, which sets what they
// claim apart from the pool (ADR-0014). The portfolio declares what it
// needs; only app knows both modules.
type budgetPots struct{ b *budget.Service }

func (p budgetPots) Pots(ctx context.Context, day time.Time) ([]portfolio.Pot, error) {
	pots, err := p.b.Pots(ctx, budget.MonthOf(day))
	if err != nil {
		return nil, err
	}
	out := make([]portfolio.Pot, len(pots))
	for i, x := range pots {
		out[i] = portfolio.Pot{Name: x.Name, Balance: x.Balance, Target: x.Target}
	}
	return out, nil
}
