-- name: InsertRate :exec
INSERT INTO market.rates (series, date, rate_ppb, source, fetched_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (series, date) DO NOTHING;

-- name: LastRateDate :one
SELECT COALESCE(max(date), '0001-01-01')::date AS last FROM market.rates WHERE series = $1;

-- name: ListRates :many
SELECT series, date, rate_ppb FROM market.rates
WHERE series = $1 AND date >= sqlc.arg(date_from)::date AND date <= sqlc.arg(date_to)::date
ORDER BY date;

-- name: SaveQuote :exec
-- A day's close is written once; the current day's price is replaced until
-- its close arrives.
INSERT INTO market.quotes (symbol, date, close_cents, currency, source, fetched_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (symbol, date) DO UPDATE
SET close_cents = excluded.close_cents, fetched_at = excluded.fetched_at
WHERE market.quotes.date >= sqlc.arg(today)::date - 1;

-- name: CountQuotes :one
SELECT count(*)::int FROM market.quotes WHERE symbol = $1;

-- name: LatestQuotes :many
SELECT DISTINCT ON (symbol) symbol, date, close_cents, source
FROM market.quotes
WHERE symbol = ANY(sqlc.arg(symbols)::text[])
ORDER BY symbol, date DESC;

-- name: ListQuotes :many
SELECT symbol, date, close_cents, source FROM market.quotes
WHERE symbol = $1 AND date >= sqlc.arg(date_from)::date
ORDER BY date;

-- name: InsertBond :exec
INSERT INTO market.bonds (name, maturity, date, buy_rate_bp, price_cents, fetched_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (name, maturity, date) DO NOTHING;

-- name: LatestBonds :many
SELECT name, maturity, date, buy_rate_bp, price_cents FROM market.bonds
WHERE date = (SELECT max(date) FROM market.bonds)
ORDER BY name, maturity;
