package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lastro/modules/ingest/internal/db"
	"lastro/modules/ledger"
)

// Kinds of raw record.
const (
	rawItem        = "item"
	rawAccount     = "account"
	rawTransaction = "transaction"
	rawInvestment  = "investment"
	// rawInvestmentTransaction's parent is the investment.
	rawInvestmentTransaction = "investment_transaction"
)

// rawRecord is one Pluggy record as received.
type rawRecord struct {
	ExternalID string
	Date       time.Time // transactions only
	Payload    json.RawMessage
}

// syncLockKey is the advisory lock that keeps two syncs from running at once.
const syncLockKey = 0x6c617374 // "last"

// Repository is ingest's persistence on the ingest schema. ingest is a thin
// module: no aggregates, so the repository is concrete and its methods are
// for the service only.
type Repository struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewRepository creates the repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: db.New(pool)}
}

// lock takes the sync lock for as long as the returned unlock is not called.
// ok is false when another sync holds it.
func (r *Repository) lock(ctx context.Context) (unlock func(), ok bool, err error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", syncLockKey).Scan(&ok); err != nil || !ok {
		conn.Release()
		return nil, false, err
	}
	return func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", syncLockKey)
		conn.Release()
	}, true, nil
}

func (r *Repository) item(ctx context.Context, id string) (ItemState, error) {
	if err := r.q.EnsureItem(ctx, id); err != nil {
		return ItemState{}, err
	}
	row, err := r.q.GetItem(ctx, id)
	if err != nil {
		return ItemState{}, err
	}
	return itemState(row), nil
}

func (r *Repository) items(ctx context.Context, ids []string) ([]ItemState, error) {
	rows, err := r.q.ListItems(ctx, append([]string{}, ids...))
	if err != nil {
		return nil, err
	}
	out := make([]ItemState, 0, len(rows))
	for _, row := range rows {
		out = append(out, itemState(row))
	}
	return out, nil
}

func (r *Repository) syncStarted(ctx context.Context, id string, at time.Time) error {
	return r.q.SyncStarted(ctx, db.SyncStartedParams{ID: id, SyncStartedAt: &at})
}

func (r *Repository) saveItemState(ctx context.Context, s ItemState) error {
	return r.q.SaveItemState(ctx, db.SaveItemStateParams{
		ID:               s.ItemID,
		Institution:      s.Institution,
		Status:           s.Status,
		ExecutionStatus:  s.ExecutionStatus,
		ErrorCode:        s.ErrorCode,
		DataUpdatedAt:    timePtr(s.DataUpdatedAt),
		ConsentExpiresAt: timePtr(s.ConsentExpiresAt),
	})
}

func (r *Repository) syncFinished(ctx context.Context, id string, at time.Time, syncErr error) error {
	if syncErr == nil {
		return r.q.SyncSucceeded(ctx, db.SyncSucceededParams{ID: id, SyncFinishedAt: &at})
	}
	return r.q.SyncFailed(ctx, db.SyncFailedParams{ID: id, SyncFinishedAt: &at, SyncError: syncErr.Error()})
}

func (r *Repository) linkAccount(ctx context.Context, itemID, externalID string, account ledger.AccountID) error {
	return r.q.LinkAccount(ctx, db.LinkAccountParams{ItemID: itemID, ExternalID: externalID, LedgerAccountID: string(account)})
}

// itemAccounts returns the ledger accounts of each item.
func (r *Repository) itemAccounts(ctx context.Context, itemIDs []string) (map[string][]ledger.AccountID, error) {
	rows, err := r.q.ListItemAccounts(ctx, append([]string{}, itemIDs...))
	if err != nil {
		return nil, err
	}
	out := map[string][]ledger.AccountID{}
	for _, row := range rows {
		out[row.ItemID] = append(out[row.ItemID], ledger.AccountID(row.LedgerAccountID))
	}
	return out, nil
}

// saveRaw stores the records Pluggy returned for a parent and marks the ones
// it no longer returns as gone: within the window for transactions (from
// zero means every date), all of them for other kinds. Like the ledger, an
// empty read marks nothing.
func (r *Repository) saveRaw(ctx context.Context, kind, parentID string, recs []rawRecord, from time.Time, now time.Time) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		params := make([]db.SaveRawRecordParams, len(recs))
		seen := make([]string, len(recs))
		for i, rec := range recs {
			sum := sha256.Sum256(rec.Payload)
			params[i] = db.SaveRawRecordParams{
				Kind:       kind,
				ExternalID: rec.ExternalID,
				ParentID:   parentID,
				RecordDate: datePtr(rec.Date),
				Payload:    rec.Payload,
				Sha256:     sum[:],
			}
			seen[i] = rec.ExternalID
		}
		var errs []error
		q.SaveRawRecord(ctx, params).Exec(func(i int, err error) {
			if err != nil {
				errs = append(errs, fmt.Errorf("save raw %s %s: %w", kind, recs[i].ExternalID, err))
			}
		})
		if err := errors.Join(errs...); err != nil {
			return err
		}
		if len(recs) == 0 {
			return nil
		}
		_, err := q.MarkRawGone(ctx, db.MarkRawGoneParams{Now: &now, Kind: kind, ParentID: parentID, Seen: seen, WindowFrom: datePtr(from)})
		return err
	})
}

// raw returns the records of a parent that Pluggy still returns.
func (r *Repository) raw(ctx context.Context, kind, parentID string) ([]json.RawMessage, error) {
	rows, err := r.q.ListRawRecords(ctx, db.ListRawRecordsParams{Kind: kind, ParentID: parentID})
	if err != nil {
		return nil, err
	}
	out := make([]json.RawMessage, len(rows))
	for i, row := range rows {
		out[i] = row.Payload
	}
	return out, nil
}

func itemState(row db.IngestItem) ItemState {
	return ItemState{
		ItemID:           row.ID,
		Institution:      row.Institution,
		Status:           row.Status,
		ExecutionStatus:  row.ExecutionStatus,
		ErrorCode:        row.ErrorCode,
		DataUpdatedAt:    deref(row.DataUpdatedAt),
		ConsentExpiresAt: deref(row.ConsentExpiresAt),
		SyncStartedAt:    deref(row.SyncStartedAt),
		SyncFinishedAt:   deref(row.SyncFinishedAt),
		SyncedAt:         deref(row.SyncedAt),
		SyncError:        row.SyncError,
	}
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func datePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	y, m, d := t.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &day
}

func deref(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
