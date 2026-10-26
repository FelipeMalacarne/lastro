# portfolio

**Responsibility:** positions, the target allocation and where each contribution should go. It also keeps the thesis for each stock and the triggers that would break it.

**Not its responsibility:** fetching prices (that is `market`) or importing positions (that is `ingest`).

## Concepts

- `Portfolio` (aggregate): positions per asset class (CDI, global stocks, IPCA+, Brazilian stocks) plus the emergency fund.
- Value objects: `Ticker` (validates the B3 format and normalizes the odd-lot ticker: ITUB4F becomes ITUB4), `Quantity`, `Allocation` (weights as `money.Percent`).
- `ContributionPlanner` (domain service): takes positions, the target allocation and the contribution amount, and returns how much goes to each class.
- `Thesis`: the one-line thesis for each stock and what would break it.

## Invariants

- The weights of an `Allocation` add up to exactly 100%.
- `ContributionPlanner` never suggests selling: it only directs new money to the classes below target.
- What the planner distributes adds up to exactly the contribution amount (it uses `Money.Allocate`, so no cents are lost).
- The emergency fund is not part of the allocation.

## Phase

4
