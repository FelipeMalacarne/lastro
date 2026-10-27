//go:build integration

package ledger

import (
	"context"
	"errors"
	"maps"
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

func TestRepositoryTransactionViews(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newService(t)
	chk, card := details(t), details(t)
	card.ExternalID, card.Type, card.Name = mustExternalID(t, "card"), AccountCreditCard, "Ultravioleta"
	for _, d := range []AccountDetails{chk, card} {
		if _, err := svc.SyncAccount(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	bread := imported(t, "bread", -1250)
	bread.Description = "Padaria São João"
	inst := imported(t, "inst", -31990)
	inst.Installment, _ = NewInstallment(3, 10)
	pay := imported(t, "pay", -148237)
	pay.Date = oct(10)
	nov := imported(t, "nov", -100)
	nov.Date = oct(31).AddDate(0, 0, 1)
	if _, err := svc.ImportTransactions(ctx, ImportBatch{Account: chk.ExternalID, Transactions: []ImportedTransaction{bread, pay, nov}}); err != nil {
		t.Fatal(err)
	}
	cardPay := imported(t, "card-pay", 148237)
	cardPay.Date, cardPay.Kind = oct(10), KindBillPayment
	if _, err := svc.ImportTransactions(ctx, ImportBatch{Account: card.ExternalID, Transactions: []ImportedTransaction{inst, cardPay}}); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.MatchTransfers(ctx, oct(1)); err != nil || n != 1 {
		t.Fatalf("the bill payment pairs: %d %v", n, err)
	}

	list, err := svc.Transactions(ctx, TransactionFilter{Month: oct(1)})
	if err != nil {
		t.Fatal(err)
	}
	if list.Summary != (ListSummary{Count: 4, InCents: 0, OutCents: -33240}) || list.Truncated {
		t.Fatalf("October without November, and without the paid bill: %+v", list.Summary)
	}
	if list.Transactions[0].Date != "2026-10-12" || list.Transactions[3].Date != "2026-10-10" {
		t.Errorf("newest first: %+v", list.Transactions)
	}
	var payView TransactionView
	for _, v := range list.Transactions {
		if v.AmountCents == -148237 {
			payView = v
		}
		if v.AmountCents == -31990 && (v.Installment == nil || v.Installment.Number != 3 || v.Account.Name != "Ultravioleta") {
			t.Errorf("installment view: %+v", v)
		}
	}
	if payView.Kind != "bill_payment" || payView.Counterpart == nil || payView.Counterpart.Type != "credit_card" {
		t.Errorf("the checking side names the card it paid: %+v", payView)
	}

	search, _ := svc.Transactions(ctx, TransactionFilter{Month: oct(1), Search: "sao joao"})
	if len(search.Transactions) != 1 || search.Transactions[0].Description != "Padaria São João" {
		t.Errorf("search ignores case and accents: %+v", search.Transactions)
	}
	out, _ := svc.Transactions(ctx, TransactionFilter{Month: oct(1), Flow: FlowTransfer})
	if len(out.Transactions) != 2 {
		t.Errorf("both sides of the bill payment are transfers in the list: %+v", out.Transactions)
	}
	a, _ := repo.AccountByExternalID(ctx, card.ExternalID)
	only, _ := svc.Transactions(ctx, TransactionFilter{Month: oct(1), Account: a.ID()})
	if len(only.Transactions) != 2 {
		t.Errorf("one account: %+v", only.Transactions)
	}
	accounts, err := svc.Accounts(ctx)
	if err != nil || len(accounts) != 2 || accounts[0].Type != "checking" {
		t.Fatalf("checking first: %+v %v", accounts, err)
	}
}

func TestRepositoryCategorizableAndUncategorized(t *testing.T) {
	ctx := context.Background()
	svc, repo, published := newService(t)
	acc := details(t)
	if _, err := svc.SyncAccount(ctx, acc); err != nil {
		t.Fatal(err)
	}
	salary := imported(t, "salary", 1288000)
	salary.Kind, salary.Description = KindIncome, "SALARIO"
	xfer := imported(t, "xfer", -50000)
	xfer.Kind = KindTransfer
	gone := imported(t, "gone", -100)
	b := ImportBatch{Account: acc.ExternalID, From: oct(1), Transactions: []ImportedTransaction{
		imported(t, "food", -4890), imported(t, "chosen", -2310), imported(t, "ruled", -900), salary, xfer, gone,
	}}
	if _, err := svc.ImportTransactions(ctx, b); err != nil {
		t.Fatal(err)
	}
	b.Transactions = b.Transactions[:5]
	if _, err := svc.ImportTransactions(ctx, b); err != nil {
		t.Fatal(err)
	}
	a, _ := repo.AccountByExternalID(ctx, acc.ExternalID)
	stored, _ := repo.TransactionsForImport(ctx, a.ID(), oct(1), nil, nil)
	byExt := map[string]TransactionID{}
	for _, x := range stored {
		byExt[x.ExternalID().String()] = x.ID()
	}
	if err := svc.Categorize(ctx, byExt["chosen"], mustCategory(t, "leisure"), SourceManual); err != nil {
		t.Fatal(err)
	}
	if err := svc.Categorize(ctx, byExt["ruled"], mustCategory(t, "health"), SourceRule); err != nil {
		t.Fatal(err)
	}

	ids := func(cs []Categorizable) map[TransactionID]bool {
		out := map[TransactionID]bool{}
		for _, c := range cs {
			if c.Account != a.ID() || c.Description == "" {
				t.Errorf("incomplete: %+v", c)
			}
			out[c.ID] = true
		}
		return out
	}
	got, err := svc.Categorizable(ctx, (*published)[0].TransactionIDs)
	if err != nil {
		t.Fatal(err)
	}
	if want := (map[TransactionID]bool{byExt["food"]: true, byExt["ruled"]: true, byExt["salary"]: true}); !maps.Equal(ids(got), want) {
		t.Errorf("expenses and income, a rule's category included; never a transfer, a removed one or a manual choice: %+v", got)
	}
	got, err = svc.Uncategorized(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := (map[TransactionID]bool{byExt["food"]: true, byExt["salary"]: true}); !maps.Equal(ids(got), want) {
		t.Errorf("only the ones without a category: %+v", got)
	}
	if got, err := svc.Categorizable(ctx, nil); got != nil || err != nil {
		t.Errorf("no ids, nothing: %+v %v", got, err)
	}
}
