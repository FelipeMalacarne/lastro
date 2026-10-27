package ledger

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"lastro/money"
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
	DeleteSnapshot(ctx context.Context, account AccountID, day time.Time) error

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
	AccountView(ctx context.Context, id AccountID) (AccountView, error)
	TransactionViews(ctx context.Context, f TransactionFilter, limit int) ([]TransactionView, error)
	TransactionView(ctx context.Context, id TransactionID) (TransactionView, error)
	// DailyNet sums an account's live transactions by day, up to a day.
	DailyNet(ctx context.Context, account AccountID, until time.Time) ([]DayNet, error)
	MonthFlows(ctx context.Context, account AccountID) ([]MonthFlowRow, error)
	Installments(ctx context.Context, account AccountID) ([]InstallmentRow, error)
	// DescriptionMonths counts the transactions that count with a
	// description (ignoring case), by month, in [from, to).
	DescriptionMonths(ctx context.Context, description string, from, to time.Time) ([]MonthTotal, error)
	DaySpending(ctx context.Context, from, to time.Time) ([]DayView, error)
	// Categorizable returns the ones among ids that automation may
	// categorize; Uncategorized, every one of those still without a category.
	Categorizable(ctx context.Context, ids []TransactionID) ([]Categorizable, error)
	Uncategorized(ctx context.Context) ([]Categorizable, error)
	Automatable(ctx context.Context) ([]Categorizable, error)
	DescriptionCounts(ctx context.Context) ([]DescriptionCount, error)
	CategoryTotals(ctx context.Context, from, to time.Time) ([]CategoryTotal, error)
	CategoryMonths(ctx context.Context, from, to time.Time) ([]CategoryMonth, error)
	MonthKinds(ctx context.Context, from, to time.Time) ([]MonthKind, error)
	Charges(ctx context.Context, from time.Time) ([]Charge, error)

	// What I add to a transaction.
	SetNote(ctx context.Context, id TransactionID, n Note) error
	SetTags(ctx context.Context, id TransactionID, tags []Tag) error
	Split(ctx context.Context, id TransactionID) ([]SplitPartView, error)
	SaveSplit(ctx context.Context, id TransactionID, s Split) error
	Tags(ctx context.Context) ([]TagView, error)
	TagCategories(ctx context.Context, tag string) ([]CategoryTotal, error)
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
		if src == SourceManual {
			// One category chosen by hand replaces a split.
			if err := r.SaveSplit(ctx, id, Split{}); err != nil {
				return err
			}
		}
		return r.SaveTransactions(ctx, txs)
	})
	if err != nil {
		return fmt.Errorf("ledger: categorize %s: %w", id, err)
	}
	return s.bus.Publish(ctx, evs...)
}

// Uncategorize takes back an automation-made category, so the transaction
// waits in the review queue again.
func (s *Service) Uncategorize(ctx context.Context, id TransactionID) error {
	err := s.repo.Tx(ctx, func(r Repository) error {
		txs, err := r.Transactions(ctx, []TransactionID{id})
		if err != nil {
			return err
		}
		if len(txs) == 0 {
			return fmt.Errorf("%w: transaction %s", ErrNotFound, id)
		}
		if err := txs[0].Uncategorize(); err != nil {
			return err
		}
		return r.SaveTransactions(ctx, txs)
	})
	if err != nil {
		return fmt.Errorf("ledger: uncategorize %s: %w", id, err)
	}
	return nil
}

// SetNote writes my note on a transaction; an empty one clears it.
func (s *Service) SetNote(ctx context.Context, id TransactionID, note string) error {
	n, err := NewNote(note)
	if err != nil {
		return err
	}
	if err := s.repo.SetNote(ctx, id, n); err != nil {
		return fmt.Errorf("ledger: note %s: %w", id, err)
	}
	return nil
}

// SetTags replaces a transaction's tags.
func (s *Service) SetTags(ctx context.Context, id TransactionID, tags []string) error {
	ts, err := NewTags(tags)
	if err != nil {
		return err
	}
	if err := s.repo.SetTags(ctx, id, ts); err != nil {
		return fmt.Errorf("ledger: tags %s: %w", id, err)
	}
	return nil
}

// Tags lists every tag in use, the most recently used first.
func (s *Service) Tags(ctx context.Context) ([]TagView, error) {
	ts, err := s.repo.Tags(ctx)
	if err != nil {
		return nil, fmt.Errorf("ledger: tags: %w", err)
	}
	return ts, nil
}

// TagCategories is one tag's money by category over every month.
func (s *Service) TagCategories(ctx context.Context, tag string) ([]CategoryTotal, error) {
	t, err := NewTag(tag)
	if err != nil {
		return nil, err
	}
	cs, err := s.repo.TagCategories(ctx, string(t))
	if err != nil {
		return nil, fmt.Errorf("ledger: tag %s: %w", t, err)
	}
	return cs, nil
}

// SplitTransaction divides a transaction across categories (see
// Transaction.SplitInto); categorize checks the categories are live first.
func (s *Service) SplitTransaction(ctx context.Context, id TransactionID, parts []SplitPart) error {
	var evs []events.Event
	err := s.repo.Tx(ctx, func(r Repository) error {
		txs, err := r.Transactions(ctx, []TransactionID{id})
		if err != nil {
			return err
		}
		if len(txs) == 0 {
			return fmt.Errorf("%w: transaction %s", ErrNotFound, id)
		}
		split, err := txs[0].SplitInto(parts)
		if err != nil {
			return err
		}
		evs = txs[0].PullEvents()
		if err := r.SaveSplit(ctx, id, split); err != nil {
			return err
		}
		return r.SaveTransactions(ctx, txs)
	})
	if err != nil {
		return fmt.Errorf("ledger: split %s: %w", id, err)
	}
	return s.bus.Publish(ctx, evs...)
}

// Unsplit removes a split; the transaction keeps the largest part's
// category, still mine.
func (s *Service) Unsplit(ctx context.Context, id TransactionID) error {
	if err := s.repo.SaveSplit(ctx, id, Split{}); err != nil {
		return fmt.Errorf("ledger: unsplit %s: %w", id, err)
	}
	return nil
}

// Restore puts back a transaction's category as it was before a choice
// that is being undone (see Transaction.Restore).
func (s *Service) Restore(ctx context.Context, id TransactionID, c CategoryID, src Source) error {
	var evs []events.Event
	err := s.repo.Tx(ctx, func(r Repository) error {
		txs, err := r.Transactions(ctx, []TransactionID{id})
		if err != nil {
			return err
		}
		if len(txs) == 0 {
			return fmt.Errorf("%w: transaction %s", ErrNotFound, id)
		}
		if err := txs[0].Restore(c, src); err != nil {
			return err
		}
		evs = txs[0].PullEvents()
		return r.SaveTransactions(ctx, txs)
	})
	if err != nil {
		return fmt.Errorf("ledger: restore %s: %w", id, err)
	}
	return s.bus.Publish(ctx, evs...)
}

// Categorizable returns, of these transactions, the ones automation may
// categorize: live expenses and income whose category nobody chose by hand.
// Transfers and card bill payments never get a category.
func (s *Service) Categorizable(ctx context.Context, ids []TransactionID) ([]Categorizable, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	txs, err := s.repo.Categorizable(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("ledger: categorizable: %w", err)
	}
	return txs, nil
}

// Uncategorized returns every live expense and income still without a
// category, oldest first.
func (s *Service) Uncategorized(ctx context.Context) ([]Categorizable, error) {
	txs, err := s.repo.Uncategorized(ctx)
	if err != nil {
		return nil, fmt.Errorf("ledger: uncategorized: %w", err)
	}
	return txs, nil
}

// Automatable returns every live expense and income whose category nobody
// chose by hand: what a changed rule may recategorize.
func (s *Service) Automatable(ctx context.Context) ([]Categorizable, error) {
	txs, err := s.repo.Automatable(ctx)
	if err != nil {
		return nil, fmt.Errorf("ledger: automatable: %w", err)
	}
	return txs, nil
}

// DescriptionCounts returns how often each description of live expenses and
// income came, by account, and when last.
func (s *Service) DescriptionCounts(ctx context.Context) ([]DescriptionCount, error) {
	out, err := s.repo.DescriptionCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("ledger: description counts: %w", err)
	}
	return out, nil
}

// CategoryTotals sums a month's live expenses and income by category.
func (s *Service) CategoryTotals(ctx context.Context, month time.Time) ([]CategoryTotal, error) {
	from := monthOf(month)
	out, err := s.repo.CategoryTotals(ctx, from, from.AddDate(0, 1, 0))
	if err != nil {
		return nil, fmt.Errorf("ledger: category totals: %w", err)
	}
	return out, nil
}

// CategoryMonths sums live expenses and income by month, category and kind,
// from the month of from through the month of to.
func (s *Service) CategoryMonths(ctx context.Context, from, to time.Time) ([]CategoryMonth, error) {
	out, err := s.repo.CategoryMonths(ctx, monthOf(from), monthOf(to).AddDate(0, 1, 0))
	if err != nil {
		return nil, fmt.Errorf("ledger: category months: %w", err)
	}
	return out, nil
}

// MonthKinds sums every live transaction by month, kind, direction and
// pairing, from the month of from through the month of to.
func (s *Service) MonthKinds(ctx context.Context, from, to time.Time) ([]MonthKind, error) {
	out, err := s.repo.MonthKinds(ctx, monthOf(from), monthOf(to).AddDate(0, 1, 0))
	if err != nil {
		return nil, fmt.Errorf("ledger: month kinds: %w", err)
	}
	return out, nil
}

// Charges lists live expenses since a day, outside installments, oldest
// first.
func (s *Service) Charges(ctx context.Context, from time.Time) ([]Charge, error) {
	out, err := s.repo.Charges(ctx, from)
	if err != nil {
		return nil, fmt.Errorf("ledger: charges: %w", err)
	}
	return out, nil
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

// Account returns one account with its latest balance and card details.
func (s *Service) Account(ctx context.Context, id AccountID) (AccountView, error) {
	v, err := s.repo.AccountView(ctx, id)
	if err != nil {
		return AccountView{}, fmt.Errorf("ledger: account %s: %w", id, err)
	}
	return v, nil
}

// BalanceHistory rebuilds an account's balance at the end of each day, from
// its first transaction to the day of the bank's latest balance.
func (s *Service) BalanceHistory(ctx context.Context, id AccountID) (BalanceHistoryView, error) {
	acc, err := s.repo.AccountView(ctx, id)
	if err != nil {
		return BalanceHistoryView{}, fmt.Errorf("ledger: balance history %s: %w", id, err)
	}
	if AccountType(acc.Type) == AccountAsset {
		// An asset has no transactions: its history is the balances I typed.
		snaps, err := s.repo.Snapshots(ctx, id)
		if err != nil {
			return BalanceHistoryView{}, fmt.Errorf("ledger: balance history %s: %w", id, err)
		}
		v := BalanceHistoryView{Currency: acc.Currency, AsOf: acc.BalanceAt, Points: make([]BalancePointView, len(snaps))}
		for i, sn := range snaps {
			v.Points[i] = BalancePointView{Date: sn.Date.Format(time.DateOnly), BalanceCents: sn.Balance.Cents()}
		}
		return v, nil
	}
	current, err := money.New(acc.BalanceCents, money.Currency(acc.Currency))
	if err != nil {
		return BalanceHistoryView{}, fmt.Errorf("ledger: balance history %s: %w", id, err)
	}
	nets, err := s.repo.DailyNet(ctx, id, acc.BalanceAt)
	if err != nil {
		return BalanceHistoryView{}, fmt.Errorf("ledger: balance history %s: %w", id, err)
	}
	points, err := BalanceHistory(current, acc.BalanceAt, nets)
	if err != nil {
		return BalanceHistoryView{}, fmt.Errorf("ledger: balance history %s: %w", id, err)
	}
	v := BalanceHistoryView{Currency: acc.Currency, AsOf: acc.BalanceAt, Points: make([]BalancePointView, len(points))}
	for i, p := range points {
		v.Points[i] = BalancePointView{Date: p.Date.Format(time.DateOnly), BalanceCents: p.Balance.Cents()}
	}
	return v, nil
}

// Months returns an account's months, oldest first, with their flows. On a
// card the months after today's hold the installments already committed.
func (s *Service) Months(ctx context.Context, id AccountID) ([]MonthView, error) {
	if _, err := s.repo.AccountView(ctx, id); err != nil {
		return nil, fmt.Errorf("ledger: months %s: %w", id, err)
	}
	rows, err := s.repo.MonthFlows(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("ledger: months %s: %w", id, err)
	}
	return monthViews(rows, s.now()), nil
}

// Installments returns an account's purchases with installments still to
// come, the one ending first first.
func (s *Service) Installments(ctx context.Context, id AccountID) ([]PurchaseView, error) {
	if _, err := s.repo.AccountView(ctx, id); err != nil {
		return nil, fmt.Errorf("ledger: installments %s: %w", id, err)
	}
	rows, err := s.repo.Installments(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("ledger: installments %s: %w", id, err)
	}
	today := s.now()
	var live []Purchase
	for _, p := range GroupInstallments(rows) {
		if p.InProgress(today) {
			live = append(live, p)
		}
	}
	slices.SortStableFunc(live, func(a, b Purchase) int { return a.Last().Compare(b.Last()) })
	out := make([]PurchaseView, len(live))
	for i, p := range live {
		out[i] = purchaseView(p, today)
	}
	return out, nil
}

// Transaction returns one live transaction with the other side of its
// pair, its purchase's installments and its description's last six months.
func (s *Service) Transaction(ctx context.Context, id TransactionID) (TransactionDetail, error) {
	v, err := s.repo.TransactionView(ctx, id)
	if err != nil {
		return TransactionDetail{}, fmt.Errorf("ledger: transaction %s: %w", id, err)
	}
	d := TransactionDetail{Transaction: v, Split: []SplitPartView{}}
	if len(v.SplitCategories) > 0 {
		if d.Split, err = s.repo.Split(ctx, id); err != nil {
			return TransactionDetail{}, fmt.Errorf("ledger: transaction %s: %w", id, err)
		}
	}
	if v.CounterpartID != nil {
		c, err := s.repo.TransactionView(ctx, *v.CounterpartID)
		switch {
		case err == nil:
			d.Counterpart = &c
		case !errors.Is(err, ErrNotFound):
			return TransactionDetail{}, fmt.Errorf("ledger: transaction %s: %w", id, err)
		}
	}
	if v.Installment != nil {
		rows, err := s.repo.Installments(ctx, v.Account.ID)
		if err != nil {
			return TransactionDetail{}, fmt.Errorf("ledger: transaction %s: %w", id, err)
		}
		for _, p := range GroupInstallments(rows) {
			if slices.ContainsFunc(p.Installments, func(r InstallmentRow) bool { return r.ID == id }) {
				pv := purchaseView(p, s.now())
				d.Purchase = &pv
			}
		}
	}
	date, err := time.Parse(time.DateOnly, v.Date)
	if err != nil {
		return TransactionDetail{}, fmt.Errorf("ledger: transaction %s: %w", id, err)
	}
	to := monthOf(date).AddDate(0, 1, 0)
	from := to.AddDate(0, -6, 0)
	months, err := s.repo.DescriptionMonths(ctx, v.Description, from, to)
	if err != nil {
		return TransactionDetail{}, fmt.Errorf("ledger: transaction %s: %w", id, err)
	}
	d.History = descriptionStats(months, from)
	return d, nil
}

// Days returns a month's spending day by day, every account together, with
// the month before. PreviousToDate compares like with like: for the month
// in progress, the month before up to the same day.
func (s *Service) Days(ctx context.Context, month time.Time) (DaysView, error) {
	from := monthOf(month)
	to := from.AddDate(0, 1, 0)
	prevFrom := from.AddDate(0, -1, 0)
	days, err := s.repo.DaySpending(ctx, from, to)
	if err != nil {
		return DaysView{}, fmt.Errorf("ledger: days %s: %w", from.Format("2006-01"), err)
	}
	prev, err := s.repo.DaySpending(ctx, prevFrom, from)
	if err != nil {
		return DaysView{}, fmt.Errorf("ledger: days %s: %w", from.Format("2006-01"), err)
	}
	v := DaysView{Month: from.Format("2006-01"), Days: days, Previous: prev}
	for _, d := range days {
		v.Summary.SpentCents += d.SpentCents
		v.Summary.InCents += d.InCents
		v.Summary.MovedCents += d.MovedCents
	}
	cutoff := 31
	if today := s.now(); monthOf(today).Equal(from) {
		cutoff = today.Day()
	}
	for _, d := range prev {
		if day, _ := time.Parse(time.DateOnly, d.Date); day.Day() <= cutoff {
			v.Summary.PreviousToDateCents += d.SpentCents
		}
	}
	return v, nil
}
