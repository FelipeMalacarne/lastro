-- A transaction I added by hand and a synced one that look like the same
-- movement (same account, same amount, a few days apart) but that I said are
-- two: they are never suggested as duplicates again.

-- +goose Up
CREATE TABLE ledger.kept_apart (
    manual_id  uuid NOT NULL REFERENCES ledger.transactions (id) ON DELETE CASCADE,
    synced_id  uuid NOT NULL REFERENCES ledger.transactions (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (manual_id, synced_id)
);

-- +goose Down
DROP TABLE ledger.kept_apart;
