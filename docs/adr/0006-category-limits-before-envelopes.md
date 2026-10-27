# ADR-0006: Category limits first, envelopes only if they fall short

**Status:** Accepted; how Not assigned is counted, and overspending, amended by [ADR-0008](0008-what-a-month-leaves.md)
**Date:** 2026-10-27
**Deciders:** Felipe

## Context

`FEATURES.md` left open whether the budget should be a limit per category or a YNAB-style envelope model, where every real of income is given a job before it is spent. Envelopes are stricter and answer "can I afford this?" from money already in hand, but they ask for work every time money arrives and every time a category runs out. My income is one salary a month and most spending is on one card, so the month is the natural unit.

## Decision

- A budget is a `MonthlyBudget`: pay yourself first (an amount and a payday) and a limit per category, each with its own rollover switch.
- **A month inherits the latest budget saved before it.** Changing a month saves a full copy of it from then on, so earlier months keep what they had and later months follow until they are changed themselves. This is "copy last month's budget" without a button: a new month starts as the one before it.
- **Rollover** carries what is left in a category into the next month, only when the category rolls over, and only a positive remainder. Overspending is shown in its month and does not shrink the next one; covering it is a choice made on the Budget screen.
- Spending is what `ledger` counts: expenses minus refunds, by category, with installments in the month of each installment. Transfers between my accounts and card bill payments never count.
- Pay yourself first is a fixed line set aside before any limit, never counted as spending. It shows as done when the month's transfers out to accounts outside lastro (the broker) reach it.
- What income is left after pay yourself first and the limits is shown as **Not assigned**, without forcing it into a category.

## Options considered

- **Envelopes ("give every real a job"):** precise, but the bookkeeping is a chore for one salary a month, and an empty envelope blocks nothing anyway, since lastro only reads the bank. Postponed: F4 P2, if limits prove insufficient.
- **Limits with one fixed value forever:** simple, but changing a limit would rewrite past months. Rejected.
- **Limits per month, inherited from the month before:** past months stay true, the next month needs no setup. Chosen.

## Consequences

- `budget` stores one row per saved month and one per category line in it; reading a month looks for the latest saved month at or before it.
- Rollover is computed, not stored: each month's carry depends on the month before, from the first saved budget onward, so a recategorized transaction updates every later month by itself.
- Not assigned can be negative when the limits promise more than came in; the screen says so instead of hiding it.
