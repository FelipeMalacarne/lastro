# ADR-0005: One binary, many roles, with a standard-library CLI

**Status:** Accepted, amended by [ADR-0010](0010-client-cli-and-server-binary.md): the server binary is now `lastrod`, and `lastro` is a client of its API
**Date:** 2026-10-26
**Deciders:** Felipe

## Context

The app needs to serve HTTP, sync with Pluggy periodically, apply migrations and answer commands in the terminal.

## Decision

A single `lastro` binary with subcommands. On K3s, the same image runs in three roles:

| Role | Resource | Command |
|---|---|---|
| API and frontend | Deployment | `lastro serve` |
| Sync | CronJob | `lastro sync` |
| Migrations | Deployment initContainer | `lastro migrate` |

The CLI uses only the standard library's `flag` package.

## Options considered

- **cobra:** nicer help, nested subcommands and completion. One more dependency for five commands.
- **Standard library:** zero dependencies and enough for the current size. Chosen.

## Consequences

- Scheduling is handled by the CronJob, so no job queue is needed for now. If one becomes necessary, River provides a queue on top of Postgres itself.
- If the CLI grows (nested subcommands, completion), switching to cobra only touches `cmd/lastro`.
