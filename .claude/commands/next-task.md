---
description: Implement the next task from docs/PLAN.md following AGENTS.md
argument-hint: "[specific task, optional]"
---

Read `AGENTS.md` and `docs/PLAN.md`.

1. If I passed a task ("$ARGUMENTS"), use it. Otherwise, take the first unchecked box in the current phase.
2. Read the `README.md` of the module involved and the related ADRs in `docs/adr/`.
3. Show me a short plan: files you will create or change, aggregates and value objects involved, invariants that will be enforced, and the tests you will write.
4. Wait for my approval.
5. Implement, starting with the domain tests.
6. Run `task check` until it passes.
7. Update the module `README.md` if a responsibility, invariant or event changed, and check the box in `docs/PLAN.md`.
