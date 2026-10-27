# planning

**Responsibility:** projections of the invested money with monthly contributions and real returns, in scenarios (pessimistic, base, optimistic), "how long until" a target, and a Monte Carlo simulation.

**Not its responsibility:** changing positions or budgets. It is read-only over `portfolio`.

## Concepts

- `Scenario`: where it starts, what goes in every month (raised with inflation, so constant in today's money), for how many months, at what real return a year. `Project` compounds monthly at the rate that makes the year's; `MonthsUntil` is how long until a target, up to 100 years.
- The scenarios are 2%, 4% and 6% a year above inflation, before tax.
- `Simulate` (Monte Carlo): paths of monthly returns, each class drawn on its own from a log-normal with its long-run real mean and spread (CDI 4% ±1,5; IPCA+ 5% ±6; Brazilian stocks 6% ±25; global stocks 6% ±17), weighted by the target allocation and rebalanced every month. Seeded by the inputs, so the same question gets the same answer. It reports the 10th, 50th and 90th percentiles per year and the share of paths that reach the target.
- Projections are estimates, so `float64` is acceptable here; results go back to `money.Money` cents for display, rounded to a thousand reais on screen.

## Invariants

- Every projection shows the assumptions it used next to the result.
- Values are in today's money: contributions rise with inflation and returns are real.
- The emergency fund stays out: the start is the allocation's value.

## API

- `GET /api/v1/planning?monthly=300000&years=15&target=100000000` (cents): the start, each scenario's value at the end and each year, when the target is reached in each, what R$ 500 more or less a month does to the base date, and the simulation's bands, mix and share of paths that reach the target.

## Milestone

M5
