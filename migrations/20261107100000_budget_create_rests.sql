-- What I chose for what each closed month left, and the default for the
-- months I did not decide.

-- +goose Up
CREATE TABLE budget.rests (
    month        date PRIMARY KEY,
    choice       text NOT NULL CHECK (choice IN ('invest', 'keep', 'split')),
    invest_cents bigint NOT NULL DEFAULT 0 CHECK (invest_cents >= 0),
    decided_at   timestamptz NOT NULL
);

ALTER TABLE budget.preferences
    ADD COLUMN rest_default text NOT NULL DEFAULT 'ask' CHECK (rest_default IN ('ask', 'invest', 'keep'));

-- +goose Down
ALTER TABLE budget.preferences DROP COLUMN rest_default;
DROP TABLE budget.rests;
