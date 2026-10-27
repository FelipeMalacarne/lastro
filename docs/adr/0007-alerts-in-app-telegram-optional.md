# ADR-0007: Alerts in the app, with Telegram as an optional push

**Status:** Accepted
**Date:** 2026-10-27
**Deciders:** Felipe

## Context

F10 asks to be told when something is unusual: a category over its limit or about to be, spending far above the usual, a subscription that changed price or appeared. `PLAN.md` left the channel open between email and Telegram. lastro runs at home behind Cloudflare Access, with no mail server, and is read mostly on the phone.

## Decision

- **Alerts are computed, not stored**, by `budget` from what it already knows: the month's limits and pace, each category's six-month average and the recurring charges. `GET /api/v1/alerts` lists them, and the Dashboard shows them first. This works with no channel configured.
- **Telegram is the push channel, and it is opt-in.** With `LASTRO_TELEGRAM_BOT_TOKEN` and `LASTRO_TELEGRAM_CHAT_ID` set, `lastro sync` sends each new alert once, in one message, after the sync. Without them nothing leaves the cluster.
- A sent alert is remembered by a key (kind, month, subject), so the four daily syncs never repeat it.
- Messages carry what an alert needs (a category name and an amount) and nothing else: no descriptions, accounts or balances.

## Options considered

- **Email:** needs an SMTP relay or a provider account and lands among everything else. Rejected for now.
- **Telegram:** one HTTPS call with a bot token, read on the phone where the alert matters. Chosen, as an option.
- **Web push:** needs a service worker and keys, and Cloudflare Access in front makes the subscription awkward. Possible later.

## Consequences

- Telegram's servers see the alert text. That is a third party holding a little financial data, which is why the channel is off until I set it, and why messages stay minimal. The local-LLM rule (no financial data to external LLMs) is unaffected.
- A different channel later only needs another sender behind the same "send new alerts" step.
