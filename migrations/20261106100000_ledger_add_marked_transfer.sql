-- A transaction I mark as a transfer: money moving to an account of mine
-- that lastro does not have (a Pix to my own account at another bank). It
-- then counts like a paired transfer: neither spending nor income. kind is
-- generated, so it is rebuilt with the mark, and the view that reads it too.

-- +goose Up
DROP VIEW ledger.category_lines;
ALTER TABLE ledger.transactions DROP COLUMN kind;
ALTER TABLE ledger.transactions ADD COLUMN marked_transfer boolean NOT NULL DEFAULT false;
ALTER TABLE ledger.transactions ADD COLUMN kind text NOT NULL
    GENERATED ALWAYS AS (COALESCE(pair_kind, CASE WHEN marked_transfer THEN 'transfer' END, source_kind)) STORED;

CREATE VIEW ledger.category_lines AS
WITH whole AS (
    SELECT s.transaction_id
    FROM ledger.splits s
    JOIN ledger.transactions t ON t.id = s.transaction_id
    GROUP BY s.transaction_id, t.amount_cents
    HAVING sum(s.amount_cents) = t.amount_cents
)
SELECT t.id, t.account_id, t.date, t.kind, t.currency, t.removed_at, t.tags,
    COALESCE(s.category_id, t.category_id) AS category_id, -- null: none yet
    COALESCE(s.amount_cents, t.amount_cents) AS amount_cents
FROM ledger.transactions t
LEFT JOIN ledger.splits s ON s.transaction_id = t.id AND t.id IN (SELECT transaction_id FROM whole);

-- +goose Down
DROP VIEW ledger.category_lines;
ALTER TABLE ledger.transactions DROP COLUMN kind;
ALTER TABLE ledger.transactions DROP COLUMN marked_transfer;
ALTER TABLE ledger.transactions ADD COLUMN kind text NOT NULL GENERATED ALWAYS AS (COALESCE(pair_kind, source_kind)) STORED;

CREATE VIEW ledger.category_lines AS
WITH whole AS (
    SELECT s.transaction_id
    FROM ledger.splits s
    JOIN ledger.transactions t ON t.id = s.transaction_id
    GROUP BY s.transaction_id, t.amount_cents
    HAVING sum(s.amount_cents) = t.amount_cents
)
SELECT t.id, t.account_id, t.date, t.kind, t.currency, t.removed_at, t.tags,
    COALESCE(s.category_id, t.category_id) AS category_id, -- null: none yet
    COALESCE(s.amount_cents, t.amount_cents) AS amount_cents
FROM ledger.transactions t
LEFT JOIN ledger.splits s ON s.transaction_id = t.id AND t.id IN (SELECT transaction_id FROM whole);
