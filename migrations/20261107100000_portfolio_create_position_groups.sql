-- How I classify a product moves from each position to its group: every
-- lot of the same bank, kind, ticker, name, indexer and rate (each Nubank
-- Caixinha contribution is a CDB of its own). The emergency fund becomes an
-- amount set apart from a group, or all of it, instead of a flag per lot.
-- The id is the first 16 hex digits of the SHA-256 of those parts joined
-- by "|", as portfolio.GroupID computes it.

-- +goose Up
CREATE TABLE portfolio.position_groups (
    id             text PRIMARY KEY,
    class_override text CHECK (class_override IN ('cdi', 'ipca', 'br_stocks', 'global')),
    reserve        text NOT NULL DEFAULT 'none' CHECK (reserve IN ('none', 'amount', 'all')),
    reserve_cents  bigint NOT NULL DEFAULT 0, -- the amount set apart, when reserve is 'amount'
    updated_at     timestamptz NOT NULL,
    CHECK ((reserve = 'amount') = (reserve_cents > 0))
);

-- A group whose open lots were all marked is all emergency fund; one with
-- some marked sets apart what those are worth today. Its class is the one
-- chosen for most of its lots.
INSERT INTO portfolio.position_groups (id, class_override, reserve, reserve_cents, updated_at)
SELECT id, class_override,
       CASE WHEN all_marked THEN 'all' WHEN marked_cents > 0 THEN 'amount' ELSE 'none' END,
       CASE WHEN all_marked THEN 0 ELSE marked_cents END,
       now()
FROM (
    SELECT id,
           mode() WITHIN GROUP (ORDER BY class_override) AS class_override,
           coalesce(bool_and(emergency) FILTER (WHERE closed_at IS NULL), false) AS all_marked,
           coalesce(sum(value_cents) FILTER (WHERE closed_at IS NULL AND emergency), 0) AS marked_cents
    FROM (
        SELECT left(encode(sha256(convert_to(concat_ws('|', institution, kind, coalesce(ticker, ''), name, indexer, rate_bps::text), 'UTF8')), 'hex'), 16) AS id,
               class_override, emergency, value_cents, closed_at
        FROM portfolio.positions
    ) lots
    GROUP BY id
) g
WHERE all_marked OR marked_cents > 0 OR class_override IS NOT NULL;

ALTER TABLE portfolio.positions DROP COLUMN class_override, DROP COLUMN emergency;

-- +goose Down
ALTER TABLE portfolio.positions
    ADD COLUMN class_override text CHECK (class_override IN ('cdi', 'ipca', 'br_stocks', 'global')),
    ADD COLUMN emergency boolean NOT NULL DEFAULT false;

-- Each lot takes its group's class; a group with any emergency fund marks
-- all its lots.
UPDATE portfolio.positions p
SET class_override = g.class_override, emergency = g.reserve <> 'none'
FROM portfolio.position_groups g
WHERE g.id = left(encode(sha256(convert_to(concat_ws('|', p.institution, p.kind, coalesce(p.ticker, ''), p.name, p.indexer, p.rate_bps::text), 'UTF8')), 'hex'), 16);

DROP TABLE portfolio.position_groups;
