# AGENTS.md

Guide for coding agents (Claude Code, Codex, Cursor and the like). It applies to humans too.

## What this is

**lastro** is a personal finance app for a single user. It syncs accounts through Meu Pluggy (Brazilian Open Finance), categorizes transactions, tracks the budget and the investment portfolio, and runs projections. It runs on the K3s cluster at home and is reached from outside through Cloudflare Tunnel, with Cloudflare Access in front.

## Current state

M0 (skeleton and first deploy), M1 (see my money), M2 (categorization) and M3 (the month at a glance) are done: Pluggy sync from my Meu Pluggy connection every 6 hours on saradomin, ledger, transfer pairing, rules and the local model with a review queue, the budget with pay yourself first and rollover, recurring charges, and the Dashboard, Transactions, Budget, Categories and Settings screens, with `postgres-main` backed up to R2. M4 (investing) is done too: market data, positions, the daily valuation, the contribution planner and theses. M5 (planning and insights) is done: projections with a Monte Carlo, the yearly scoreboard and alerts. M6 is done too: splitting a transaction, notes and tags, review undo, dividends, valuation metrics, price alerts, accounts outside Open Finance and the monthly report. The next task is always the first unchecked box in `docs/PLAN.md`.

## Commands

| Command | What it does |
|---|---|
| `nix develop` | dev shell from `flake.nix` with every tool except Docker |
| `task setup` | `go mod tidy` and `npm install` for the frontend |
| `task db` | starts the local Postgres (compose) |
| `task migrate` | applies migrations to the database in `.env` |
| `task dev` | API on :8080 |
| `task app` | the whole app in Docker on :8080 (Postgres, migrations, API with the embedded frontend) |
| `task web:dev` | frontend on :5173 with a proxy to the API |
| `task test` | Go tests |
| `task test:integration` | Go tests plus the integration tests (repositories against a real Postgres; needs Docker) |
| `task lint` | golangci-lint, including the boundary rules |
| `task check` | tests and lint; **must pass before a task is considered done** (CI also runs the integration tests) |
| `task sqlc` | regenerates query code |

## Repo map

```
cmd/lastro/      CLI: serve, sync, contribute, migrate, fixtures, seed (dev data), version
app/             composition root: the only place that knows every module
modules/<name>/  one Go package per module (ledger, ingest, categorize, budget, market, portfolio, planning)
money/           shared kernel: Money and Percent
platform/        infrastructure: config, db, events, httpx, auth, llm, notify
migrations/      goose SQL, embedded in the binary
web/             React + Vite, embedded in the binary
deploy/k8s/      Kustomize: base + overlays/saradomin
docs/            PLAN.md (milestones), pluggy.md (what Pluggy sends) and adr/ (decisions)
```

## Architecture rules

These rules are not negotiable. To change one, write a new ADR in `docs/adr/`.

1. **Modular monolith.** Each module is one Go package in `modules/<name>/`.
2. **Modules talk only through their public facade**, meaning the exported types of the module's root package. Never import `modules/<other>/internal/...`; the compiler blocks it.
3. **One Postgres schema per module.** No query touches another module's schema. If you need data from another module, call its `Service`.
4. **Queries are calls, reactions are events.** To read another module's data, call its `Service`. To react to something that happened, subscribe to an event in `platform/events`. Events are published only after the commit.
5. **Only `app/` wires modules.** Modules receive dependencies through their constructor and never reach for globals.
6. **Pluggy only exists inside `modules/ingest/`.** No Pluggy type leaves it.
7. **Money is always `money.Money`, in cents.** Never `float64` for money. Percentages are `money.Percent`, in basis points.

## Pragmatic DDD: how to write a module

File layout, using `ledger` as the example:

| File | Contents |
|---|---|
| `transaction.go` | aggregate: unexported fields, methods that enforce invariants, events recorded internally and exposed through `PullEvents()` |
| `values.go` | value objects: immutable, with a validating constructor (`NewX`, `ParseX`) that returns an error instead of building an invalid value |
| `transfer.go` | domain service, when a rule spans more than one aggregate |
| `events.go` | events published by the module: value structs with `EventName()` |
| `service.go` | public facade: use cases (commands) and queries |
| `repository.go` | Postgres implementation: maps sqlc rows to the domain and back |
| `http.go` | thin handlers and `RegisterRoutes(mux)` |
| `queries.sql`, `internal/db/` | queries and sqlc-generated code |
| `README.md` | responsibility, invariants and events; keep it up to date |

Rules:

- **Business rules live in the aggregate, the value object or the domain service.** Never in `service.go` or `http.go`. `service.go` only orchestrates: load, call the domain, save, publish events.
- **CQRS-lite:** writes go through the domain. Reads for screens and reports may go straight to SQL and return DTOs.
- **Domain files do not import `pgx`, `net/http` or `platform/llm`.** depguard blocks it in lint.
- **Interfaces only where something is actually swapped:** repository, external client, LLM. The interface is declared in the module that uses it.
- **Avoid:** generic repositories, a DTO per layer, interfaces for everything, getters and setters that only expose fields.
- **Thin modules are fine.** `market` and simple CRUD can skip aggregates entirely.

## Database and migrations

- Files go in `migrations/<timestamp>_<module>_<description>.sql`, in goose format (`-- +goose Up` and `-- +goose Down`).
- Never edit a migration that has already been applied. Create a new one.
- Each module only creates and alters objects in its own schema.
- After changing migrations or `queries.sql`, run `task sqlc`.

## Tests

- **Domain:** table-driven, no database, no network. Most tests live here.
- **`repository.go`:** integration tests against a real Postgres (testcontainers), behind the `integration` build tag.
- **HTTP:** `httptest`.
- **Pluggy fixtures:** in `modules/ingest/testdata/pluggy/`, always anonymized.

## Conventions

- **Everything in English:** code, comments, docs, routes, log messages and user-facing messages.
- Money and percentages are formatted the Brazilian way for display (`R$ 1.234,56`, `12,5%`), since the amounts are BRL.
- Errors carry context: `fmt.Errorf("ledger: import transactions: %w", err)`. Export sentinel errors when callers need to tell them apart with `errors.Is`.
- Logging with `log/slog`.
- Configuration only through `platform/config`, with `LASTRO_*` variables. No `os.Getenv` anywhere else.
- New dependencies only with a clear reason. Prefer the standard library.

## Security and privacy

- **Real financial data never goes into fixtures, logs, issues or commits.** Never log tokens, Pluggy credentials or full transactions.
- Every new route sits behind Cloudflare Access (`platform/auth`). Only `/healthz` and `/readyz` are left out.
- `LASTRO_AUTH_DISABLED` only works with `LASTRO_ENV=dev`. Do not work around this.
- The LLM is local (Ollama). Do not add calls to external LLMs with financial data.

## When finishing a task

1. `task check` passes.
2. The module README is updated if a responsibility, invariant or event changed.
3. The box is checked in `docs/PLAN.md`.
