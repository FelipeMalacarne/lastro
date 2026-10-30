---
description: Implement the next task from docs/PLAN.md following AGENTS.md
argument-hint: "[specific task, optional]"
---

Read `AGENTS.md` and `docs/PLAN.md`.

1. If I passed a task ("$ARGUMENTS"), use it. Otherwise, take the first unchecked box in the current milestone that no other worktree is on: `orca worktree list --json` shows each worktree's name and comment. Name the task at the top of your plan.
2. Read the `README.md` of the module involved and the related ADRs in `docs/adr/`. If the task creates or changes a screen, find its artboard on the design canvas (link in `CLAUDE.md`). If there is none, or it does not show the change, design it on the canvas first and wait for my approval of the design.
3. Show me a short plan: files you will create or change, aggregates and value objects involved, invariants that will be enforced, and the tests you will write.
4. Wait for my approval. Then mark the task as taken: `orca worktree set --worktree active --comment "<task>" --json`.
5. Implement, starting with the domain tests.
6. Run `task check` until it passes. If the task changed a screen, see it in the running app (`AGENTS.md`, "When finishing a task").
7. Update the module `README.md` if a responsibility, invariant or event changed, and check the box in `docs/PLAN.md`.
