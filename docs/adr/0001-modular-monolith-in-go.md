# ADR-0001: Modular monolith in Go

**Status:** Accepted
**Date:** 2026-10-26
**Deciders:** Felipe

## Context

lastro is a single-user app running on a home server (Intel N97, K3s, GitOps deploys with ArgoCD). Besides the web UI it needs a CLI (`sync`, `contribute`) and scheduled jobs. The business rules are rich enough to justify DDD: transaction classification, transfers, portfolio allocation.

## Decision

A modular monolith in Go: one binary, one database, and modules whose boundaries are enforced by the compiler and the linter. The frontend is React, embedded in the binary.

## Options considered

### Option A: Laravel + Inertia/React

| Dimension | Assessment |
|---|---|
| Complexity | Low |
| Operational cost | Medium: PHP runtime, queue and scheduler processes |
| Familiarity | High |
| CLI and jobs | Good through Artisan, but not a standalone binary |

**Pros:** lots of batteries included (auth, queues, migrations, validation); faster MVP.
**Cons:** larger image; CLI tied to the PHP runtime.

### Option B: modular monolith in Go

| Dimension | Assessment |
|---|---|
| Complexity | Medium |
| Operational cost | Low: static binary, small distroless image |
| Familiarity | High |
| CLI and jobs | Excellent: the same binary is app, CLI and job |

**Pros:** clean deploys; CLI and app share the domain; boundaries checked at compile time.
**Cons:** more infrastructure code, and CRUD screens take more manual work.

### Option C: microservices

**Cons:** operational complexity with no benefit for a single user. Rejected.

## Trade-off analysis

Laravel's advantage is initial speed. Two of Go's drawbacks disappear in this context: authentication is handled by Cloudflare Access (ADR-0004) and scheduling by the K3s CronJob. What remains is the cost of CRUD screens, which is hours, not weeks.

## Consequences

- Easier: deploys (one image, one tag), a CLI that reuses the domain, pure domain tests.
- Harder: CRUD and forms need more code than in a full-stack framework.
- To revisit: if CRUD screens become a bottleneck, consider a generator for them.
