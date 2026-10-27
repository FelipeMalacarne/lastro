-- Accounts and transactions. Amounts are cents (bigint) with the currency
-- next to them; a transaction's currency is always its account's.

-- +goose Up
CREATE TABLE ledger.accounts (
    id            uuid PRIMARY KEY,
    external_id   text NOT NULL UNIQUE,
    name          text NOT NULL,
    institution   text NOT NULL,
    type          text NOT NULL CHECK (type IN ('checking', 'savings', 'credit_card')),
    number        text NOT NULL, -- last four digits at most
    currency      text NOT NULL,
    balance_cents bigint NOT NULL, -- negative on a card: the amount owed
    balance_at    timestamptz NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ledger.transactions (
    id                 uuid PRIMARY KEY,
    account_id         uuid NOT NULL REFERENCES ledger.accounts (id),
    external_id        text NOT NULL,
    bank_ref           text NOT NULL DEFAULT '', -- the bank's own id, when the source has it
    date               date NOT NULL,
    description        text NOT NULL,
    amount_cents       bigint NOT NULL, -- negative when money leaves the account
    currency           text NOT NULL,
    source_kind        text NOT NULL CHECK (source_kind IN ('expense', 'income', 'transfer', 'bill_payment')),
    status             text NOT NULL CHECK (status IN ('pending', 'posted')),
    installment_number smallint,
    installment_total  smallint,
    category_id        text,
    category_source    text CHECK (category_source IN ('rule', 'llm', 'manual')),
    counterpart_id     uuid REFERENCES ledger.transactions (id) DEFERRABLE INITIALLY DEFERRED,
    pair_kind          text CHECK (pair_kind IN ('transfer', 'bill_payment')),
    -- What reports use: the pair's kind while paired, the source's otherwise.
    kind               text NOT NULL GENERATED ALWAYS AS (COALESCE(pair_kind, source_kind)) STORED,
    removed_at         timestamptz, -- gone from the source; kept in case it comes back
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (account_id, external_id),
    CHECK ((installment_number IS NULL) = (installment_total IS NULL)),
    CHECK (installment_number IS NULL OR (installment_number BETWEEN 1 AND installment_total AND installment_total >= 2)),
    CHECK ((category_id IS NULL) = (category_source IS NULL)),
    CHECK ((counterpart_id IS NULL) = (pair_kind IS NULL))
);

CREATE INDEX transactions_date_idx ON ledger.transactions (date DESC, id) WHERE removed_at IS NULL;
CREATE INDEX transactions_account_date_idx ON ledger.transactions (account_id, date);
CREATE INDEX transactions_bank_ref_idx ON ledger.transactions (account_id, bank_ref) WHERE bank_ref <> '';

-- +goose Down
DROP TABLE ledger.transactions;
DROP TABLE ledger.accounts;
