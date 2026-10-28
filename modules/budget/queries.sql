-- name: ListMonths :many
-- Every saved budget up to a month, oldest first: tracking walks them all
-- for rollover.
SELECT month, payday, pyf_cents, updated_at
FROM budget.months
WHERE month <= sqlc.arg(until)::date
ORDER BY month;

-- name: ListLines :many
SELECT month, line_id, name, limit_cents, rollover, fund
FROM budget.lines
WHERE month <= sqlc.arg(until)::date
ORDER BY month, line_id;

-- name: ListLineCategories :many
SELECT month, line_id, category_id
FROM budget.line_categories
WHERE month <= sqlc.arg(until)::date
ORDER BY month, line_id, category_id;

-- name: SaveMonth :exec
INSERT INTO budget.months (month, payday, pyf_cents, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (month) DO UPDATE
SET payday = excluded.payday, pyf_cents = excluded.pyf_cents, updated_at = excluded.updated_at;

-- name: DeleteLines :exec
-- Their categories go with them (ON DELETE CASCADE).
DELETE FROM budget.lines WHERE month = $1;

-- name: InsertLine :exec
INSERT INTO budget.lines (month, line_id, name, limit_cents, rollover, fund)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: InsertLineCategory :exec
INSERT INTO budget.line_categories (month, line_id, category_id)
VALUES ($1, $2, $3);

-- name: ListAdjustments :many
SELECT month, line_id, cents
FROM budget.adjustments
WHERE month <= sqlc.arg(until)::date
ORDER BY month, line_id;

-- name: SaveAdjustment :exec
INSERT INTO budget.adjustments (month, line_id, cents)
VALUES ($1, $2, $3)
ON CONFLICT (month, line_id) DO UPDATE SET cents = excluded.cents;

-- name: DeleteAdjustment :execrows
DELETE FROM budget.adjustments WHERE month = $1 AND line_id = $2;

-- name: ListSentAlerts :many
SELECT key FROM budget.alerts_sent WHERE key = ANY(sqlc.arg(keys)::text[]);

-- name: MarkAlertSent :exec
INSERT INTO budget.alerts_sent (key, sent_at) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING;

-- name: GetPreferences :one
SELECT rollover_new FROM budget.preferences WHERE id;

-- name: SavePreferences :exec
INSERT INTO budget.preferences (id, rollover_new) VALUES (true, $1)
ON CONFLICT (id) DO UPDATE SET rollover_new = excluded.rollover_new;
