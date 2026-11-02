# ADR-0014: Pots, the money set apart, held in one pool

**Status:** Accepted (amends ADR-0011, its pay yourself first included, and the emergency fund in `portfolio`)
**Date:** 2026-11-02
**Deciders:** Felipe

## Context

Two things set money apart from the allocation today, and both draw on the same Nubank Caixinhas:

- **The emergency fund** is an amount set on a portfolio group (`Reserve`: none, an amount or all of it). It is a target, not a balance: with R$ 15.000,00 set and the Caixinhas holding R$ 11.000,00, all of the group is the emergency fund and it is R$ 4.000,00 short, which only the group's row in Positions says.
- **The budget's funds** (ADR-0011) carry a balance on Budget, computed from their monthly amounts, adjustments and spending, and are kept invested. The portfolio does not see them yet: the box in `docs/PLAN.md` would set them apart after the emergency fund (canvas: "Positions: the funds set apart").

Drawing that box showed what is wrong with the pair:

- **Two mechanisms share one pool** and need an order between them (the emergency fund first, the funds after). With today's numbers the funds would get nothing and show only as short.
- **A fund never earns.** What the Caixinhas yield on a fund's money stays in the group and counts as CDI in the allocation. A fund saved over a year is worth more than Budget says, and the difference reads as long-term investing.
- **Nothing reconciles.** No screen says "the Caixinhas hold X, what is set apart claims Y".
- **The money is too small to move the allocation** (the funds hold under 2% of what is invested), so setting the funds apart alone is not worth building. Getting the model right is.

Open Finance still cannot tell one Caixinha from another (ADR-0011): every contribution is a CDB of Nu Financeira at 100% of CDI, without the Caixinha's name.

## Decision

- **A pot is money set apart for something.** Every fund is a pot. The emergency fund becomes a pot too: a fund with no categories to spend from and a target. Any pot may have a target; a pot that reaches it takes no monthly amount until it falls below it again, so the month's Not assigned gets that money back. Spending comes out of a pot as a fund's does today (ADR-0011), and an adjustment still moves money already invested into or out of one.
- **The pots live in one pool.** One or more portfolio groups are marked as holding the pots (the Caixinhas); `portfolio` reads the pots' balances through an interface it declares, which `app` fills with `budget`'s Service. The pool's value is what those groups are worth. One line reconciles them: what the pool holds, what the pots claim (their positive values), and what is left over or short. What is left over counts in its group's class; what the pots claim is out of the allocation and the contribution planner. When the pool is short, the shortfall belongs to the pots together; no pot comes first.
- **Pots earn what the pool earns, through quotas.** The pool has a quota price, starting at 1 on the first valued day and following the pool's daily return (the modified Dietz return `portfolio` already computes, so money moved in or out is not earnings). Money going into a pot (its monthly amount, an adjustment in) buys quotas at the price on the day it counts; spending and adjustments out redeem them. A pot's value is its quotas at today's price; its earnings are its value minus its balance in reais. The earnings are gross, as the bank reports the CDB; the tax comes out when the money is redeemed and shows as the pool's value falling. A pot in the red holds no quotas: what it owes is in reais, earns nothing, and is paid back before the pot buys quotas again.
- **Budget keeps the flows, Portfolio values them.** `budget` remains the owner of each pot's monthly amounts, adjustments, spending and balance in reais, as today. `portfolio` already reads `budget`'s Service (ADR-0011), so it computes the quota price, each pot's quotas and value, and the reconciliation, and serves them at `GET /api/v1/pots`. The Budget screen reads both; `budget` never imports `portfolio`.
- **Pay yourself first is what went to the long term**, read from the investments' own movements instead of inferred from checking. Every group outside the pool is long-term money, in the allocation; pay yourself first is the month's net contributions to those groups: what was bought or transferred in, minus what was sold, redeemed or transferred out, never below zero. The adjustments ADR-0011 needed fall away: redeeming an RDB to buy stocks is a sale and a purchase on the same side, filling or spending a pot happens in the pool, and money sent to the broker counts once it is invested. Filling the emergency fund is no longer pay yourself first: it is a pot. It is still saving, so the savings rate counts pay yourself first plus what the pots kept (their money in minus their spending).
- **`budget` asks for it through an interface.** `budget` declares what it needs (`LongTerm`: the net contributions to the long term in a month, with the day of the last one) and takes it in its constructor; `app` implements it with `portfolio`'s Service. No import cycle: `budget` never imports `portfolio`, and the plan, the alert, the "done on" day and the monthly report stay in `budget`.
- **Pay yourself first becomes "Invest first".** The old name named a habit (put money aside before spending), which fit when any money put aside counted. It now counts only what went to the allocation, and "I paid myself first into the emergency fund" would still read as true. "Invest first" says what counts and keeps the priority and the payday. The rename goes everywhere the name is: the screens, alerts and monthly report, the API (`investFirst`, `PUT /api/v1/budget/{month}/invest-first`), MCP, `lastro contribute`, the code and the design system's names. The client and the server ship together, so the old names are not kept.
- **Net worth has one line for the pots**, their value, instead of "Emergency fund". The stored daily valuation keeps the pots' total, and its emergency fund column becomes that total, so the history does not jump.
- **The emergency fund's `Reserve` goes, and the emergency pot is made by hand.** A migration only touches its own module's schema, and moving a reserve would read `portfolio` and write `budget`. With one reserve to move, the owner makes the "Emergency fund" pot on Budget after the deploy: its target the reserve's amount, a monthly amount of zero, and a starting adjustment of what the group holds towards it. Until then that money counts in its class. The groups that had a reserve are marked as the pool by the same migration that drops the reserve, inside `portfolio`. With a monthly amount of zero, the pot changes nothing in pay yourself first before step 2.
- **The pool can be more than one group.** Marking a group as holding the pots never unmarks another; the claims are split among the pool's groups in proportion to their value.

## Rollout

1. Pots and the pool: the emergency fund as a pot, targets, the pool and its reconciliation, the allocation and the planner without the pots, the net worth line. Pots carry their balance in reais.
2. Invest first: pay yourself first renamed and read from the movements, through `LongTerm`, and the savings rate with the pots.
3. Quotas: the pool's quota price, each pot's value and earnings, on Budget, Positions and the monthly report.

Each step is its own box in `docs/PLAN.md`, and the Budget, Positions and Dashboard screens change on the canvas first.

## Options considered

- **Set the funds apart after the emergency fund, as `docs/PLAN.md` has it:** small, and drawn. Rejected: two mechanisms with an order between them, funds that never earn, and, with today's numbers, nothing set apart at all.
- **Estimate each fund's earnings from the CDI series:** no pool to mark, but `budget` would depend on `market`, every figure would be an estimate (≈), and it would drift from what the bank reports. Quotas use the pool's real value instead.
- **A fund per Caixinha, matched by its transactions:** rejected again for the reason in ADR-0011: Open Finance does not say which contribution went where.
- **A module of its own for the pots:** the flows already live in `budget` and the values in `portfolio`; a third module would only pass them between the two. Revisit if pots grow rules of their own.
- **Leave the emergency fund as a reserve and only add quotas to the funds:** keeps the order problem and the hidden shortfall. Rejected.
- **Keep pay yourself first on checking's transfers, net of what the pots kept (ADR-0011):** works without investment movements, but stays an inference with a correction for each case, and cannot tell money invested from money waiting at the broker. Rejected once the movements prove complete.
- **Pots with nothing to spend from (the emergency fund, a goal) count as pay yourself first:** keeps today's reading of the emergency fund, but pay yourself first would then mix money outside the allocation into what the allocation receives. Rejected: the savings rate already counts it.
- **`portfolio` serves pay yourself first and `budget` keeps only the plan:** no interface, but the alert and the monthly report would have to be put together outside `budget`. Rejected for `LongTerm`.

## Open questions

None of these blocks step 1; each is settled before its step starts.

- **Are the movements complete?** Pluggy sends investment movements a day or more late, and some products may come without them. Before this is accepted, the last three months' movements are checked against checking's transfers to investments. A product without movements falls back to its value change between months, minus its income.
- **Past months.** Read from the movements, an old month's pay yourself first may change, and with it whether it was done. Either accept it, or keep the transfer rule for the months before a cutoff.
- **When a monthly amount counts:** on the month's first day, or on the day of the month's last contribution, as pay yourself first does. The first is simpler; the second follows the money more closely.

## Consequences

- `portfolio` loses `Reserve` and the per-group emergency amount; groups gain whether they hold the pots. `budget` lines gain a target, and `budget` gains the `LongTerm` dependency; `SetAside` stops reading checking's transfers. A migration renames the stored pay yourself first (amount and payday) to invest first. The pool's quota price is computed from the stored valuations, never stored on its own.
- One concept for the owner: "Emergency fund" and "Budget funds" become the pots on Budget, each with its value, earnings and target, and the pool's line says at once when the Caixinhas hold less than the pots claim.
- The box "`portfolio`: one group holds the funds, set apart after the emergency fund" in `docs/PLAN.md` is replaced by the two steps above, and its artboard stays on the canvas as a design that was not built.
