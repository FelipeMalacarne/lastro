# portfolio

**Responsibility:** positions and their groups, the target allocation and where each contribution should go; the daily valuation and net worth; the thesis for each stock and the purchase schedule.

**Not its responsibility:** fetching prices (that is `market`) or reading the banks (that is `ingest`, which hands over each sync's investments as `Report`s; nothing about Pluggy reaches here).

## Concepts

- Value objects: `Ticker` (validates the B3 format, a four-character root that may hold digits but not only digits, as in B5P211, and a share-class number; normalizes the odd-lot ticker: ITUB4F becomes ITUB4), `Quantity` (in millionths, since banks report fractions), `Class` (CDI, IPCA+, Brazilian stocks, global stocks), `Allocation` (target weights as `money.Percent`) and `Band` (how far a class may drift from its target).
- `Position` (entity): what the bank reported last, its basis (what is still in it at cost: the bank's figure when it has one; otherwise buying adds at the day's unit price and selling takes in proportion), what it realized (money taken out beyond its cost: a sale at the day's price, a redemption at the position's value, a closing at its last value). Its net in (basis minus realized) is the flow returns are measured against, so cashed-out gains stay gains. `Classify` puts inflation-linked bonds and the ETFs that hold them (IMA-B and IMA-B 5, such as B5P211 and IMAB11, or any ETF named IMA-B) in IPCA+, the rest of fixed income and the ETFs of Selic and fixed-rate Treasury bonds in CDI, BDRs and the ETFs of stocks abroad in global, and Brazilian stocks, FIIs and stock ETFs in Brazilian stocks; an ETF the bank sends as a stock is classified as the ETF it is.
- `Group` (aggregate): every lot of one product, open or closed: the same bank, kind, ticker, name, indexer and rate (`GroupID`; the maturity does not split it, since each Nubank Caixinha contribution is a CDB of its own with its own maturity). How I classify a product belongs to its group: the class (Classify's unless I move it) and the `Reserve`, so a new lot joins already classified.
- `Reserve` (value object): how much of a group is the emergency fund: none, a fixed amount (`AmountReserve`, positive) or all of it. With an amount, the group's net in splits in the same proportion as its value, so the set-apart amount earns what its group earns; when the group holds less, all of it is the emergency fund and `Short` says how much is missing.
- `Targets` (aggregate): the allocation and the band, 25/25/10/40 and 2 points until I set mine.
- `Value` (domain service): the open lots on a day, group by group, a stock or ETF at the latest quote when it is newer than its bank's report; each group goes to its class but for the part set apart as the emergency fund. `Return` is the modified Dietz return between two valuations, with what was put in minus what was taken out (at value) counted for half the period. A class's flows include its closed positions.
- `PlanContribution` (the contribution planner): takes each class's value, the targets, the band and the amount. Classes below their band come first, up to the band's lower edge; what is left goes to every class under its target in proportion to its gap; once every gap is closed, the rest follows the targets. A class above its target gets nothing.
- `HeldHistory`: how many shares of each ticker were held on each valued day (`Valuate` writes them). `Earned` counts each dividend at the shares held on its com date, only from the first valued day on and only once the com date has passed. `Dropped` tells whether a close fell at least a price alert's threshold from the one before.
- `Thesis`: a stock's one-line thesis, what would break it and its next earnings report. `Purchase`: a planned buy (when, what, how much, on what condition), marked done or cancelled by hand.

## Flow

`lastrod sync` reads the banks, `ingest` calls `SyncPositions` per item (what an item stops reporting closes), then `market` refreshes the quotes of the tickers held and `Valuate` stores the day: each class's value and basis, and the net worth with cash and card bills from `ledger`. The history starts at the first sync.

## Invariants

- The weights of an `Allocation` add up to exactly 100%.
- The planner never suggests selling, and what it distributes adds up to exactly the contribution amount (`Money.Allocate`, so no cents are lost).
- The emergency fund is not part of the allocation, and is shown apart. So are the assets outside Open Finance (`Other` in net worth); a wallet counts as cash.
- The emergency fund is set per group, never per lot, and what a group sets apart plus what it leaves in its class is the whole group, value and net in.
- A purchase is decided once: done or cancelled.
- A dividend whose com date is before lastro knew the holdings is not counted: dividends are never guessed with today's quantity, except in the forward estimate, which says so.
- A price alert is between 0,5% and 50% in a day, and fires only on a close from the last few days of a ticker still held.

## API

- `GET /api/v1/portfolio`: today's valuation (invested, basis, the returns since about a month ago with `monthFrom`, money put in left out, twelve months put in, returns and return), net worth (cash, cards, invested, emergency fund), each class against its target and band with its twelve-month return, the growth by month, CDI and IPCA over the same months, the groups held with their lots and emergency fund, and the theses with days to the next report.
- `GET /api/v1/net-worth?months=12`: the stored net worth per day, and today's.
- `GET /api/v1/scoreboard`: each calendar year of the Brazilian stocks class (the satellite) against the Ibovespa (`^BVSP` at brapi, which needs `LASTRO_BRAPI_TOKEN`) and CDI, from the stored valuations.
- `PUT /api/v1/portfolio/targets` `{"targets": {"cdi": 2500, ...}, "bandBp": 200}`: 400 unless they add up to 100%.
- `PATCH /api/v1/position-groups/{id}` `{"class": "ipca" | "", "reserve": "none" | "amount" | "all", "reserveCents": 1200000}`: `""` goes back to the automatic class; each field is optional.
- `GET /api/v1/contribute?amount=300000`: the split, with each class's weight before and after and a suggestion of what to buy.
- `PUT /api/v1/theses/{ticker}` `{"thesis", "breaks", "nextReport"}`, `DELETE /api/v1/theses/{ticker}`, `GET /api/v1/reminders` (reports in the next week for stocks held).
- `GET /api/v1/dividends`: the year's and each year's dividends, each ticker's year, twelve months per share, yield and next payment, what was earned and is still to be paid, the forward estimate at today's quantities, and the tickers held with no payments known.
- `GET /api/v1/price-alerts`, `PUT /api/v1/price-alerts/{ticker}` `{"dropBps": 500}`, `DELETE /api/v1/price-alerts/{ticker}`. `PriceDrops` is read by `app`, which adds the drops to `GET /api/v1/alerts`.
- `GET /api/v1/purchases`, `POST /api/v1/purchases` `{"plannedOn", "what", "class", "amountCents", "condition"}`, `PATCH /api/v1/purchases/{id}` `{"status": "done" | "cancelled", "note"}`.

`lastro contribute --amount 3000` (the client, through `GET /api/v1/contribute`) prints the same split; without `--amount` it takes this month's pay yourself first.

## Milestone

M4
