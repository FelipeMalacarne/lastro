# market

**Responsibility:** price and index series. B3 quotes (brapi), CDI and IPCA (Banco Central's SGS) and Tesouro Direto rates (Tesouro Transparente).

**Not its responsibility:** positions, allocation or recommendations. `market` only knows what things are worth and were worth.

## Concepts

- `Rate`: a period's rate as a fraction in parts per billion (basis points are too coarse for a day of CDI). `Compound` chains them exactly.
- `Point` (CDI per business day, IPCA per month), `Quote` (a ticker's close on a day) and `Bond` (a Tesouro Direto title's buy rate and price on a day).
- `Dividend` (a cash payment per share: its com date, label and rate in millionths of a real) and `Fundamental` (a day's earnings and book value per share, and the source's own multiples). `Valuation` turns them into P/E, P/B and dividend yield at a day's close: per-share figures win over the source's multiples, a multiple over a loss is left out, and the yield is the twelve months of payments up to the day over its close.
- Thin on purpose: it is data, not rules. Each source is a client in `internal/` (`sgs`, `brapi`, `tesouro`), swapped for fakes in tests.

## Refresh

`Refresh(symbols)` reads only what is new: CDI and IPCA from the day after the last stored (two years back the first time), each symbol's closes (two years the first time, the last days after), and Tesouro Direto's latest day. A source that fails is logged and named in the report; it never stops the others.

Everything read is stored, so history grows with each refresh. brapi's free plan goes back three months at most (`range` always counts from today, so it cannot be paged back): when it refuses two years for a new ticker, the ticker starts with three months. SGS sometimes answers "nothing in the range" as an `{"erro":{"statusCode":404}}` object with a 200, which is an empty answer like the 404. Tesouro Direto's file (every day since 2002) is read only when the titles stored are older than the last weekday. `lastrod sync` refreshes after every sync with the tickers held, and `lastrod market --symbols PETR4,IVVB11` does it by hand.

After each symbol's closes, its fundamentals and dividends are read once a day (`fundamental=true&dividends=true`, with the key statistics module when the plan has it; brapi refuses it on the free plan, so the request is made again without it). A failure there is logged and never fails the refresh. `Record` stores closes, dividends and fundamentals that did not come from a refresh (the dev seed's).

`LASTRO_BRAPI_TOKEN` is optional: without it brapi answers only a few tickers (PETR4 does; most return 401). Tesouro Transparente rate-limits often (a 429 is logged and the titles stored stand); Pluggy's own valuation of the titles held still stands.

## Invariants

- Series are append-only: a published value is never edited. The only exception is a quote for the current day (or the day before), which the close replaces.
- Every quote has a date and a source. Only reais: a quote in another currency is refused.

## API

- `GET /api/v1/market`: `CDI` and `IPCA`, each with the last value's date, the twelve months to it compounded (`last12mBp`) and the latest value (CDI a year at the latest day's rate), plus `bonds`, Tesouro Direto's latest titles with their buy rate and price.

- `GET /api/v1/quotes/{symbol}?months=12`: a symbol's daily closes, oldest first.
- `GET /api/v1/valuation/{symbol}?months=24`: `points`, each day lastro read the company's fundamentals with `peMilli`, `pbMilli` (null when there is none) and `yieldBps`.

## Milestone

M4
