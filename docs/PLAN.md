# Plan

Milestones are vertical slices: each one goes from the domain to the API to a screen, and ends with something I use on the phone. One milestone at a time; within it, tasks are listed in the order that makes sense, and the next task is always the first unchecked box. When a task is done, check its box in the same commit.

Each box is one small PR. A task that creates or changes a screen starts on the design canvas (see `CLAUDE.md`).

`FEATURES.md` says what gets built and how to know it is done; this file says in what order.

## M0: skeleton and first deploy

- [x] Repo layout, `go.mod` and CLI (`serve`, `sync`, `contribute`, `migrate`, `version`)
- [x] `platform`: config, db, events, httpx, auth (Cloudflare Access) and llm (Ollama)
- [x] `money` kernel: `Money` and `Percent` with tests
- [x] Migration creating one schema per module
- [x] Embedded frontend (React + Vite) and `GET /api/v1/me`
- [x] Dockerfile, compose, Taskfile, golangci with depguard, CI and Kustomize
- [x] `AGENTS.md`, `CLAUDE.md`, ADRs and a README per module
- [x] Dev shell (Nix flake) and the whole app in Docker Compose (`task app`)
- [x] First deploy to saradomin: Cloudflare Access application, tunnel route, Secret and the overlay in ArgoCD

## M1: see my money (F1, F2)

Every account and transaction on the phone, synced every day, with transfers and card bills never counted as spending.

- [x] `ingest`: Pluggy client in `internal/pluggy` (auth, items, accounts, transactions and investments)
- [x] Pluggy spike from the docs: `lastro fixtures` (download, a report safe to share, anonymizer), synthetic fixtures and `docs/pluggy.md` answering the open questions
- [x] Pluggy spike with my connections: run `lastro fixtures`, commit the anonymized fixtures and fill the "Confirmed" column of `docs/pluggy.md`
- [x] `ledger`: value objects `ExternalID`, `Kind`, `Installment`, `CategoryID` and `Source` (plus `Status` and `AccountType`), with tests
- [x] `ledger`: `Account` and `Transaction` aggregates, with invariants and events
- [x] `ledger`: migrations, `queries.sql` and `repository.go` with idempotent upsert
- [x] `ingest`: Pluggy → ledger mapping, tested against the fixtures
- [x] `ingest`: raw payloads and sync state per item
- [x] `lastro sync` end to end, and the CronJob turned on
- [x] `ledger`: daily balance snapshot per account, so the net worth history in M4 starts from the first sync
- [x] `ledger`: `TransferMatcher` for transfers between own accounts and card bill payments
- [x] web: Tailwind, shadcn/ui and the Cédula tokens (self-hosted fonts, light and dark), and the app shell (sidebar, month switcher, phone tab bar)
- [x] web: `Amount`, `Badge`, `CategoryTag`, `TransactionRow` and `SyncStatus` components
- [x] `GET /api/v1/transactions` and the Transactions screen (list, filters, search), with each account's sync status and the stale-sync notice
- [x] Backup of `postgres-main` in gielinor: the Barman Cloud plugin to R2, ready to turn on (gielinor `clusters/saradomin/apps/postgres/backup`)
- [x] Turn the backup on: R2 bucket and token, then the component (gielinor `apps/postgres/backup/README.md`; needs me)
- [x] Pluggy credentials and item ids on saradomin (gielinor `apps/lastro/pluggy-secrets.yaml`, the `lastro-pluggy` Secret)
- [x] First sync on saradomin, seen on lastro.ftm.dev.br (the CronJob runs at minute 17 of 0h, 6h, 12h and 18h; needs me to look)

## Between M1 and M2: a UI round

Asked for after the first real sync: more to click and more to see. Designed on the canvas first ("UI round" row).

- [x] Card details from Pluggy (limit, available, due day) on the account
- [x] Read endpoints: an account, its rebuilt daily balance, its months and flows, its purchases in installments, one transaction in full, a month's spending by day
- [x] Account page: balance chart, money in and out by month, where a month went; for a card, the limit in use, spending by month with committed installments, installments in progress and bills paid
- [x] Transactions: the month's spending by day against the month before, click a day to list it; click a transaction for its detail (side panel on desktop, sheet on a phone), with ↑ ↓ and Esc
- [x] Pluggy's built copies of card bill payments left out of the ledger; a reprocess removes what the raw records no longer hold

## M2: categorized without effort (F3)

Every transaction has a category or is waiting in the review queue, and every correction teaches a rule.

- [x] `categorize`: categories (CRUD, seeded with the eight note colours) and the `Rule` aggregate, with tests
- [x] Rule engine, from the most specific rule to the most generic
- [x] `TransactionsImported` handler
- [ ] Review queue screen, where a manual correction becomes a rule
- [ ] Transactions: each row shows its category's name and colour, and the list filters by category (the last open item of F2)
- [ ] `Classifier` via Ollama, validating that the answer is an existing category, with its suggestions in the queue
- [ ] List, edit and archive categories and rules (the "Categories and rules" screen)
- [ ] web: the shared sidebar (search, accounts with sync state, Categories and rules, Settings, theme) and the Settings page with Connections, Categorization and Appearance; its Budget and Investments sections arrive with M3 and M4

## M3: the month at a glance (F4, F5, dashboard)

- [ ] ADR: category limits first, envelopes only if they prove insufficient (closes the open question in `FEATURES.md`)
- [ ] `budget`: `MonthlyBudget`, pay yourself first and rollover, with tests
- [ ] Budget screen
- [ ] Dashboard: the month so far, spending by category against the 6-month average, spending through the month, and the review queue
- [ ] Recurring charges detection and the fixed costs panel

## M4: invest in a minute (F6, F7, F8)

- [ ] `market`: CDI and IPCA (Banco Central's SGS), quotes (brapi) and Tesouro Direto rates
- [ ] `portfolio`: `Ticker`, `Quantity` and `Allocation`, with tests
- [ ] Positions from Pluggy investments, plus the B3 trade statement import if the M1 spike shows it is needed
- [ ] Daily net worth valuation and the net worth card on the dashboard
- [ ] `ContributionPlanner`, with tests
- [ ] `lastro contribute` in the terminal and the "where to contribute" screen
- [ ] Per-stock theses and earnings-date reminders

## M5: planning and insights (F8, F9, F10)

- [ ] Projection with monthly contributions and real interest rate, in pessimistic, base and optimistic scenarios
- [ ] Yearly scoreboard of the stock picks against the Ibovespa and CDI
- [ ] Alerts for unusual spending and for categories about to exceed the budget, after choosing the channel (email or Telegram)
- [ ] Monte Carlo (optional)

## Out of the way until M4

Splitting a transaction across categories, notes and tags on transactions, valuation metrics, and price alerts.

## Parked

Natural-language questions turned into read-only SQL (F10): the "Ask" card was dropped from the Planning design; it comes back only with a better interaction than a chat box.
