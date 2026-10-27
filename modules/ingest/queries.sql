-- name: EnsureItem :exec
INSERT INTO ingest.items (id) VALUES ($1) ON CONFLICT (id) DO NOTHING;

-- name: GetItem :one
SELECT * FROM ingest.items WHERE id = $1;

-- name: ListItems :many
-- The configured items and any other that feeds a ledger account, so a
-- process started without the item ids still shows every account's connection.
SELECT * FROM ingest.items i
WHERE i.id = ANY (sqlc.arg(ids)::text[])
   OR EXISTS (SELECT 1 FROM ingest.item_accounts a WHERE a.item_id = i.id)
ORDER BY i.institution, i.id;

-- name: SyncStarted :exec
UPDATE ingest.items SET sync_started_at = $2, updated_at = now() WHERE id = $1;

-- name: SaveItemState :exec
UPDATE ingest.items SET
    institution = $2,
    status = $3,
    execution_status = $4,
    error_code = $5,
    data_updated_at = $6,
    consent_expires_at = $7,
    updated_at = now()
WHERE id = $1;

-- name: SyncSucceeded :exec
UPDATE ingest.items SET sync_finished_at = $2, synced_at = $2, sync_error = '', updated_at = now() WHERE id = $1;

-- name: SyncFailed :exec
UPDATE ingest.items SET sync_finished_at = $2, sync_error = $3, updated_at = now() WHERE id = $1;

-- name: LinkAccount :exec
INSERT INTO ingest.item_accounts (item_id, external_id, ledger_account_id) VALUES ($1, $2, $3)
ON CONFLICT (item_id, external_id) DO UPDATE SET ledger_account_id = EXCLUDED.ledger_account_id;

-- name: ListItemAccounts :many
SELECT * FROM ingest.item_accounts WHERE item_id = ANY (sqlc.arg(item_ids)::text[]) ORDER BY item_id, external_id;

-- name: SaveRawRecord :batchexec
-- Rewrites a record only when its payload changed or it had been gone.
INSERT INTO ingest.raw_records (kind, external_id, parent_id, record_date, payload, sha256)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (kind, external_id) DO UPDATE SET
    parent_id = EXCLUDED.parent_id,
    record_date = EXCLUDED.record_date,
    payload = EXCLUDED.payload,
    sha256 = EXCLUDED.sha256,
    updated_at = now(),
    gone_at = NULL
WHERE ingest.raw_records.sha256 <> EXCLUDED.sha256 OR ingest.raw_records.gone_at IS NOT NULL;

-- name: MarkRawGone :execrows
-- Records of a parent that a full read (or a transaction window) no longer
-- returned. A NULL window_from means the read covered every date.
UPDATE ingest.raw_records SET gone_at = sqlc.arg(now)
WHERE kind = sqlc.arg(kind)
  AND parent_id = sqlc.arg(parent_id)
  AND gone_at IS NULL
  AND external_id <> ALL (sqlc.arg(seen)::text[])
  AND (sqlc.narg(window_from)::date IS NULL OR record_date >= sqlc.narg(window_from)::date);

-- name: ListRawRecords :many
SELECT kind, external_id, parent_id, payload FROM ingest.raw_records
WHERE kind = $1 AND parent_id = $2 AND gone_at IS NULL
ORDER BY record_date NULLS FIRST, external_id;
