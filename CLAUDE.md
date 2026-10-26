@AGENTS.md

## Claude Code specifics

- Project commands live in `.claude/commands/`: `/next-task`, `/new-module` and `/new-migration`.
- Before editing a module, read its `modules/<name>/README.md`.
- Before implementing, show a short plan (files, aggregates, value objects and tests) and wait for approval.
- `.env` is blocked from reading in `.claude/settings.json`. If you need a variable, ask.
- Design every screen on the canvas before building it. The canvas is "Lastro Screens" (https://claude.ai/artifact/L2vSWGh3A5zFWGLiD7yFUg), built with the "Lastro Cédula" design system (https://claude.ai/artifact/4oV6HQswX8nSzouPdvVYRT). If a screen, or the change to it, is not on the canvas yet, draw it there first and wait for approval of the design before writing frontend code. The approved artboard is the spec.
