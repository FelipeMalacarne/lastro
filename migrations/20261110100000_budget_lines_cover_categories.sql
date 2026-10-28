-- A budget line covers one or more categories (ADR-0012) and keeps an id
-- from month to month, so what it carried follows it. Every line saved so
-- far becomes a line with one category whose id is its category's. A
-- category is in at most one line a month: line_categories is keyed by it.

-- +goose Up
ALTER TABLE budget.lines ADD COLUMN line_id uuid;
ALTER TABLE budget.lines ADD COLUMN name text CHECK (name IS NULL OR char_length(name) BETWEEN 1 AND 40);
UPDATE budget.lines SET line_id = category_id;
ALTER TABLE budget.lines ALTER COLUMN line_id SET NOT NULL;

CREATE TABLE budget.line_categories (
    month        date NOT NULL,
    line_id      uuid NOT NULL,
    category_id  uuid NOT NULL,
    PRIMARY KEY (month, category_id)
);
INSERT INTO budget.line_categories (month, line_id, category_id)
SELECT month, line_id, category_id FROM budget.lines;

ALTER TABLE budget.lines DROP CONSTRAINT lines_pkey;
ALTER TABLE budget.lines DROP COLUMN category_id;
ALTER TABLE budget.lines ADD PRIMARY KEY (month, line_id);
ALTER TABLE budget.line_categories
    ADD FOREIGN KEY (month, line_id) REFERENCES budget.lines (month, line_id) ON DELETE CASCADE;

-- +goose Down
-- A line shared by several categories keeps the first of them.
ALTER TABLE budget.lines ADD COLUMN category_id uuid;
UPDATE budget.lines l
SET category_id = (
    SELECT min(c.category_id::text)::uuid FROM budget.line_categories c
    WHERE c.month = l.month AND c.line_id = l.line_id
);
DELETE FROM budget.lines WHERE category_id IS NULL;
DROP TABLE budget.line_categories;
ALTER TABLE budget.lines DROP CONSTRAINT lines_pkey;
ALTER TABLE budget.lines DROP COLUMN line_id;
ALTER TABLE budget.lines DROP COLUMN name;
ALTER TABLE budget.lines ALTER COLUMN category_id SET NOT NULL;
ALTER TABLE budget.lines ADD PRIMARY KEY (month, category_id);
