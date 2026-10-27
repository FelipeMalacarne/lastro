-- Write side: load and save aggregates. repository.go maps these rows.

-- name: GetAccountByExternalID :one
SELECT * FROM ledger.accounts WHERE external_id = $1;

-- name: ListAccounts :many
SELECT * FROM ledger.accounts ORDER BY institution, type, name, id;

-- name: SaveAccount :exec
INSERT INTO ledger.accounts (id, external_id, name, institution, type, number, currency, balance_cents, balance_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    institution = EXCLUDED.institution,
    number = EXCLUDED.number,
    balance_cents = EXCLUDED.balance_cents,
    balance_at = EXCLUDED.balance_at,
    updated_at = now();

-- name: ListTransactionsForImport :many
-- Everything an import of one account may touch: the transactions dated in
-- the window, plus any (outside it or removed) with one of the batch's
-- external ids or bank references.
SELECT sqlc.embed(t), a.type AS account_type
FROM ledger.transactions t
JOIN ledger.accounts a ON a.id = t.account_id
WHERE t.account_id = sqlc.arg(account_id)
  AND (
      (sqlc.narg(window_from)::date IS NOT NULL AND t.date >= sqlc.narg(window_from)::date)
      OR t.external_id = ANY (sqlc.arg(external_ids)::text[])
      OR (t.bank_ref <> '' AND t.bank_ref = ANY (sqlc.arg(bank_refs)::text[]))
  );

-- name: GetTransactions :many
SELECT sqlc.embed(t), a.type AS account_type
FROM ledger.transactions t
JOIN ledger.accounts a ON a.id = t.account_id
WHERE t.id = ANY (sqlc.arg(ids)::uuid[]);

-- name: SaveTransaction :batchexec
INSERT INTO ledger.transactions (
    id, account_id, external_id, bank_ref, date, description, amount_cents, currency, source_kind, status,
    installment_number, installment_total, category_id, category_source, counterpart_id, pair_kind, removed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (id) DO UPDATE SET
    external_id = EXCLUDED.external_id,
    bank_ref = EXCLUDED.bank_ref,
    date = EXCLUDED.date,
    description = EXCLUDED.description,
    amount_cents = EXCLUDED.amount_cents,
    source_kind = EXCLUDED.source_kind,
    status = EXCLUDED.status,
    installment_number = EXCLUDED.installment_number,
    installment_total = EXCLUDED.installment_total,
    category_id = EXCLUDED.category_id,
    category_source = EXCLUDED.category_source,
    counterpart_id = EXCLUDED.counterpart_id,
    pair_kind = EXCLUDED.pair_kind,
    removed_at = EXCLUDED.removed_at,
    updated_at = now();

-- name: SaveBalanceSnapshot :exec
-- The latest balance of the day wins; an older read never replaces a newer one.
INSERT INTO ledger.balance_snapshots (account_id, date, balance_cents, currency, taken_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (account_id, date) DO UPDATE SET
    balance_cents = EXCLUDED.balance_cents,
    taken_at = EXCLUDED.taken_at
WHERE EXCLUDED.taken_at >= ledger.balance_snapshots.taken_at;

-- name: ListBalanceSnapshots :many
SELECT * FROM ledger.balance_snapshots WHERE account_id = $1 ORDER BY date;
