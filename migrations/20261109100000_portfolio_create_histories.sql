-- What each investment did, as the bank reports it: what it is and its
-- movements (purchases, withdrawals, transfers, taxes, income). Kept for
-- every investment, also one fully withdrawn before lastro knew it, which
-- never becomes a position: the days before the first sync are rebuilt
-- from these.

-- +goose Up
CREATE TABLE portfolio.histories (
    external_id    text PRIMARY KEY,
    item_id        text NOT NULL,
    institution    text NOT NULL,
    kind           text NOT NULL,
    name           text NOT NULL,
    ticker         text,
    indexer        text NOT NULL DEFAULT '',
    rate_bps       integer NOT NULL DEFAULT 0,
    maturity       date,
    issued_on      date, -- fixed income: the day it was bought
    invested_cents bigint, -- what was first put in, when the bank says
    quantity_micro bigint NOT NULL DEFAULT 0, -- as last reported
    value_cents    bigint NOT NULL,
    active         boolean NOT NULL,
    updated_at     timestamptz NOT NULL
);

CREATE TABLE portfolio.movements (
    external_id    text PRIMARY KEY,
    history_id     text NOT NULL REFERENCES portfolio.histories (external_id) ON DELETE CASCADE,
    date           date NOT NULL,
    kind           text NOT NULL CHECK (kind IN ('buy', 'sell', 'transfer_in', 'transfer_out', 'tax', 'income', 'amortization')),
    quantity_micro bigint CHECK (quantity_micro >= 0), -- null when the bank sends none
    amount_cents   bigint NOT NULL CHECK (amount_cents >= 0), -- gross
    net_cents      bigint -- after taxes and fees, when sent
);

CREATE INDEX movements_history_idx ON portfolio.movements (history_id, date);

-- +goose Down
DROP TABLE portfolio.movements;
DROP TABLE portfolio.histories;
