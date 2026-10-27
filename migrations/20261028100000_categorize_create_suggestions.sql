-- What the local model said about a merchant (the pattern a correction
-- would teach): the category it named, if any, and how sure it was. One
-- answer per merchant, so the model is asked once, not per transaction.

-- +goose Up
CREATE TABLE categorize.suggestions (
    pattern     text PRIMARY KEY, -- a contains pattern, normalized like the rules'
    category_id uuid REFERENCES categorize.categories (id), -- null: the model did not know
    confidence  smallint NOT NULL CHECK (confidence BETWEEN 0 AND 100), -- percent
    model       text NOT NULL,
    asked_at    timestamptz NOT NULL
);

-- +goose Down
DROP TABLE categorize.suggestions;
