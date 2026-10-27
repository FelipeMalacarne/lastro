-- What the bank says about a card: its limit, what is still available and
-- the bill's due and closing days, as of the account's balance. Each one is
-- null when the bank does not report it (Nubank sends no closing day).

-- +goose Up
ALTER TABLE ledger.accounts
    ADD COLUMN credit_limit_cents     bigint CHECK (credit_limit_cents >= 0),
    ADD COLUMN available_credit_cents bigint, -- negative on a card over its limit
    ADD COLUMN bill_due_date          date,
    ADD COLUMN bill_close_date        date,
    ADD CONSTRAINT accounts_credit_limit_check CHECK ((credit_limit_cents IS NULL) = (available_credit_cents IS NULL));

-- +goose Down
ALTER TABLE ledger.accounts
    DROP CONSTRAINT accounts_credit_limit_check,
    DROP COLUMN bill_close_date,
    DROP COLUMN bill_due_date,
    DROP COLUMN available_credit_cents,
    DROP COLUMN credit_limit_cents;
