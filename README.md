# lastro

Personal finance for a single person: syncs accounts through Meu Pluggy (Brazilian Open Finance), categorizes transactions, tracks the budget and the investment portfolio, and runs projections. Runs at home on K3s, behind Cloudflare Access.

> Status: **Phase 0** (skeleton). The roadmap is in [`docs/PLAN.md`](docs/PLAN.md).

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

To run it the way the cluster does (one container with the embedded frontend, migrations first), use `task app` and open `http://localhost:8080`.

Opening `http://localhost:5173` should show "Signed in as dev@localhost".

## Binary commands

```
lastro serve        API + embedded frontend
lastro sync         pull accounts and transactions from Pluggy (Phase 1)
lastro contribute   show where this month's contribution should go (Phase 4)
lastro migrate      apply pending migrations
lastro version      print the version
```

## Configuration

Everything through `LASTRO_*` variables. The full list, with explanations, is in [`.env.example`](.env.example).

| Variable | Purpose |
|---|---|
| `LASTRO_ENV` | `dev` or `prod` (default `prod`) |
| `LASTRO_DATABASE_URL` | Postgres connection |
| `LASTRO_CF_ACCESS_TEAM_DOMAIN`, `LASTRO_CF_ACCESS_AUD` | Cloudflare Access JWT verification |
| `LASTRO_AUTH_DISABLED` | turns authentication off; only accepted with `LASTRO_ENV=dev` |
| `LASTRO_OLLAMA_URL`, `LASTRO_OLLAMA_MODEL` | local LLM; empty means rules only |
| `LASTRO_PLUGGY_*` | Dashboard credentials and Meu Pluggy item IDs |

## Deploying to saradomin

The app is served at `https://lastro.ftm.dev.br`, through the saradomin Cloudflare tunnel and behind Cloudflare Access.

1. On every push to `main`, CI builds and pushes `ghcr.io/felipemalacarne/lastro:<sha>`, then commits that tag to `deploy/k8s/overlays/saradomin/kustomization.yaml`.
2. Argo CD on saradomin follows that overlay on `main`, so the new image goes live without further steps.
3. The cluster side lives in the [gielinor](https://github.com/FelipeMalacarne/gielinor) repository, in `clusters/saradomin/apps/lastro`. It holds the Argo CD `Application`, the `lastro` namespace and the SOPS-encrypted `lastro-secrets`. The database is `lastro` on saradomin's shared CloudNativePG cluster, `postgres-main` in `clusters/saradomin/apps/postgres`. The tunnel route, DNS record and Access application are in its `terraform/cloudflare/main.tf`. Its README there lists the steps to turn it on.

The Deployment runs `lastro migrate` as an initContainer before `lastro serve`. The sync CronJob starts suspended and is turned on in Phase 1.

## Architecture

A modular monolith in Go with pragmatic DDD. The rules are in [`AGENTS.md`](AGENTS.md) and the decisions in [`docs/adr/`](docs/adr/). Each module has a README with its responsibilities, invariants and events.

```
cmd/lastro/   CLI          modules/   ledger, ingest, categorize, budget, market, portfolio, planning
app/          wiring       money/     Money and Percent
platform/     infra        web/       embedded React
```
