# planning

**Responsibility:** net worth projections with monthly contributions and real interest rates, in scenarios (pessimistic, base, optimistic), plus Monte Carlo simulations if they prove useful.

**Not its responsibility:** changing positions or budgets. It is a read-only module over `portfolio` and `market`.

## Concepts

- `Scenario`: monthly contribution, horizon, real interest rate and inflation.
- Projections are estimates, so `float64` is acceptable here. Results are converted to `money.Money` only for display.

## Invariants

- Every projection shows the assumptions it used next to the result.
- Projected values are shown in today's money (inflation-adjusted), unless the screen says otherwise.

## Phase

5
