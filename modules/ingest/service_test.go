//go:build integration

package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"lastro/modules/ingest/internal/fixtures"
	"lastro/modules/ingest/internal/pluggy"
	"lastro/modules/ledger"
	"lastro/money"
	"lastro/platform/db/dbtest"
	"lastro/platform/events"
)

// fakePluggy answers from the fixtures. drop hides transactions, as if
// Pluggy had stopped returning them.
type fakePluggy struct {
	set   fixtures.Set
	drop  map[string]bool
	reads []time.Time
}

func (f *fakePluggy) Item(_ context.Context, id string) (pluggy.Item, error) {
	for _, it := range f.set.Items {
		if it.ID == id {
			return it, nil
		}
	}
	return pluggy.Item{}, &pluggy.Error{Status: 404, Message: "item not found"}
}

func (f *fakePluggy) Accounts(_ context.Context, itemID string) ([]pluggy.Account, error) {
	return f.set.Accounts[itemID], nil
}

func (f *fakePluggy) Transactions(_ context.Context, accountID string, from time.Time) ([]pluggy.Transaction, error) {
	f.reads = append(f.reads, from)
	var out []pluggy.Transaction
	for _, t := range f.set.Transactions[accountID] {
		d, _ := parseDate(t.Date)
		if f.drop[t.ID] || (!from.IsZero() && d.Before(from.Truncate(24*time.Hour))) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func (f *fakePluggy) Investments(_ context.Context, itemID string) ([]pluggy.Investment, error) {
	return f.set.Investments[itemID], nil
}

func newSync(t *testing.T) (*Service, *fakePluggy, *ledger.Service) {
	t.Helper()
	pool := dbtest.New(t)
	set, err := fixtures.Load("testdata/pluggy")
	if err != nil {
		t.Fatal(err)
	}
	led := ledger.New(ledger.NewRepository(pool), nil)
	fake := &fakePluggy{set: set, drop: map[string]bool{}}
	s := &Service{
		repo:    NewRepository(pool),
		pluggy:  fake,
		ledger:  led,
		itemIDs: []string{"a0000000-0000-4000-8000-000000000001", "a0000000-0000-4000-8000-000000000002"},
		bus:     nil,
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		now:     func() time.Time { return time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC) },
	}
	s.bus = events.NewBus(nil)
	return s, fake, led
}

func TestSyncEndToEnd(t *testing.T) {
	ctx := context.Background()
	s, fake, led := newSync(t)

	rep, err := s.Sync(ctx, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Items) != 2 || rep.Items[0].Institution != "Nubank" || rep.Items[0].Accounts != 2 || rep.Items[0].Created != 14 || rep.Items[1].Created != 4 {
		t.Fatalf("first sync: %+v", rep.Items)
	}
	if rep.Items[0].Investments != 1 {
		t.Errorf("investments are stored: %+v", rep.Items[0])
	}
	if fake.reads[0] != (time.Time{}) {
		t.Errorf("the first sync reads everything, read from %v", fake.reads[0])
	}
	if rep.Paired != 2 {
		t.Errorf("the Pix from Nubank to Itaú and the card bill paid from checking pair: %d", rep.Paired)
	}

	// The next sync reads a window and changes nothing.
	fake.reads = nil
	rep, err = s.Sync(ctx, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Items[0].Unchanged != 14 || rep.Items[0].Created+rep.Items[0].Updated+rep.Items[0].Removed != 0 {
		t.Fatalf("second sync must change nothing: %+v", rep.Items[0])
	}
	if want := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC); !fake.reads[0].Equal(want) {
		t.Errorf("incremental reads start 90 days back: %v, want %v", fake.reads[0], want)
	}

	// The pending Uber ride vanishes from Pluggy.
	fake.drop["c0000000-0000-4000-8000-000000000010"] = true
	rep, err = s.Sync(ctx, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Items[0].Removed != 1 {
		t.Fatalf("a vanished pending purchase is removed: %+v", rep.Items[0])
	}

	// Reprocessing from the raw records reproduces the same ledger.
	rep, err = s.Sync(ctx, SyncOptions{Reprocess: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := rep.Items[0]; got.Created+got.Updated+got.Removed != 0 || got.Unchanged != 13 {
		t.Fatalf("reprocess must change nothing: %+v", got)
	}

	// A transaction the raw records do not hold (imported under an older
	// rule, say) goes on the next reprocess, whatever its date.
	var card pluggy.Account
	for _, a := range fake.set.Accounts["a0000000-0000-4000-8000-000000000001"] {
		if a.Type == "CREDIT" {
			card = a
		}
	}
	ext, _ := ledger.NewExternalID(card.ID)
	stray, _ := ledger.NewExternalID("stray-1")
	if _, err := led.ImportTransactions(ctx, ledger.ImportBatch{Account: ext, Transactions: []ledger.ImportedTransaction{{
		ExternalID: stray, Date: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), Description: "Pagamento recebido",
		Amount: money.BRLCents(148237), Kind: ledger.KindBillPayment, Status: ledger.StatusPosted,
	}}}); err != nil {
		t.Fatal(err)
	}
	rep, err = s.Sync(ctx, SyncOptions{Reprocess: true})
	if err != nil {
		t.Fatal(err)
	}
	if removed := rep.Items[0].Removed; removed != 1 {
		t.Fatalf("the stray transaction is removed: %+v", rep.Items[0])
	}

	conns, err := s.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 2 {
		t.Fatalf("want 2 connections, got %+v", conns)
	}
	byInst := map[string]ConnectionView{}
	for _, c := range conns {
		byInst[c.Institution] = c
	}
	if c := byInst["Nubank"]; c.Health != HealthOK || len(c.AccountIDs) != 2 {
		t.Errorf("Nubank: %+v", c)
	}
	if c := byInst["Itaú"]; c.Health != HealthStale || c.Detail != "Last synced 3 days ago" || len(c.AccountIDs) != 1 {
		t.Errorf("Itaú's data is from 23 Oct: %+v", c)
	}

	s.itemIDs = nil // serve started before its Secret existed
	if conns, err := s.Connections(ctx); err != nil || len(conns) != 2 || len(conns[0].AccountIDs) == 0 {
		t.Fatalf("every item behind an account still shows: %+v %v", conns, err)
	}
}

func TestSyncRecordsFailures(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newSync(t)
	s.itemIDs = append(s.itemIDs, "a0000000-0000-4000-8000-00000000dead")
	rep, err := s.Sync(ctx, SyncOptions{})
	if err == nil || len(rep.Items) != 3 || rep.Items[2].Err == nil || rep.Items[0].Err != nil {
		t.Fatalf("one bad item fails the sync but not the others: %v %+v", err, rep.Items)
	}
	st, err := s.repo.item(ctx, "a0000000-0000-4000-8000-00000000dead")
	if err != nil || st.SyncError == "" || !st.SyncedAt.IsZero() {
		t.Fatalf("the failure is recorded: %+v %v", st, err)
	}
}

func TestSyncRefusesAnExpiredAuthorization(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newSync(t)
	s.now = func() time.Time { return time.Date(2027, 10, 1, 0, 0, 0, 0, time.UTC) }
	rep, err := s.Sync(ctx, SyncOptions{})
	if err == nil || rep.Items[0].Accounts != 0 {
		t.Fatalf("an expired authorization reads nothing: %v %+v", err, rep.Items)
	}
	conns, _ := s.Connections(ctx)
	if conns[0].Health != HealthReauth {
		t.Errorf("and the connection says so: %+v", conns[0])
	}
}

func TestOnlyOneSyncAtATime(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newSync(t)
	unlock, ok, err := s.repo.lock(ctx)
	if err != nil || !ok {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := s.Sync(ctx, SyncOptions{}); !errors.Is(err, ErrSyncRunning) {
		t.Fatalf("want ErrSyncRunning, got %v", err)
	}
}

func TestSyncMeuPluggyFixtures(t *testing.T) {
	ctx := context.Background()
	s, fake, _ := newSync(t)
	set, err := fixtures.Load("testdata/meupluggy")
	if err != nil {
		t.Fatal(err)
	}
	fake.set = set
	s.itemIDs = []string{set.Items[0].ID}
	s.now = func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) }

	rep, err := s.Sync(ctx, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	it := rep.Items[0]
	if it.Institution != "Nubank" || it.Accounts != 2 || it.Created != 549 || it.Skipped != 0 || it.Investments != 115 || rep.Paired != 22 {
		t.Fatalf("first sync of the real connection: %+v, paired %d", it, rep.Paired)
	}
	rep, err = s.Sync(ctx, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := rep.Items[0]; got.Created+got.Updated+got.Removed != 0 || rep.Paired != 0 {
		t.Fatalf("a second sync of the same data changes nothing: %+v, paired %d", got, rep.Paired)
	}
	conns, err := s.Connections(ctx)
	if err != nil || len(conns) != 1 || conns[0].Institution != "Nubank" || len(conns[0].AccountIDs) != 2 {
		t.Fatalf("the connection is named after the bank: %+v %v", conns, err)
	}
}

// The read models the account page and the transaction detail use, on my
// real connection. Nothing here depends on today's date.
func TestReadModelsOnMeuPluggyFixtures(t *testing.T) {
	ctx := context.Background()
	s, fake, led := newSync(t)
	set, err := fixtures.Load("testdata/meupluggy")
	if err != nil {
		t.Fatal(err)
	}
	fake.set = set
	s.itemIDs = []string{set.Items[0].ID}
	s.now = func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) }
	if _, err := s.Sync(ctx, SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	accounts, err := led.Accounts(ctx)
	if err != nil || len(accounts) != 2 {
		t.Fatalf("accounts: %+v %v", accounts, err)
	}
	var checking, card ledger.AccountView
	for _, a := range accounts {
		if a.Type == "credit_card" {
			card = a
		} else {
			checking = a
		}
	}

	// Card details from creditData: a limit and a due day, no closing day.
	if card.Card == nil || card.Card.LimitCents == nil || card.Card.AvailableCents == nil || card.Card.DueDate == nil || card.Card.CloseDate != nil {
		t.Errorf("card details: %+v", card.Card)
	}
	if checking.Card != nil {
		t.Errorf("checking has no card details: %+v", checking.Card)
	}

	// The rebuilt balance ends at the bank's and starts at the first transaction.
	h, err := led.BalanceHistory(ctx, checking.ID)
	if err != nil {
		t.Fatal(err)
	}
	if last := h.Points[len(h.Points)-1]; last.BalanceCents != checking.BalanceCents || last.Date != checking.BalanceAt.Format(time.DateOnly) {
		t.Errorf("the last point is the bank's balance: %+v, want %d on %s", last, checking.BalanceCents, checking.BalanceAt.Format(time.DateOnly))
	}
	if h.Points[0].Date != "2025-03-21" {
		t.Errorf("history starts with checking's first transaction: %s", h.Points[0].Date)
	}

	// October's spending is the same seen by account and by day.
	var spent int64
	for _, a := range accounts {
		months, err := led.Months(ctx, a.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range months {
			if m.Month == "2026-10" {
				spent += m.SpentCents
			}
		}
	}
	days, err := led.Days(ctx, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if spent == 0 || days.Summary.SpentCents != spent || len(days.Previous) == 0 {
		t.Errorf("October spending: by account %d, by day %d (%d days before)", spent, days.Summary.SpentCents, len(days.Previous))
	}

	// Purchases in progress add up, and the one ending first comes first.
	purchases, err := led.Installments(ctx, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range purchases {
		if p.RemainingCount != p.Total-p.Current || p.RemainingCount <= 0 || p.RemainingCents != int64(p.RemainingCount)*p.AmountCents {
			t.Errorf("purchase %s: %+v", p.Description, p)
		}
		if i > 0 && p.LastDate < purchases[i-1].LastDate {
			t.Errorf("by last installment: %s before %s", purchases[i-1].LastDate, p.LastDate)
		}
	}

	// A paired bill payment shows its other side; an installment, its purchase.
	var bill *ledger.TransactionView
	for m := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC); bill == nil && m.Year() >= 2025; m = m.AddDate(0, -1, 0) {
		list, err := led.Transactions(ctx, ledger.TransactionFilter{Month: m, Account: checking.ID, Flow: ledger.FlowTransfer})
		if err != nil {
			t.Fatal(err)
		}
		for i, tx := range list.Transactions {
			if tx.Kind == "bill_payment" {
				bill = &list.Transactions[i]
				break
			}
		}
	}
	if bill == nil {
		t.Fatal("no bill payment from checking")
	}
	d, err := led.Transaction(ctx, bill.ID)
	if err != nil || d.Counterpart == nil || d.Counterpart.AmountCents != -bill.AmountCents || d.Counterpart.Account.ID != card.ID {
		t.Fatalf("the bill payment's other side is on the card: %+v %v", d.Counterpart, err)
	}
	if len(d.History.Months) != 6 {
		t.Errorf("six months of history: %+v", d.History)
	}
	if len(purchases) > 0 {
		inst := purchases[0].Installments[0].ID
		d, err := led.Transaction(ctx, inst)
		if err != nil || d.Purchase == nil || d.Purchase.Description != purchases[0].Description {
			t.Errorf("an installment shows its purchase: %+v %v", d.Purchase, err)
		}
	}
}
