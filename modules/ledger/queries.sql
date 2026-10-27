-- Write side: load and save aggregates. repository.go maps these rows.

-- name: GetAccountByExternalID :one
SELECT * FROM ledger.accounts WHERE external_id = $1;

-- name: ListAccounts :many
SELECT * FROM ledger.accounts ORDER BY institution, type, name, id;

-- name: SaveAccount :exec
INSERT INTO ledger.accounts (
    id, external_id, name, institution, type, number, currency, balance_cents, balance_at,
    credit_limit_cents, available_credit_cents, bill_due_date, bill_close_date
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    institution = EXCLUDED.institution,
    number = EXCLUDED.number,
    balance_cents = EXCLUDED.balance_cents,
    balance_at = EXCLUDED.balance_at,
    credit_limit_cents = EXCLUDED.credit_limit_cents,
    available_credit_cents = EXCLUDED.available_credit_cents,
    bill_due_date = EXCLUDED.bill_due_date,
    bill_close_date = EXCLUDED.bill_close_date,
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
-- Live transactions in a date range (or one, by id), newest first, with each
-- one's account and, for pairs, the other side and its account. flow is 'in'
-- or 'out' (money that counts, by direction) or 'transfer' (transfers and
-- card bill payments). search matches the description ignoring case and
-- accents. category is a category id, or 'none' for the expenses and income
-- still without one.
SELECT
    t.id, t.date, t.description, t.amount_cents, t.currency, t.kind, t.status,
    t.installment_number, t.installment_total, t.category_id, t.category_source, t.counterpart_id,
    t.note, t.tags,
    COALESCE((SELECT array_agg(s.category_id ORDER BY abs(s.amount_cents) DESC, s.position) FROM ledger.splits s WHERE s.transaction_id = t.id), '{}')::text[] AS split_categories,
    a.id AS account_id, a.name AS account_name, a.institution AS account_institution, a.type AS account_type,
    ca.id AS counterpart_account_id, ca.name AS counterpart_account_name,
    ca.institution AS counterpart_institution, ca.type AS counterpart_account_type
FROM ledger.transactions t
JOIN ledger.accounts a ON a.id = t.account_id
LEFT JOIN ledger.transactions c ON c.id = t.counterpart_id
LEFT JOIN ledger.accounts ca ON ca.id = c.account_id
WHERE t.removed_at IS NULL
  AND (sqlc.narg(id)::uuid IS NULL OR t.id = sqlc.narg(id)::uuid)
  AND (sqlc.narg(date_from)::date IS NULL OR t.date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR t.date < sqlc.narg(date_to)::date)
  AND (sqlc.narg(account_id)::uuid IS NULL OR t.account_id = sqlc.narg(account_id)::uuid)
  AND (
      sqlc.narg(category)::text IS NULL
      OR (sqlc.narg(category)::text = 'none' AND t.category_id IS NULL AND t.kind IN ('expense', 'income'))
      OR t.category_id = sqlc.narg(category)::text
      OR EXISTS (SELECT 1 FROM ledger.splits s WHERE s.transaction_id = t.id AND s.category_id = sqlc.narg(category)::text)
  )
  AND (sqlc.narg(tag)::text IS NULL OR sqlc.narg(tag)::text = ANY (t.tags))
  AND (
      sqlc.narg(flow)::text IS NULL
      OR (sqlc.narg(flow)::text = 'in' AND t.kind IN ('expense', 'income') AND t.amount_cents > 0)
      OR (sqlc.narg(flow)::text = 'out' AND t.kind IN ('expense', 'income') AND t.amount_cents < 0)
      OR (sqlc.narg(flow)::text = 'transfer' AND t.kind IN ('transfer', 'bill_payment'))
  )
  AND (
      sqlc.narg(search)::text IS NULL
      OR translate(lower(t.description || ' ' || t.note), 'áàâãäéèêëíìîïóòôõöúùûüç', 'aaaaaeeeeiiiiooooouuuuc')
         LIKE '%' || translate(lower(sqlc.narg(search)::text), 'áàâãäéèêëíìîïóòôõöúùûüç', 'aaaaaeeeeiiiiooooouuuuc') || '%' ESCAPE '\'
  )
ORDER BY t.date DESC, t.created_at DESC, t.id
LIMIT sqlc.arg(max_rows);

-- name: ListAccountViews :many
SELECT * FROM ledger.accounts
ORDER BY institution, CASE type WHEN 'checking' THEN 0 WHEN 'savings' THEN 1 ELSE 2 END, name, id;

-- name: GetAccountView :one
SELECT * FROM ledger.accounts WHERE id = $1;

-- name: ListDailyNet :many
-- What an account's live transactions add up to each day, up to a day.
SELECT date, sum(amount_cents)::bigint AS net_cents
FROM ledger.transactions
WHERE account_id = $1 AND removed_at IS NULL AND date <= sqlc.arg(until)::date
GROUP BY date
ORDER BY date;

-- name: ListMonthFlows :many
-- An account's live transactions by month, kind and direction, and for pairs
-- by the other side's account.
SELECT
    date_trunc('month', t.date)::date AS month, t.kind, t.amount_cents > 0 AS inflow,
    ca.id AS counterpart_account_id, ca.name AS counterpart_account_name,
    ca.institution AS counterpart_institution, ca.type AS counterpart_account_type,
    sum(t.amount_cents)::bigint AS cents, count(*)::int AS n
FROM ledger.transactions t
LEFT JOIN ledger.transactions c ON c.id = t.counterpart_id
LEFT JOIN ledger.accounts ca ON ca.id = c.account_id
WHERE t.account_id = $1 AND t.removed_at IS NULL AND t.amount_cents <> 0
GROUP BY 1, 2, 3, 4, 5, 6, 7
ORDER BY 1, 2, 3, 5;

-- name: ListInstallments :many
-- An account's live installments, for GroupInstallments.
SELECT id, description, installment_number, installment_total, amount_cents, currency, date, status
FROM ledger.transactions
WHERE account_id = $1 AND removed_at IS NULL AND installment_total IS NOT NULL
ORDER BY date, id;

-- name: ListDescriptionMonths :many
-- Each month's live transactions with one description (ignoring case), in
-- every account, that count: what the detail's history shows.
SELECT date_trunc('month', date)::date AS month, sum(amount_cents)::bigint AS cents, count(*)::int AS n
FROM ledger.transactions
WHERE removed_at IS NULL
  AND kind IN ('expense', 'income')
  AND lower(description) = lower(sqlc.arg(description))
  AND date >= sqlc.arg(date_from)::date AND date < sqlc.arg(date_to)::date
GROUP BY 1
ORDER BY 1;

-- name: ListDaySpending :many
-- Every account's live transactions by day: spending (outflows that count),
-- money in that counts, and money moved (transfers out and card bills paid).
SELECT
    date,
    COALESCE(sum(-amount_cents) FILTER (WHERE kind IN ('expense', 'income') AND amount_cents < 0), 0)::bigint AS spent_cents,
    count(*) FILTER (WHERE kind IN ('expense', 'income') AND amount_cents < 0)::int AS purchases,
    COALESCE(sum(amount_cents) FILTER (WHERE kind IN ('expense', 'income') AND amount_cents > 0), 0)::bigint AS in_cents,
    COALESCE(sum(-amount_cents) FILTER (WHERE kind IN ('transfer', 'bill_payment') AND amount_cents < 0), 0)::bigint AS moved_cents
FROM ledger.transactions
WHERE removed_at IS NULL AND date >= sqlc.arg(date_from)::date AND date < sqlc.arg(date_to)::date
GROUP BY date
ORDER BY date;

-- What categorize may categorize: live expenses and income (never transfers
-- or card bill payments) whose category nobody chose by hand.

-- name: ListCategorizable :many
SELECT id, account_id, description, date, amount_cents, category_id, category_source FROM ledger.transactions
WHERE id = ANY (sqlc.arg(ids)::uuid[])
  AND removed_at IS NULL
  AND kind IN ('expense', 'income')
  AND category_source IS DISTINCT FROM 'manual'
ORDER BY date, id;

-- name: ListUncategorized :many
SELECT id, account_id, description, date, amount_cents, category_id, category_source FROM ledger.transactions
WHERE category_id IS NULL
  AND removed_at IS NULL
  AND kind IN ('expense', 'income')
ORDER BY date, id;

-- name: ListAutomatable :many
-- Every live expense and income whose category nobody chose by hand: what
-- rules and the model may (re)categorize.
SELECT id, account_id, description, date, amount_cents, category_id, category_source FROM ledger.transactions
WHERE removed_at IS NULL
  AND kind IN ('expense', 'income')
  AND category_source IS DISTINCT FROM 'manual'
ORDER BY date, id;

-- name: ListDescriptionCounts :many
-- Each description of live expenses and income, by account: how often it
-- came and when last. What a rule would meet, without loading every row.
SELECT description, account_id, count(*)::int AS n, max(date)::date AS last_date
FROM ledger.transactions
WHERE removed_at IS NULL AND kind IN ('expense', 'income')
GROUP BY description, account_id;

-- name: ListCategoryTotals :many
-- Live expenses and income in a date range by category (null: none yet):
-- money out, money in and how many.
SELECT COALESCE(category_id, '')::text AS category_id,
    COALESCE(sum(-amount_cents) FILTER (WHERE amount_cents < 0), 0)::bigint AS out_cents,
    COALESCE(sum(amount_cents) FILTER (WHERE amount_cents > 0), 0)::bigint AS in_cents,
    count(*)::int AS n
FROM ledger.category_lines
WHERE removed_at IS NULL AND kind IN ('expense', 'income')
  AND currency = 'BRL'
  AND date >= sqlc.arg(date_from)::date AND date < sqlc.arg(date_to)::date
GROUP BY 1;

-- name: ListCategoryMonths :many
-- Live expenses and income by month, category (null: none yet) and kind:
-- money out, money in and how many. What budgets and averages are built on,
-- in reais only. A split counts each part in its category.
SELECT date_trunc('month', date)::date AS month, COALESCE(category_id, '')::text AS category_id, kind,
    COALESCE(sum(-amount_cents) FILTER (WHERE amount_cents < 0), 0)::bigint AS out_cents,
    COALESCE(sum(amount_cents) FILTER (WHERE amount_cents > 0), 0)::bigint AS in_cents,
    count(*)::int AS n
FROM ledger.category_lines
WHERE removed_at IS NULL AND kind IN ('expense', 'income')
  AND currency = 'BRL'
  AND date >= sqlc.arg(date_from)::date AND date < sqlc.arg(date_to)::date
GROUP BY 1, 2, 3
ORDER BY 1, 2, 3;

-- name: ListMonthKinds :many
-- Every live transaction by month, kind, direction and whether it is paired
-- with one in another of my accounts: the month's income, and transfers out
-- to accounts outside lastro (the broker).
SELECT date_trunc('month', date)::date AS month, kind, amount_cents > 0 AS inflow,
    (counterpart_id IS NOT NULL)::boolean AS paired,
    sum(amount_cents)::bigint AS cents, count(*)::int AS n, max(date)::date AS last
FROM ledger.transactions
WHERE removed_at IS NULL AND amount_cents <> 0
  AND currency = 'BRL'
  AND date >= sqlc.arg(date_from)::date AND date < sqlc.arg(date_to)::date
GROUP BY 1, 2, 3, 4
ORDER BY 1, 2, 3, 4;

-- name: ListCharges :many
-- Live expenses since a day, outside installments: what recurring charges
-- are found in.
SELECT t.id, t.description, t.date, t.amount_cents, t.category_id, a.name AS account_name, a.institution
FROM ledger.transactions t
JOIN ledger.accounts a ON a.id = t.account_id
WHERE t.removed_at IS NULL AND t.kind = 'expense' AND t.amount_cents < 0
  AND t.currency = 'BRL'
  AND t.installment_total IS NULL
  AND t.date >= sqlc.arg(date_from)::date
ORDER BY t.date, t.id;

-- name: SetNote :execrows
UPDATE ledger.transactions SET note = sqlc.arg(note), updated_at = now()
WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: SetTags :execrows
UPDATE ledger.transactions SET tags = sqlc.arg(tags)::text[], updated_at = now()
WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: ListSplit :many
SELECT category_id, amount_cents FROM ledger.splits
WHERE transaction_id = sqlc.arg(transaction_id)
ORDER BY position;

-- name: DeleteSplit :exec
DELETE FROM ledger.splits WHERE transaction_id = sqlc.arg(transaction_id);

-- name: InsertSplitPart :exec
INSERT INTO ledger.splits (transaction_id, position, category_id, amount_cents)
VALUES (sqlc.arg(transaction_id), sqlc.arg(position), sqlc.arg(category_id), sqlc.arg(amount_cents));

-- name: ListTags :many
-- Every tag in use, with how much money it holds: what the tag picker
-- offers and the tag filter sums.
SELECT tag::text AS tag, count(*)::int AS n,
    COALESCE(sum(-t.amount_cents) FILTER (WHERE t.kind IN ('expense', 'income') AND t.amount_cents < 0), 0)::bigint AS out_cents,
    COALESCE(sum(t.amount_cents) FILTER (WHERE t.kind IN ('expense', 'income') AND t.amount_cents > 0), 0)::bigint AS in_cents,
    max(t.date)::date AS last
FROM ledger.transactions t, unnest(t.tags) AS tag
WHERE t.removed_at IS NULL
GROUP BY tag
ORDER BY max(t.date) DESC, tag;

-- name: ListTagCategories :many
-- One tag's money by category, every month, a split counted by its parts.
SELECT COALESCE(category_id, '')::text AS category_id,
    COALESCE(sum(-amount_cents) FILTER (WHERE amount_cents < 0), 0)::bigint AS out_cents,
    COALESCE(sum(amount_cents) FILTER (WHERE amount_cents > 0), 0)::bigint AS in_cents,
    count(*)::int AS n
FROM ledger.category_lines
WHERE removed_at IS NULL AND kind IN ('expense', 'income') AND sqlc.arg(tag)::text = ANY (tags)
GROUP BY 1
ORDER BY 2 DESC;

-- name: DeleteBalanceSnapshot :execrows
DELETE FROM ledger.balance_snapshots WHERE account_id = $1 AND date = $2;
