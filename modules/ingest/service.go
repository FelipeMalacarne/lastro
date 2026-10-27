package ingest

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"lastro/modules/ingest/internal/pluggy"
	"lastro/modules/ledger"
	"lastro/platform/config"
	"lastro/platform/events"
)

var (
	// ErrSyncRunning means another sync holds the lock.
	ErrSyncRunning = errors.New("ingest: another sync is running")
	// ErrAuthorizationExpired means the Meu Pluggy authorization lapsed: reads
	// would come back empty, so the item is not read at all.
	ErrAuthorizationExpired = errors.New("ingest: Meu Pluggy authorization expired; renew it in the Pluggy Dashboard (Connect account, MeuPluggy)")
)

const (
	// lookback is how far back an incremental sync reads. Re-reading a window
	// catches pending purchases that settled, card bills restated around
	// their closing and installments posted late.
	lookback = 90 * 24 * time.Hour
	// overlap is how far before the last good sync a read starts, at least.
	overlap = 7 * 24 * time.Hour
)

// source is the Pluggy API as ingest reads it. Tests replace it.
type source interface {
	Item(ctx context.Context, id string) (pluggy.Item, error)
	Accounts(ctx context.Context, itemID string) ([]pluggy.Account, error)
	Transactions(ctx context.Context, accountID string, from time.Time) ([]pluggy.Transaction, error)
	Investments(ctx context.Context, itemID string) ([]pluggy.Investment, error)
}

// Service is ingest's facade: it reads Pluggy and sends the ledger commands.
type Service struct {
	repo    *Repository
	pluggy  source
	ledger  *ledger.Service
	itemIDs []string
	bus     *events.Bus
	logger  *slog.Logger
	now     func() time.Time
}

// New creates the service for the configured items.
func New(repo *Repository, cfg config.Pluggy, l *ledger.Service, bus *events.Bus, logger *slog.Logger) *Service {
	return &Service{
		repo:    repo,
		pluggy:  pluggy.New(cfg.ClientID, cfg.ClientSecret),
		ledger:  l,
		itemIDs: cfg.ItemIDs,
		bus:     cmp.Or(bus, events.NewBus(logger)),
		logger:  logger,
		now:     time.Now,
	}
}

// SyncOptions changes what a sync reads.
type SyncOptions struct {
	// Full reads every item's whole history instead of the recent window.
	Full bool
	// Reprocess rebuilds the ledger from the stored raw records, without
	// calling Pluggy.
	Reprocess bool
}

// SyncReport says what a sync did, item by item.
type SyncReport struct {
	Items []ItemReport
	// Paired is how many transfers and bill payments the ledger paired.
	Paired int
}

// ItemReport is one item's part of a sync.
type ItemReport struct {
	ItemID      string
	Institution string
	Accounts    int
	ledger.ImportResult
	Skipped     int // records the ledger has no place for, or that could not be read
	Investments int
	Err         error

	from time.Time // the first day read; zero means the whole history
}

// Sync reads every configured item and imports what changed. An item that
// fails does not stop the others; the error lists every failure.
func (s *Service) Sync(ctx context.Context, opts SyncOptions) (SyncReport, error) {
	unlock, ok, err := s.repo.lock(ctx)
	if err != nil {
		return SyncReport{}, fmt.Errorf("ingest: sync lock: %w", err)
	}
	if !ok {
		return SyncReport{}, ErrSyncRunning
	}
	defer unlock()

	var report SyncReport
	var errs []error
	done := SyncCompleted{}
	// Pairs cross items, so the matcher runs once, over the earliest window
	// any item read (zero, everything, after a first or full sync).
	matchFrom, readSome := s.now(), false
	for _, id := range s.itemIDs {
		var rep ItemReport
		if opts.Reprocess {
			rep, err = s.reprocessItem(ctx, id)
		} else {
			rep, err = s.syncItem(ctx, id, opts.Full)
		}
		rep.ItemID, rep.Err = id, err
		report.Items = append(report.Items, rep)
		if err == nil {
			readSome = true
			if rep.from.Before(matchFrom) {
				matchFrom = rep.from
			}
		}
		done.ItemIDs = append(done.ItemIDs, id)
		if err != nil {
			done.Failed = append(done.Failed, id)
			errs = append(errs, fmt.Errorf("%s (%s): %w", cmp.Or(rep.Institution, "item"), id, err))
			s.logger.ErrorContext(ctx, "sync item failed", "item", id, "err", err)
		}
	}
	if readSome {
		n, err := s.ledger.MatchTransfers(ctx, matchFrom)
		if err != nil {
			errs = append(errs, err)
		}
		report.Paired = n
	}
	if err := s.bus.Publish(ctx, done); err != nil {
		errs = append(errs, err)
	}
	return report, errors.Join(errs...)
}

// syncItem reads one item from Pluggy, stores the raw records and imports
// them. The item's sync state records the outcome either way.
func (s *Service) syncItem(ctx context.Context, id string, full bool) (rep ItemReport, err error) {
	started := s.now()
	state, err := s.repo.item(ctx, id)
	if err != nil {
		return rep, err
	}
	if err := s.repo.syncStarted(ctx, id, started); err != nil {
		return rep, err
	}
	defer func() {
		if ferr := s.repo.syncFinished(ctx, id, s.now(), err); ferr != nil && err == nil {
			err = ferr
		}
	}()

	it, err := s.pluggy.Item(ctx, id)
	if err != nil {
		return rep, err
	}
	rep.Institution = it.Connector.Name
	if err := s.repo.saveRaw(ctx, rawItem, id, []rawRecord{{ExternalID: id, Payload: it.Raw}}, time.Time{}, started); err != nil {
		return rep, err
	}
	fresh := withPluggy(state, it)
	if err := s.repo.saveItemState(ctx, fresh); err != nil {
		return rep, err
	}
	if !fresh.ConsentExpiresAt.IsZero() && !started.Before(fresh.ConsentExpiresAt) {
		return rep, ErrAuthorizationExpired
	}

	accounts, err := s.pluggy.Accounts(ctx, id)
	if err != nil {
		return rep, err
	}
	// A MeuPluggy item's connector is "MeuPluggy"; the bank is in its accounts.
	if inst := institutionOf(it, accounts); inst != fresh.Institution {
		rep.Institution, fresh.Institution = inst, inst
		if err := s.repo.saveItemState(ctx, fresh); err != nil {
			return rep, err
		}
	}
	recs := make([]rawRecord, len(accounts))
	for i, a := range accounts {
		recs[i] = rawRecord{ExternalID: a.ID, Payload: a.Raw}
	}
	if err := s.repo.saveRaw(ctx, rawAccount, id, recs, time.Time{}, started); err != nil {
		return rep, err
	}

	from := window(state, full, started)
	rep.from = from
	balanceAt := cmp.Or(fresh.DataUpdatedAt, started)
	for _, pa := range accounts {
		txs, err := s.pluggy.Transactions(ctx, pa.ID, from)
		if err != nil {
			return rep, err
		}
		if err := s.importAccount(ctx, &rep, id, pa, txs, from, balanceAt, true); err != nil {
			return rep, err
		}
	}

	// Positions arrive in M4; until then they are only stored. A failure here
	// must not fail the accounts that were already imported.
	invs, err := s.pluggy.Investments(ctx, id)
	if err != nil {
		s.logger.WarnContext(ctx, "investments not read", "item", id, "err", err)
		return rep, nil
	}
	recs = make([]rawRecord, len(invs))
	for i, inv := range invs {
		recs[i] = rawRecord{ExternalID: inv.ID, Payload: inv.Raw}
	}
	if err := s.repo.saveRaw(ctx, rawInvestment, id, recs, time.Time{}, started); err != nil {
		return rep, err
	}
	rep.Investments = len(invs)
	return rep, nil
}

// importAccount maps one account and its transactions into the ledger. With
// storeRaw it also stores the raw transactions (a reprocess reads them from
// there instead).
func (s *Service) importAccount(ctx context.Context, rep *ItemReport, itemID string, pa pluggy.Account, txs []pluggy.Transaction, from, balanceAt time.Time, storeRaw bool) error {
	d, err := mapAccount(pa, rep.Institution, balanceAt)
	if errors.Is(err, errUnsupported) {
		s.logger.WarnContext(ctx, "account skipped", "item", itemID, "account", pa.ID, "err", err)
		rep.Skipped++
		return nil
	}
	if err != nil {
		return err
	}
	accID, err := s.ledger.SyncAccount(ctx, d)
	if err != nil {
		return err
	}
	if err := s.repo.linkAccount(ctx, itemID, pa.ID, accID); err != nil {
		return err
	}

	batch := ledger.ImportBatch{Account: d.ExternalID, From: from}
	recs := make([]rawRecord, 0, len(txs))
	for _, t := range txs {
		in, err := mapTransaction(t, pa)
		if err != nil {
			s.logger.WarnContext(ctx, "transaction skipped", "account", pa.ID, "transaction", t.ID, "err", err)
			rep.Skipped++
			// Without it the batch is incomplete: removing what is missing
			// would remove this one too.
			batch.From = time.Time{}
			continue
		}
		batch.Transactions = append(batch.Transactions, in)
		recs = append(recs, rawRecord{ExternalID: t.ID, Date: in.Date, Payload: t.Raw})
	}
	if storeRaw {
		if err := s.repo.saveRaw(ctx, rawTransaction, pa.ID, recs, batch.From, s.now()); err != nil {
			return err
		}
	}
	res, err := s.ledger.ImportTransactions(ctx, batch)
	if err != nil {
		return err
	}
	rep.Accounts++
	rep.Created += res.Created
	rep.Updated += res.Updated
	rep.Unchanged += res.Unchanged
	rep.Removed += res.Removed
	return nil
}

// reprocessItem rebuilds the item's accounts and transactions in the ledger
// from the stored raw records. Nothing is removed: the stored records are
// the ones Pluggy still returned on the last read.
func (s *Service) reprocessItem(ctx context.Context, id string) (ItemReport, error) {
	var rep ItemReport
	items, err := s.repo.raw(ctx, rawItem, id)
	if err != nil || len(items) == 0 {
		return rep, cmp.Or(err, errors.New("ingest: nothing stored for this item; run a sync first"))
	}
	var it pluggy.Item
	if err := json.Unmarshal(items[0], &it); err != nil {
		return rep, err
	}
	balanceAt := cmp.Or(parseTimestamp(it.LastUpdatedAt), s.now())

	accounts, err := decodeRaw[pluggy.Account](ctx, s, rawAccount, id)
	if err != nil {
		return rep, err
	}
	rep.Institution = institutionOf(it, accounts)
	for _, pa := range accounts {
		txs, err := decodeRaw[pluggy.Transaction](ctx, s, rawTransaction, pa.ID)
		if err != nil {
			return rep, err
		}
		if err := s.importAccount(ctx, &rep, id, pa, txs, time.Time{}, balanceAt, false); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

func decodeRaw[T any](ctx context.Context, s *Service, kind, parent string) ([]T, error) {
	raws, err := s.repo.raw(ctx, kind, parent)
	if err != nil {
		return nil, err
	}
	out := make([]T, len(raws))
	for i, r := range raws {
		if err := json.Unmarshal(r, &out[i]); err != nil {
			return nil, fmt.Errorf("ingest: stored %s: %w", kind, err)
		}
	}
	return out, nil
}

// window is the first day an item's read covers: everything on its first
// sync or with full, otherwise the lookback, stretched back to a week before
// the last good sync when that was longer ago.
func window(s ItemState, full bool, now time.Time) time.Time {
	if full || s.SyncedAt.IsZero() {
		return time.Time{}
	}
	from := now.Add(-lookback)
	if early := s.SyncedAt.Add(-overlap); early.Before(from) {
		from = early
	}
	return from.In(saoPaulo)
}

// withPluggy updates an item's state with what Pluggy says about it now.
func withPluggy(s ItemState, it pluggy.Item) ItemState {
	// A MeuPluggy item keeps the bank name its accounts gave it (institutionOf).
	if it.Connector.ID != meuPluggyConnector {
		s.Institution = cmp.Or(it.Connector.Name, s.Institution)
	}
	s.Institution = cmp.Or(s.Institution, "Meu Pluggy")
	s.Status, s.ExecutionStatus, s.ErrorCode = it.Status, it.ExecutionStatus, ""
	if it.Error != nil {
		s.ErrorCode = it.Error.Code
	}
	s.DataUpdatedAt = parseTimestamp(it.LastUpdatedAt)
	s.ConsentExpiresAt = parseTimestamp(it.ConsentExpiresAt)
	return s
}

func parseTimestamp(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// ConnectionView is one connection for the screens: its health and the
// ledger accounts it feeds.
type ConnectionView struct {
	Connection
	AccountIDs []ledger.AccountID
}

// Connections returns the health of every configured item and of any other
// item that feeds a ledger account. It does not depend on this process's
// item ids: serve may have started before they were set.
func (s *Service) Connections(ctx context.Context) ([]ConnectionView, error) {
	states, err := s.repo.items(ctx, s.itemIDs)
	if err != nil {
		return nil, fmt.Errorf("ingest: connections: %w", err)
	}
	ids := make([]string, len(states))
	for i, st := range states {
		ids[i] = st.ItemID
	}
	accounts, err := s.repo.itemAccounts(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("ingest: connections: %w", err)
	}
	now := s.now()
	out := make([]ConnectionView, 0, len(states))
	for _, st := range states {
		out = append(out, ConnectionView{Connection: st.Connection(now), AccountIDs: accounts[st.ItemID]})
	}
	return out, nil
}
