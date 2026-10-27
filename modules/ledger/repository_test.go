//go:build integration

package ledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"lastro/money"
	"lastro/platform/db/dbtest"
	"lastro/platform/events"
)

func newService(t *testing.T) (*Service, *PostgresRepository, *[]TransactionsImported) {
	t.Helper()
	repo := NewRepository(dbtest.New(t))
	bus := events.NewBus(nil)
	var got []TransactionsImported
	events.Subscribe(bus, func(_ context.Context, e TransactionsImported) error { got = append(got, e); return nil })
	svc := New(repo, bus)
	svc.now = func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) }
	return svc, repo, &got
}

func TestRepositoryAccountRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newService(t)
	d := details(t)
	id, err := svc.SyncAccount(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	d.Balance, d.BalanceAt = money.BRLCents(-500), d.BalanceAt.Add(time.Hour)
	again, err := svc.SyncAccount(ctx, d)
	if err != nil || again != id {
		t.Fatalf("syncing the same account keeps its id: %q %q %v", id, again, err)
	}
	a, err := repo.AccountByExternalID(ctx, d.ExternalID)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID() != id || a.Balance() != money.BRLCents(-500) || a.Number() != "5432" || a.Type() != AccountChecking || !a.BalanceAt().Equal(d.BalanceAt) {
		t.Fatalf("unexpected account: %+v", a)
	}
	if _, err := repo.AccountByExternalID(ctx, mustExternalID(t, "nope")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestRepositoryImportIsIdempotent(t *testing.T) {
	ctx := context.Background()
	svc, repo, published := newService(t)
	acc := details(t)
	accID, err := svc.SyncAccount(ctx, acc)
	if err != nil {
		t.Fatal(err)
	}

	inst := imported(t, "t2", -31990)
	inst.Installment, _ = NewInstallment(3, 10)
	pending := imported(t, "t3", -2310)
	pending.Status = StatusPending
	b := ImportBatch{Account: acc.ExternalID, From: oct(1), Transactions: []ImportedTransaction{imported(t, "t1", -4890), inst, pending}}

	res, err := svc.ImportTransactions(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if res != (ImportResult{Created: 3}) || len(*published) != 1 || len((*published)[0].TransactionIDs) != 3 {
		t.Fatalf("first import: %+v, events %+v", res, *published)
	}
	res, err = svc.ImportTransactions(ctx, b)
	if err != nil || res != (ImportResult{Unchanged: 3}) || len(*published) != 1 {
		t.Fatalf("second import must change nothing and publish nothing: %+v %v", res, err)
	}

	txs, err := repo.TransactionsForImport(ctx, accID, oct(1), nil, nil)
	if err != nil || len(txs) != 3 {
		t.Fatalf("want 3 stored transactions, got %d (%v)", len(txs), err)
	}
	byExt := map[string]*Transaction{}
	for _, x := range txs {
		byExt[x.ExternalID().String()] = x
	}
	if got := byExt["t2"]; got.Installment().String() != "3 of 10" || got.Amount() != money.BRLCents(-31990) || !got.Date().Equal(oct(12)) {
		t.Fatalf("round trip lost data: %+v", got)
	}

	// t3 settles and t1 disappears.
	pending.Status = StatusPosted
	res, err = svc.ImportTransactions(ctx, ImportBatch{Account: acc.ExternalID, From: oct(1), Transactions: []ImportedTransaction{inst, pending}})
	if err != nil || res != (ImportResult{Unchanged: 1, Updated: 1, Removed: 1}) {
		t.Fatalf("got %+v %v", res, err)
	}
	gone, err := repo.Transactions(ctx, []TransactionID{byExt["t1"].ID()})
	if err != nil || len(gone) != 1 || !gone[0].IsRemoved() {
		t.Fatalf("t1 must be stored as removed: %+v %v", gone, err)
	}
}

func TestRepositoryRekeyKeepsTheManualCategory(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newService(t)
	acc := details(t)
	if _, err := svc.SyncAccount(ctx, acc); err != nil {
		t.Fatal(err)
	}
	first := imported(t, "t1", -4890)
	if _, err := svc.ImportTransactions(ctx, ImportBatch{Account: acc.ExternalID, From: oct(1), Transactions: []ImportedTransaction{first}}); err != nil {
		t.Fatal(err)
	}
	a, _ := repo.AccountByExternalID(ctx, acc.ExternalID)
	stored, _ := repo.TransactionsForImport(ctx, a.ID(), oct(1), nil, nil)
	if err := svc.Categorize(ctx, stored[0].ID(), mustCategory(t, "restaurants"), SourceManual); err != nil {
		t.Fatal(err)
	}
	if err := svc.Categorize(ctx, stored[0].ID(), mustCategory(t, "health"), SourceLLM); !errors.Is(err, ErrManualCategory) {
		t.Fatalf("want ErrManualCategory, got %v", err)
	}

	recreated := imported(t, "t1-new", -4890)
	recreated.BankRef = first.BankRef
	res, err := svc.ImportTransactions(ctx, ImportBatch{Account: acc.ExternalID, From: oct(1), Transactions: []ImportedTransaction{recreated}})
	if err != nil || res != (ImportResult{Updated: 1}) {
		t.Fatalf("got %+v %v", res, err)
	}
	after, _ := repo.Transactions(ctx, []TransactionID{stored[0].ID()})
	if c, src := after[0].Category(); after[0].ExternalID().String() != "t1-new" || c.String() != "restaurants" || src != SourceManual {
		t.Fatalf("re-keyed transaction: %+v", after[0])
	}
}

func TestRepositoryPairsSurviveARoundTripAndBreak(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newService(t)
	chk, sav := details(t), details(t)
	sav.ExternalID, sav.Type = mustExternalID(t, "sav"), AccountSavings
	for _, d := range []AccountDetails{chk, sav} {
		if _, err := svc.SyncAccount(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.ImportTransactions(ctx, ImportBatch{Account: chk.ExternalID, From: oct(1), Transactions: []ImportedTransaction{imported(t, "out", -50000)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportTransactions(ctx, ImportBatch{Account: sav.ExternalID, From: oct(1), Transactions: []ImportedTransaction{imported(t, "in", 50000)}}); err != nil {
		t.Fatal(err)
	}
	ca, _ := repo.AccountByExternalID(ctx, chk.ExternalID)
	sa, _ := repo.AccountByExternalID(ctx, sav.ExternalID)
	outs, _ := repo.TransactionsForImport(ctx, ca.ID(), oct(1), nil, nil)
	ins, _ := repo.TransactionsForImport(ctx, sa.ID(), oct(1), nil, nil)
	if err := Pair(outs[0], ins[0]); err != nil {
		t.Fatal(err)
	}
	if err := repo.Tx(ctx, func(r Repository) error { return r.SaveTransactions(ctx, []*Transaction{outs[0], ins[0]}) }); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Transactions(ctx, []TransactionID{outs[0].ID(), ins[0].ID()})
	for _, x := range got {
		if !x.IsPaired() || x.Kind() != KindTransfer {
			t.Fatalf("pair lost in the round trip: %+v", x)
		}
	}

	// The checking side vanishes: the savings side must leave the pair too.
	if _, err := svc.ImportTransactions(ctx, ImportBatch{Account: chk.ExternalID, From: oct(1), Transactions: []ImportedTransaction{imported(t, "other", -1)}}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Transactions(ctx, []TransactionID{ins[0].ID()})
	if got[0].IsPaired() || got[0].Kind() != KindExpense {
		t.Fatalf("the savings side must be unpaired: %+v", got[0])
	}
}

func TestRepositorySnapshotsOnePerDay(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newService(t)
	d := details(t)
	morning := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)
	sync := func(at time.Time, cents int64) AccountID {
		t.Helper()
		d.BalanceAt, d.Balance = at, money.BRLCents(cents)
		id, err := svc.SyncAccount(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	id := sync(morning, 100)
	sync(morning.Add(6*time.Hour), 200)  // later the same day: replaces
	sync(morning.Add(-time.Hour), 50)    // an older read: ignored
	sync(morning.Add(24*time.Hour), 300) // the next day: a new snapshot
	snaps, err := repo.Snapshots(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 || snaps[0].Balance != money.BRLCents(200) || snaps[1].Balance != money.BRLCents(300) ||
		!snaps[0].Date.Equal(oct(26)) || !snaps[1].Date.Equal(oct(27)) {
		t.Fatalf("want 26 Oct at R$ 2,00 and 27 Oct at R$ 3,00, got %+v", snaps)
	}
}
