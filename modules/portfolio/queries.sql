-- name: ListPositions :many
SELECT * FROM portfolio.positions ORDER BY institution, name, external_id;

-- name: SavePosition :exec
INSERT INTO portfolio.positions (
    external_id, item_id, institution, kind, name, ticker, quantity_micro, value_cents, invested_cents,
    basis_cents, indexer, rate_bps, maturity, as_of, class_override, emergency, first_seen, closed_at, updated_at, realized_cents
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
ON CONFLICT (external_id) DO UPDATE SET
    item_id = excluded.item_id, institution = excluded.institution, kind = excluded.kind, name = excluded.name,
    ticker = excluded.ticker, quantity_micro = excluded.quantity_micro, value_cents = excluded.value_cents,
    invested_cents = excluded.invested_cents, basis_cents = excluded.basis_cents, indexer = excluded.indexer,
    rate_bps = excluded.rate_bps, maturity = excluded.maturity, as_of = excluded.as_of,
    class_override = excluded.class_override, emergency = excluded.emergency, closed_at = excluded.closed_at,
    updated_at = excluded.updated_at, realized_cents = excluded.realized_cents;

-- name: GetSettings :one
SELECT targets, band_bps FROM portfolio.settings WHERE id = 1;

-- name: SaveSettings :exec
INSERT INTO portfolio.settings (id, targets, band_bps, updated_at) VALUES (1, $1, $2, $3)
ON CONFLICT (id) DO UPDATE SET targets = excluded.targets, band_bps = excluded.band_bps, updated_at = excluded.updated_at;

-- name: SaveValuation :exec
INSERT INTO portfolio.valuations (date, class, value_cents, basis_cents) VALUES ($1, $2, $3, $4)
ON CONFLICT (date, class) DO UPDATE SET value_cents = excluded.value_cents, basis_cents = excluded.basis_cents;

-- name: DeleteValuations :exec
DELETE FROM portfolio.valuations WHERE date = $1;

-- name: ListValuations :many
SELECT date, class, value_cents, basis_cents FROM portfolio.valuations
WHERE date >= sqlc.arg(date_from)::date ORDER BY date, class;

-- name: SaveNetWorth :exec
INSERT INTO portfolio.net_worth (date, cash_cents, cards_cents, invested_cents, emergency_cents, basis_cents, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (date) DO UPDATE SET cash_cents = excluded.cash_cents, cards_cents = excluded.cards_cents,
    invested_cents = excluded.invested_cents, emergency_cents = excluded.emergency_cents,
    basis_cents = excluded.basis_cents, updated_at = excluded.updated_at;

-- name: ListNetWorth :many
SELECT date, cash_cents, cards_cents, invested_cents, emergency_cents, basis_cents FROM portfolio.net_worth
WHERE date >= sqlc.arg(date_from)::date ORDER BY date;

-- name: ListTheses :many
SELECT ticker, thesis, breaks, next_report, updated_at FROM portfolio.theses ORDER BY ticker;

-- name: SaveThesis :exec
INSERT INTO portfolio.theses (ticker, thesis, breaks, next_report, updated_at) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (ticker) DO UPDATE SET thesis = excluded.thesis, breaks = excluded.breaks,
    next_report = excluded.next_report, updated_at = excluded.updated_at;

-- name: DeleteThesis :exec
DELETE FROM portfolio.theses WHERE ticker = $1;

-- name: ListPurchases :many
SELECT id, planned_on, what, class, amount_cents, condition, status, note, created_at, decided_at
FROM portfolio.purchases ORDER BY planned_on DESC, created_at DESC;

-- name: SavePurchase :exec
INSERT INTO portfolio.purchases (id, planned_on, what, class, amount_cents, condition, status, note, created_at, decided_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (id) DO UPDATE SET planned_on = excluded.planned_on, what = excluded.what, class = excluded.class,
    amount_cents = excluded.amount_cents, condition = excluded.condition, status = excluded.status,
    note = excluded.note, decided_at = excluded.decided_at;

-- name: GetPurchase :one
SELECT id, planned_on, what, class, amount_cents, condition, status, note, created_at, decided_at
FROM portfolio.purchases WHERE id = $1;

-- name: DeleteHoldings :exec
DELETE FROM portfolio.holdings WHERE date = $1;

-- name: SaveHolding :exec
INSERT INTO portfolio.holdings (date, ticker, quantity_micro) VALUES ($1, $2, $3);

-- name: ListHoldings :many
SELECT date, ticker, quantity_micro FROM portfolio.holdings ORDER BY ticker, date;

-- name: ListPriceAlerts :many
SELECT ticker, drop_bps FROM portfolio.price_alerts ORDER BY ticker;

-- name: SavePriceAlert :exec
INSERT INTO portfolio.price_alerts (ticker, drop_bps, updated_at) VALUES ($1, $2, $3)
ON CONFLICT (ticker) DO UPDATE SET drop_bps = excluded.drop_bps, updated_at = excluded.updated_at;

-- name: DeletePriceAlert :exec
DELETE FROM portfolio.price_alerts WHERE ticker = $1;
