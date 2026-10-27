-- How many shares of each ticker were held each day, written with the daily
-- valuation, so a dividend can be counted with the quantity held on its com
-- date; and the daily fall that should raise an alert, per ticker.

-- +goose Up
CREATE TABLE portfolio.holdings (
    date           date NOT NULL,
    ticker         text NOT NULL,
    quantity_micro bigint NOT NULL CHECK (quantity_micro > 0),
    PRIMARY KEY (date, ticker)
);

CREATE TABLE portfolio.price_alerts (
    ticker     text PRIMARY KEY,
    drop_bps   integer NOT NULL CHECK (drop_bps BETWEEN 50 AND 5000), -- a fall in one day
    updated_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE portfolio.price_alerts;
DROP TABLE portfolio.holdings;
