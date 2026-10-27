-- name: ListMonths :many
-- Every saved budget up to a month, oldest first: tracking walks them all
-- for rollover.
SELECT month, payday, pyf_cents, updated_at
FROM budget.months
WHERE month <= sqlc.arg(until)::date
ORDER BY month;

-- name: ListLines :many
SELECT month, category_id, limit_cents, rollover
FROM budget.lines
WHERE month <= sqlc.arg(until)::date
ORDER BY month, category_id;

-- name: SaveMonth :exec
INSERT INTO budget.months (month, payday, pyf_cents, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (month) DO UPDATE
SET payday = excluded.payday, pyf_cents = excluded.pyf_cents, updated_at = excluded.updated_at;

-- name: DeleteLines :exec
DELETE FROM budget.lines WHERE month = $1;

-- name: InsertLine :exec
INSERT INTO budget.lines (month, category_id, limit_cents, rollover)
VALUES ($1, $2, $3, $4);

-- name: ListSentAlerts :many
SELECT key FROM budget.alerts_sent WHERE key = ANY(sqlc.arg(keys)::text[]);

-- name: MarkAlertSent :exec
INSERT INTO budget.alerts_sent (key, sent_at) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING;

-- name: GetPreferences :one
SELECT rollover_new, rest_default FROM budget.preferences WHERE id;

-- name: SavePreferences :exec
INSERT INTO budget.preferences (id, rollover_new, rest_default) VALUES (true, $1, $2)
ON CONFLICT (id) DO UPDATE SET rollover_new = excluded.rollover_new, rest_default = excluded.rest_default;

-- name: ListRests :many
SELECT month, choice, invest_cents, decided_at
FROM budget.rests
WHERE month <= sqlc.arg(until)::date
ORDER BY month;

-- name: SaveRest :exec
INSERT INTO budget.rests (month, choice, invest_cents, decided_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (month) DO UPDATE
SET choice = excluded.choice, invest_cents = excluded.invest_cents, decided_at = excluded.decided_at;

-- name: DeleteRest :exec
DELETE FROM budget.rests WHERE month = $1;
