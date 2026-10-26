---
description: Create a goose migration named by the project convention
argument-hint: "<module> <description>"
---

Create a migration for: $ARGUMENTS

1. Name: `migrations/<UTC timestamp as YYYYMMDDHHMMSS>_<module>_<snake_case_description>.sql`.
2. goose format, with `-- +goose Up` and `-- +goose Down`. Down must undo Up.
3. Only create or alter objects in the module's own schema, always schema-qualified (`ledger.transactions`, never `transactions`).
4. Money as `bigint` (cents) with the currency in a `currency text` column. Never `numeric` or `float` for money.
5. Never edit an existing migration.
6. Run `task sqlc` if the module already has queries.
