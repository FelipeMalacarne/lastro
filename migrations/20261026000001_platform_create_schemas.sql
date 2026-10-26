-- One schema per module. No module reads or writes another module's schema:
-- data from another module always comes through that module's Service.

-- +goose Up
CREATE SCHEMA IF NOT EXISTS ledger;
CREATE SCHEMA IF NOT EXISTS ingest;
CREATE SCHEMA IF NOT EXISTS categorize;
CREATE SCHEMA IF NOT EXISTS budget;
CREATE SCHEMA IF NOT EXISTS market;
CREATE SCHEMA IF NOT EXISTS portfolio;
CREATE SCHEMA IF NOT EXISTS planning;

-- +goose Down
-- No CASCADE on purpose: dropping a schema that holds data must be a manual decision.
DROP SCHEMA IF EXISTS planning;
DROP SCHEMA IF EXISTS portfolio;
DROP SCHEMA IF EXISTS market;
DROP SCHEMA IF EXISTS budget;
DROP SCHEMA IF EXISTS categorize;
DROP SCHEMA IF EXISTS ingest;
DROP SCHEMA IF EXISTS ledger;
