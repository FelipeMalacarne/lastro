-- My categories, each drawn with one of the eight banknote colours, and the
-- rules that put a transaction in one by its description. The category a
-- transaction has lives in the ledger, which only stores the id.

-- +goose Up
CREATE TABLE categorize.categories (
    id          uuid PRIMARY KEY,
    name        text NOT NULL,
    color       text NOT NULL CHECK (color IN ('note-1', 'note-2', 'note-5', 'note-10', 'note-20', 'note-50', 'note-100', 'note-200')),
    budgeted    boolean NOT NULL DEFAULT true, -- counts toward the monthly budget (Income does not)
    archived_at timestamptz, -- hidden from pickers and the LLM; old transactions keep showing it
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Two live categories never share a name; an archived one frees its name.
CREATE UNIQUE INDEX categories_name_idx ON categorize.categories (lower(name)) WHERE archived_at IS NULL;

CREATE TABLE categorize.rules (
    id           uuid PRIMARY KEY,
    pattern_kind text NOT NULL CHECK (pattern_kind IN ('exact', 'contains')),
    pattern      text NOT NULL, -- normalized: no accents, upper case, words separated by one space
    account_id   uuid, -- the ledger's account it is limited to, or null for every account; no foreign key across modules
    category_id  uuid NOT NULL REFERENCES categorize.categories (id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE NULLS NOT DISTINCT (pattern_kind, pattern, account_id)
);

CREATE INDEX rules_category_idx ON categorize.rules (category_id);

-- The categories drawn on the canvas, one per note.
INSERT INTO categorize.categories (id, name, color, budgeted) VALUES
    (gen_random_uuid(), 'Transport', 'note-1', true),
    (gen_random_uuid(), 'Health', 'note-2', true),
    (gen_random_uuid(), 'Leisure', 'note-5', true),
    (gen_random_uuid(), 'Restaurants', 'note-10', true),
    (gen_random_uuid(), 'Shopping', 'note-20', true),
    (gen_random_uuid(), 'Groceries', 'note-50', true),
    (gen_random_uuid(), 'Housing', 'note-100', true),
    (gen_random_uuid(), 'Other', 'note-200', true),
    (gen_random_uuid(), 'Income', 'note-2', false);

-- +goose Down
DROP TABLE categorize.rules;
DROP TABLE categorize.categories;
