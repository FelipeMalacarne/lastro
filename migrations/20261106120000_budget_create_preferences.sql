-- How a new limit starts: whether what is left of it rolls over. One row.

-- +goose Up
CREATE TABLE budget.preferences (
    id           boolean PRIMARY KEY DEFAULT true CHECK (id),
    rollover_new boolean NOT NULL DEFAULT false
);

-- +goose Down
DROP TABLE budget.preferences;
