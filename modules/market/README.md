# market

**Responsibility:** price and index series. B3 quotes (brapi), CDI and IPCA (Banco Central's SGS) and Tesouro Direto rates (Tesouro Transparente).

**Not its responsibility:** positions, allocation or recommendations. `market` only knows what things are worth and were worth.

## Concepts

- `Rate`: a period's rate as a fraction in parts per billion (basis points are too coarse for a day of CDI). `Compound` chains them exactly.
- `Point` (CDI per business day, IPCA per month), `Quote` (a ticker's close on a day) and `Bond` (a Tesouro Direto title's buy rate and price on a day).
- Thin on purpose: it is data, not rules. Each source is a client in `internal/` (`sgs`, `brapi`, `tesouro`), swapped for fakes in tests.

## Refresh

`Refresh(symbols)` reads only what is new: CDI and IPCA from the day after the last stored (two years back the first time), each symbol's closes (two years the first time, the last days after), and Tesouro Direto's latest day. A source that fails is logged and named in the report; it never stops the others. `lastro sync` refreshes after every sync with the tickers held, and `lastro market --symbols PETR4,IVVB11` does it by hand.

`LASTRO_BRAPI_TOKEN` is optional: without it brapi answers only a few tickers (PETR4 does; most return 401). Tesouro Transparente rate-limits often; Pluggy's own valuation of the titles held still stands.

## Invariants

- Series are append-only: a published value is never edited. The only exception is a quote for the current day (or the day before), which the close replaces.
- Every quote has a date and a source. Only reais: a quote in another currency is refused.

## API

- `GET /api/v1/market`: `CDI` and `IPCA`, each with the last value's date, the twelve months to it compounded (`last12mBp`) and the latest value (CDI a year at the latest day's rate), plus `bonds`, Tesouro Direto's latest titles with their buy rate and price.

- `GET /api/v1/quotes/{symbol}?months=12`: a symbol's daily closes, oldest first.

## Milestone

M4
