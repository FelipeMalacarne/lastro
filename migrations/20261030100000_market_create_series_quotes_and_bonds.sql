-- What things are worth and were worth: SGS indices, B3 closes and Tesouro
-- Direto rates. Append-only: a published value is not edited, except a
-- quote for the current day, which the close replaces.

-- +goose Up
CREATE TABLE market.rates (
    series     text NOT NULL CHECK (series IN ('CDI', 'IPCA')),
    date       date NOT NULL, -- the day (CDI) or the month's first day (IPCA)
    rate_ppb   bigint NOT NULL, -- the period's rate as a fraction, in parts per billion
    source     text NOT NULL,
    fetched_at timestamptz NOT NULL,
    PRIMARY KEY (series, date)
);

CREATE TABLE market.quotes (
    symbol      text NOT NULL,
    date        date NOT NULL,
    close_cents bigint NOT NULL CHECK (close_cents >= 0),
    currency    text NOT NULL DEFAULT 'BRL',
    source      text NOT NULL,
    fetched_at  timestamptz NOT NULL,
    PRIMARY KEY (symbol, date)
);

CREATE TABLE market.bonds (
    name        text NOT NULL, -- "Tesouro IPCA+"
    maturity    date NOT NULL,
    date        date NOT NULL,
    buy_rate_bp integer NOT NULL, -- a year
    price_cents bigint NOT NULL,
    fetched_at  timestamptz NOT NULL,
    PRIMARY KEY (name, maturity, date)
);

-- +goose Down
DROP TABLE market.bonds;
DROP TABLE market.quotes;
DROP TABLE market.rates;
