# ADR-0012: A budget line can cover several categories

**Status:** Proposed (amends ADR-0006)
**Date:** 2026-10-28
**Deciders:** Felipe

## Context

ADR-0006 gives each category a limit of its own. The way I plan a month is coarser: what comes in, what I invest first, the bills, and one amount for whatever I feel like, be it going out or a part for the PC. With a limit per category, that amount has to be cut in advance between Leisure, Shopping and Dev, and a month that spends it all on Shopping shows Shopping over and Leisure untouched, when the month went exactly as planned.

Categories still matter: they say where the money went, rules and the classifier fill them in, and the reports and the transactions read them. What I want to share is the limit, not the category.

## Decision

- **A budget line covers one or more categories** with one amount for all of them: a limit, or a fund's monthly amount (ADR-0011). A line with one category is today's limit and goes by its category's name; a line with more has a name of its own ("Free to spend": Leisure, Shopping and Dev).
- **A category is in at most one line in a month,** so no real counts twice. A category in no line is spending without a limit, as today.
- **A line has an identity that lasts from month to month.** Rollover and a fund's balance belong to the line: adding Dev to "Free to spend" keeps what the line carried, and a category that leaves it takes nothing along.
- **The line is what is tracked.** Its spending is its categories' together, and so are its pace, its fixed and upcoming charges and its alerts. Its six months before are its current categories' spending, also in months before they were grouped, so the comparison is like with like; rollover walks each month as it was saved. The screen breaks a line's spending down by category.
- **The grouping is the budget's, not `categorize`'s.** It is saved per month and inherited like the rest of a budget (ADR-0006), so changing it never rewrites a past month.

## Options considered

- **Parent categories in `categorize`** ("Wants" holding Leisure and Shopping): reads well, but a grouping I may change next year would rewrite every past month, and it reaches rules, the classifier, the review queue, the filters and every screen that shows a category. Rejected: the grouping is a budgeting choice.
- **One line for everything that has no line of its own:** no list to keep, but it quietly takes groceries, health and every new category into the money to spend freely. Rejected for now: a line lists what it covers, and what is left out shows as spending without a limit, in plain sight.
- **A shared limit with limits per category inside it:** more control than I need. Not now.
- **Merging Leisure and Shopping into one category:** loses where the money went. Rejected.
- **Bills as a line made of the recurring charges, whatever their category:** matches "the bills" exactly, but a transaction would count where a heuristic puts it instead of where its category does, and the pace already leaves recurring charges out. Not now: a line of the categories that hold the bills does the job.

## Consequences

- `budget.lines` is keyed by a line id, and a line's categories get a table of their own with one row per month and category, so the database refuses a category in two lines. Today's lines become one-category lines whose id is their category's, so their alerts keep their keys.
- The month's view lists lines, each with its categories' spending; the endpoints that set limits take a line. `PUT /api/v1/budget/{month}/limits/{category}` keeps working for a one-category line.
- Copying a month copies its lines, leaving out archived categories and any line left without one.
- The monthly report and the MCP budget tool speak of lines.
- Funds (ADR-0011) are built on lines: a fund can cover several categories, and its balance and adjustments are the line's.
- The Budget screen changes, on the canvas first.
