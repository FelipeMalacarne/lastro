-- Net worth counts the assets outside Open Finance (balances I type in) on
-- their own line, next to the emergency fund.

-- +goose Up
ALTER TABLE portfolio.net_worth ADD COLUMN other_cents bigint NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE portfolio.net_worth DROP COLUMN other_cents;
