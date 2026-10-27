-- Cash dividends per share as the companies approved them, and a daily copy
-- of each company's earnings and book value per share (and the multiples
-- brapi computed), so their history starts the first day lastro reads them.

-- +goose Up
CREATE TABLE market.dividends (
    symbol       text NOT NULL,
    com_date     date NOT NULL, -- the last day holding the share earns it
    label        text NOT NULL, -- DIVIDENDO, JCP, RENDIMENTO
    rate_micro   bigint NOT NULL CHECK (rate_micro > 0), -- reais per share, in millionths
    payment_date date,
    source       text NOT NULL,
    fetched_at   timestamptz NOT NULL,
    PRIMARY KEY (symbol, com_date, label, rate_micro)
);

CREATE TABLE market.fundamentals (
    symbol     text NOT NULL,
    date       date NOT NULL,
    eps_micro  bigint, -- earnings per share, last 12 months
    book_micro bigint, -- book value per share
    pe_milli   bigint, -- as the source computed it
    pb_milli   bigint,
    source     text NOT NULL,
    fetched_at timestamptz NOT NULL,
    PRIMARY KEY (symbol, date)
);

-- +goose Down
DROP TABLE market.fundamentals;
DROP TABLE market.dividends;
