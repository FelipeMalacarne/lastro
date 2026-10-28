# budget

**Responsibility:** the monthly budget per category, pay yourself first, and tracking spending against them; the recurring charges that make up the fixed cost of living.

**Not its responsibility:** choosing categories (`categorize`) or deciding where the contribution goes (`portfolio`).

Decisions: [ADR-0006](../../docs/adr/0006-category-limits-before-envelopes.md), category limits first; [ADR-0012](../../docs/adr/0012-a-line-covers-several-categories.md), a line can cover several categories.

## Concepts

- `Month` (value object): a calendar month, the budget's unit. `Payday`: the day of the month pay yourself first is due, clamped to the month's last day.
- `MonthlyBudget` (aggregate): the budget saved for one month, with pay yourself first (an amount and a payday) and its `Line`s.
- `Line`: one limit for one or more categories, and whether what is left rolls over. A line with one category goes by its category's name; a line shared by several (`Shared`) has a name of its own ("Free to spend": Leisure, Shopping and Dev). Its `LineID` lasts from month to month, so rollover follows the line, not its categories: a category that joins keeps what the line carried, one that leaves takes nothing along. A line saved before lines could be shared has its category's id.
- Lines change by id: `AddLine` and `ChangeLine` take their categories from the lines they were in (a line left without any goes); `SetLimit` and `RemoveLimit` act on a category's own line, and refuse a category that shares one (`ErrShared`). `SetLimits` replaces every line, and a line given without an id keeps the id of the one that covered the same categories; `CopyLines` (copying another month) gives a line the id it had in the month before, where rollover comes from.
- **Inheritance:** a month without a saved budget follows the latest saved before it (`Effective`). Changing a month saves it whole, so earlier months keep what they had.
- `Track` (domain service): a month's lines against their categories' spending together. Rollover walks every month from the first saved budget, each month as it was saved: a line that rolls over passes on what it has left, never a debt. Pace compares the share of the limit used with the share of the month gone, leaving out what recurring charges already took (rent on the 5th is not spending ahead of the month). Over: spent more than available. Ahead: more than 10 points past the month's share.
- `FindRecurring` (domain service): charges grouped by merchant (the pattern a correction would teach) that come back once a month, on about the same day (within six days, counted around the month's turn, so the 30th and the 1st are close), for three months or more (two for a new one with a steady amount; four when the amount varies). Fixed when every charge is within 10% of the usual amount, or after a single change of price; varies up to 60%. The next charge is a month after the last, on the same day or the month's last. A charge not seen for 45 days stopped.
- Spending is `ledger`'s, in reais only: expenses minus refunds by category, installments in the month of each installment. Categories that do not count toward the budget (Income) are left out; transactions without a category count only when they are expenses. Income is the month's income, less what came in under a budgeted category: that is a refund (an insurer paying back, a friend paying their share), already taken off the category's spending, and counting it as income too would count it twice.
- Pay yourself first is what the month's transfers out to accounts outside lastro put aside, net of what came back from them (`SetAside`): redeeming an RDB to buy stocks puts nothing aside, and a month that redeemed more than it put in counts zero. It shows as done when that reaches the plan, on the day of the last transfer out. Not assigned is income minus pay yourself first minus the limits, and may be negative.

- A line's six months before are its current categories' spending together, also in months before they shared it, so the comparison is like with like; each category also carries its own average.
- `FindAlerts` (domain service, F10): a line over its limit; ahead of pace while the month runs; spending past the whole month's six-month average by a third and R$ 100 (on a line or category that usually sees R$ 50 or more); a recurring charge that changed price this month or is new. Each has a stable key, a line's by its id (so a line saved before lines could be shared keeps its category's key), and a push channel sends it once (ADR-0007).

## Invariants

- At most one saved budget per month; limits and pay yourself first are never negative and always in reais.
- A category is in at most one line a month, also when every line is set at once; the database refuses it twice (`budget.line_categories` is keyed by month and category). A line has at least one category, each once, and a line shared by several has a name, at most 40 characters, that no other line of the month has.
- Only a live category that counts toward the budget takes a limit, alone or in a line.
- Pay yourself first is never counted as spending.
- An installment purchase counts by the month's installment, not by its total.

## API

- `GET /api/v1/budget?month=2026-10`: the month (this one by default): whether it has a budget and which saved month it follows, the share of the month gone, pay yourself first (planned, moved, done on), income, limits, rollover, spent, left, not assigned, what recurring charges should still take, each line under `categories` (its `id`, its `name` when shared, `category`: its only one or a shared line's first, `byCategory`: each category's spending, count and six-month average; limit, carried, available, spent, left, pace, fixed and upcoming charges, the six months before and their average), the spending without a limit and a summary (income, spending, contribution, savings rate).
- `PUT /api/v1/budget/{month}/limits/{category}` `{"limitCents", "rollover"}`: the category's own line, from that month on. 400 for a category that does not count toward the budget, 409 for one that shares a line.
- `PUT /api/v1/budget/{month}/limits` `{"limits": [{"category" or "categories", "name", "id", "limitCents", "rollover"}]}`: every line at once, from that month on (Edit all limits); a category left out has none, a line without an id keeps the one that covered the same categories.
- `POST /api/v1/budget/{month}/lines` `{"name", "categories", "limitCents", "rollover"}`: a new line from that month on, answering `{"id"}` (201); `PUT /api/v1/budget/{month}/lines/{id}` the same body changes it, keeping what it carried; `DELETE /api/v1/budget/{month}/lines/{id}`. 404 for a line the month's budget lacks.
- `POST /api/v1/budget/{month}/copy` `{"from"}`: the lines another month had, from this month on, leaving out categories archived since and a line left without any; answers `{"copied"}`. The month's view says `prevDiffers` when the month before had other lines.
- `PUT /api/v1/budget/preferences` `{"rolloverNew"}`: whether a category given its first limit rolls over by default; the month's view carries it as `rolloverNew`.
- `DELETE /api/v1/budget/{month}/limits/{category}`: the category's own line, from that month on; 409 for one that shares a line.
- `PUT /api/v1/budget/{month}/pay-yourself-first` `{"amountCents", "payday"}`: from that month on.
- `Alerts` is this month's alerts, each with a severity, a title, a detail and where to look. `app` serves them at `GET /api/v1/alerts`, together with the portfolio's price alerts.
- `GET /api/v1/recurring`: the recurring charges, most expensive first, with what they cost a month, what was already charged this month and what is still to come, the new ones and the price changes.

## Events

None yet. Every read is computed from `ledger`, so a recategorized transaction updates every month by itself. After each sync, `app` pushes the alerts no channel has seen (`Unsent`, `MarkSent`) when Telegram is configured.

## Milestone

M3
