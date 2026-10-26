# lastro

Personal finance for a single person: syncs accounts through Meu Pluggy (Brazilian Open Finance), categorizes transactions, tracks the budget and the investment portfolio, and runs projections. Runs at home on K3s, behind Cloudflare Access.

> Status: **Phase 0** (skeleton). The roadmap is in [`docs/PLAN.md`](docs/PLAN.md).

## Getting started

Requires Go 1.24+, Node 22+, Docker and [Task](https://taskfile.dev).

```sh
cp .env.example .env
task setup        # go mod tidy + npm install (creates go.sum the first time)
task db           # local Postgres
task migrate
task dev          # API on http://localhost:8080
task web:dev      # in another terminal: frontend on http://localhost:5173
```

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

1. CI builds and pushes `ghcr.io/<user>/lastro:<sha>` on every push to `main`.
2. Set the image name and tag in `deploy/k8s/overlays/saradomin/kustomization.yaml` and point ArgoCD at that overlay.
3. Create the `lastro-secrets` Secret in whatever secrets format the cluster already uses, using `secret.example.yaml` as a reference. If the GHCR image is private, the namespace also needs an `imagePullSecret`.
4. In Cloudflare: create an Access application for the hostname, copy its AUD tag into the Secret, and point the tunnel route at `http://lastro.lastro.svc:80`.

The Deployment runs `lastro migrate` as an initContainer before `lastro serve`. The sync CronJob starts suspended and is turned on in Phase 1.

## Architecture

A modular monolith in Go with pragmatic DDD. The rules are in [`AGENTS.md`](AGENTS.md) and the decisions in [`docs/adr/`](docs/adr/). Each module has a README with its responsibilities, invariants and events.

```
cmd/lastro/   CLI          modules/   ledger, ingest, categorize, budget, market, portfolio, planning
app/          wiring       money/     Money and Percent
platform/     infra        web/       embedded React
```
