-- Each investment's transactions are stored raw too, under the investment.

-- +goose Up
ALTER TABLE ingest.raw_records DROP CONSTRAINT raw_records_kind_check;
ALTER TABLE ingest.raw_records ADD CONSTRAINT raw_records_kind_check
    CHECK (kind IN ('item', 'account', 'transaction', 'investment', 'investment_transaction'));

-- +goose Down
DELETE FROM ingest.raw_records WHERE kind = 'investment_transaction';
ALTER TABLE ingest.raw_records DROP CONSTRAINT raw_records_kind_check;
ALTER TABLE ingest.raw_records ADD CONSTRAINT raw_records_kind_check
    CHECK (kind IN ('item', 'account', 'transaction', 'investment'));
