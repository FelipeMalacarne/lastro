# budget

**Responsibility:** the monthly budget per category, pay yourself first, and tracking spending against them; the recurring charges that make up the fixed cost of living.

**Not its responsibility:** choosing categories (`categorize`) or deciding where the contribution goes (`portfolio`).

Decisions: [ADR-0006](../../docs/adr/0006-category-limits-before-envelopes.md), category limits first; [ADR-0008](../../docs/adr/0008-what-a-month-leaves.md), what a month leaves is invested or kept, by choice.

## Concepts

- `Month` (value object): a calendar month, the budget's unit. `Payday`: the day of the month pay yourself first is due, clamped to the month's last day.
- `MonthlyBudget` (aggregate): the budget saved for one month, with pay yourself first (an amount and a payday) and a `Line` per category (a limit and whether it rolls over).
- **Inheritance:** a month without a saved budget follows the latest saved before it (`Effective`). Changing a month saves it whole, so earlier months keep what they had.
- `Track` (domain service): a month's limits against its spending. Rollover walks every month from the first saved budget: a category that rolls over passes on what it has left, never a debt. Pace compares the share of the limit used with the share of the month gone, leaving out what recurring charges already took (rent on the 5th is not spending ahead of the month). Over: spent more than available. Ahead: more than 10 points past the month's share.
- `FindRecurring` (domain service): charges grouped by merchant (the pattern a correction would teach) that come back once a month, on about the same day (within six days, counted around the month's turn, so the 30th and the 1st are close), for three months or more (two for a new one with a steady amount; four when the amount varies). Fixed when every charge is within 10% of the usual amount, or after a single change of price; varies up to 60%. The next charge is a month after the last, on the same day or the month's last. A charge not seen for 45 days stopped.
- Spending is `ledger`'s, in reais only: expenses minus refunds by category, installments in the month of each installment. Categories that do not count toward the budget (Income) are left out; transactions without a category count only when they are expenses.
- Pay yourself first shows as done when the month's transfers out to accounts outside lastro reach it, plus what the month before left and I chose to invest.
- `Rests` (domain service, `rest.go`): what each month leaves, walking from the first saved budget: income, plus what the month before kept, minus pay yourself first, spending without a limit (uncategorized included) and each limited category's cost (its limit, or what it spent when it went over). While the month runs that is Not assigned, and may be negative. Once the month is over, a category that does not roll over costs only what it spent, so its unspent money joins the rest.
- `Decision` (value object, `Decide`): what I chose for a closed month's rest: invest it (it joins the next contribution), keep it (it joins the next month's Not assigned) or split it. Months I did not decide follow the default in `Preferences` (ask, invest or keep). A rest below zero is spending past what came in after pay yourself first: nothing to decide, and no debt.

- `RestAlert`: the month before, once closed, left money that waits for a choice, or spent past what came in after pay yourself first. `Alerts` adds it first.
- `FindAlerts` (domain service, F10): a category over its limit; ahead of pace while the month runs; spending past the whole month's six-month average by a third and R$ 100 (on a category that usually sees R$ 50 or more); a recurring charge that changed price this month or is new. Each has a stable key, so a push channel sends it once (ADR-0007).

## Invariants

- At most one saved budget per month; limits and pay yourself first are never negative and always in reais. A month has at most one limit per category, also when they are all set at once.
- Only a live category that counts toward the budget takes a limit.
- Pay yourself first is never counted as spending.
- An installment purchase counts by the month's installment, not by its total.
- Only a closed month that left something can be decided; a split invests between nothing and all of it. When a month later leaves less than a split invested, the investment stands and nothing is kept.

## API

- `GET /api/v1/budget?month=2026-10`: the month (this one by default): whether it has a budget and which saved month it follows, the share of the month gone, pay yourself first (planned, moved, done on), income, limits, rollover, spent, left, not assigned, what recurring charges should still take, each limited category (limit, carried, available, spent, left, pace, fixed and upcoming charges, the six months before and their average), the spending without a limit and a summary (income, spending, contribution, savings rate).
- `PUT /api/v1/budget/{month}/limits/{category}` `{"limitCents", "rollover"}`: from that month on. 400 for a category that does not count toward the budget.
- `PUT /api/v1/budget/{month}/limits` `{"limits": [{"category", "limitCents", "rollover"}]}`: every limit at once, from that month on (Edit all limits); a category left out has none.
- `POST /api/v1/budget/{month}/copy` `{"from"}`: the limits another month had, from this month on, leaving out categories archived since; answers `{"copied"}`. The month's view says `prevDiffers` when the month before had other limits.
- `PUT /api/v1/budget/preferences` `{"rolloverNew", "rest"}`, either one: whether a category given its first limit rolls over by default, and what happens to a closed month's rest when I do not decide (`ask`, `invest` or `keep`). The month's view carries them as `rolloverNew` and `restDefault`.
- `PUT /api/v1/budget/{month}/rest` `{"choice": "invest" | "keep" | "split", "investCents"}` and `DELETE /api/v1/budget/{month}/rest`: decide a closed month's rest, or take the choice back. 400 for a month that runs or left nothing. The month's view has `rest` (its own: income, kept, pay yourself first, limits, over, without a limit, unspent, left, the choice and what is invested and kept) and `prev` (the month before's, once closed, with `pending` while it waits); `payYourselfFirst.fromRestCents` is what the month before invests now.
- `DELETE /api/v1/budget/{month}/limits/{category}`: from that month on.
- `PUT /api/v1/budget/{month}/pay-yourself-first` `{"amountCents", "payday"}`: from that month on.
- `Alerts` is this month's alerts, each with a severity, a title, a detail and where to look. `app` serves them at `GET /api/v1/alerts`, together with the portfolio's price alerts.
- `GET /api/v1/recurring`: the recurring charges, most expensive first, with what they cost a month, what was already charged this month and what is still to come, the new ones and the price changes.

## Events

None yet. Every read is computed from `ledger`, so a recategorized transaction updates every month by itself. After each sync, `app` pushes the alerts no channel has seen (`Unsent`, `MarkSent`) when Telegram is configured.

## Milestone

M3
