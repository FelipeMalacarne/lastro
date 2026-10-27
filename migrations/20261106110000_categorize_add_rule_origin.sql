-- Who made a rule: a correction in the review queue or a transaction's
-- detail, or by hand on the Categories and rules screen.

-- +goose Up
ALTER TABLE categorize.rules
    ADD COLUMN origin text NOT NULL DEFAULT 'correction' CHECK (origin IN ('correction', 'hand'));

-- +goose Down
ALTER TABLE categorize.rules DROP COLUMN origin;
