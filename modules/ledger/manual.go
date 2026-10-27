package ledger

import (
	"context"
	"fmt"
	"time"

	"lastro/money"
	"lastro/platform/events"
)

// ManualInstitution is where the accounts I type in are listed.
const ManualInstitution = "Outside Open Finance"

// NewManualAccount opens an account outside Open Finance: a wallet, whose
// transactions I add, or an asset, whose balance I update now and then.
func NewManualAccount(name string, typ AccountType, balance money.Money, at time.Time) (*Account, error) {
	if !typ.IsManual() {
		return nil, fmt.Errorf("%w: an account typed in is a wallet or an asset", ErrInvalid)
	}
	ext, err := NewExternalID("manual-" + newID())
	if err != nil {
		return nil, err
	}
	return NewAccount(AccountDetails{ExternalID: ext, Name: name, Institution: ManualInstitution, Type: typ, Balance: balance, BalanceAt: at})
}

// SetBalance is a balance I typed, as of a moment. An earlier one only adds
// to the history; the account keeps its latest.
func (a *Account) SetBalance(b money.Money, at time.Time) (BalanceSnapshot, error) {
	if !a.typ.IsManual() {
		return BalanceSnapshot{}, fmt.Errorf("%w: only an account typed in takes a balance by hand", ErrInvalid)
	}
	if b.Currency() != a.currency {
		return BalanceSnapshot{}, fmt.Errorf("%w: account %s is in %s", ErrCurrencyChanged, a.externalID, a.currency)
	}
	if !at.Before(a.balanceAt) {
		a.balance, a.balanceAt = b, at
	}
	return BalanceSnapshot{AccountID: a.id, Date: civilDate(at), Balance: b, At: at}, nil
}

// Move is a wallet's balance changing by an entry I added or removed.
func (a *Account) Move(by money.Money, at time.Time) error {
	if a.typ != AccountCash {
		return fmt.Errorf("%w: only a wallet takes transactions by hand", ErrInvalid)
	}
	b, err := a.balance.Add(by)
	if err != nil {
		return err
	}
	a.balance = b
	if at.After(a.balanceAt) {
		a.balanceAt = at
	}
	return nil
}

// Manual entries: the Service's commands for the accounts I type in.

// CreateManualAccount opens a wallet ("cash") or an asset ("asset") with its
// balance today.
func (s *Service) CreateManualAccount(ctx context.Context, name string, typ AccountType, balance money.Money) (AccountID, error) {
	a, err := NewManualAccount(name, typ, balance, s.now())
	if err != nil {
		return "", err
	}
	err = s.repo.Tx(ctx, func(r Repository) error {
		if err := r.SaveAccount(ctx, a); err != nil {
			return err
		}
		return r.SaveSnapshot(ctx, a.Snapshot())
	})
	if err != nil {
		return "", fmt.Errorf("ledger: create account: %w", err)
	}
	return a.ID(), nil
}

// SetManualBalance records a balance I counted on a day: today's is the
// account's balance from now on, an earlier day's goes into the history.
func (s *Service) SetManualBalance(ctx context.Context, id AccountID, day time.Time, balance money.Money) error {
	err := s.repo.Tx(ctx, func(r Repository) error {
		a, err := accountByID(ctx, r, id)
		if err != nil {
			return err
		}
		snap, err := a.SetBalance(balance, s.moment(day))
		if err != nil {
			return err
		}
		if err := r.SaveAccount(ctx, a); err != nil {
			return err
		}
		return r.SaveSnapshot(ctx, snap)
	})
	if err != nil {
		return fmt.Errorf("ledger: set balance: %w", err)
	}
	return nil
}

// DeleteManualBalance removes a day's balance; the account takes the latest
// one left.
func (s *Service) DeleteManualBalance(ctx context.Context, id AccountID, day time.Time) error {
	err := s.repo.Tx(ctx, func(r Repository) error {
		a, err := accountByID(ctx, r, id)
		if err != nil {
			return err
		}
		if !a.typ.IsManual() {
			return fmt.Errorf("%w: only an account typed in loses a balance by hand", ErrInvalid)
		}
		if err := r.DeleteSnapshot(ctx, id, day); err != nil {
			return err
		}
		snaps, err := r.Snapshots(ctx, id)
		if err != nil {
			return err
		}
		if len(snaps) == 0 {
			return fmt.Errorf("%w: an account keeps at least one balance", ErrInvalid)
		}
		last := snaps[len(snaps)-1]
		a.balance, a.balanceAt = last.Balance, last.At
		return r.SaveAccount(ctx, a)
	})
	if err != nil {
		return fmt.Errorf("ledger: delete balance: %w", err)
	}
	return nil
}

// AddManualTransaction adds a wallet entry: money out is negative. It goes
// through the rules and the model like a synced one, and a withdrawal from
// checking pairs with it as a transfer.
func (s *Service) AddManualTransaction(ctx context.Context, id AccountID, day time.Time, description string, amount money.Money) (TransactionID, error) {
	var tx *Transaction
	err := s.repo.Tx(ctx, func(r Repository) error {
		a, err := accountByID(ctx, r, id)
		if err != nil {
			return err
		}
		if a.typ != AccountCash {
			return fmt.Errorf("%w: only a wallet takes transactions by hand", ErrInvalid)
		}
		ext, err := NewExternalID("manual-" + newID())
		if err != nil {
			return err
		}
		kind := KindExpense
		if amount.IsPositive() {
			kind = KindIncome
		}
		if tx, err = NewTransaction(a, ImportedTransaction{ExternalID: ext, Date: day, Description: description, Amount: amount, Kind: kind, Status: StatusPosted}); err != nil {
			return err
		}
		if err := a.Move(amount, s.now()); err != nil {
			return err
		}
		if err := r.SaveTransactions(ctx, []*Transaction{tx}); err != nil {
			return err
		}
		if err := r.SaveAccount(ctx, a); err != nil {
			return err
		}
		return r.SaveSnapshot(ctx, a.Snapshot())
	})
	if err != nil {
		return "", fmt.Errorf("ledger: add transaction: %w", err)
	}
	if _, err := s.MatchTransfers(ctx, day.AddDate(0, 0, -transferWindow)); err != nil {
		return tx.ID(), err
	}
	return tx.ID(), s.bus.Publish(ctx, TransactionsImported{TransactionIDs: []TransactionID{tx.ID()}})
}

// RemoveManualTransaction takes a wallet entry back, and its amount from the
// wallet's balance.
func (s *Service) RemoveManualTransaction(ctx context.Context, id TransactionID) error {
	var evs []events.Event
	err := s.repo.Tx(ctx, func(r Repository) error {
		txs, err := r.Transactions(ctx, []TransactionID{id})
		if err != nil {
			return err
		}
		if len(txs) == 0 || txs[0].IsRemoved() {
			return fmt.Errorf("%w: transaction %s", ErrNotFound, id)
		}
		t := txs[0]
		a, err := accountByID(ctx, r, t.AccountID())
		if err != nil {
			return err
		}
		if a.typ != AccountCash {
			return fmt.Errorf("%w: only a transaction I added can be removed", ErrInvalid)
		}
		now := s.now()
		if err := a.Move(t.Amount().Neg(), now); err != nil {
			return err
		}
		changed := []*Transaction{t}
		if other := t.Remove(now); other != "" {
			// It was paired: the other side counts on its own again.
			os, err := r.Transactions(ctx, []TransactionID{other})
			if err != nil {
				return err
			}
			for _, o := range os {
				o.LeavePair()
				changed = append(changed, o)
			}
		}
		evs = t.PullEvents()
		if err := r.SaveTransactions(ctx, changed); err != nil {
			return err
		}
		if err := r.SaveAccount(ctx, a); err != nil {
			return err
		}
		return r.SaveSnapshot(ctx, a.Snapshot())
	})
	if err != nil {
		return fmt.Errorf("ledger: remove transaction: %w", err)
	}
	return s.bus.Publish(ctx, evs...)
}

func accountByID(ctx context.Context, r Repository, id AccountID) (*Account, error) {
	as, err := r.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range as {
		if a.ID() == id {
			return a, nil
		}
	}
	return nil, fmt.Errorf("%w: account %s", ErrNotFound, id)
}

// moment is when a balance typed for a day was read: now for today, noon of
// the day otherwise.
func (s *Service) moment(day time.Time) time.Time {
	now := s.now()
	if civilDate(now).Equal(civilDate(day)) {
		return now
	}
	y, m, d := day.Date()
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}
