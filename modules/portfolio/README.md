# portfolio

**Responsibility:** positions, the target allocation and where each contribution should go; the daily valuation and net worth; the thesis for each stock and the purchase schedule.

**Not its responsibility:** fetching prices (that is `market`) or reading the banks (that is `ingest`, which hands over each sync's investments as `Report`s; nothing about Pluggy reaches here).

## Concepts

- Value objects: `Ticker` (validates the B3 format and normalizes the odd-lot ticker: ITUB4F becomes ITUB4), `Quantity` (in millionths, since banks report fractions), `Class` (CDI, IPCA+, Brazilian stocks, global stocks), `Allocation` (target weights as `money.Percent`) and `Band` (how far a class may drift from its target).
- `Position` (entity): what the bank reported last, its basis (what was put in: the bank's figure when it has one; otherwise buying adds at the day's unit price and selling takes in proportion) and how I classify it. `Classify` puts inflation-linked bonds in IPCA+, the rest of fixed income in CDI, BDRs and the ETFs of stocks abroad in global, and Brazilian stocks, FIIs and ETFs in Brazilian stocks; any position can be moved by hand or set apart as emergency fund.
- `Targets` (aggregate): the allocation and the band, 25/25/10/40 and 2 points until I set mine.
- `Value` (domain service): the open positions on a day, a stock or ETF at the latest quote when it is newer than its bank's report. `Return` is the modified Dietz return between two valuations.
- `PlanContribution` (the contribution planner): takes each class's value, the targets, the band and the amount. Classes below their band come first, up to the band's lower edge; what is left goes to every class under its target in proportion to its gap; once every gap is closed, the rest follows the targets. A class above its target gets nothing.
- `Thesis`: a stock's one-line thesis, what would break it and its next earnings report. `Purchase`: a planned buy (when, what, how much, on what condition), marked done or cancelled by hand.

## Flow

`lastro sync` reads the banks, `ingest` calls `SyncPositions` per item (what an item stops reporting closes), then `market` refreshes the quotes of the tickers held and `Valuate` stores the day: each class's value and basis, and the net worth with cash and card bills from `ledger`. The history starts at the first sync.

## Invariants

- The weights of an `Allocation` add up to exactly 100%.
- The planner never suggests selling, and what it distributes adds up to exactly the contribution amount (`Money.Allocate`, so no cents are lost).
- The emergency fund is not part of the allocation, and is shown apart.
- A purchase is decided once: done or cancelled.

## API

- `GET /api/v1/portfolio`: today's valuation (invested, basis, the month's change, twelve months put in, returns and return), net worth (cash, cards, invested, emergency fund), each class against its target and band with its twelve-month return, the growth by month, CDI and IPCA over the same months, the holdings and the theses with days to the next report.
- `GET /api/v1/net-worth?months=12`: the stored net worth per day, and today's.
- `GET /api/v1/scoreboard`: each calendar year of the Brazilian stocks class (the satellite) against the Ibovespa (`^BVSP` at brapi, which needs `LASTRO_BRAPI_TOKEN`) and CDI, from the stored valuations.
- `PUT /api/v1/portfolio/targets` `{"targets": {"cdi": 2500, ...}, "bandBp": 200}`: 400 unless they add up to 100%.
- `PATCH /api/v1/positions/{id}` `{"class": "ipca" | "", "emergency": true}`: `""` goes back to the automatic class.
- `GET /api/v1/contribute?amount=300000`: the split, with each class's weight before and after and a suggestion of what to buy.
- `PUT /api/v1/theses/{ticker}` `{"thesis", "breaks", "nextReport"}`, `DELETE /api/v1/theses/{ticker}`, `GET /api/v1/reminders` (reports in the next week for stocks held).
- `GET /api/v1/purchases`, `POST /api/v1/purchases` `{"plannedOn", "what", "class", "amountCents", "condition"}`, `PATCH /api/v1/purchases/{id}` `{"status": "done" | "cancelled", "note"}`.

`lastro contribute --amount 3000` prints the same split; without `--amount` it takes this month's pay yourself first.

## Milestone

M4
