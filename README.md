# lastro

Personal finance for a single person: syncs accounts through Meu Pluggy (Brazilian Open Finance), categorizes transactions, tracks the budget and the investment portfolio, and runs projections. Runs at home on K3s, signed in through a self-hosted Zitadel.

> Status: **M0 to M6 are built**: accounts and transactions synced from Meu Pluggy (and typed in, for accounts outside Open Finance), transfers and card bills paired, rules and the local model with a review queue, the budget with pay yourself first and rollover, recurring charges, the portfolio with the contribution planner, dividends and valuation metrics, projections with a Monte Carlo, alerts and the monthly report. The roadmap is in [`docs/PLAN.md`](docs/PLAN.md).

## Getting started

Requires Go 1.24+, Node 22+, Docker and [Task](https://taskfile.dev). With Nix, `nix develop` (or `direnv allow`, which reads `.envrc`) gives you everything except Docker: Go, Node 22, Task, golangci-lint, sqlc, gopls and the Postgres 17 client.

```sh
cp .env.example .env
task setup        # go mod tidy + npm install (creates go.sum the first time)
task db           # local Postgres
task migrate
task dev          # API on http://localhost:8080
task web:dev      # in another terminal: frontend on http://localhost:5173
```

Each Orca worktree runs on its own ports and database: `scripts/worktree.sh setup` prepares it (see `AGENTS.md`), `task up` starts the API and the frontend, and `task urls` says where they are.

To run it the way the cluster does (one container with the embedded frontend, migrations first), use `task app` and open `http://localhost:8080`.

Opening `http://localhost:5173` should show "Signed in as dev@localhost".

## Binaries

Two binaries ([ADR-0010](docs/adr/0010-client-cli-and-server-binary.md)). `lastrod` is the server: it runs in the image and works against the database.

```
lastrod serve        API + embedded frontend
lastrod sync         pull accounts and transactions from Pluggy (--full for the whole history, --reprocess to rebuild from stored payloads)
lastrod market       refresh CDI, IPCA, Tesouro Direto and the quotes of what is held (--symbols for more)
lastrod migrate      apply pending migrations
lastrod seed         fill a dev database with fourteen months of made-up money (LASTRO_ENV=dev only)
lastrod fixtures     download Pluggy responses, print a report safe to share and anonymize them into test fixtures
lastrod version      print the version
```

`lastro` is the client: every command goes through the HTTP API of a server, with the same authentication as the web app.

```
lastro connect <url>   point at a server and sign in with a code in the browser (device flow); in dev, lastro connect http://localhost:8080
lastro whoami          the server and who it sees
lastro sync            sync the banks on the server and follow it (--detach to not wait)
lastro contribute      where this month's contribution should go (--amount 3000)
lastro disconnect      forget the server and the token (~/.config/lastro/config.json)
```

## Configuration

Everything through `LASTRO_*` variables. The full list, with explanations, is in [`.env.example`](.env.example).

| Variable | Purpose |
|---|---|
| `LASTRO_ENV` | `dev` or `prod` (default `prod`) |
| `LASTRO_DATABASE_URL` | Postgres connection |
| `LASTRO_PUBLIC_URL` | how lastro is reached, e.g. `https://lastro.ftm.dev.br` |
| `LASTRO_OIDC_ISSUER`, `LASTRO_OIDC_AUDIENCE`, `LASTRO_OIDC_OWNER_SUBJECT` | the identity provider, what access tokens carry in `aud` (Zitadel's project ID) and the owner's `sub` ([ADR-0008](docs/adr/0008-identity-with-zitadel.md)) |
| `LASTRO_OIDC_SCOPES` | scopes clients ask for, space separated (Zitadel's project audience scope) |
| `LASTRO_OIDC_WEB_CLIENT_ID`, `LASTRO_OIDC_WEB_CLIENT_SECRET`, `LASTRO_SESSION_KEY` | the web login and the key that seals its session cookie (`openssl rand -base64 32`) |
| `LASTRO_OIDC_CLI_CLIENT_ID`, `LASTRO_OIDC_WRITE_CLIENTS` | the public client of `lastro connect`, and other clients that may write; the rest only read |
| `LASTRO_AUTH_DISABLED` | turns authentication off; only accepted with `LASTRO_ENV=dev` |
| `LASTRO_OLLAMA_URL`, `LASTRO_OLLAMA_MODEL` | local LLM; empty means rules only |
| `LASTRO_PLUGGY_*` | Dashboard credentials and Meu Pluggy item IDs |

## Deploying to saradomin

The app is served at `https://lastro.ftm.dev.br`, through the saradomin Cloudflare tunnel. Sign-in is lastro's own, against the Zitadel at `https://auth.ftm.dev.br` ([`docs/zitadel.md`](docs/zitadel.md)).

1. On every push to `main`, CI builds and pushes `ghcr.io/felipemalacarne/lastro:<sha>`, then commits that tag to `deploy/k8s/overlays/saradomin/kustomization.yaml`.
2. Argo CD on saradomin follows that overlay on `main`, so the new image goes live without further steps.
3. The cluster side lives in the [gielinor](https://github.com/FelipeMalacarne/gielinor) repository, in `clusters/saradomin/apps/lastro`. It holds the Argo CD `Application`, the `lastro` namespace and the SOPS-encrypted `lastro-secrets`. The database is `lastro` on saradomin's shared CloudNativePG cluster, `postgres-main` in `clusters/saradomin/apps/postgres`. The tunnel route and DNS record are in its `terraform/cloudflare/main.tf`, and Zitadel in `clusters/saradomin/apps/zitadel`. Its README there lists the steps to turn it on.

The image holds only `lastrod`. The Deployment runs `lastrod migrate` as an initContainer before `lastrod serve`. The sync CronJob runs `lastrod sync` four times a day. "Sync now" in the app (the sidebar, the Dashboard and Settings) runs the same sync inside `lastrod serve`, one at a time: `POST /api/v1/sync` starts it (409 while one runs, 503 without Pluggy credentials) and `GET /api/v1/sync` reports how it went.

## Sign-in, the CLI and MCP

`platform/auth` takes two ways in ([ADR-0008](docs/adr/0008-identity-with-zitadel.md)): the web app's session, set by the login against the OIDC provider, and bearer tokens from that provider, for `lastro` and MCP clients. Without a provider, lastro does not start outside dev. Only the owner gets in, and a client writes only when it is listed as a writer.

Claude reads lastro through MCP at `/mcp` ([ADR-0009](docs/adr/0009-mcp-server-for-claude.md)), with read-only tools. Setting up Zitadel, the clients and the Claude connector is in [`docs/zitadel.md`](docs/zitadel.md).

## Architecture

A modular monolith in Go with pragmatic DDD. The rules are in [`AGENTS.md`](AGENTS.md) and the decisions in [`docs/adr/`](docs/adr/). Each module has a README with its responsibilities, invariants and events.

```
cmd/lastrod/  server       modules/   ledger, ingest, categorize, budget, market, portfolio, planning
cmd/lastro/   client       money/     Money and Percent
app/          wiring       web/       embedded React
platform/     infra
```
