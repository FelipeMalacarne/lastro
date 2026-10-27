# Features

What lastro should do, from the point of view of the person using it (me). `docs/PLAN.md` says **in what order** things get built; this document says **what** gets built and **how to know it is done**.

## My input

What I asked for when starting the project:

> A personal finance app just for me. Sync my accounts into my own database using Meu Pluggy (free for personal use). Monitor transactions, categorize them, do monthly budgeting (maybe YNAB-style, maybe not), take care of my investments, run projections, calculate contributions, and analyze stock data and the like. It should be really smart, interactive and beautiful.

And the way I invest, which the app has to support:

- A Boglehead core: a global stock ETF, IPCA+ bonds and CDI, plus an emergency fund that is never invested.
- A small, explicit Brazilian stock-picking satellite, with a written thesis per stock, a rule for what breaks each thesis, purchases staged around earnings reports, and a yearly scoreboard against the index.
- A fixed monthly contribution split by target allocation, with rebalancing done through new contributions instead of selling.

## Problem

My financial picture is spread across several banks, a brokerage and a spreadsheet. Answering simple questions (how much did I spend on food this month? where should this month's contribution go? is my stock picking beating the index?) means opening many apps and doing math by hand. Generic finance apps don't understand Brazilian card bills, Pix transfers between my own accounts or my allocation strategy, and they send my data to third parties.

## Goals

1. **One place for everything:** every account, card and investment visible in lastro, updated automatically every day, with no manual data entry.
2. **Trustworthy numbers:** monthly totals match the bank statements. No double-counted card bills or transfers between my own accounts.
3. **Low effort:** categorization and the monthly review take minutes, not an evening.
4. **Better decisions:** "where does this month's contribution go?" and "how am I doing against my plan?" answered in under a minute, with no spreadsheet.
5. **Private by design:** data lives on my server and the LLM runs locally.

## Non-goals

- **Multiple users or a SaaS.** Built for one person. Sharing, roles and billing are out of scope.
- **Moving money or trading.** lastro reads accounts; it never makes payments, transfers or orders.
- **Real-time quotes.** End-of-day prices are enough for a long-term investor.
- **Tax filing (IR).** Not now. Exporting data for the yearly tax return is in the parking lot.
- **External LLMs with financial data.** All intelligence runs on the local Ollama.

## Feature areas

Priorities: **P0** = the area does not work without it. **P1** = clearly wanted, right after P0. **P2** = later, but the design must not block it.

### F1. Account sync

*M1 · modules: `ingest`, `ledger`*

- As the owner, I want my checking accounts, cards and investment accounts to sync automatically, so that I never type a transaction by hand.
- As the owner, I want to know when a connection breaks, so that the data does not go stale silently.

**P0**
- [x] Every item in `LASTRO_PLUGGY_ITEM_IDS` syncs on a schedule (CronJob) and on demand (`lastro sync`).
- [x] Re-syncing never creates duplicates; changed transactions are updated in place.
- [x] Pending transactions that disappear or change on the next sync are handled correctly.
- [x] Each account shows when it was last synced successfully.

**P1**
- [x] A visible warning when an item has not synced for more than 2 days or needs re-authentication in Meu Pluggy.
- [ ] Import of the B3 trade statement for positions that do not come through Open Finance.

**P2**
- [ ] Manual entries for cash or for accounts outside Open Finance (e.g. private pension plans).

### F2. Transactions

*M1 · modules: `ledger`, web*

- As the owner, I want to see, search and filter all my transactions in one list, so that I can find anything quickly.
- As the owner, I want transfers between my own accounts and card bill payments to be recognized automatically, so that they never inflate my spending.

**P0**
- [ ] One list with every account, filterable by period, account, category and type, and searchable by description. (Done except the category filter, which arrives with categories in M2.)
- [x] Transfers between my own accounts (Pix to a savings pot, to the brokerage...) are paired and marked as transfers.
- [x] Card bill payments are marked as such; the card purchases are what counts as spending.
- [x] Installment purchases show "n of m" and count in the month of each installment.

**P1**
- [ ] Split a transaction into several categories.
- [ ] Notes and tags on a transaction.

### F3. Categorization

*M2 · module: `categorize`*

- As the owner, I want transactions categorized automatically, so that I only fix the few mistakes.
- As the owner, I want every correction to teach the app, so that the same mistake does not happen twice.

**P0**
- [ ] Rules by merchant and description categorize known transactions instantly.
- [ ] Transactions no rule matches go to the local LLM, which must answer with an existing category.
- [ ] Anything the LLM cannot classify, or when the LLM is offline, goes to a "to review" queue; nothing gets a guessed category.
- [ ] A manual correction creates a rule and is never overwritten by automation.

**P1**
- [ ] The review queue suggests a category and accepts it with one click or key.
- [ ] Rules can be listed, edited and deleted.

### F4. Monthly budget

*M3 · module: `budget`*

- As the owner, I want to set aside my contribution first and then see how much I can spend per category, so that saving is never what is left over.
- As the owner, I want to see during the month whether a category is on track, so that I can adjust before it is too late.

**P0**
- [ ] "Pay yourself first": the monthly contribution is a fixed line set aside on payday and never counted as spending.
- [ ] A monthly limit per category, with spent, remaining and the percentage of the month elapsed.
- [ ] Money left in a category can roll over to the next month, configurable per category.

**P1**
- [ ] Copy last month's budget as a starting point.
- [ ] Monthly summary: income, spending, contribution and savings rate.

**P2**
- [ ] A full YNAB-style envelope model ("give every real a job"), if the simpler model proves insufficient. See open questions.

### F5. Recurring expenses and subscriptions

*M3 · modules: `ledger`, `budget`*

- As the owner, I want my subscriptions and fixed bills detected automatically, so that I know my fixed cost of living and notice when something new appears or a price goes up.

**P1**
- [ ] Recurring charges (same merchant, similar amount, regular interval) are detected and listed with their monthly total.
- [ ] A warning when a recurring charge changes amount or a new one appears.

### F6. Net worth and positions

*M4 · modules: `portfolio`, `market`*

- As the owner, I want to see my total net worth and how it is split across asset classes, so that I know where I stand at a glance.
- As the owner, I want to see how my net worth evolved over time, so that I can tell real progress from market noise.

**P0**
- [ ] Positions per asset class (CDI, global stocks, IPCA+, Brazilian stocks), valued at the latest available price.
- [ ] The emergency fund is shown separately and is never part of the allocation.
- [ ] A net worth snapshot is stored every day, building a history from the first sync onward.

**P1**
- [ ] Net worth chart over time, split by contributions versus returns.
- [ ] Return per class, compared with CDI and inflation over the same period.

### F7. Contribution planning

*M4 · module: `portfolio`*

- As the owner, I want to type my contribution amount and get exactly how much goes to each class, so that I can invest in a minute instead of opening a spreadsheet.
- As the owner, I want rebalancing to happen through new money, so that I avoid selling and paying tax.

**P0**
- [ ] A target allocation per class, which must add up to 100%.
- [ ] Given an amount, the planner returns the split per class; it never suggests selling, and the parts add up to exactly the amount.
- [ ] Classes outside their tolerance band get priority in the split.
- [ ] Same answer in the web UI and in `lastro contribute --amount <value>`.

**P1**
- [ ] A purchase schedule: planned buys with date, amount and condition (e.g. "only after the earnings report"), marked as done or cancelled.
- [ ] A yearly reminder to review the target allocation.

### F8. Stock analysis and theses

*M4 and M5 · modules: `portfolio`, `market`*

- As the owner, I want a one-line thesis for each stock I pick, plus what would break it, so that my decisions stay disciplined.
- As the owner, I want to know if my stock picking actually beats the index, so that I can drop it if it does not.

**P0**
- [ ] Each stock in the satellite has a thesis and a break condition, visible next to the position.
- [ ] Reminder before each company's earnings report, with the thesis shown alongside.

**P1**
- [ ] Price history chart per stock and ETF.
- [ ] Valuation metrics per stock (P/E, P/B, dividend yield), with their history.
- [ ] Dividends received, per stock and per year.
- [ ] Yearly scoreboard: the satellite's return against the Ibovespa and against CDI.

**P2**
- [ ] Price alerts (e.g. a stock falls more than X% in a day).

### F9. Projections

*M5 · module: `planning`*

- As the owner, I want to see where my net worth will be in N years with my current contributions, so that I can tell whether I am on track.

**P1**
- [ ] Projection with monthly contribution, horizon and real interest rate, in pessimistic, base and optimistic scenarios.
- [ ] Values shown in today's money, with the assumptions listed next to the result.
- [ ] "How long until X?" for a target amount.

**P2**
- [ ] Monte Carlo simulation using historical return distributions.

### F10. Insights and questions

*M5 · all modules, read-only*

- As the owner, I want to ask questions in plain language ("how much did I spend on delivery since August?"), so that I don't have to build a report for every question.
- As the owner, I want to be told when something is unusual, so that I don't have to look for it.

**P1**
- [ ] Natural-language questions answered by the local LLM through read-only SQL over dedicated views, showing the query it used. (Parked: see `docs/PLAN.md`.)
- [ ] Alerts for unusual spending in a category and for categories about to exceed the budget.

**P2**
- [ ] Monthly report generated automatically at the start of each month.

## Experience

"Smart, interactive and beautiful" applies to every area above:

- **Fast and keyboard-friendly:** the common flows (review queue, search, contribution) work without the mouse.
- **Beautiful:** a consistent visual language with charts that explain instead of decorating; light and dark themes.
- **Works on the phone:** every screen is usable on a phone browser through Cloudflare Access.
- **CLI parity for the frequent actions:** `sync`, `contribute` and, later, quick queries.
- **Never lies:** stale data, pending transactions and estimates are always labeled as such.

## Success metrics

Measured on my own usage, starting two months after each area ships:

| Metric | Target |
|---|---|
| Accounts synced in the last 24 hours | 100% on at least 95% of days |
| Transactions categorized correctly without my help | at least 90% |
| Time spent on corrections | under 15 minutes per month |
| Monthly totals matching the bank statements | 100%, with zero double-counted transfers or card bills |
| Time to decide a monthly contribution | under 1 minute, with no spreadsheet |
| Spreadsheet still needed for allocation | no, after M4 |

## Open questions

| Question | Blocks |
|---|---|
| Category limits or YNAB-style envelopes? Start with limits and see if they are enough? | M3 |
| ~~Which of my investments actually come through Pluggy, and which need the B3 import?~~ Answered in `docs/pluggy.md`: Nubank sends its CDBs, stocks, ETFs and Tesouro; only an unconnected broker would need the B3 import. | M4 |
| Where do valuation metrics and dividends come from (brapi free tier or another source), and what are the limits? | F8 P1 |
| ~~How far back does Open Finance return history on the first sync?~~ Answered in `docs/pluggy.md`: about 19 months on my connection, more than the 12 Pluggy promises. | F6 history, F5 detection |
| Which channel for alerts and reminders: email, Telegram or browser notifications? | F1 P1, F8, F10 |
| Is a responsive web app enough on the phone, or is a PWA worth it? | Experience |

## Parking lot

Good ideas that are explicitly out of scope for now:

- Export data for the yearly tax return (IR).
- Goals ("trip in December") with their own savings pots.
- Detecting fees and charges that could be avoided (bank fees, card annual fees).
- Comparing my spending month over month with a seasonality adjustment.
