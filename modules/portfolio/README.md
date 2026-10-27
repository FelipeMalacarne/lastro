# portfolio

**Responsibility:** positions, the target allocation and where each contribution should go. It also keeps the thesis for each stock and the triggers that would break it.

**Not its responsibility:** fetching prices (that is `market`) or importing positions (that is `ingest`).

## Concepts

- `Portfolio` (aggregate): positions per asset class (CDI, global stocks, IPCA+, Brazilian stocks) plus the emergency fund.
- Value objects: `Ticker` (validates the B3 format and normalizes the odd-lot ticker: ITUB4F becomes ITUB4), `Quantity` (in millionths, since banks report fractions), `Class` (CDI, IPCA+, Brazilian stocks, global stocks), `Allocation` (target weights as `money.Percent`) and `Band` (how far a class may drift from its target).
- `PlanContribution` (the contribution planner, a domain service): takes each class's value, the targets, the band and the amount. Classes below their band come first, up to the band's lower edge; what is left goes to every class under its target in proportion to its gap; once every gap is closed, the rest follows the targets. A class above its target gets nothing.
- `Thesis`: the one-line thesis for each stock and what would break it.

## Invariants

- The weights of an `Allocation` add up to exactly 100%.
- `ContributionPlanner` never suggests selling: it only directs new money to the classes below target.
- What the planner distributes adds up to exactly the contribution amount (it uses `Money.Allocate`, so no cents are lost).
- The emergency fund is not part of the allocation.

## Milestone

M4
