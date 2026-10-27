-- Sync state per Pluggy item, which ledger account each Pluggy account
-- became, and every record exactly as Pluggy sent it, so the ledger can be
-- rebuilt without calling the API again.

-- +goose Up
CREATE TABLE ingest.items (
    id                 text PRIMARY KEY, -- Pluggy item id
    institution        text NOT NULL DEFAULT '',
    status             text NOT NULL DEFAULT '', -- Pluggy's: UPDATED, LOGIN_ERROR, OUTDATED...
    execution_status   text NOT NULL DEFAULT '',
    error_code         text NOT NULL DEFAULT '',
    data_updated_at    timestamptz, -- when Pluggy last refreshed it from the bank
    consent_expires_at timestamptz, -- when my Meu Pluggy authorization lapses
    sync_started_at    timestamptz, -- lastro's own reads
    sync_finished_at   timestamptz,
    synced_at          timestamptz, -- last read that succeeded
    sync_error         text NOT NULL DEFAULT '', -- why the last read failed, if it did
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ingest.item_accounts (
    item_id           text NOT NULL REFERENCES ingest.items (id),
    external_id       text NOT NULL, -- Pluggy account id
    ledger_account_id uuid NOT NULL, -- the ledger's id; no foreign key across modules
    PRIMARY KEY (item_id, external_id)
);

CREATE TABLE ingest.raw_records (
    kind          text NOT NULL CHECK (kind IN ('item', 'account', 'transaction', 'investment')),
    external_id   text NOT NULL,
    parent_id     text NOT NULL, -- the item of an account or investment, the account of a transaction
    record_date   date, -- transactions only: the date the window is about
    payload       jsonb NOT NULL,
    sha256        bytea NOT NULL, -- of the payload as received; unchanged payloads are not rewritten
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    gone_at       timestamptz, -- Pluggy stopped returning it
    PRIMARY KEY (kind, external_id)
);

CREATE INDEX raw_records_parent_idx ON ingest.raw_records (kind, parent_id) WHERE gone_at IS NULL;

-- +goose Down
DROP TABLE ingest.raw_records;
DROP TABLE ingest.item_accounts;
DROP TABLE ingest.items;
