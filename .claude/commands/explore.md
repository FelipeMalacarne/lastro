---
description: Explore the running app for bugs, UX problems and feature ideas, and report them without fixing anything
argument-hint: "[screen or flow, optional]"
---

Explore the app in this worktree like a demanding user, and report what you find. Scope: "$ARGUMENTS" (empty means every screen). Read `AGENTS.md` ("Working in a worktree") first.

Only this worktree's app, with its seed data. Never production, and never the main checkout, whose database may hold my real money. Screenshots and the report never carry real financial data.

1. **Start.** `task urls` must not show :8080 or :5173 (those mean the main checkout). If `orca ports --json` does not list both, start the app in its own Orca terminal (`orca terminal create --worktree active --title app --command "nix develop -c task up" --json`) and wait for `/healthz`.
2. **Context.** Read `FEATURES.md`, `docs/PLAN.md`, the routes in `page()` in `web/src/App.tsx`, and the screen's artboards on the canvas (link in `CLAUDE.md`). They say what each screen is for and how it should look.
3. **Record.** Open a tab on the frontend URL (`orca tab create --url <url> --json`) and run `orca capture start --json`, so `orca console` and `orca network` see everything from then on.
4. **Each screen in scope:**
   - Load it, `orca wait --load networkidle --json`, `orca snapshot --json`, and a screenshot. Look at the screenshot: layout, alignment, truncation, contrast, what draws the eye first.
   - Try every control: buttons, menus, dialogs, filters, month navigation, links between screens, the `/` search, the keyboard (Tab order, Enter, Esc, focus after a dialog closes).
   - Forms with awkward input: empty, zero, negative, R$ 999.999.999,99, three decimals, a 200-character name, accents and emoji, a double submit. Check the result on screen and through the API (`curl` the API URL; auth is off in dev).
   - Repeat at phone width (`orca viewport --width 390 --height 844 --scale 3 --mobile --json`; navigation resets it) and in the other theme.
   - After each step, `orca console --json` and `orca network --json`: every error, warning, failed request or 4xx/5xx is a lead.
5. **Across screens.** The same number agrees wherever it shows (dashboard, budget, transactions, account, report). Money and percentages read the Brazilian way (`R$ 1.234,56`, `12,5%`); copy is in English. Loading, empty and error states exist (without Pluggy credentials, "Sync now" fails: see how that reads).
6. **Confirm.** Reproduce every bug a second time before reporting it. A finding you could not reproduce goes under "Not sure".
7. **Report** in `tmp/explore/<YYYY-MM-DD>/report.md`, with the screenshots next to it:
   - **Bugs:** steps, expected, actual, screenshot, and how bad (wrong money, broken flow, cosmetic).
   - **UX:** what gets in the way, for whom and when, and a concrete change. Point to the artboard when the screen departs from it.
   - **Feature ideas:** what a screen is missing for what I do with it, grounded in `FEATURES.md` and what you saw. Mark the ones `docs/PLAN.md` already has.
   - **Not sure** and **Not covered** (sync against Pluggy, real data shapes, an empty database).
8. Fix nothing, open no issues and leave `docs/PLAN.md` alone. Give me a short summary, most important first, and ask what to do with each finding: fix now, a box in `docs/PLAN.md`, an issue, or drop it.
