package ledger

import "testing"

func account(t *testing.T, ext string, typ AccountType) *Account {
	t.Helper()
	d := details(t)
	d.ExternalID, d.Type = mustExternalID(t, ext), typ
	a, err := NewAccount(d)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func tx(t *testing.T, acc *Account, ext string, day int, cents int64, kind Kind) *Transaction {
	t.Helper()
	in := imported(t, ext, cents)
	in.Date, in.Kind = oct(day), kind
	return newTx(t, acc, in)
}

func TestMatchTransfers(t *testing.T) {
	nu, itau, nuCard := account(t, "nu", AccountChecking), account(t, "itau", AccountChecking), account(t, "nu-card", AccountCreditCard)

	t.Run("pix between my accounts", func(t *testing.T) {
		out, in := tx(t, nu, "out", 6, -100000, KindExpense), tx(t, itau, "in", 6, 100000, KindIncome)
		got := MatchTransfers([]*Transaction{out, in})
		if len(got) != 2 || out.Kind() != KindTransfer || in.Counterpart() != out.ID() {
			t.Fatalf("want a transfer, got %d paired, kind %q", len(got), out.Kind())
		}
	})
	t.Run("ted lands the next day", func(t *testing.T) {
		out, in := tx(t, nu, "out", 6, -100000, KindExpense), tx(t, itau, "in", 7, 100000, KindIncome)
		if got := MatchTransfers([]*Transaction{out, in}); len(got) != 2 {
			t.Fatal("one day apart pairs")
		}
	})
	t.Run("two days apart between bank accounts is not a transfer", func(t *testing.T) {
		out, in := tx(t, nu, "out", 6, -100000, KindExpense), tx(t, itau, "in", 8, 100000, KindIncome)
		if got := MatchTransfers([]*Transaction{out, in}); len(got) != 0 {
			t.Fatal("must not pair")
		}
	})
	t.Run("card bill paid from checking", func(t *testing.T) {
		out, in := tx(t, nu, "out", 10, -148237, KindExpense), tx(t, nuCard, "in", 12, 148237, KindBillPayment)
		got := MatchTransfers([]*Transaction{out, in})
		if len(got) != 2 || out.Kind() != KindBillPayment || in.Kind() != KindBillPayment {
			t.Fatalf("want a bill payment, got %d, %q", len(got), out.Kind())
		}
	})
	t.Run("a bill payment posted four days later is too far", func(t *testing.T) {
		out, in := tx(t, nu, "out", 10, -148237, KindExpense), tx(t, nuCard, "in", 14, 148237, KindBillPayment)
		if got := MatchTransfers([]*Transaction{out, in}); len(got) != 0 {
			t.Fatal("must not pair")
		}
	})
	t.Run("a card refund never pairs", func(t *testing.T) {
		out, refund := tx(t, nu, "out", 18, -5990, KindExpense), tx(t, nuCard, "refund", 18, 5990, KindExpense)
		if got := MatchTransfers([]*Transaction{out, refund}); len(got) != 0 {
			t.Fatal("a refund on the card is not a bill payment")
		}
	})
	t.Run("same account", func(t *testing.T) {
		out, in := tx(t, nu, "out", 6, -100, KindExpense), tx(t, nu, "in", 6, 100, KindIncome)
		if got := MatchTransfers([]*Transaction{out, in}); len(got) != 0 {
			t.Fatal("must not pair")
		}
	})
	t.Run("an inflow two outflows could claim stays unpaired", func(t *testing.T) {
		sav := account(t, "sav", AccountSavings)
		a, b := tx(t, nu, "a", 6, -5000, KindExpense), tx(t, itau, "b", 6, -5000, KindExpense)
		in := tx(t, sav, "in", 6, 5000, KindIncome)
		if got := MatchTransfers([]*Transaction{a, b, in}); len(got) != 0 {
			t.Fatal("ambiguous: must not guess")
		}
	})
	t.Run("an outflow two inflows could take stays unpaired", func(t *testing.T) {
		sav := account(t, "sav", AccountSavings)
		out := tx(t, nu, "out", 6, -5000, KindExpense)
		a, b := tx(t, itau, "a", 6, 5000, KindIncome), tx(t, sav, "b", 6, 5000, KindIncome)
		if got := MatchTransfers([]*Transaction{out, a, b}); len(got) != 0 {
			t.Fatal("ambiguous: must not guess")
		}
	})
	t.Run("the closer date wins", func(t *testing.T) {
		out := tx(t, nu, "out", 6, -5000, KindExpense)
		near, far := tx(t, itau, "near", 6, 5000, KindIncome), tx(t, itau, "far", 7, 5000, KindIncome)
		got := MatchTransfers([]*Transaction{out, far, near})
		if len(got) != 2 || out.Counterpart() != near.ID() || far.IsPaired() {
			t.Fatal("the same-day inflow must win")
		}
	})
	t.Run("two equal transfers on the same day pair one to one", func(t *testing.T) {
		o1, o2 := tx(t, nu, "o1", 6, -5000, KindExpense), tx(t, nu, "o2", 6, -5000, KindExpense)
		i1, i2 := tx(t, itau, "i1", 6, 5000, KindIncome), tx(t, itau, "i2", 6, 5000, KindIncome)
		got := MatchTransfers([]*Transaction{o1, o2, i1, i2})
		if len(got) != 4 {
			t.Fatalf("both pairs go between the same two accounts, so either way is right: got %d", len(got))
		}
	})
	t.Run("paired and removed transactions are left alone", func(t *testing.T) {
		out, in := tx(t, nu, "out", 6, -100, KindExpense), tx(t, itau, "in", 6, 100, KindIncome)
		_ = Pair(out, in)
		other := tx(t, itau, "other", 6, 100, KindIncome)
		gone := tx(t, itau, "gone", 6, 100, KindIncome)
		gone.Remove(oct(7))
		if got := MatchTransfers([]*Transaction{out, in, other, gone}); len(got) != 0 {
			t.Fatal("nothing new to pair")
		}
	})
	t.Run("matching again changes nothing", func(t *testing.T) {
		out, in := tx(t, nu, "out", 6, -100, KindExpense), tx(t, itau, "in", 6, 100, KindIncome)
		MatchTransfers([]*Transaction{out, in})
		if got := MatchTransfers([]*Transaction{out, in}); len(got) != 0 {
			t.Fatal("idempotent")
		}
	})
}

func TestWalletIsOnlyTheInflow(t *testing.T) {
	chk, wallet := account(t, "chk", AccountChecking), account(t, "wallet", AccountCash)
	feira := tx(t, wallet, "feira", 26, -5000, KindExpense)
	pix := tx(t, chk, "pix", 26, 5000, KindIncome)
	if got := MatchTransfers([]*Transaction{feira, pix}); len(got) != 0 {
		t.Errorf("spending from the wallet is never a transfer, even against income of the same amount: %d paired", len(got))
	}
	atm := tx(t, chk, "atm", 26, -30000, KindExpense)
	in := tx(t, wallet, "in", 26, 30000, KindIncome)
	if got := MatchTransfers([]*Transaction{atm, in}); len(got) != 2 {
		t.Errorf("a withdrawal into the wallet pairs: %d paired", len(got))
	}
}
