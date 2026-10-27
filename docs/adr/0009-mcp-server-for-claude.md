# ADR-0009: An MCP server, and Claude reading my financial data

**Status:** Accepted
**Date:** 2026-10-27
**Deciders:** Felipe

## Context

I want to ask Claude about my money: "how much did I spend on food this quarter?", "where should this month's contribution go?". The way to do it is an MCP server in lastro that Claude connects to.

`AGENTS.md` says the LLM is local (Ollama) and that no calls to external LLMs carry financial data. An MCP server breaks that letter: whatever a tool returns goes to Anthropic.

## Decision

- **I accept it, for MCP only.** The rule changes to:
  - lastro itself never sends financial data to an external LLM. Categorization and every automatic process stay on Ollama.
  - I may pull my data into an LLM client of my choice through MCP. Those requests are always started by me, from a client I authorized.
- **Read-only first.**
  - The Claude client gets the `read` permission (ADR-0008).
  - Write tools (recategorize, add a note) come later. Each is a separate tool that asks for `write`.
- **Tools answer with what the question needs.**
  - Prefer summaries: totals by category, the budget's state, the portfolio's allocation.
  - Transactions are listed only with filters (period, category, account).
  - Account numbers, Pluggy IDs and anything secret never leave the server.
- **Transport:** streamable HTTP at `/mcp`, stateless, with the official Go SDK (`github.com/modelcontextprotocol/go-sdk`), in `app/mcp.go`, since the tools read across modules through their `Service`s.
- **Tools:** `monthly_report`, `budget`, `transactions` (by month, with search, category, account, flow or tag), `accounts` (without numbers), `categories`, `portfolio` and `contribution_plan`. Errors from inside lastro reach the model as a plain message; the details stay in the log.
- **Revocable:** removing the `lastro-claude` application in Zitadel cuts Claude off without touching anything else.

## Options considered

- **No MCP:** keeps the rule intact and loses the use case. Rejected.
- **Local MCP only** (stdio on the laptop, talking to the API with the CLI's token): the data still goes to the model. It only changes which client I use, so it is not safer in a meaningful way. Possible later as a mode of `lastro connect`.
- **Remote MCP with OAuth, read-only:** works from Claude on the phone, the web and the desktop. Chosen.

## Consequences

- Anthropic sees what the tools return during a conversation. This is the same trade-off ADR-0007 made for Telegram alerts, with more data, and it is why the tools are read-only and lean.
- Once accepted, `AGENTS.md`'s security section must state the new rule, not the old one.
