---
description: Create a new module following the AGENTS.md conventions
argument-hint: "<module-name>"
---

Create the `$ARGUMENTS` module following `AGENTS.md`:

1. `modules/$ARGUMENTS/doc.go` with the package comment.
2. `modules/$ARGUMENTS/README.md` with the same sections as the other modules: Responsibility, Not its responsibility, Concepts, Invariants, Events and Milestone. Ask me about anything unclear.
3. A migration creating the `$ARGUMENTS` schema (use `/new-migration`).
4. The module's block in `sqlc.yaml`, commented out until there are queries.
5. The module's field and wiring in `app/app.go`, still commented out.
6. The module in the repo map in `AGENTS.md` and a milestone for it in `docs/PLAN.md`.

Do not create subfolders other than `internal/db/` (and `internal/<client>/` if the module talks to something external).
