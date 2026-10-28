# ADR-0011: Funds, categories that save up month by month

**Status:** Accepted (amends ADR-0006)
**Date:** 2026-10-28
**Deciders:** Felipe

## Context

ADR-0006 chose a monthly limit per category, with rollover, and left envelopes for when limits fell short. They fall short for spending that comes in lumps: a monthly amount for going out with my girlfriend, which some months I do not spend and some months I spend twice over; the car, which costs nothing for months and then a repair at once. For those I put the month's amount in a Nubank Caixinha and take it out when the spending comes. A limit with rollover gets three things wrong about that:

- **Rollover forgets a debt.** It passes on what is left, never what is missing. A car fund with R$ 1.200 available that takes a R$ 2.000 repair starts the next month at zero instead of minus R$ 800, and an insurer's refund that lands the month after is added on top of a debt already forgotten. The month also shows the category over its limit and raises an alert, when the money had been put away for exactly that.
- **Pay yourself first reads the caixinha as investing.** It counts what went to investments net of what came back. Putting the month's amount in the caixinha counts as set aside; taking it out to pay the car counts against the month, and can leave pay yourself first undone in a month that put in exactly the plan.
- **The portfolio cannot tell one caixinha from another.** Open Finance sends every contribution as a CDB of Nu Financeira at 100% of CDI, without the caixinha's name, so the car's money and the emergency fund are one group (`portfolio` README). The emergency fund is an amount set apart from that group, so spending the car's money shows as an emergency fund short.

I do not want envelopes for everything ("give every real a job"): most categories are fine with a limit. I want a few categories that save up.

## Decision

- **A budget line is a limit or a fund.** A fund's monthly amount is what goes into it each month. It is inherited like a limit (ADR-0006), counts with the limits in Not assigned and, like any line, may cover several categories (ADR-0012).
- **A fund carries its balance, debt included.** Its balance belongs to the line: what went in (its monthly amounts and adjustments) minus its net spending, walked from the first saved budget as rollover is. A refund in its categories goes back into it. Spending past the balance leaves it in the red, and the months after pay it back. Rollover keeps its meaning for limits: what is left, never a debt.
- **A fund is not paced.** Its spending comes in lumps, so a fund is never ahead of pace and never unusual. Its one alert is going into the red (`overdrawn`).
- **An adjustment moves money already invested into or out of a fund:** what was already saved for it when it starts, covering a fund in the red from my long-term investments, or moving money from one fund to another. At most one per fund and month. It is old money, so it touches neither the month's Not assigned nor pay yourself first.
- **Funds are kept invested** (in a caixinha), so pay yourself first is what went to investments for the long term: what went out, minus what came back, minus what the funds kept this month (their monthly amounts minus their spending), never below zero. A month with R$ 4.000 of pay yourself first, a car fund of R$ 300 and a R$ 1.600 repair paid from the caixinha sends R$ 4.300 out and brings R$ 1.600 back; the funds kept 300 − 1.600 = −1.300, so pay yourself first moved 4.300 − 1.600 + 1.300 = 4.000, done. Which transfer went to which caixinha is never asked, since Open Finance cannot tell: the rule is on totals.
- **The month's totals (available, spent, left) are the limits'.** The funds have totals of their own, so a car fund filled over a year does not read as this month's money to spend.
- **The portfolio sets the funds apart.** One group is marked as holding the funds. Their balance, where positive, is set apart from it after the emergency fund: out of the allocation and the contribution planner, and a line of its own in net worth. What the group lacks for both shows as short. `portfolio` reads the balance through `budget`'s Service.

## Options considered

- **Full envelopes (YNAB):** solves this and much more, with the bookkeeping ADR-0006 turned down. Still postponed.
- **A limit with rollover, as today:** saves up while nothing goes wrong, and gets the three things above wrong. Kept for limits, rejected for lumpy spending.
- **A fund per caixinha, matched by its transactions:** exact, if lastro could tell which "Aplicação RDB" went to which caixinha. Open Finance does not say (no name, the same product), so every contribution would have to be tagged by hand. Rejected: the totals give the same answer without the chore.
- **Funds that never go into the red,** forgetting spending past the balance as rollover does: simpler, but it is the first problem again. Rejected.
- **Funds kept in checking, outside pay yourself first:** possible later, as a switch per fund. Not now: the funds I have in mind live in a caixinha.

## Consequences

- `budget.lines` gets whether a line is a fund, and adjustments, per line and month, get a table of their own. The balance is computed, never stored, so a recategorized transaction moves every later balance, as with rollover.
- Pay yourself first reads lower in a month that fills the funds without spending them, and higher in one that spends them. Filling a fund without moving the money to the caixinha shows as pay yourself first not reached: the fund promises money the investments do not hold.
- The savings rate stays on real spending: a month that uses a fund saves less, the months that filled it saved more.
- `portfolio` depends on `budget` (no cycle: `budget` reads `ledger` and `categorize`). The stored daily valuation gains the funds next to the emergency fund; days before the first fund have none.
- Goals with a target and a date (the parking lot in `FEATURES.md`) can later be a fund with a target.
- The Budget, Positions and Dashboard screens change, on the canvas first.
