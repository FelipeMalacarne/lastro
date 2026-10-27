package ledger

import (
	"testing"

	"lastro/money"
)

// importOnce runs reconcile and returns the transactions the ledger would
// have afterwards (the existing ones plus the new ones), keyed by external id.
func importOnce(t *testing.T, acc *Account, existing []*Transaction, b ImportBatch) (reconciliation, map[string]*Transaction) {
	t.Helper()
	rec, err := reconcile(acc, existing, b, oct(27))
	if err != nil {
		t.Fatal(err)
	}
	all := map[string]*Transaction{}
	for _, x := range append(existing, rec.save...) {
		all[x.ExternalID().String()] = x
	}
	return rec, all
}

func values(m map[string]*Transaction) []*Transaction {
	out := make([]*Transaction, 0, len(m))
	for _, t := range m {
		out = append(out, t)
	}
	return out
}

func TestReconcileIsIdempotent(t *testing.T) {
	acc := checking(t)
	b := ImportBatch{Account: acc.ExternalID(), From: oct(1), Transactions: []ImportedTransaction{
		imported(t, "t1", -4890), imported(t, "t2", -2310),
	}}
	rec, all := importOnce(t, acc, nil, b)
	if rec.result != (ImportResult{Created: 2}) || len(rec.touched) != 2 {
		t.Fatalf("first import: %+v", rec.result)
	}
	rec, _ = importOnce(t, acc, values(all), b)
	if rec.result != (ImportResult{Unchanged: 2}) || len(rec.save) != 0 || len(rec.touched) != 0 {
		t.Fatalf("the same batch again changes nothing: %+v", rec.result)
	}
}

func TestReconcileUpdatesRemovesAndRestores(t *testing.T) {
	acc := checking(t)
	pending := imported(t, "p1", -2310)
	pending.Status = StatusPending
	old := imported(t, "old", -1000)
	old.Date = oct(1) // before the window of the second import
	_, all := importOnce(t, acc, nil, ImportBatch{Account: acc.ExternalID(), From: oct(1), Transactions: []ImportedTransaction{
		pending, imported(t, "t2", -100), old,
	}})

	// Next sync reads from 10 Oct: p1 settled with another amount, t2 vanished.
	settled := pending
	settled.Status, settled.Amount = StatusPosted, money.BRLCents(-2290)
	rec, all := importOnce(t, acc, values(all), ImportBatch{Account: acc.ExternalID(), From: oct(10), Transactions: []ImportedTransaction{settled}})
	if rec.result != (ImportResult{Updated: 1, Removed: 1}) {
		t.Fatalf("got %+v", rec.result)
	}
	if !all["t2"].IsRemoved() || all["p1"].Status() != StatusPosted || all["old"].IsRemoved() {
		t.Fatal("t2 must be removed, p1 posted, and old (outside the window) kept")
	}

	// t2 comes back a few days later.
	rec, all = importOnce(t, acc, values(all), ImportBatch{Account: acc.ExternalID(), From: oct(10), Transactions: []ImportedTransaction{settled, imported(t, "t2", -100)}})
	if rec.result != (ImportResult{Unchanged: 1, Updated: 1}) || all["t2"].IsRemoved() {
		t.Fatalf("a transaction that comes back is restored: %+v", rec.result)
	}
}

func TestReconcileNeverRemovesOnAnEmptyRead(t *testing.T) {
	acc := checking(t)
	_, all := importOnce(t, acc, nil, ImportBatch{Account: acc.ExternalID(), From: oct(1), Transactions: []ImportedTransaction{imported(t, "t1", -100)}})
	rec, all := importOnce(t, acc, values(all), ImportBatch{Account: acc.ExternalID(), From: oct(1)})
	if rec.result.Removed != 0 || all["t1"].IsRemoved() {
		t.Fatal("an empty read must not remove anything")
	}
	rec, _ = importOnce(t, acc, values(all), ImportBatch{Account: acc.ExternalID(), Transactions: []ImportedTransaction{imported(t, "t9", -1)}})
	if rec.result.Removed != 0 {
		t.Fatal("a batch without a window must not remove anything")
	}
}

func TestReconcileRekeysByBankRef(t *testing.T) {
	acc := checking(t)
	_, all := importOnce(t, acc, nil, ImportBatch{Account: acc.ExternalID(), From: oct(1), Transactions: []ImportedTransaction{imported(t, "t1", -4890)}})
	first := all["t1"]
	if err := first.Categorize(mustCategory(t, "restaurants"), SourceManual); err != nil {
		t.Fatal(err)
	}

	// The source deleted t1 and created it again as t1b, with the same bank reference.
	again := imported(t, "t1b", -4890)
	again.BankRef = first.BankRef()
	rec, all := importOnce(t, acc, values(all), ImportBatch{Account: acc.ExternalID(), From: oct(1), Transactions: []ImportedTransaction{again}})
	if rec.result != (ImportResult{Updated: 1}) {
		t.Fatalf("got %+v", rec.result)
	}
	got := all["t1b"]
	if got != first || got.IsRemoved() {
		t.Fatal("t1b must be the same transaction as t1, re-keyed")
	}
	if c, src := got.Category(); c.String() != "restaurants" || src != SourceManual {
		t.Errorf("the manual category must survive the re-key, got %q by %q", c, src)
	}
}

func TestReconcileTwoTransactionsSharingABankRef(t *testing.T) {
	acc := checking(t)
	a, b := imported(t, "a", -100), imported(t, "b", -100)
	b.BankRef = a.BankRef
	rec, _ := importOnce(t, acc, nil, ImportBatch{Account: acc.ExternalID(), From: oct(1), Transactions: []ImportedTransaction{a}})
	_, all := importOnce(t, acc, rec.save, ImportBatch{Account: acc.ExternalID(), From: oct(1), Transactions: []ImportedTransaction{a, b}})
	if len(all) != 2 || all["a"] == all["b"] {
		t.Fatal("when the old id is still in the batch, the new one is another transaction")
	}
}

func TestReconcileBreaksPairs(t *testing.T) {
	acc, sav := checking(t), func() *Account {
		d := details(t)
		d.ExternalID, d.Type = mustExternalID(t, "sav"), AccountSavings
		a, _ := NewAccount(d)
		return a
	}()
	out := newTx(t, acc, imported(t, "out", -50000))
	in := newTx(t, sav, imported(t, "in", 50000))
	if err := Pair(out, in); err != nil {
		t.Fatal(err)
	}
	// The outflow vanishes from the checking account.
	rec, err := reconcile(acc, []*Transaction{out}, ImportBatch{Account: acc.ExternalID(), From: oct(1), Transactions: []ImportedTransaction{imported(t, "other", -1)}}, oct(27))
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsRemoved() || out.IsPaired() || len(rec.broken) != 1 || rec.broken[0] != in.ID() {
		t.Fatalf("the service must be told to unpair the savings side: %+v", rec.broken)
	}
}

func TestReconcileDuplicatesInABatch(t *testing.T) {
	acc := checking(t)
	first, second := imported(t, "t1", -100), imported(t, "t1", -200)
	rec, all := importOnce(t, acc, nil, ImportBatch{Account: acc.ExternalID(), Transactions: []ImportedTransaction{first, second}})
	if rec.result.Created != 1 || all["t1"].Amount() != money.BRLCents(-200) {
		t.Fatalf("the last occurrence wins: %+v", rec.result)
	}
}
