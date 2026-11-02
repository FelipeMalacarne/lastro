-- A fund is a pot (ADR-0014): it may have a target, past which it takes
-- no monthly amount, and it may cover no category at all (the emergency
-- fund, a goal), so its money leaves only through an adjustment. Only a
-- fund has a target. Nothing here stops a line without categories: the
-- domain keeps a limit from having none.

-- +goose Up
ALTER TABLE budget.lines ADD COLUMN target_cents bigint
    CHECK (target_cents IS NULL OR (fund AND target_cents > 0));

-- +goose Down
-- A pot without categories covered nothing before pots existed.
DELETE FROM budget.lines l
WHERE NOT EXISTS (
    SELECT 1 FROM budget.line_categories c WHERE c.month = l.month AND c.line_id = l.line_id
);
ALTER TABLE budget.lines DROP COLUMN target_cents;
