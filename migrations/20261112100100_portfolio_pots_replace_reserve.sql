-- The emergency fund stops being an amount set on a group (ADR-0014): it
-- is a pot on Budget, and groups are marked as holding the pots instead.
-- Every group that had a reserve holds the pots. The amount is not moved:
-- the emergency pot is made by hand on Budget, since a migration only
-- touches its own module's schema. The stored history keeps the emergency
-- fund's days as the pots'.

-- +goose Up
ALTER TABLE portfolio.position_groups ADD COLUMN holds_pots boolean NOT NULL DEFAULT false;
UPDATE portfolio.position_groups SET holds_pots = reserve <> 'none';
-- Dropping the columns drops the check that tied them.
ALTER TABLE portfolio.position_groups DROP COLUMN reserve, DROP COLUMN reserve_cents;

ALTER TABLE portfolio.valuations DROP CONSTRAINT valuations_class_check;
UPDATE portfolio.valuations SET class = 'pots' WHERE class = 'emergency';
ALTER TABLE portfolio.valuations ADD CONSTRAINT valuations_class_check
    CHECK (class IN ('cdi', 'ipca', 'br_stocks', 'global', 'pots'));

ALTER TABLE portfolio.net_worth RENAME COLUMN emergency_cents TO pots_cents;

-- +goose Down
-- A group that held the pots goes back to being all emergency fund: the
-- amount it had is gone.
ALTER TABLE portfolio.net_worth RENAME COLUMN pots_cents TO emergency_cents;

ALTER TABLE portfolio.valuations DROP CONSTRAINT valuations_class_check;
UPDATE portfolio.valuations SET class = 'emergency' WHERE class = 'pots';
ALTER TABLE portfolio.valuations ADD CONSTRAINT valuations_class_check
    CHECK (class IN ('cdi', 'ipca', 'br_stocks', 'global', 'emergency'));

ALTER TABLE portfolio.position_groups
    ADD COLUMN reserve text NOT NULL DEFAULT 'none' CHECK (reserve IN ('none', 'amount', 'all')),
    ADD COLUMN reserve_cents bigint NOT NULL DEFAULT 0;
UPDATE portfolio.position_groups SET reserve = 'all' WHERE holds_pots;
ALTER TABLE portfolio.position_groups ADD CHECK ((reserve = 'amount') = (reserve_cents > 0));
ALTER TABLE portfolio.position_groups DROP COLUMN holds_pots;
