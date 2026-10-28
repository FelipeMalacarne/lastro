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
- [x] Review queue screen, where a manual correction becomes a rule
- [x] Transactions: each row shows its category's name and colour, and the list filters by category (the last open item of F2)
- [x] `Classifier` via Ollama, validating that the answer is an existing category, with its suggestions in the queue
- [x] List, edit and archive categories and rules (the "Categories and rules" screen)
- [x] web: the shared sidebar (search, accounts with sync state, Categories and rules, Settings, theme) and the Settings page with Connections, Categorization and Appearance; its Budget and Investments sections arrive with M3 and M4

## M3: the month at a glance (F4, F5, dashboard)

- [x] ADR: category limits first, envelopes only if they prove insufficient (closes the open question in `FEATURES.md`)
- [x] `budget`: `MonthlyBudget`, pay yourself first and rollover, with tests
- [x] Budget screen
- [x] Dashboard: the month so far, spending by category against the 6-month average, spending through the month, and the review queue
- [x] Recurring charges detection and the fixed costs panel

## M4: invest in a minute (F6, F7, F8)

- [x] `market`: CDI and IPCA (Banco Central's SGS), quotes (brapi) and Tesouro Direto rates
- [x] `portfolio`: `Ticker`, `Quantity` and `Allocation`, with tests
- [x] Positions from Pluggy investments, plus the B3 trade statement import if the M1 spike shows it is needed
- [x] Daily net worth valuation and the net worth card on the dashboard
- [x] `ContributionPlanner`, with tests
- [x] `lastro contribute` in the terminal and the "where to contribute" screen
- [x] Per-stock theses and earnings-date reminders

## M5: planning and insights (F8, F9, F10)

- [x] Projection with monthly contributions and real interest rate, in pessimistic, base and optimistic scenarios
- [x] Yearly scoreboard of the stock picks against the Ibovespa and CDI
- [x] Alerts for unusual spending and for categories about to exceed the budget, after choosing the channel (ADR-0007: in the app, with Telegram as an optional push)
- [x] Monte Carlo (optional)

## M6: the rest of the features (backlog)

What `FEATURES.md` still lists, in the order that pays most. Each screen change starts on the canvas.

- [x] Split a transaction across categories (F2 P1)
- [x] Notes and tags on a transaction (F2 P1)
- [x] Dividends received, per stock and per year (F8 P1)
- [x] Valuation metrics per stock (P/E, P/B, dividend yield) and their history, from brapi (F8 P1)
- [x] Monthly report at the start of each month, through the alerts channel (F10 P2)
- [x] Price alerts, e.g. a stock down more than X% in a day (F8 P2)
- [x] Manual entries for cash or accounts outside Open Finance (F1 P2)

After M6, a design audit against the canvas added what the boards drew and the screens lacked: "Sync now" in the sidebar, Dashboard and Settings; marking a transaction as a transfer to an account lastro does not see (in To review and on the transaction); writing a rule by hand and filtering rules by where they came from; copying last month's limits, editing every limit at once and the phone layout on the Budget; the default rollover for new limits in Settings; replacing the contribution plan already on payday; and the target line on the projection.

## From use

What living with the app asked for. Each screen change starts on the canvas.

- [x] Caixinha contributions grouped: lots of the same product are one row in Positions, classified together, and the emergency fund is an amount set apart from a group (canvas: "Positions: contributions grouped, the emergency fund as an amount")
- [x] Pay yourself first counts what went to investments net of what came back, so redeeming an RDB to buy stocks puts nothing aside
- [x] A transaction the bank never sent (a Pix Meu Pluggy missed) can be added by hand to a synced account; when the bank sends it later, lastro asks whether the two are the same instead of merging on its own (canvas: "A transaction the bank never sent")
- [x] On Transactions, the month chart and its figures follow the filters (search, type, account, category, tag), the month before under the same ones, and the type picks what the chart draws (canvas: "Transactions: the month chart and its figures follow the filters")
- [x] ADR-0011 and ADR-0012 accepted: funds that save up month by month, kept invested, and a line that covers several categories (both amend ADR-0006)
- [x] `budget`: a line has an id of its own and covers one or more categories, a category in at most one line a month; today's lines migrate as one-category lines; tracking, alerts, copy, the API, the monthly report and MCP follow
- [x] `budget`: a line is a limit or a fund; a fund carries its balance, debt included, and takes adjustments; the month's totals leave the funds out; pay yourself first nets what the funds kept; the `overdrawn` alert; the API and the monthly report follow
- [ ] Budget: lines with several categories, broken down by category; funds with their balance and the month's totals without them; a line's editor with its categories, limit or fund, and the starting balance; a line or fund in a sheet on a phone, and covering a fund, moving a category or removing a line at once with Undo in a toast (canvas: "Budget: shared lines and funds", "Budget, phone: a fund in a sheet")
- [ ] `portfolio`: one group holds the funds, set apart after the emergency fund, out of the allocation and the planner, and a line of their own in net worth; Positions and Dashboard follow (canvas: "Positions: the funds set apart")

## M7: my own identity (ADR-0008, ADR-0009, ADR-0010)

lastro leaves Cloudflare Access for a Zitadel on saradomin, and opens up to clients that are not a browser: the CLI on my laptop, Claude through MCP and, later, a phone app. Boxes marked "gielinor" are done in that repository.

- [x] Split the binaries (ADR-0010): `cmd/lastrod` with `serve`, `sync`, `market`, `migrate`, `fixtures` and `seed`; `cmd/lastro` as a client, kept out of `app`, the modules and `platform` by depguard; Dockerfile, Taskfile, `AGENTS.md` and the READMEs follow
- [x] `platform/auth`: generic OIDC verifier (discovery on first use, JWKS, RS256 and ES256, issuer, audience, the owner by `sub`), `Identity` with the client and its permission, `read` for `GET` and `write` for the rest, writers listed in config
- [x] `/.well-known/oauth-protected-resource` (and `/mcp`'s), naming the CLI's client, and `WWW-Authenticate` on `401`
- [x] Web login through a BFF: `/auth/login`, `/auth/callback`, `/auth/logout`, with PKCE, `state` and `nonce`, the session in a sealed cookie (`LASTRO_SESSION_KEY`), and the web app sent to the login on a `401`
- [x] `lastro connect <url>`: discovery from the protected resource metadata, the device flow, tokens in `~/.config/lastro` (0600) with refresh, plus `lastro disconnect` and `lastro whoami`
- [x] Client commands: `lastro sync` (starts a sync on the server and follows it) and `lastro contribute`, which left `lastrod`, with the integration test that runs the client against `app.Handler()`
- [x] `/mcp`, read-only (ADR-0009): monthly report, budget, transactions with filters, accounts without numbers, categories, portfolio and contribution plan, through the official Go SDK
- [x] Spike, against the Zitadel on saradomin instead of one in the dev compose: the project ID in `aud` of JWT access tokens, the device flow with `lastro connect` and the web login, with the `lastro` project and its applications as in `docs/zitadel.md` (what it found is there too). ADR-0008 is accepted
- [x] gielinor: Zitadel on saradomin (namespace, a database on `postgres-main` inside its backup, SOPS secrets, tunnel route and DNS for `auth.ftm.dev.br`), login with a passkey, self-registration off, recovery codes in Vaultwarden. How it is set up is in gielinor's `clusters/saradomin/apps/zitadel/README.md`
- [ ] Sign out in Settings, and the screen for a signed-out or expired session (on the canvas first)
- [x] Switch saradomin to Zitadel: the OIDC settings in gielinor's Secret, the Access application out of gielinor's Terraform, the Cloudflare Access verifier removed (lastro does not start without a provider outside dev) and ADR-0004 superseded
- [ ] The Claude connector in use from the phone, with the pre-registered `lastro-claude` (and whether Zitadel needs it on Basic or POST)

## UI round 2: what the screens do

The look stays; what the screens do gets better. An audit of every screen on a phone and on desktop, with the keyboard, touch and a slow network, found the gaps. The canvas has the answers in the "UI round 2" row, built on Cédula's new components (`Sheet`, `ConfirmDialog`, `Toast`, `Tooltip`, `Skeleton`, `Switch`, `Checkbox` and `ChartCursor`), each a Radix primitive, as shadcn/ui builds them, restyled with Cédula's tokens; no new dependency.

- [x] `Sheet` for the detail on a phone: Tab stays inside, focus goes back to the row, the list keeps its scroll, the scrim stays dark in the dark theme, and it rises and closes by dragging. The budget category and the rule editor open in it too, instead of under the list (canvas: "a transaction in a sheet", "a budget category in a sheet", "a rule in a sheet")
- [x] `Toast` for what an action did, with Undo where the server answers with one (a category choice, the review queue, copying last month's limits), and the outcome of Sync now wherever you are (canvas: "toasts", "a review choice with Undo")
- [x] `ConfirmDialog` before archiving a category and deleting a rule; a removed limit, split, balance update, price alert or thesis goes at once, with Undo in a toast (canvas: "archive a category", "delete a rule, phone")
- [x] `ChartCursor` on every line chart, from what the account's balance chart already does: spending through the month, growth, a position's price and the projection, with the mouse, touch and the arrow keys (canvas: "every line chart reads")
- [x] `Skeleton` in place of "Loading…", at the size of what comes, so the page does not jump (canvas: "the dashboard while it loads")
- [x] `Switch` for "Roll over new categories" in Settings, `Checkbox` in the budget and category forms, `Tooltip` on icon-only buttons and on the markers that only had a `title`, tag suggestions that follow what you type, and the accounts list on a phone opening without a jump (canvas: "switch, checkbox, tooltips, tag suggestions, accounts on a phone")

## Net worth before the first sync

The history starts at the first sync, so returns over 12 months, the comparison with CDI and inflation, the scoreboard and the net worth in the monthly report stay empty for a year. Open Finance holds the movements behind each investment; rebuilding the days before the first sync from them fills those now. A rebuilt day is an estimate and says so.

- [x] `lastrod fixtures` downloads each investment's transactions (`GET /investments/{id}/transactions`), reports them in aggregate and anonymizes them
- [x] Spike with my connection: the anonymized dump in `testdata/meupluggy`, now with each investment's transactions, and the "Confirmed" column of `docs/pluggy.md` (CDBs reconcile, stocks do not, movements arrive weeks late)
- [x] `ingest`: the sync stores investment transactions as raw records and hands them, with each investment's issue date and amount first invested, to `portfolio` (`portfolio.histories` and `portfolio.movements`)
- [ ] `portfolio`: fixed income before the first sync, lot by lot: from its issue date at the amount first invested, less each withdrawal, valued at cost plus the CDI or IPCA accrued at its rate; a backfill through `lastrod` that never overwrites a valued day and marks the rebuilt ones as estimates
- [ ] `portfolio`: stocks and ETFs before the first sync, from their movements and the ledger's purchases of variable income, valued at the day's close
- [ ] Net worth on the rebuilt days, with cash from the ledger's daily balance and the card bills

## On the phone: an installed app (ADR-0013)

lastro installs from the browser to the home screen, as a PWA: the same screens, the same login, and a deploy is the update. A native app stays parked until it needs something a PWA cannot do.

- [x] The app icon on the canvas (option B: an l in a guilloché ring), drawn in `web/icons/icon.svg` and rendered to `web/public/icons` (canvas: "Phone app: the icon, three options")
- [x] `manifest.json` (opens Dashboard in `standalone`), the icons and the favicon, all behind the login (the manifest asked for with the session); the status bar takes the paper of the theme in use
- [x] A new version arrives with nothing on screen: back after 10 minutes or more away it reloads, sooner the next navigation loads it in full; files under `/assets/` kept for good, privately
- [ ] Installed on the iPhone and on Android: the login with the passkey inside the installed app, the icon on the home screen, and an update after a deploy

## Parked

- A native phone app (ADR-0013), only for what the PWA cannot do: a home screen widget with the month, or a lock with Face ID. The `lastro-app` Native client with PKCE in Zitadel; the API already accepts its tokens.
- Web push for alerts (ADR-0013), beside Telegram or instead of it: a service worker, and the app installed on the iPhone.
- MCP write tools (recategorize, notes), each one asking for `write`.
- Dynamic registration or CIMD for MCP clients, once Zitadel ships them and a second client needs it.
- Natural-language questions turned into read-only SQL (F10): the "Ask" card was dropped from the Planning design; it comes back only with a better interaction than a chat box.
