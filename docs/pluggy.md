# Pluggy: what the API gives us and how lastro reads it

The M1 spike note. It answers the open questions from `FEATURES.md` with what Pluggy documents (docs.pluggy.ai, read on 2026-10-27), and marks what my own connections still have to confirm. To confirm it, run `lastro fixtures` (see `modules/ingest/testdata/pluggy/README.md`) and update the "Confirmed" column from its report.

## The path: Meu Pluggy

- Each bank connected in Meu Pluggy becomes one **item** in my Pluggy application, through the MeuPluggy connector. These are proxy items: they reflect every refresh of the Meu Pluggy connection (every 24 hours), cannot be updated through the API and are not auto-synced on their own. lastro only reads.
- `GET /v2/items` is not available for Meu Pluggy accounts, so the item ids come from `LASTRO_PLUGGY_ITEM_IDS`.
- The authorization I give my application when signing in to Meu Pluggy **lasts one year** (`consentExpiresAt` on the item). After it lapses, reads come back **empty** while the item may still say `UPDATED`; the next update fails with `USER_AUTHORIZATION_EXPIRED`. Renewing it means connecting MeuPluggy again in the Dashboard, and the item id may change.
- Free plan: personal use, accounts under my own CPF, up to 5 connections.

## Reading

| What | Endpoint | Notes |
|---|---|---|
| API key | `POST /auth` | Lives 2 hours. The client reuses it for 100 minutes and renews it once on a 401. |
| Item status | `GET /items/{id}` | `status`, `executionStatus`, `lastUpdatedAt` (when Pluggy last refreshed it), `consentExpiresAt`, `error.code`. |
| Accounts | `GET /accounts?itemId=` | `BANK` (checking, savings) and `CREDIT` (credit card). |
| Transactions | `GET /v2/transactions?accountId=&dateFrom=` | Cursor pages of up to 500; `next` is a ready-made query string. The page-based `GET /transactions` is deprecated on 2026-12-31. |
| Investments | `GET /investments?itemId=&page=` | Page numbers. |

Rate limits are per IP and per endpoint: 360 requests a minute for each read, which a daily sync of five items never approaches. A 429 carries `RateLimit-Reset`; the client waits it out up to three times.

The limits that matter are the Open Finance ones, on Pluggy's side of the connection (per CPF, per bank, per month: 4 calls for 12-month history, 240 for the last 7 days, 30 for card bills). They are spent by Meu Pluggy's daily refresh, not by lastro: lastro reads what Pluggy already stored, as often as it likes.

## Answers to the open questions

| Question | What the docs say | Confirmed with my data |
|---|---|---|
| How much history does the first sync return? | Up to 12 months (365 days) on the first collection; later refreshes merge the recent days (the last 7 for cards, up to 4 days before the last update otherwise). | Pending: the report prints the first and last date per account. |
| How do card bill payments arrive? | On the card, as a `CREDIT` with a negative amount and `operationType` `PAGAMENTO_FATURA`. When the bank does not report it, Pluggy builds it from the bill data as "Pagamento recebido", also `PAGAMENTO_FATURA`. On checking, the payment is an ordinary outflow (no operation type says "card bill"). | Pending: the report lists the inflow descriptions of each card and the checking outflows that mention "fatura" or "cartão". |
| How do installments arrive? | One transaction per installment with `creditCardMetadata.installmentNumber` and `totalInstallments`. Banks differ: some post every installment at once (later ones dated on their bill), most post one each month. There is no id that groups the installments of one purchase, and `totalAmount` is not sent on Open Finance. | Pending: the report counts installments and the totals seen per card. |
| How do pending transactions behave? | `PENDING` is a card purchase on an open bill, a future installment, or a scheduled or blocked checking movement (scheduled ones are dated in the future). They usually become `POSTED` keeping their id. | Pending: the report counts pending and future-dated transactions. |
| Are ids stable? | No. Pluggy keeps the id when its match key (the bank's id, or description + amount + date) still matches. When description, amount or date change, typically around a card bill closing, the transaction is deleted and created again with a new id, and nothing links the two. `providerId` (the bank's own id, Open Finance only) is the stable anchor. A transaction can also vanish for a few days and come back with a new id. | Pending: the report says how many transactions carry `providerId`. |
| Which investments come through? | Whatever the bank shares over Open Finance: CDBs (including Nubank's Caixinhas and PicPay's Cofrinhos), LCI/LCA, Tesouro Direto, funds, stocks, BDRs, FIIs and ETFs, with `type` and `subtype`. On Open Finance `amountProfit` and the performance rates are null. PGBL and VGBL do not come through. | Pending: the report counts investments by type, subtype and status per item. This decides whether M4 needs the B3 trade statement import. |
| What do dates mean? | ISO 8601 in UTC; to read a time as Brazilian time, convert to GMT−3. | Pending: the report counts dates at midnight UTC versus dates with a time of day. |

## Decisions this leads to

1. **Ingest reads a window, not a delta.** Each sync reads every transaction of each account dated on or after a start date: the whole history on an item's first sync (or with `lastro sync --full`), and the last 90 days afterwards. Reading the window again is cheap and catches pending → posted changes, restatements and late installments, which webhooks would otherwise be needed for.
2. **The window is the truth for its dates.** A transaction lastro has inside the window that Pluggy no longer returns is removed (soft-deleted), so vanished pending transactions and the old half of a delete-and-recreate disappear. An empty read never removes anything: that is what a lapsed authorization looks like.
3. **Transactions are keyed by Pluggy id, and re-keyed by the bank's id.** A new Pluggy id whose `providerId` matches a transaction lastro already has (live or removed) updates that transaction instead of creating one, so its category survives the delete-and-recreate.
4. **Direction comes from `type`, not from the sign.** `DEBIT` is money out and `CREDIT` money in, on cards too (Pluggy normalizes card purchases to `DEBIT`). In the ledger an amount is negative when money leaves the account, for checking and for cards alike.
5. **Amounts are read as decimals, never as floats.** The client keeps the JSON literal and ingest converts it to cents, rounding half away from zero. Purchases in another currency use `amountInAccountCurrency`, so every ledger amount is in the account's currency.
6. **A date at midnight UTC is a plain date**; any other time is converted to America/São Paulo before taking the day. This is the assumption the report has to confirm.
7. **Card bill payments**: the card side is recognized from `PAGAMENTO_FATURA` (or a "pagamento recebido/de fatura" description) and the checking side by the `TransferMatcher` pairing it with that card inflow. **Investment redemptions** (`RESGATE_APLIC_FINANCEIRA`) are transfers; yields (`RENDIMENTO_APLIC_FINANCEIRA`) are income.
8. **Nothing that identifies a person is decoded.** The client reads ids, amounts, dates, descriptions and enums; owner, CPF and `paymentData` stay only in the raw payload stored in the `ingest` schema.
9. **Freshness comes from Pluggy.** "Last synced" on screen is `lastUpdatedAt` of the item (when Meu Pluggy last refreshed the bank), not when lastro last read it. An item older than 2 days, in `LOGIN_ERROR`, with an `error`, or past `consentExpiresAt` gets the stale-sync notice.
