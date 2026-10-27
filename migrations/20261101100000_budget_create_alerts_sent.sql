-- Alerts already pushed, by key, so a sync never sends one twice.

-- +goose Up
CREATE TABLE budget.alerts_sent (
    key     text PRIMARY KEY, -- kind:month:subject
    sent_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE budget.alerts_sent;
