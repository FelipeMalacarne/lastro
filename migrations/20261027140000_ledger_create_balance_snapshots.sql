-- One balance per account per day: the last one known that day. It is the
-- start of the net worth history (M4), so it begins with the first sync.

-- +goose Up
CREATE TABLE ledger.balance_snapshots (
    account_id    uuid NOT NULL REFERENCES ledger.accounts (id),
    date          date NOT NULL,
    balance_cents bigint NOT NULL,
    currency      text NOT NULL,
    taken_at      timestamptz NOT NULL, -- when the source read this balance
    PRIMARY KEY (account_id, date)
);

-- +goose Down
DROP TABLE ledger.balance_snapshots;
