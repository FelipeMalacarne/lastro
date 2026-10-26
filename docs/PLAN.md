# Plan

One phase at a time. Within a phase, tasks are listed in the order that makes sense.
When a task is done, check its box in the same commit.

## Phase 0: skeleton

- [x] Repo layout, `go.mod` and CLI (`serve`, `sync`, `contribute`, `migrate`, `version`)
- [x] `platform`: config, db, events, httpx, auth (Cloudflare Access) and llm (Ollama)
- [x] `money` kernel: `Money` and `Percent` with tests
- [x] Migration creating one schema per module
- [x] Embedded frontend (React + Vite) and `GET /api/v1/me`
- [x] Dockerfile, compose, Taskfile, golangci with depguard, CI and Kustomize
- [x] `AGENTS.md`, `CLAUDE.md`, ADRs and a README per module
- [ ] First deploy to saradomin: Cloudflare Access application, tunnel route, Secret and the overlay in ArgoCD

## Phase 1: ingest + ledger

- [ ] `ledger`: value objects `ExternalID`, `Kind`, `Installment`, `CategoryID` and `Source`, with tests
- [ ] `ledger`: `Account` and `Transaction` aggregates, with invariants and events
- [ ] `ledger`: `TransferMatcher` for transfers between own accounts and card bill payments
- [ ] `ledger`: migrations, `queries.sql` and `repository.go` with idempotent upsert
- [ ] `ingest`: Pluggy client in `internal/pluggy` (auth, accounts, transactions and investments)
- [ ] `ingest`: anonymized fixtures and tests for the Pluggy → ledger mapping
- [ ] `ingest`: raw payloads and sync state per item
- [ ] `lastro sync` end to end, and the CronJob turned on
- [ ] `GET /api/v1/transactions`, read straight through sqlc

## Phase 2: categorize

- [ ] Categories (CRUD) and the `Rule` aggregate, with tests
- [ ] Rule engine, from the most specific rule to the most generic
- [ ] `Classifier` via Ollama, validating that the answer is an existing category
- [ ] `TransactionsImported` handler
- [ ] A manual correction becomes a rule
- [ ] "To review" queue

## Phase 3: web + budget

- [ ] Tailwind, shadcn/ui, layout and light/dark theme
- [ ] Monthly dashboard: inflows, outflows and spending per category
- [ ] Transactions screen with category correction
- [ ] `budget`: `MonthlyBudget`, pay yourself first and rollover
- [ ] Budget screen

## Phase 4: market + portfolio

- [ ] `market`: CDI and IPCA (Banco Central's SGS), quotes (brapi) and Tesouro Direto rates
- [ ] `portfolio`: `Ticker`, `Quantity` and `Allocation`, with tests
- [ ] Positions from Pluggy investments and from the B3 trade statement
- [ ] `ContributionPlanner`, with tests
- [ ] `lastro contribute` in the terminal and the "where to contribute" screen
- [ ] Per-stock theses and earnings-date reminders

## Phase 5: planning

- [ ] Projection with monthly contributions and real interest rate
- [ ] Pessimistic, base and optimistic scenarios
- [ ] Monte Carlo (optional)

## Phase 6: insights

- [ ] Natural-language questions turned into read-only SQL (Ollama), over dedicated views
- [ ] Alerts: unusual spending and categories going over budget
- [ ] Yearly scoreboard of the stock picks against the Ibovespa
