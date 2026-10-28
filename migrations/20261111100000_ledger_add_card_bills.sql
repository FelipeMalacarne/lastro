-- A card's closed bills, as the source reports them, and the bill each card
-- transaction went into. bill_ref is the source's id for the bill, opaque to
-- the ledger: empty while the transaction is on the open bill, or when the
-- source does not say.

-- +goose Up
ALTER TABLE ledger.transactions ADD COLUMN bill_ref text NOT NULL DEFAULT '';
CREATE INDEX transactions_bill_ref_idx ON ledger.transactions (account_id, bill_ref) WHERE bill_ref <> '';

CREATE TABLE ledger.bills (
    account_id    uuid NOT NULL REFERENCES ledger.accounts (id),
    external_id   text NOT NULL, -- the source's id, what bill_ref points to
    due_date      date NOT NULL,
    total_cents   bigint NOT NULL, -- what the bill asked for, positive when owed
    minimum_cents bigint, -- the least the bank takes, when it says
    charges_cents bigint NOT NULL DEFAULT 0, -- interest and fees in it
    currency      text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, external_id)
);

-- +goose Down
DROP TABLE ledger.bills;
DROP INDEX ledger.transactions_bill_ref_idx;
ALTER TABLE ledger.transactions DROP COLUMN bill_ref;
