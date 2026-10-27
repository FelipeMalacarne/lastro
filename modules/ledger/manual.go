package ledger

import (
	"context"
	"fmt"
	"time"
	_ "time/tzdata" // days turn in São Paulo, even without the system's zoneinfo

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
	return NewAccount(AccountDetails{ExternalID: newManualID(), Name: name, Institution: ManualInstitution, Type: typ, Balance: balance, BalanceAt: at})
}

// Counted is a balance I counted on a day, for an asset: it replaces that
// day's balance in the history (see Latest for the account's own).
func (a *Account) Counted(b money.Money, day, at time.Time) (BalanceSnapshot, error) {
	if a.typ != AccountAsset {
		return BalanceSnapshot{}, fmt.Errorf("%w: only an asset takes a balance by hand", ErrInvalid)
	}
	if b.Currency() != a.currency {
		return BalanceSnapshot{}, fmt.Errorf("%w: account %s is in %s", ErrCurrencyChanged, a.externalID, a.currency)
	}
	return BalanceSnapshot{AccountID: a.id, Date: civilDate(day), Balance: b, At: at}, nil
}

// Latest makes an asset's balance the one of its latest day.
func (a *Account) Latest(snaps []BalanceSnapshot) error {
	if a.typ != AccountAsset {
		return fmt.Errorf("%w: only an asset takes its balance from what I counted", ErrInvalid)
	}
	if len(snaps) == 0 {
		return fmt.Errorf("%w: an asset keeps at least one balance", ErrInvalid)
	}
	last := snaps[0]
	for _, sn := range snaps[1:] {
		if sn.Date.After(last.Date) || (sn.Date.Equal(last.Date) && sn.At.After(last.At)) {
			last = sn
		}
	}
	a.balance, a.balanceAt = last.Balance, last.At
	return nil
}

// on is the account's balance as a snapshot of a given day.
func (a *Account) on(day time.Time) BalanceSnapshot {
	return BalanceSnapshot{AccountID: a.id, Date: civilDate(day), Balance: a.balance, At: a.balanceAt}
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
		return r.SaveSnapshot(ctx, a.on(s.today()))
	})
	if err != nil {
		return "", fmt.Errorf("ledger: create account: %w", err)
	}
	return a.ID(), nil
}

// SetManualBalance records a balance I counted on a day for an asset: it
// replaces that day's, and the latest day's is the account's balance.
func (s *Service) SetManualBalance(ctx context.Context, id AccountID, day time.Time, balance money.Money) error {
	err := s.repo.Tx(ctx, func(r Repository) error {
		a, err := accountByID(ctx, r, id)
		if err != nil {
			return err
		}
		snap, err := a.Counted(balance, day, s.now())
		if err != nil {
			return err
		}
		if err := r.ReplaceSnapshot(ctx, snap); err != nil {
			return err
		}
		return s.takeLatest(ctx, r, a)
	})
	if err != nil {
		return fmt.Errorf("ledger: set balance: %w", err)
	}
	return nil
}

// DeleteManualBalance removes a day's balance from an asset; the account
// takes the latest one left.
func (s *Service) DeleteManualBalance(ctx context.Context, id AccountID, day time.Time) error {
	err := s.repo.Tx(ctx, func(r Repository) error {
		a, err := accountByID(ctx, r, id)
		if err != nil {
			return err
		}
		if a.typ != AccountAsset {
			return fmt.Errorf("%w: only an asset loses a balance by hand", ErrInvalid)
		}
		if err := r.DeleteSnapshot(ctx, id, day); err != nil {
			return err
		}
		return s.takeLatest(ctx, r, a)
	})
	if err != nil {
		return fmt.Errorf("ledger: delete balance: %w", err)
	}
	return nil
}

func (s *Service) takeLatest(ctx context.Context, r Repository, a *Account) error {
	snaps, err := r.Snapshots(ctx, a.ID())
	if err != nil {
		return err
	}
	if err := a.Latest(snaps); err != nil {
		return err
	}
	return r.SaveAccount(ctx, a)
}

// AddManualTransaction adds a transaction by hand: money out is negative. In
// a wallet it moves the balance. In a synced account (checking, savings or a
// card) it is one the source failed to send: the bank's balance already
// counts it, so it moves nothing, and a sync never removes it. It goes through
// the rules and the model like a synced one, and a withdrawal from checking
// pairs with it as a transfer.
func (s *Service) AddManualTransaction(ctx context.Context, id AccountID, day time.Time, description string, amount money.Money) (TransactionID, error) {
	var tx *Transaction
	err := s.repo.Tx(ctx, func(r Repository) error {
		a, err := accountByID(ctx, r, id)
		if err != nil {
			return err
		}
		if a.typ == AccountAsset {
			return fmt.Errorf("%w: an asset takes balances, not transactions", ErrInvalid)
		}
		if tx, err = NewTransaction(a, ImportedTransaction{ExternalID: newManualID(), Date: day, Description: description, Amount: amount, Kind: manualKind(a.typ, amount), Status: StatusPosted}); err != nil {
			return err
		}
		if err := r.SaveTransactions(ctx, []*Transaction{tx}); err != nil {
			return err
		}
		if a.typ != AccountCash {
			return nil
		}
		if err := a.Move(amount, s.now()); err != nil {
			return err
		}
		if err := r.SaveAccount(ctx, a); err != nil {
			return err
		}
		return r.SaveSnapshot(ctx, a.on(s.today()))
	})
	if err != nil {
		return "", fmt.Errorf("ledger: add transaction: %w", err)
	}
	// The entry is saved: a withdrawal it fails to pair now pairs on the next
	// sync, which runs the matcher again.
	_, _ = s.MatchTransfers(ctx, day.AddDate(0, 0, -transferWindow))
	return tx.ID(), s.bus.Publish(ctx, TransactionsImported{TransactionIDs: []TransactionID{tx.ID()}})
}

// manualKind is the kind of a transaction I add: money in is income, except
// on a card, where it is a refund that reduces spending, as ingest reads one.
func manualKind(typ AccountType, amount money.Money) Kind {
	if amount.IsPositive() && !typ.IsCard() {
		return KindIncome
	}
	return KindExpense
}

// RemoveManualTransaction removes a transaction I added; in a wallet its
// amount goes back.
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
		if !t.IsManual() {
			return fmt.Errorf("%w: only a transaction I added can be removed", ErrInvalid)
		}
		now := s.now()
		wallet := a.typ == AccountCash
		if wallet {
			if err := a.Move(t.Amount().Neg(), now); err != nil {
				return err
			}
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
		if !wallet {
			return nil
		}
		if err := r.SaveAccount(ctx, a); err != nil {
			return err
		}
		return r.SaveSnapshot(ctx, a.on(s.today()))
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

// today is the calendar day in São Paulo, where the days of what I type
// turn.
func (s *Service) today() time.Time {
	return civilDate(s.now().In(saoPaulo))
}

var saoPaulo, _ = time.LoadLocation("America/Sao_Paulo")
