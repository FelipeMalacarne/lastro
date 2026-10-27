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

-- name: ListPairCandidates :many
-- What the TransferMatcher may pair: unpaired, live, non-zero transactions
-- whose source kind could be one side of a transfer or a bill payment.
SELECT sqlc.embed(t), a.type AS account_type
FROM ledger.transactions t
JOIN ledger.accounts a ON a.id = t.account_id
WHERE t.removed_at IS NULL
  AND t.counterpart_id IS NULL
  AND t.amount_cents <> 0
  AND t.source_kind IN ('expense', 'income', 'bill_payment')
  AND (sqlc.narg(window_from)::date IS NULL OR t.date >= sqlc.narg(window_from)::date)
ORDER BY t.date, t.id;

-- Read side: what the Transactions screen lists (CQRS-lite: straight to a DTO).

-- name: ListTransactionViews :many
-- A month of live transactions, newest first, with each one's account and,
-- for pairs, the other side's account. flow is 'in' or 'out' (money that
-- counts, by direction) or 'transfer' (transfers and card bill payments).
-- search matches the description ignoring case and accents.
SELECT
    t.id, t.date, t.description, t.amount_cents, t.currency, t.kind, t.status,
    t.installment_number, t.installment_total, t.category_id, t.category_source,
    a.id AS account_id, a.name AS account_name, a.institution AS account_institution, a.type AS account_type,
    ca.id AS counterpart_account_id, ca.name AS counterpart_account_name,
    ca.institution AS counterpart_institution, ca.type AS counterpart_account_type
FROM ledger.transactions t
JOIN ledger.accounts a ON a.id = t.account_id
LEFT JOIN ledger.transactions c ON c.id = t.counterpart_id
LEFT JOIN ledger.accounts ca ON ca.id = c.account_id
WHERE t.removed_at IS NULL
  AND t.date >= sqlc.arg(date_from)::date
  AND t.date < sqlc.arg(date_to)::date
  AND (sqlc.narg(account_id)::uuid IS NULL OR t.account_id = sqlc.narg(account_id)::uuid)
  AND (
      sqlc.narg(flow)::text IS NULL
      OR (sqlc.narg(flow)::text = 'in' AND t.kind IN ('expense', 'income') AND t.amount_cents > 0)
      OR (sqlc.narg(flow)::text = 'out' AND t.kind IN ('expense', 'income') AND t.amount_cents < 0)
      OR (sqlc.narg(flow)::text = 'transfer' AND t.kind IN ('transfer', 'bill_payment'))
  )
  AND (
      sqlc.narg(search)::text IS NULL
      OR translate(lower(t.description), 'áàâãäéèêëíìîïóòôõöúùûüç', 'aaaaaeeeeiiiiooooouuuuc')
         LIKE '%' || translate(lower(sqlc.narg(search)::text), 'áàâãäéèêëíìîïóòôõöúùûüç', 'aaaaaeeeeiiiiooooouuuuc') || '%' ESCAPE '\'
  )
ORDER BY t.date DESC, t.created_at DESC, t.id
LIMIT sqlc.arg(max_rows);

-- name: ListAccountViews :many
SELECT id, name, institution, type, number, currency, balance_cents, balance_at FROM ledger.accounts
ORDER BY institution, CASE type WHEN 'checking' THEN 0 WHEN 'savings' THEN 1 ELSE 2 END, name, id;

-- What categorize may categorize: live expenses and income (never transfers
-- or card bill payments) whose category nobody chose by hand.

-- name: ListCategorizable :many
SELECT id, account_id, description FROM ledger.transactions
WHERE id = ANY (sqlc.arg(ids)::uuid[])
  AND removed_at IS NULL
  AND kind IN ('expense', 'income')
  AND category_source IS DISTINCT FROM 'manual'
ORDER BY date, id;

-- name: ListUncategorized :many
SELECT id, account_id, description FROM ledger.transactions
WHERE category_id IS NULL
  AND removed_at IS NULL
  AND kind IN ('expense', 'income')
ORDER BY date, id;
