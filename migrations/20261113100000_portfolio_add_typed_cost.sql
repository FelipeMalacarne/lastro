-- The average price I type for a stock or ETF whose bank does not send what
-- it cost: how far it moved the position's basis from lastro's estimate (the
-- value on the first sync), and when. The shift is what "Use the estimate"
-- takes back.

-- +goose Up
ALTER TABLE portfolio.positions
    ADD COLUMN cost_shift_cents bigint,
    ADD COLUMN cost_typed_at timestamptz;

-- +goose Down
ALTER TABLE portfolio.positions
    DROP COLUMN cost_typed_at,
    DROP COLUMN cost_shift_cents;
