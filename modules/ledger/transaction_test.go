package ledger

import (
	"errors"
	"strings"
	"testing"
	"time"

	"lastro/money"
)

var oct = func(day int) time.Time { return time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC) }

func imported(t *testing.T, ext string, cents int64) ImportedTransaction {
	t.Helper()
	return ImportedTransaction{
		ExternalID:  mustExternalID(t, ext),
		BankRef:     "bank-" + ext,
		Date:        oct(12),
		Description: "iFood",
		Amount:      money.BRLCents(cents),
		Kind:        KindExpense,
		Status:      StatusPosted,
	}
}

func newTx(t *testing.T, acc *Account, in ImportedTransaction) *Transaction {
	t.Helper()
	tx, err := NewTransaction(acc, in)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func checking(t *testing.T) *Account {
	a, err := NewAccount(details(t))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func card(t *testing.T) *Account {
	d := details(t)
	d.ExternalID = mustExternalID(t, "card-1")
	d.Type = AccountCreditCard
	d.Balance = money.BRLCents(-153020)
	a, err := NewAccount(d)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestNewTransaction(t *testing.T) {
	acc := checking(t)
	in := imported(t, "t1", -4890)
	in.Date = time.Date(2026, 10, 12, 22, 30, 0, 0, time.FixedZone("BRT", -3*3600))
	in.Description = "  iFood  "
	tx := newTx(t, acc, in)
	if tx.ID() == "" || tx.AccountID() != acc.ID() || tx.Description() != "iFood" {
		t.Fatalf("unexpected transaction: %+v", tx)
	}
	if got := tx.Date(); !got.Equal(oct(12)) {
		t.Errorf("the date is the calendar day the source gave, got %v", got)
	}
	if tx.Kind() != KindExpense || tx.IsPaired() || tx.IsRemoved() {
		t.Errorf("unexpected state: %+v", tx)
	}

	bad := []struct {
		name string
		edit func(*ImportedTransaction)
		want error
	}{
		{"no description", func(i *ImportedTransaction) { i.Description = " " }, ErrInvalid},
		{"no date", func(i *ImportedTransaction) { i.Date = time.Time{} }, ErrInvalid},
		{"no external id", func(i *ImportedTransaction) { i.ExternalID = ExternalID{} }, ErrInvalid},
		{"no kind", func(i *ImportedTransaction) { i.Kind = "" }, ErrInvalid},
		{"no status", func(i *ImportedTransaction) { i.Status = "" }, ErrInvalid},
		{"other currency", func(i *ImportedTransaction) { i.Amount, _ = money.New(-100, money.USD) }, ErrCurrencyChanged},
	}
	for _, b := range bad {
		in := imported(t, "t2", -100)
		b.edit(&in)
		if _, err := NewTransaction(acc, in); !errors.Is(err, b.want) {
			t.Errorf("%s: want %v, got %v", b.name, b.want, err)
		}
	}

	long := imported(t, "t3", -100)
	long.Description = strings.Repeat("á", 600)
	if tx := newTx(t, acc, long); len([]rune(tx.Description())) != 500 {
		t.Errorf("descriptions are cut at 500 characters, got %d", len([]rune(tx.Description())))
	}
}

func TestApplyImport(t *testing.T) {
	acc := checking(t)
	in := imported(t, "t1", -2310)
	in.Status = StatusPending
	tx := newTx(t, acc, in)

	if changed, _, err := tx.ApplyImport(in); err != nil || changed {
		t.Fatalf("the same data again changes nothing: %v %v", changed, err)
	}

	posted := in
	posted.Status = StatusPosted
	posted.Amount = money.BRLCents(-2290)
	changed, _, err := tx.ApplyImport(posted)
	if err != nil || !changed || tx.Status() != StatusPosted || tx.Amount() != money.BRLCents(-2290) {
		t.Fatalf("pending → posted with a new amount must apply: %v %v %+v", changed, err, tx)
	}

	usd := posted
	usd.Amount, _ = money.New(-2290, money.USD)
	if _, _, err := tx.ApplyImport(usd); !errors.Is(err, ErrCurrencyChanged) {
		t.Errorf("want ErrCurrencyChanged, got %v", err)
	}

	rekeyed := posted
	rekeyed.ExternalID = mustExternalID(t, "t1-new")
	if changed, _, err := tx.ApplyImport(rekeyed); err != nil || !changed || tx.ExternalID().String() != "t1-new" {
		t.Errorf("a new source id for the same bank transaction re-keys it: %v %v", changed, err)
	}
}

func TestRemoveAndComeBack(t *testing.T) {
	acc := checking(t)
	in := imported(t, "t1", -2310)
	tx := newTx(t, acc, in)
	tx.Remove(oct(20))
	if !tx.IsRemoved() {
		t.Fatal("not removed")
	}
	if err := tx.Categorize(mustCategory(t, "transport"), SourceRule); !errors.Is(err, ErrRemoved) {
		t.Errorf("a removed transaction cannot be categorized, got %v", err)
	}
	changed, _, err := tx.ApplyImport(in)
	if err != nil || !changed || tx.IsRemoved() {
		t.Errorf("a transaction the source returns again comes back: %v %v", changed, err)
	}
}

func mustCategory(t *testing.T, s string) CategoryID {
	t.Helper()
	c, err := NewCategoryID(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCategorize(t *testing.T) {
	tx := newTx(t, checking(t), imported(t, "t1", -4890))
	food, transport := mustCategory(t, "restaurants"), mustCategory(t, "transport")

	if err := tx.Categorize(food, SourceLLM); err != nil {
		t.Fatal(err)
	}
	if err := tx.Categorize(food, SourceLLM); err != nil {
		t.Fatal(err)
	}
	evs := tx.PullEvents()
	if len(evs) != 1 {
		t.Fatalf("the same category twice is one event, got %d", len(evs))
	}
	if e, ok := evs[0].(TransactionCategorized); !ok || e.TransactionID != tx.ID() || e.Category != food || e.Source != SourceLLM {
		t.Fatalf("unexpected event %#v", evs[0])
	}
	if len(tx.PullEvents()) != 0 {
		t.Error("PullEvents empties the list")
	}

	if err := tx.Categorize(transport, SourceManual); err != nil {
		t.Fatal(err)
	}
	if err := tx.Categorize(food, SourceRule); !errors.Is(err, ErrManualCategory) {
		t.Errorf("a rule never overwrites a manual choice, got %v", err)
	}
	if c, src := tx.Category(); c != transport || src != SourceManual {
		t.Errorf("category = %q by %q", c, src)
	}
	if err := tx.Categorize(food, SourceManual); err != nil {
		t.Errorf("a manual choice replaces a manual choice: %v", err)
	}
}

func TestPair(t *testing.T) {
	chk, crd := checking(t), card(t)
	out := newTx(t, chk, imported(t, "out", -148237))
	in := imported(t, "in", 148237)
	in.Kind = KindBillPayment
	pay := newTx(t, crd, in)

	if err := Pair(out, pay); err != nil {
		t.Fatal(err)
	}
	if out.Kind() != KindBillPayment || pay.Kind() != KindBillPayment || out.Counterpart() != pay.ID() || pay.Counterpart() != out.ID() {
		t.Fatalf("pair not set: %+v %+v", out, pay)
	}
	if out.SourceKind() != KindExpense {
		t.Error("pairing keeps what the source said, so unpairing can restore it")
	}

	other := newTx(t, crd, imported(t, "other", 148237))
	if err := Pair(out, other); !errors.Is(err, ErrPaired) {
		t.Errorf("a transaction pairs at most once, got %v", err)
	}

	// The checking side changes its amount on the next sync: the pair breaks on both sides.
	changedIn := imported(t, "out", -148000)
	changed, broken, err := out.ApplyImport(changedIn)
	if err != nil || !changed || broken != pay.ID() || out.IsPaired() {
		t.Fatalf("a new amount breaks the pair: %v %q %v", changed, broken, err)
	}
	pay.LeavePair()
	if pay.IsPaired() || pay.Kind() != KindBillPayment || out.Kind() != KindExpense {
		t.Errorf("after leaving, each side has the kind its source gave: %q %q", pay.Kind(), out.Kind())
	}
}

func TestPairRules(t *testing.T) {
	chk, crd := checking(t), card(t)
	savings := func() *Account {
		d := details(t)
		d.ExternalID = mustExternalID(t, "sav-1")
		d.Type = AccountSavings
		a, _ := NewAccount(d)
		return a
	}()

	out, in := newTx(t, chk, imported(t, "a", -100)), newTx(t, savings, imported(t, "b", 100))
	if err := Pair(out, in); err != nil || out.Kind() != KindTransfer || in.Kind() != KindTransfer {
		t.Fatalf("between two bank accounts it is a transfer: %v %q", err, out.Kind())
	}
	out, in = newTx(t, chk, imported(t, "a", -100)), newTx(t, crd, imported(t, "b", 100))
	if err := Pair(out, in); err != nil || out.Kind() != KindBillPayment || in.Kind() != KindBillPayment {
		t.Fatalf("into a card it is a bill payment: %v %q", err, out.Kind())
	}

	tests := []struct {
		name    string
		out, in *Transaction
	}{
		{"same account", newTx(t, chk, imported(t, "a", -100)), newTx(t, chk, imported(t, "b", 100))},
		{"different amounts", newTx(t, chk, imported(t, "a", -100)), newTx(t, savings, imported(t, "b", 99))},
		{"both outflows", newTx(t, chk, imported(t, "a", -100)), newTx(t, savings, imported(t, "b", -100))},
		{"in and out swapped", newTx(t, savings, imported(t, "a", 100)), newTx(t, chk, imported(t, "b", -100))},
		{"zero amounts", newTx(t, chk, imported(t, "a", 0)), newTx(t, savings, imported(t, "b", 0))},
		{"out of a card", newTx(t, crd, imported(t, "a", -100)), newTx(t, chk, imported(t, "b", 100))},
	}
	for _, tt := range tests {
		if err := Pair(tt.out, tt.in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", tt.name, err)
		}
	}

	removed := newTx(t, savings, imported(t, "r", 100))
	removed.Remove(oct(13))
	if err := Pair(newTx(t, chk, imported(t, "a", -100)), removed); !errors.Is(err, ErrRemoved) {
		t.Errorf("removed transactions do not pair, got %v", err)
	}

	out, in = newTx(t, chk, imported(t, "a", -100)), newTx(t, savings, imported(t, "b", 100))
	if err := Pair(out, in); err != nil {
		t.Fatal(err)
	}
	if broken := out.Remove(oct(14)); broken != in.ID() || out.IsPaired() {
		t.Errorf("removing one side leaves the pair and reports the other, got %q", broken)
	}
}

func TestRestore(t *testing.T) {
	tx := newTx(t, checking(t), imported(t, "t1", -4890))
	food, transport := mustCategory(t, "restaurants"), mustCategory(t, "transport")
	if err := tx.Categorize(food, SourceManual); err != nil {
		t.Fatal(err)
	}
	tx.PullEvents()

	if err := tx.Restore(transport, SourceRule); err != nil {
		t.Fatalf("an undo puts back a rule's category over a manual one: %v", err)
	}
	if c, src := tx.Category(); c != transport || src != SourceRule {
		t.Errorf("category = %q by %q", c, src)
	}
	if len(tx.PullEvents()) != 1 {
		t.Error("a restored category is published")
	}
	if err := tx.Restore(CategoryID{}, ""); err != nil {
		t.Fatal(err)
	}
	if c, src := tx.Category(); !c.IsZero() || src != "" {
		t.Errorf("an empty category leaves it waiting, got %q by %q", c, src)
	}
	if err := tx.Restore(food, Source("guess")); err == nil {
		t.Error("an unknown source is refused")
	}
}
