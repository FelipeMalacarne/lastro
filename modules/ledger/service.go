package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"lastro/platform/events"
)

// ErrNotFound means there is no account or transaction with that id.
var ErrNotFound = errors.New("ledger: not found")

// Repository is the ledger's persistence; repository.go implements it on
// Postgres. Tx runs fn against a repository bound to one database
// transaction: everything fn saves commits together or not at all.
type Repository interface {
	Tx(ctx context.Context, fn func(Repository) error) error

	AccountByExternalID(ctx context.Context, id ExternalID) (*Account, error)
	Accounts(ctx context.Context) ([]*Account, error)
	SaveAccount(ctx context.Context, a *Account) error
	// SaveSnapshot stores the balance of a day, unless a later one is stored.
	SaveSnapshot(ctx context.Context, s BalanceSnapshot) error
	Snapshots(ctx context.Context, account AccountID) ([]BalanceSnapshot, error)

	// TransactionsForImport returns the account's transactions dated on or
	// after from (none for a zero from) plus any with one of the external
	// ids or bank references, removed ones included.
	TransactionsForImport(ctx context.Context, account AccountID, from time.Time, ids []ExternalID, bankRefs []string) ([]*Transaction, error)
	Transactions(ctx context.Context, ids []TransactionID) ([]*Transaction, error)
	// PairCandidates returns the unpaired, live transactions dated on or
	// after from (all, for a zero from) that could be one side of a pair.
	PairCandidates(ctx context.Context, from time.Time) ([]*Transaction, error)
	SaveTransactions(ctx context.Context, txs []*Transaction) error

	// Read side.
	AccountViews(ctx context.Context) ([]AccountView, error)
	TransactionViews(ctx context.Context, f TransactionFilter, limit int) ([]TransactionView, error)
}

// Service is the ledger's public facade: the commands ingest and categorize
// send, and the queries the screens read.
type Service struct {
	repo Repository
	bus  *events.Bus
	now  func() time.Time
}

// New creates the service. bus may be nil when nothing listens (tests, CLI).
func New(repo Repository, bus *events.Bus) *Service {
	if bus == nil {
		bus = events.NewBus(nil)
	}
	return &Service{repo: repo, bus: bus, now: time.Now}
}

// SyncAccount creates the account the first time a source reports it and
// refreshes it afterwards, and records the day's balance snapshot. It
// returns the ledger's id for the account.
func (s *Service) SyncAccount(ctx context.Context, d AccountDetails) (AccountID, error) {
	var id AccountID
	err := s.repo.Tx(ctx, func(r Repository) error {
		a, err := r.AccountByExternalID(ctx, d.ExternalID)
		switch {
		case errors.Is(err, ErrNotFound):
			if a, err = NewAccount(d); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if err := a.Refresh(d); err != nil {
				return err
			}
		}
		id = a.ID()
		if err := r.SaveAccount(ctx, a); err != nil {
			return err
		}
		return r.SaveSnapshot(ctx, a.Snapshot())
	})
	if err != nil {
		return "", fmt.Errorf("ledger: sync account %s: %w", d.ExternalID, err)
	}
	return id, nil
}

// ImportTransactions upserts a batch into an account (see reconcile for the
// rules). Running the same batch twice changes nothing the second time.
// After the commit it publishes TransactionsImported with the new and
// changed transactions.
func (s *Service) ImportTransactions(ctx context.Context, b ImportBatch) (ImportResult, error) {
	var rec reconciliation
	err := s.repo.Tx(ctx, func(r Repository) error {
		acc, err := r.AccountByExternalID(ctx, b.Account)
		if err != nil {
			return err
		}
		ids, refs := make([]ExternalID, 0, len(b.Transactions)), []string{}
		for _, in := range b.Transactions {
			ids = append(ids, in.ExternalID)
			if in.BankRef != "" {
				refs = append(refs, in.BankRef)
			}
		}
		existing, err := r.TransactionsForImport(ctx, acc.ID(), b.From, ids, refs)
		if err != nil {
			return err
		}
		if rec, err = reconcile(acc, existing, b, s.now()); err != nil {
			return err
		}
		// The other side of a broken pair lives in another account.
		others, err := r.Transactions(ctx, rec.broken)
		if err != nil {
			return err
		}
		for _, t := range others {
			t.LeavePair()
		}
		return r.SaveTransactions(ctx, append(rec.save, others...))
	})
	if err != nil {
		return ImportResult{}, fmt.Errorf("ledger: import transactions into %s: %w", b.Account, err)
	}
	if len(rec.touched) > 0 {
		if err := s.bus.Publish(ctx, TransactionsImported{TransactionIDs: rec.touched}); err != nil {
			return rec.result, fmt.Errorf("ledger: import transactions into %s: publish: %w", b.Account, err)
		}
	}
	return rec.result, nil
}

// MatchTransfers runs the TransferMatcher over the transactions dated on or
// after from (all of them, for a zero from), a few days earlier so a pair
// straddling from is seen whole. It returns how many pairs it made.
func (s *Service) MatchTransfers(ctx context.Context, from time.Time) (int, error) {
	if !from.IsZero() {
		from = from.AddDate(0, 0, -billPaymentWindow)
	}
	var paired []*Transaction
	err := s.repo.Tx(ctx, func(r Repository) error {
		cands, err := r.PairCandidates(ctx, from)
		if err != nil {
			return err
		}
		paired = MatchTransfers(cands)
		return r.SaveTransactions(ctx, paired)
	})
	if err != nil {
		return 0, fmt.Errorf("ledger: match transfers: %w", err)
	}
	return len(paired) / 2, nil
}

// Categorize sets a transaction's category. It returns ErrManualCategory
// when automation tries to replace a manual choice.
func (s *Service) Categorize(ctx context.Context, id TransactionID, c CategoryID, src Source) error {
	var evs []events.Event
	err := s.repo.Tx(ctx, func(r Repository) error {
		txs, err := r.Transactions(ctx, []TransactionID{id})
		if err != nil {
			return err
		}
		if len(txs) == 0 {
			return fmt.Errorf("%w: transaction %s", ErrNotFound, id)
		}
		t := txs[0]
		if err := t.Categorize(c, src); err != nil {
			return err
		}
		evs = t.PullEvents()
		return r.SaveTransactions(ctx, txs)
	})
	if err != nil {
		return fmt.Errorf("ledger: categorize %s: %w", id, err)
	}
	return s.bus.Publish(ctx, evs...)
}

// Accounts lists every account with its latest balance.
func (s *Service) Accounts(ctx context.Context) ([]AccountView, error) {
	views, err := s.repo.AccountViews(ctx)
	if err != nil {
		return nil, fmt.Errorf("ledger: accounts: %w", err)
	}
	return views, nil
}

// Transactions lists a month of transactions, newest first, and sums them.
func (s *Service) Transactions(ctx context.Context, f TransactionFilter) (TransactionList, error) {
	if f.Month.IsZero() {
		return TransactionList{}, fmt.Errorf("%w: a month is required", ErrInvalid)
	}
	views, err := s.repo.TransactionViews(ctx, f, maxListed+1)
	if err != nil {
		return TransactionList{}, fmt.Errorf("ledger: transactions: %w", err)
	}
	list := TransactionList{Transactions: views}
	if len(views) > maxListed {
		list.Transactions, list.Truncated = views[:maxListed], true
	}
	list.Summary = summarize(list.Transactions)
	return list, nil
}
