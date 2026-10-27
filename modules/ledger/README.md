# ledger

**Responsibility:** accounts and transactions, and the classification of each movement. It is the source of truth about what happened to the money.

**Not its responsibility:** talking to Pluggy (that is `ingest`) or choosing categories (that is `categorize`). The `ledger` stores the category it receives and where it came from.

## Concepts

- `Account` (aggregate): checking account, credit card, investment account or savings pot.
- `Transaction` (aggregate): one movement in an account.
- Value objects (`values.go`): `ExternalID` (the source's id, today Pluggy's), `Kind` (expense, income, transfer, card bill payment; only the first two count in totals), `Installment` ("3 of 10"), `CategoryID` (opaque, owned by `categorize`), `Source` (rule, LLM or manual), `Status` (pending or posted) and `AccountType` (checking, savings, credit card). Constructors return an error wrapping `ErrInvalid` instead of an invalid value.
- `TransferMatcher` (domain service): pairs an outflow in one of your accounts with the matching inflow in another.

## Invariants

- A transaction is unique per account and `ExternalID`. Importing again never duplicates: it is an upsert.
- Card bill payments and transfers between your own accounts never count as expense or income.
- A transaction's amount never changes currency.
- A manual category beats an automatic one. Rules and the LLM never overwrite a manual choice.
- Pending transactions may change or disappear on the next sync. Only posted transactions go into closed reports.

## Events

- Publishes: `TransactionsImported` (new or changed IDs), `TransactionCategorized`.
- Consumes: none. It receives commands from `ingest` and `categorize` through its Service.

## Milestone

M1
