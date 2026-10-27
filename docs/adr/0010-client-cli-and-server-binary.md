# ADR-0010: `lastro` is a client, `lastrod` is the server

**Status:** Accepted (amends ADR-0005)
**Date:** 2026-10-27
**Deciders:** Felipe

## Context

ADR-0005 put every role in one `lastro` binary. Every command loads `config`, runs `app.New` and talks to the database, so the CLI only works inside the cluster.

With `lastro connect <url>` (ADR-0008), I run the CLI on my laptop, against the server. There, `lastro sync` obviously means "sync on the server". Inside the CronJob, though, the same `lastro sync` means "sync here, against the database". One command with two meanings, picked by whether a config file happens to exist, is a trap.

## Decision

Two binaries, from the same module and the same repository:

| Binary | Where it runs | Commands |
|---|---|---|
| `lastrod` | the image: Deployment, CronJob, initContainer, and dev | `serve`, `sync`, `market`, `migrate`, `fixtures`, `seed`, `version` |
| `lastro` | my laptop, and later anywhere | `connect`, `disconnect`, `whoami`, `sync`, `contribute`, `version`, and whatever else gains an endpoint |

- **`lastro` is always a client.** Every command goes through the HTTP API, with the token from `lastro connect`, and is subject to the same authentication and permissions as the web app, the app and MCP. In dev it connects to `http://localhost:8080`, where auth is disabled.
- **`lastrod` does the work where it runs.** Its commands are unchanged from today, renamed.
- **`lastro sync` starts a sync on the server** (`POST /api/v1/sync`) and follows it (`GET /api/v1/sync`) until it ends. The CronJob keeps running `lastrod sync`.
- **The client knows no module.** `cmd/lastro` may not import `lastro/app`, `lastro/modules/...`, `lastro/platform/...` or `lastro/web`; depguard enforces it.
  - Its wire types live in `cmd/lastro/internal/client`, written by hand, only for the endpoints it calls.
  - `money` is the exception: a kernel with no dependencies, used for formatting.
  - An integration test runs the client against the real `app.Handler()`, so a renamed JSON field fails CI.
- **Still the standard library's `flag`.** Neither binary needs cobra yet.

## Options considered

- **One binary, client when connected:** the double meaning above. Rejected.
- **One binary with namespaces** (`lastro server sync` and `lastro sync`): clear, but the laptop gets a binary that carries pgx, the embedded frontend and Pluggy, and still knows how to talk to the database. Rejected.
- **Two binaries:** no ambiguity. The client is small, holds no database or Pluggy secrets, and can be shipped on its own (Nix, `go install`). Chosen.

## Consequences

- The image keeps only `lastrod`. The Dockerfile, Kustomize (`args`), the Taskfile, `AGENTS.md` and the README rename the server commands.
- A CLI feature now needs an endpoint first. That is the point: the API is the single contract for every interface.
- An OpenAPI spec with generated types is only worth it if the client's surface grows large. Until then, the hand-written types and the integration test are enough.
