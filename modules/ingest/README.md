# ingest

**Responsibility:** fetch data from outside and translate it into commands for the other modules. Today there are two sources: the Pluggy API (through Meu Pluggy) and the trade statement from B3's investor portal (Área do Investidor).

**Not its responsibility:** business rules about transactions or positions. `ingest` only translates and delivers.

## Concepts

- Pluggy client in `internal/pluggy/`. No other module sees Pluggy types.
- Sync state per item: when the last sync ran and how far it got.
- Raw payloads stored in the `ingest` schema, so data can be reprocessed without calling the API again.

## Mapping (`mapping.go`)

Pure functions from Pluggy records to ledger commands; `docs/pluggy.md` explains each rule.

- Accounts: `BANK`/`CHECKING_ACCOUNT` → checking, `BANK`/`SAVINGS_ACCOUNT` → savings, `CREDIT` → credit card, whose balance is negated (Pluggy reports what I owe). Anything else is skipped.
- Amounts: direction from `type` (`DEBIT` out, `CREDIT` in), magnitude from the literal, in the account's currency (`amountInAccountCurrency` for purchases abroad).
- Dates: midnight UTC is a calendar day; any other time is read in America/São Paulo.
- Kinds: on a card, an inflow with `PAGAMENTO_FATURA` (or "pagamento recebido/de fatura") is a bill payment and everything else an expense (refunds reduce spending); on a bank account, a redemption or automatic investment move is a transfer, other inflows income and outflows expenses. Transfers between my accounts are paired later by the ledger's `TransferMatcher`.
- `providerId` becomes the ledger's bank reference.

## Invariants

- Reprocessing the same payload produces exactly the same result in `ledger`.
- The Meu Pluggy API does not list items: item IDs come from configuration (`LASTRO_PLUGGY_ITEM_IDS`).
- Private pension plans (PGBL/VGBL) do not come through Open Finance. If they are ever added, it will be by manual import.
- On the free plan, only accounts under your own CPF, up to 5 connections.

## Tests

Fixtures in `testdata/pluggy/` hold Pluggy API responses. Today they are synthetic; `lastro fixtures` downloads the real ones, prints a report safe to share and writes an anonymized copy (names, documents, account numbers, ids and free-text words replaced, amounts scaled) to review before committing. Never commit the raw dump. What Pluggy sends and why ingest reads it the way it does is in `docs/pluggy.md`.

## Events

- Publishes: `SyncCompleted`.
- Consumes: none.

## Milestone

M1
