-- A card's closed bills are stored raw too, under the card's account.

-- +goose Up
ALTER TABLE ingest.raw_records DROP CONSTRAINT raw_records_kind_check;
ALTER TABLE ingest.raw_records ADD CONSTRAINT raw_records_kind_check
    CHECK (kind IN ('item', 'account', 'transaction', 'investment', 'investment_transaction', 'bill'));

-- +goose Down
DELETE FROM ingest.raw_records WHERE kind = 'bill';
ALTER TABLE ingest.raw_records DROP CONSTRAINT raw_records_kind_check;
ALTER TABLE ingest.raw_records ADD CONSTRAINT raw_records_kind_check
    CHECK (kind IN ('item', 'account', 'transaction', 'investment', 'investment_transaction'));
