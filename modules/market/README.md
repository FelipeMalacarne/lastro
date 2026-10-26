# market

**Responsibility:** price and index series. B3 quotes (brapi), CDI and IPCA (Banco Central's SGS) and Tesouro Direto rates.

**Not its responsibility:** positions, allocation or recommendations. `market` only knows what things are worth and were worth.

## Concepts

- Simple time series: (asset or index, date, value, source).
- Thin on purpose: it is data, not rules. Reads and writes can go straight to SQL.

## Invariants

- Series are append-only: a published value is never edited, only corrected by a new record.
- Every quote has a date and a source.

## Milestone

M4
