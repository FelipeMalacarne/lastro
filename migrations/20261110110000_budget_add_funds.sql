-- A line is a limit or a fund (ADR-0011): a fund saves its monthly amount
-- up and carries its balance, debt included, computed and never stored. An
-- adjustment moves money already invested into a fund or out of it, at
-- most one per line and month; it refers to the line by id only, since the
-- month may follow an earlier saved budget instead of having its own.

-- +goose Up
ALTER TABLE budget.lines ADD COLUMN fund boolean NOT NULL DEFAULT false;

CREATE TABLE budget.adjustments (
    month     date NOT NULL CHECK (extract(day FROM month) = 1), -- the month's first day
    line_id   uuid NOT NULL,
    cents     bigint NOT NULL CHECK (cents <> 0), -- into the fund when positive
    currency  text NOT NULL DEFAULT 'BRL' CHECK (currency = 'BRL'),
    PRIMARY KEY (month, line_id)
);

-- +goose Down
DROP TABLE budget.adjustments;
ALTER TABLE budget.lines DROP COLUMN fund;
