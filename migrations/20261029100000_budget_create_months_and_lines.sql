-- Budgets saved per month (ADR-0006): a month without a row inherits the
-- latest one before it. Category ids come from categorize; no foreign key
-- crosses schemas, and an archived category simply stops being shown.

-- +goose Up
CREATE TABLE budget.months (
    month       date PRIMARY KEY CHECK (extract(day FROM month) = 1), -- the month's first day
    payday      smallint NOT NULL CHECK (payday BETWEEN 1 AND 31),
    pyf_cents   bigint NOT NULL CHECK (pyf_cents >= 0), -- pay yourself first
    currency    text NOT NULL DEFAULT 'BRL' CHECK (currency = 'BRL'),
    updated_at  timestamptz NOT NULL
);

CREATE TABLE budget.lines (
    month        date NOT NULL REFERENCES budget.months (month) ON DELETE CASCADE,
    category_id  uuid NOT NULL,
    limit_cents  bigint NOT NULL CHECK (limit_cents >= 0),
    rollover     boolean NOT NULL,
    PRIMARY KEY (month, category_id)
);

-- +goose Down
DROP TABLE budget.lines;
DROP TABLE budget.months;
