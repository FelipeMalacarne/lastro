# ADR-0008: What a month leaves is invested or kept, by choice

**Status:** Accepted
**Date:** 2026-10-27
**Deciders:** Felipe

## Context

ADR-0006 left what income is left after pay yourself first and the limits as **Not assigned**, computed from the limits alone. Two things went wrong with that:

- An overspend changed nothing. A category that went R$ 300 past its limit was red in its month and forgotten the next, and Not assigned still showed the same money as if it had not been spent. Spending without a limit, and expenses still without a category, did not count either.
- Whatever was left had nowhere to go. Pay yourself first invests a fixed amount; the rest of a good month stayed in checking with no decision behind it.

Investing the rest automatically was considered, but some months the rest is better spent next month (a trip, a bigger purchase), so the choice stays mine.

## Decision

- **The rest of a month** is income, plus what the month before kept, minus pay yourself first, spending without a limit (uncategorized included) and the cost of each limited category: its limit, or what it spent when it went over. While the month runs this is **Not assigned**, so an overspend shows up in it at once.
- **Once the month is over**, a category that does not roll over costs only what it spent, so what it did not spend joins the rest. One that rolls over keeps its leftover, as before.
- **The rest of a closed month waits for a choice:** invest it (it joins the next contribution, on top of pay yourself first, and "done" waits for both), keep it (it joins the next month's Not assigned), or split it.
- **A default in Settings** can skip the question: ask each month (the default), always invest or always keep. A month I decided keeps its choice; the default applies to every closed month I did not decide.
- **No debt.** A rest below zero means the month spent past what came in after pay yourself first. It is shown and alerted, and nothing carries into the next month.
- An overspent category still never shrinks the next month's limit. The rest already paid for it.

## Options considered

- **Carry an overspend into the category's next limit** (envelope debt): exact, but it punishes the category twice when the rest already covered it, and it brings back the bookkeeping ADR-0006 avoided. Rejected.
- **Invest the rest automatically:** simple, but it takes away the choice to spend a good month's rest next month. Available as the "always invest" default instead.
- **A choice per closed month, with a default:** chosen.

## Consequences

- `budget` stores one row per decided month (`budget.rests`) and the default in `budget.preferences`. The rests themselves are computed, not stored: each depends on what the month before kept, from the first saved budget on, so a late or recategorized transaction updates every later month by itself.
- A split stores how much was invested. If the month later turns out to have left less, the investment stands and nothing is kept.
- Changing the default re-decides every closed month without its own choice, and with it what each one kept.
