-- What I add to a transaction: a note, tags, and a split across categories.
-- The bank's data never touches these. category_lines is what totals by
-- category read: a transaction once, or each part of its split. A split
-- whose parts no longer add up to the amount (the bank restated it) is
-- ignored until it is split again.

-- +goose Up
ALTER TABLE ledger.transactions
    ADD COLUMN note text NOT NULL DEFAULT '' CHECK (length(note) <= 500),
    ADD COLUMN tags text[] NOT NULL DEFAULT '{}' CHECK (cardinality(tags) <= 10);

CREATE INDEX transactions_tags_idx ON ledger.transactions USING gin (tags);

CREATE TABLE ledger.splits (
    transaction_id uuid NOT NULL REFERENCES ledger.transactions (id) ON DELETE CASCADE,
    position       smallint NOT NULL CHECK (position >= 0),
    category_id    text NOT NULL,
    amount_cents   bigint NOT NULL CHECK (amount_cents <> 0),
    PRIMARY KEY (transaction_id, position),
    UNIQUE (transaction_id, category_id)
);

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
DROP TABLE ledger.splits;
DROP INDEX ledger.transactions_tags_idx;
ALTER TABLE ledger.transactions DROP COLUMN tags, DROP COLUMN note;
