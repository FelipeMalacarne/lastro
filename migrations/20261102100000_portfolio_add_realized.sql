-- What came out of a position beyond its cost, so returns count money taken
-- out at what it was worth.

-- +goose Up
ALTER TABLE portfolio.positions ADD COLUMN realized_cents bigint NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE portfolio.positions DROP COLUMN realized_cents;
