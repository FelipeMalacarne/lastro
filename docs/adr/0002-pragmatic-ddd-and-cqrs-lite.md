# ADR-0002: Pragmatic DDD, one package per module and CQRS-lite

**Status:** Accepted
**Date:** 2026-10-26
**Deciders:** Felipe

## Context

Business logic should be centralized in aggregates, value objects and domain services, testable without a database. At the same time, Go favors few, larger packages, and package-per-layer tends to become ceremony.

## Decision

- Each module is **a single Go package**, with files split by concern (aggregate, value objects, service, repository, HTTP).
- `internal/` inside a module only for sqlc-generated code and clients of external APIs.
- Domain purity is enforced by depguard: domain files do not import `pgx`, `net/http` or `platform/llm`.
- **CQRS-lite:** writes go through the domain; reads for screens and reports go straight to SQL and return DTOs.
- Domain events are recorded in the aggregate and published by `service.go` after the commit.

## Options considered

### Option A: package per layer (`domain/`, `app/`, `store/` inside each module)

**Pros:** separation enforced by the compiler.
**Cons:** many packages, exported getters just for mapping, and a Java feel.

### Option B: one package per module

**Pros:** idiomatic; unexported fields encapsulate aggregates from everything outside the module; simple mapping.
**Cons:** separation between domain and infrastructure inside the module relies on convention and lint.

### Option C: no DDD (transaction script)

**Cons:** invariants (a transfer is not an expense, an allocation adds up to 100%) would spread across handlers.

## Consequences

- Easier: most tests are domain tests, fast and infrastructure-free.
- Harder: mapping between sqlc rows and the domain is explicit boilerplate in `repository.go`.
- To revisit: if a module grows too large (`ledger` is the likely candidate), split a `domain/` subpackage in that module only.
