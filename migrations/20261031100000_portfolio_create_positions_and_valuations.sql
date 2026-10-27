-- Positions as the banks report them, the targets, and the daily
-- valuation: per class, and the net worth with cash and cards. Theses and
-- the purchase schedule are mine, typed in.

-- +goose Up
CREATE TABLE portfolio.positions (
    external_id    text PRIMARY KEY, -- the bank's id, through ingest
    item_id        text NOT NULL,
    institution    text NOT NULL,
    kind           text NOT NULL CHECK (kind IN ('fixed_income', 'treasury', 'stock', 'etf', 'bdr', 'fii', 'fund', 'other')),
    name           text NOT NULL,
    ticker         text,
    quantity_micro bigint NOT NULL DEFAULT 0, -- millionths of a unit
    value_cents    bigint NOT NULL CHECK (value_cents >= 0), -- net of taxes
    invested_cents bigint, -- what the bank says was put in
    basis_cents    bigint NOT NULL, -- what was put in, for returns
    indexer        text NOT NULL DEFAULT '',
    rate_bps       integer NOT NULL DEFAULT 0,
    maturity       date,
    as_of          date NOT NULL,
    class_override text CHECK (class_override IN ('cdi', 'ipca', 'br_stocks', 'global')),
    emergency      boolean NOT NULL DEFAULT false,
    first_seen     timestamptz NOT NULL,
    closed_at      timestamptz,
    updated_at     timestamptz NOT NULL
);

CREATE TABLE portfolio.settings (
    id         smallint PRIMARY KEY CHECK (id = 1),
    targets    jsonb NOT NULL, -- {"cdi": 2500, ...} in basis points
    band_bps   integer NOT NULL CHECK (band_bps BETWEEN 0 AND 1000),
    updated_at timestamptz NOT NULL
);

-- One row per day and class; the emergency fund is its own row.
CREATE TABLE portfolio.valuations (
    date        date NOT NULL,
    class       text NOT NULL CHECK (class IN ('cdi', 'ipca', 'br_stocks', 'global', 'emergency')),
    value_cents bigint NOT NULL,
    basis_cents bigint NOT NULL,
    PRIMARY KEY (date, class)
);

CREATE TABLE portfolio.net_worth (
    date            date PRIMARY KEY,
    cash_cents      bigint NOT NULL, -- checking and savings
    cards_cents     bigint NOT NULL, -- what the cards owe, negative
    invested_cents  bigint NOT NULL, -- the allocation's value
    emergency_cents bigint NOT NULL,
    basis_cents     bigint NOT NULL, -- what was put into invested and emergency
    updated_at      timestamptz NOT NULL
);

CREATE TABLE portfolio.theses (
    ticker      text PRIMARY KEY,
    thesis      text NOT NULL,
    breaks      text NOT NULL, -- what would break it
    next_report date, -- the company's next earnings report
    updated_at  timestamptz NOT NULL
);

CREATE TABLE portfolio.purchases (
    id           uuid PRIMARY KEY,
    planned_on   date NOT NULL,
    what         text NOT NULL, -- "Tesouro IPCA+ 2035", "WEGE3"
    class        text NOT NULL CHECK (class IN ('cdi', 'ipca', 'br_stocks', 'global')),
    amount_cents bigint NOT NULL CHECK (amount_cents > 0),
    condition    text NOT NULL DEFAULT '',
    status       text NOT NULL CHECK (status IN ('planned', 'done', 'cancelled')),
    note         text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL,
    decided_at   timestamptz
);

-- +goose Down
DROP TABLE portfolio.purchases;
DROP TABLE portfolio.theses;
DROP TABLE portfolio.net_worth;
DROP TABLE portfolio.valuations;
DROP TABLE portfolio.settings;
DROP TABLE portfolio.positions;
