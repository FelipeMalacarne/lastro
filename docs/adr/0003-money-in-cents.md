# ADR-0003: Money in cents and percentages in basis points

**Status:** Accepted
**Date:** 2026-10-26
**Deciders:** Felipe

## Context

Floating point cannot represent cents exactly. Summing transactions, splitting contributions and applying percentages in `float64` accumulates rounding errors.

## Decision

- Money is `money.Money`: cents as `int64` plus the currency. In the database, `bigint` plus a `currency` column.
- Percentages are `money.Percent`, in basis points (1 bp = 0.01%).
- Splits use `Money.Allocate`, which never loses cents.
- For rates that need more precision (compound interest), use a decimal library when needed.
- `float64` is only accepted inside `planning`, where everything is an estimate, and results are converted to `Money` for display.

## Options considered

- **`float64` everywhere:** simple but wrong for money. Rejected.
- **A decimal library everywhere:** exact, but more verbose and slower than needed for cents.
- **`int64` cents:** exact, fast and sufficient. Chosen.

## Consequences

- API values never pass through a float. Pluggy sends JSON numbers: `ingest` keeps the literal and converts it to cents, rounding anything past two places (floating-point residue on Pluggy's side) half away from zero. Typed-in values go through `money.ParseDecimal`, which rejects more than two decimal places.
- Overflow only happens above roughly 90 quadrillion reais, so it is not a concern.
