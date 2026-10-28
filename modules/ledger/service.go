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
	// ReplaceSnapshot stores a day's balance whatever the one stored.
	ReplaceSnapshot(ctx context.Context, s BalanceSnapshot) error

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
	// SaveBills stores a card's closed bills as the source reports them now.
	SaveBills(ctx context.Context, account AccountID, bills []ImportedBill) error
	Bills(ctx context.Context, account AccountID) ([]Bill, error)
	// BillItems returns a card's live transactions, newest first, each with
	// its category lines.
	BillItems(ctx context.Context, account AccountID) ([]BillItem, error)
	// DescriptionMonths counts the transactions that count with a
	// description (ignoring case), by month, in [from, to).
	DescriptionMonths(ctx context.Context, description string, from, to time.Time) ([]MonthTotal, error)
	// DaySpending sums the filter's live transactions by day in [from, to),
	// whatever the filter's own month and day.
	DaySpending(ctx context.Context, f TransactionFilter, from, to time.Time) ([]DayView, error)
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

	// Possible duplicates: a transaction I added and a synced one.
	PossibleDuplicates(ctx context.Context, id TransactionID) ([][2]TransactionID, error)
	KeepApart(ctx context.Context, manual, synced TransactionID) error
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

// MarkTransfers marks transactions as transfers to accounts lastro does not
// have (or takes the mark back), together. It returns how many changed.
func (s *Service) MarkTransfers(ctx context.Context, ids []TransactionID, on bool) (int, error) {
	n := 0
	err := s.repo.Tx(ctx, func(r Repository) error {
		txs, err := r.Transactions(ctx, ids)
		if err != nil {
			return err
		}
		if len(txs) != len(ids) {
			return fmt.Errorf("%w: %d of %d transactions", ErrNotFound, len(ids)-len(txs), len(ids))
		}
		var changed []*Transaction
		for _, t := range txs {
			if t.MarkedTransfer() == on {
				continue
			}
			if err := t.MarkTransfer(on); err != nil {
				return err
			}
			changed = append(changed, t)
		}
		n = len(changed)
		return r.SaveTransactions(ctx, changed)
	})
	if err != nil {
		return 0, fmt.Errorf("ledger: mark transfers: %w", err)
	}
	return n, nil
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

// ImportBills stores a card's closed bills as a source reports them now; a
// bill read again replaces what it said before. It returns how many it
// stored.
func (s *Service) ImportBills(ctx context.Context, account ExternalID, bills []ImportedBill) (int, error) {
	acc, err := s.repo.AccountByExternalID(ctx, account)
	if err != nil {
		return 0, fmt.Errorf("ledger: import bills into %s: %w", account, err)
	}
	if !acc.Type().IsCard() {
		return 0, fmt.Errorf("ledger: import bills into %s: %w: not a credit card", account, ErrInvalid)
	}
	for _, b := range bills {
		if err := b.validate(acc.Currency()); err != nil {
			return 0, fmt.Errorf("ledger: import bills into %s: %w", account, err)
		}
	}
	if err := s.repo.SaveBills(ctx, acc.ID(), bills); err != nil {
		return 0, fmt.Errorf("ledger: import bills into %s: %w", account, err)
	}
	return len(bills), nil
}

// Bills returns a card's bills (CardBills): the next ones, the open one, then
// the closed ones, the latest first. ErrInvalid for an account that is not a
// card.
func (s *Service) Bills(ctx context.Context, id AccountID) ([]BillView, error) {
	acc, err := s.repo.AccountView(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("ledger: bills %s: %w", id, err)
	}
	if acc.Type != string(AccountCreditCard) {
		return nil, fmt.Errorf("ledger: bills %s: %w: not a credit card", id, ErrInvalid)
	}
	closed, err := s.repo.Bills(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("ledger: bills %s: %w", id, err)
	}
	items, err := s.repo.BillItems(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("ledger: bills %s: %w", id, err)
	}
	accounts, err := s.repo.AccountViews(ctx)
	if err != nil {
		return nil, fmt.Errorf("ledger: bills %s: %w", id, err)
	}
	var due time.Time
	if acc.Card != nil && acc.Card.DueDate != nil {
		due, _ = time.Parse(time.DateOnly, *acc.Card.DueDate)
	}
	bills := CardBills(closed, items, due, s.now(), money.Currency(acc.Currency))
	return billViews(bills, accounts), nil
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
	dups, err := s.duplicates(ctx, id)
	if err != nil {
		return TransactionDetail{}, fmt.Errorf("ledger: transaction %s: %w", id, err)
	}
	d.Duplicates = []TransactionView{}
	for _, p := range dups {
		other := p.Synced
		if other.ID == id {
			other = p.Manual
		}
		d.Duplicates = append(d.Duplicates, other)
	}
	return d, nil
}

// Days returns the filter's month day by day, every account together unless
// it names one, with the month before under the same filter. It is always
// the whole month: the filter's day only narrows the list. PreviousToDate
// compares like with like: for the month in progress, the month before up
// to the same day.
func (s *Service) Days(ctx context.Context, f TransactionFilter) (DaysView, error) {
	if f.Month.IsZero() || f.AllMonths {
		return DaysView{}, fmt.Errorf("%w: days are one month's", ErrInvalid)
	}
	from, to := f.MonthRange()
	prevFrom := from.AddDate(0, -1, 0)
	days, err := s.repo.DaySpending(ctx, f, from, to)
	if err != nil {
		return DaysView{}, fmt.Errorf("ledger: days %s: %w", from.Format("2006-01"), err)
	}
	prev, err := s.repo.DaySpending(ctx, f, prevFrom, from)
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
			v.Summary.PreviousInToDateCents += d.InCents
			v.Summary.PreviousMovedToDateCents += d.MovedCents
		}
	}
	return v, nil
}

// PossibleDuplicates returns every transaction I added by hand that may be
// the same movement as a synced one (PossibleDuplicate), with it, closest
// dates first: I say whether they are.
func (s *Service) PossibleDuplicates(ctx context.Context) ([]DuplicateView, error) {
	out, err := s.duplicates(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("ledger: possible duplicates: %w", err)
	}
	return out, nil
}

func (s *Service) duplicates(ctx context.Context, id TransactionID) ([]DuplicateView, error) {
	pairs, err := s.repo.PossibleDuplicates(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]DuplicateView, 0, len(pairs))
	for _, p := range pairs {
		m, err := s.repo.TransactionView(ctx, p[0])
		if err != nil {
			return nil, err
		}
		sy, err := s.repo.TransactionView(ctx, p[1])
		if err != nil {
			return nil, err
		}
		out = append(out, DuplicateView{Manual: m, Synced: sy})
	}
	return out, nil
}

// MergeDuplicate says a transaction I added and a synced one are the same
// movement: the synced one stays, with what I gave mine (see Merge), and
// mine is removed. A pair I kept apart does not merge.
func (s *Service) MergeDuplicate(ctx context.Context, manual, synced TransactionID) error {
	var evs []events.Event
	err := s.repo.Tx(ctx, func(r Repository) error {
		m, sy, err := duplicatePair(ctx, r, manual, synced)
		if err != nil {
			return err
		}
		pairs, err := r.PossibleDuplicates(ctx, manual)
		if err != nil {
			return err
		}
		if !slices.Contains(pairs, [2]TransactionID{manual, synced}) {
			return fmt.Errorf("%w: I said %s and %s are two transactions", ErrNotDuplicate, manual, synced)
		}
		mine, err := annotations(ctx, r, m.ID())
		if err != nil {
			return err
		}
		theirs, err := annotations(ctx, r, sy.ID())
		if err != nil {
			return err
		}
		merged, broken, err := Merge(m, sy, mine, theirs, s.now())
		if err != nil {
			return err
		}
		changed := []*Transaction{m, sy}
		if broken != "" {
			others, err := r.Transactions(ctx, []TransactionID{broken})
			if err != nil {
				return err
			}
			for _, o := range others {
				o.LeavePair()
				changed = append(changed, o)
			}
		}
		if len(merged.Split) > 0 && len(theirs.Split) == 0 {
			split, err := sy.SplitInto(merged.Split)
			if err != nil {
				return err
			}
			if err := r.SaveSplit(ctx, sy.ID(), split); err != nil {
				return err
			}
		}
		evs = append(m.PullEvents(), sy.PullEvents()...)
		if err := r.SaveTransactions(ctx, changed); err != nil {
			return err
		}
		if err := r.SetNote(ctx, sy.ID(), merged.Note); err != nil {
			return err
		}
		return r.SetTags(ctx, sy.ID(), merged.Tags)
	})
	if err != nil {
		return fmt.Errorf("ledger: merge %s into %s: %w", manual, synced, err)
	}
	return s.bus.Publish(ctx, evs...)
}

// KeepApart says a transaction I added and a synced one are two movements:
// both stay and they are never suggested again.
func (s *Service) KeepApart(ctx context.Context, manual, synced TransactionID) error {
	err := s.repo.Tx(ctx, func(r Repository) error {
		if _, _, err := duplicatePair(ctx, r, manual, synced); err != nil {
			return err
		}
		return r.KeepApart(ctx, manual, synced)
	})
	if err != nil {
		return fmt.Errorf("ledger: keep %s and %s apart: %w", manual, synced, err)
	}
	return nil
}

// duplicatePair loads the two and checks they may be one movement.
func duplicatePair(ctx context.Context, r Repository, manual, synced TransactionID) (*Transaction, *Transaction, error) {
	txs, err := r.Transactions(ctx, []TransactionID{manual, synced})
	if err != nil {
		return nil, nil, err
	}
	var m, sy *Transaction
	for _, t := range txs {
		switch t.ID() {
		case manual:
			m = t
		case synced:
			sy = t
		}
	}
	if m == nil || sy == nil {
		return nil, nil, fmt.Errorf("%w: transaction %s or %s", ErrNotFound, manual, synced)
	}
	if err := PossibleDuplicate(m, sy); err != nil {
		return nil, nil, err
	}
	return m, sy, nil
}

// annotations reads what I added to a transaction.
func annotations(ctx context.Context, r Repository, id TransactionID) (Annotations, error) {
	v, err := r.TransactionView(ctx, id)
	if err != nil {
		return Annotations{}, err
	}
	a := Annotations{Note: Note(v.Note)}
	for _, t := range v.Tags {
		a.Tags = append(a.Tags, Tag(t))
	}
	if len(v.SplitCategories) == 0 {
		return a, nil
	}
	parts, err := r.Split(ctx, id)
	if err != nil {
		return Annotations{}, err
	}
	for _, p := range parts {
		c, err := NewCategoryID(p.Category)
		if err != nil {
			return Annotations{}, err
		}
		amount, err := money.New(p.AmountCents, money.Currency(v.Currency))
		if err != nil {
			return Annotations{}, err
		}
		a.Split = append(a.Split, SplitPart{Category: c, Amount: amount})
	}
	return a, nil
}
