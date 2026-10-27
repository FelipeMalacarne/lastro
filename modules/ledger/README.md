# ledger

**Responsibility:** accounts and transactions, and the classification of each movement. It is the source of truth about what happened to the money.

**Not its responsibility:** talking to Pluggy (that is `ingest`) or choosing categories (that is `categorize`). The `ledger` stores the category it receives and where it came from.

## Concepts

- `Account` (aggregate, `account.go`): checking account, savings account or credit card. Its balance is from my point of view: negative on a card (the amount owed on the open bill). Only the last four digits of its number are kept. A card also keeps `CardDetails`: its limit and what is still available, and the bill's due and closing days, each one only when the bank reports it.
- `Transaction` (aggregate, `transaction.go`): one movement in an account. The amount is negative when money leaves the account, on checking and on cards alike. Its kind is the source's until the transaction is paired; while paired, the pair's.
- `BalanceSnapshot` (in `account.go`): an account's balance on a day, the last one known that day. Every `SyncAccount` records one, so the net worth history (M4) starts with the first sync.
- `Pair` (in `transaction.go`): links the two sides of a movement between my own accounts. Into a card it makes both sides a card bill payment; between bank accounts, a transfer.
- Value objects (`values.go`): `ExternalID` (the source's id, today Pluggy's), `Kind` (expense, income, transfer, card bill payment; only the first two count in totals), `Installment` ("3 of 10"), `CategoryID` (opaque, owned by `categorize`), `Source` (rule, LLM or manual), `Status` (pending or posted), `AccountType` (checking, savings, credit card) and `CardDetails`. Constructors return an error wrapping `ErrInvalid` instead of an invalid value.
- `BalanceHistory` (`history.go`): rebuilds an account's balance at the end of each day, walking back from the bank's latest balance through each day's net. Days after that balance are left out: the bank does not count them yet.
- `GroupInstallments` (`installments.go`): the source has no purchase id, so a `Purchase` is the installments with the same description (without the "3/12" Nubank appends, ignoring case), the same count and the same amount, within a cent per installment (the rounding lands on the first one). A number appears once per purchase. A purchase knows its current installment, what remains and its last day, a month apart per installment the source has not sent.
- `TransferMatcher` (`MatchTransfers` in `transfer.go`, a domain service): among unpaired transactions, pairs an outflow from one of my bank accounts with an inflow of the same amount in another account. Into a card, the inflow must be what the card calls a bill payment, up to 3 days apart; between bank accounts, an expense with an income, up to 1 day apart. Closer dates pair first, and an outflow or inflow with two equally good candidates in different accounts is left alone rather than guessed. `ingest` runs it after every sync.

## Invariants

- A transaction is unique per account and `ExternalID`. Importing again never duplicates: it is an upsert. When the source recreates a transaction with a new `ExternalID` but the same bank reference, the existing one is re-keyed, keeping its category.
- An account's type and currency never change, and neither does a transaction's currency: it is always the account's.
- Card bill payments and transfers between my own accounts never count as expense or income.
- A pair joins an outflow and an inflow of the same amount in two different accounts, never out of a card. If one side is removed or its amount changes, the pair breaks on both sides and each gets its source's kind back.
- A manual category beats an automatic one. Rules and the LLM never overwrite a manual choice (`ErrManualCategory`).
- Pending transactions may change or disappear on the next sync. A transaction the source stops returning is removed (kept, but out of every list and total); if it comes back, it is restored. Removed transactions cannot be categorized or paired. Only posted transactions go into closed reports.
- Only a credit card has card details, in the account's currency, with a limit that is never negative (what is available can be, over the limit). They travel with the balance: an older read replaces neither.
- A balance older than the one already known never replaces it, in the account or in its day's snapshot. There is at most one snapshot per account per day.

## Read side

`views.go` holds the DTOs the screens read, straight from SQL (CQRS-lite), and `http.go` serves them:

- `GET /api/v1/accounts`: every account with its latest balance and, for a card, `card` (limit, available, due and closing days, each null when unknown).
- `GET /api/v1/accounts/{id}`: one account, the same shape.
- `GET /api/v1/accounts/{id}/balance`: the balance at the end of each day (`BalanceHistory`).
- `GET /api/v1/accounts/{id}/months`: each month in and out, what counts (spending, refunds), and its flows by kind, direction and, for pairs, the other side's account. Months after today's are `future`: on a card, the installments already committed.
- `GET /api/v1/accounts/{id}/installments`: the purchases with installments still to come, the one ending first first.
- `GET /api/v1/transactions?month=2026-10&day=2026-10-22&account=<id>&flow=in|out|transfer&q=<text>`: a month (or one of its days) of live transactions, newest first, each with its account and, for pairs, the other side and its account, plus a summary (count, pending, money in and out that counts). The search ignores case and accents.
- `GET /api/v1/transactions/{id}`: one transaction, the other side of its pair, its purchase's installments and how often its description came up in the last six months.
- `GET /api/v1/transactions/days?month=2026-10`: spending, money in and money moved day by day, every account together, with the month before and what it had spent by the same day.

For `categorize`, the Service answers what automation may categorize: live expenses and income whose category nobody chose by hand. `Categorizable(ids)` filters the given transactions; `Uncategorized()` returns every one still without a category. Transfers and card bill payments are never handed over.

## Events

- Publishes: `TransactionsImported` (ids of new or changed transactions, after the import commits), `TransactionCategorized` (recorded by the aggregate, published after the commit).
- Consumes: none. It receives commands from `ingest` and `categorize` through its Service.

## Milestone

M1
