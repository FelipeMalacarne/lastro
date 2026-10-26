@AGENTS.md

## Claude Code specifics

- Project commands live in `.claude/commands/`: `/next-task`, `/new-module` and `/new-migration`.
- Before editing a module, read its `modules/<name>/README.md`.
- Before implementing, show a short plan (files, aggregates, value objects and tests) and wait for approval.
- `.env` is blocked from reading in `.claude/settings.json`. If you need a variable, ask.
