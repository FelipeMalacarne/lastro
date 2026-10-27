package ingest

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"lastro/modules/ingest/internal/fixtures"
	"lastro/modules/ingest/internal/pluggy"
	"lastro/modules/ledger"
	"lastro/money"
)

func loadFixtures(t *testing.T) fixtures.Set {
	t.Helper()
	s, err := fixtures.Load("testdata/pluggy")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// accountsByID indexes every fixture account.
func accountsByID(s fixtures.Set) map[string]pluggy.Account {
	out := map[string]pluggy.Account{}
	for _, accs := range s.Accounts {
		for _, a := range accs {
			out[a.ID] = a
		}
	}
	return out
}

func TestMapAccounts(t *testing.T) {
	s := loadFixtures(t)
	at := time.Date(2026, 10, 26, 9, 12, 0, 0, time.UTC)
	accs := accountsByID(s)
	tests := []struct {
		id      string
		typ     ledger.AccountType
		name    string
		number  string
		balance int64
	}{
		{"b0000000-0000-4000-8000-000000000011", ledger.AccountChecking, "Conta Corrente", "5432", 321055},
		{"b0000000-0000-4000-8000-000000000012", ledger.AccountCreditCard, "Nubank Ultravioleta Mastercard", "4321", -153020},
		{"b0000000-0000-4000-8000-000000000021", ledger.AccountChecking, "Conta Corrente", "7890", 81240},
	}
	for _, tt := range tests {
		d, err := mapAccount(accs[tt.id], "Nubank", at)
		if err != nil {
			t.Fatalf("%s: %v", tt.id, err)
		}
		if d.Type != tt.typ || d.Name != tt.name || d.Balance != money.BRLCents(tt.balance) || !d.BalanceAt.Equal(at) {
			t.Errorf("%s: got %+v", tt.id, d)
		}
		a, err := ledger.NewAccount(d)
		if err != nil {
			t.Fatalf("%s: the ledger refuses the mapped account: %v", tt.id, err)
		}
		if a.Number() != tt.number {
			t.Errorf("%s: number %q, want %q", tt.id, a.Number(), tt.number)
		}
	}

	card, _ := mapAccount(accs["b0000000-0000-4000-8000-000000000012"], "Nubank", at)
	limit, available, ok := card.Card.Limit()
	if !ok || limit != money.BRLCents(1500000) || available != money.BRLCents(1346980) ||
		card.Card.Due().Format(time.DateOnly) != "2026-11-10" || card.Card.Closes().Format(time.DateOnly) != "2026-11-03" {
		t.Errorf("the card's limit and bill days: %+v", card.Card)
	}
	if chk, _ := mapAccount(accs["b0000000-0000-4000-8000-000000000011"], "Nubank", at); !chk.Card.IsZero() {
		t.Errorf("checking has no card details: %+v", chk.Card)
	}
	broken := accs["b0000000-0000-4000-8000-000000000012"]
	broken.CreditData = &pluggy.CreditData{CreditLimit: "-5", AvailableCreditLimit: "10", BalanceDueDate: "soon"}
	if d, err := mapAccount(broken, "Nubank", at); err != nil || !d.Card.IsZero() {
		t.Errorf("card data that does not parse is left out, not fatal: %+v %v", d.Card, err)
	}

	if _, err := mapAccount(pluggy.Account{ID: "x", Type: "INVESTMENT", Balance: "1", CurrencyCode: "BRL"}, "Bank", at); !errors.Is(err, errUnsupported) {
		t.Errorf("want errUnsupported for an unknown type, got %v", err)
	}
}

func TestMapTransactions(t *testing.T) {
	s := loadFixtures(t)
	accs := accountsByID(s)
	byID := map[string]ledger.ImportedTransaction{}
	for accID, txs := range s.Transactions {
		for _, tx := range txs {
			in, err := mapTransaction(tx, accs[accID])
			if err != nil {
				t.Fatalf("%s: %v", tx.ID, err)
			}
			byID[tx.ID] = in
		}
	}
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	tests := []struct {
		name   string
		id     string
		cents  int64
		kind   ledger.Kind
		status ledger.Status
		date   time.Time
		inst   string
	}{
		{"salary", "c0000000-0000-4000-8000-000000000001", 850000, ledger.KindIncome, ledger.StatusPosted, day(10, 5), ""},
		{"pix to my other account, until paired", "c0000000-0000-4000-8000-000000000002", -100000, ledger.KindExpense, ledger.StatusPosted, day(10, 6), ""},
		{"card bill paid from checking, until paired", "c0000000-0000-4000-8000-000000000003", -148237, ledger.KindExpense, ledger.StatusPosted, day(10, 10), ""},
		{"yield", "c0000000-0000-4000-8000-000000000005", 231, ledger.KindIncome, ledger.StatusPosted, day(10, 15), ""},
		{"redemption from my own investment", "c0000000-0000-4000-8000-000000000006", 30000, ledger.KindTransfer, ledger.StatusPosted, day(10, 20), ""},
		{"scheduled payment", "c0000000-0000-4000-8000-000000000007", -8990, ledger.KindExpense, ledger.StatusPending, day(11, 5), ""},
		{"card purchase", "c0000000-0000-4000-8000-000000000008", -4890, ledger.KindExpense, ledger.StatusPosted, day(10, 2), ""},
		{"installment", "c0000000-0000-4000-8000-000000000009", -31990, ledger.KindExpense, ledger.StatusPosted, day(10, 14), "3 of 10"},
		{"pending card purchase", "c0000000-0000-4000-8000-000000000010", -2310, ledger.KindExpense, ledger.StatusPending, day(10, 24), ""},
		{"bill payment on the card", "c0000000-0000-4000-8000-000000000011", 148237, ledger.KindBillPayment, ledger.StatusPosted, day(10, 10), ""},
		{"refund reduces spending", "c0000000-0000-4000-8000-000000000012", 5990, ledger.KindExpense, ledger.StatusPosted, day(10, 18), ""},
		{"purchase in dollars, in reais", "c0000000-0000-4000-8000-000000000013", -12000, ledger.KindExpense, ledger.StatusPosted, day(10, 22), ""},
		{"future installment", "c0000000-0000-4000-8000-000000000014", -31990, ledger.KindExpense, ledger.StatusPending, day(11, 14), "4 of 10"},
		{"pix received from my other account", "c0000000-0000-4000-8000-000000000015", 100000, ledger.KindIncome, ledger.StatusPosted, day(10, 6), ""},
		{"afternoon in UTC is the same day in São Paulo", "c0000000-0000-4000-8000-000000000017", -21000, ledger.KindExpense, ledger.StatusPosted, day(10, 9), ""},
		{"01:30 UTC is the evening before in São Paulo", "c0000000-0000-4000-8000-000000000018", -3500, ledger.KindExpense, ledger.StatusPosted, day(10, 10), ""},
	}
	for _, tt := range tests {
		got, ok := byID[tt.id]
		if !ok {
			t.Fatalf("%s: fixture %s missing", tt.name, tt.id)
		}
		y, m, d := got.Date.Date()
		if got.Amount != money.BRLCents(tt.cents) || got.Kind != tt.kind || got.Status != tt.status ||
			!time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Equal(tt.date) || got.Installment.String() != tt.inst {
			t.Errorf("%s: got %s %s %s %v %q", tt.name, got.Amount, got.Kind, got.Status, got.Date, got.Installment)
		}
	}
	if byID["c0000000-0000-4000-8000-000000000001"].BankRef != "of-0001" {
		t.Error("providerId is the bank reference")
	}
}

func TestMapTransactionsIntoTheLedger(t *testing.T) {
	s := loadFixtures(t)
	at := time.Date(2026, 10, 26, 9, 12, 0, 0, time.UTC)
	for _, it := range s.Items {
		for _, pa := range s.Accounts[it.ID] {
			d, err := mapAccount(pa, it.Connector.Name, at)
			if err != nil {
				t.Fatal(err)
			}
			acc, err := ledger.NewAccount(d)
			if err != nil {
				t.Fatal(err)
			}
			for _, pt := range s.Transactions[pa.ID] {
				in, err := mapTransaction(pt, pa)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := ledger.NewTransaction(acc, in); err != nil {
					t.Errorf("the ledger refuses %s: %v", pt.ID, err)
				}
			}
		}
	}
}

func TestParseCents(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"142.41", 14241}, {"-10", -1000}, {"0.1", 10}, {"1e3", 100000},
		{"33.330000000000005", 3333}, {"0.005", 1}, {"-0.005", -1}, {"0.0049", 0},
	}
	for _, tt := range tests {
		got, err := parseCents(json.Number(tt.in))
		if err != nil || got != tt.want {
			t.Errorf("parseCents(%q) = %d, %v; want %d", tt.in, got, err, tt.want)
		}
	}
	for _, bad := range []string{"", "abc", "1e30"} {
		if _, err := parseCents(json.Number(bad)); !errors.Is(err, money.ErrInvalidAmount) {
			t.Errorf("parseCents(%q): want ErrInvalidAmount, got %v", bad, err)
		}
	}
}

func TestDirectionWithoutType(t *testing.T) {
	cardAcc := pluggy.Account{Type: "CREDIT", CurrencyCode: "BRL"}
	bank := pluggy.Account{Type: "BANK", CurrencyCode: "BRL"}
	base := pluggy.Transaction{ID: "t", Description: "x", Date: "2026-10-01", CurrencyCode: "BRL"}
	tests := []struct {
		acc    pluggy.Account
		amount string
		want   int64
	}{
		{cardAcc, "10", -1000}, {cardAcc, "-10", 1000}, {bank, "-10", -1000}, {bank, "10", 1000},
	}
	for _, tt := range tests {
		tx := base
		tx.Amount = json.Number(tt.amount)
		got, err := mapTransaction(tx, tt.acc)
		if err != nil || got.Amount.Cents() != tt.want {
			t.Errorf("%s %s: got %d, %v; want %d", tt.acc.Type, tt.amount, got.Amount.Cents(), err, tt.want)
		}
	}
}

func TestKindByDescription(t *testing.T) {
	tests := []struct {
		card        bool
		cents       int64
		op          string
		description string
		want        ledger.Kind
	}{
		{false, 30000, "OUTROS", "Valor recebido de Investimentos", ledger.KindTransfer},
		{false, 30000, "RESGATE_APLIC_FINANCEIRA", "Valor recebido de Investimentos", ledger.KindTransfer},
		{false, -30000, "", "APLICACAO AUTOMATICA", ledger.KindTransfer},
		{false, 30000, "", "Resgate RDB", ledger.KindTransfer},
		{false, 1500, "OUTROS", "Resgate de Cashback", ledger.KindIncome},
		{false, 30000, "PIX", "Transferência recebida", ledger.KindIncome},
		{true, 148237, "", "Pagamento recebido", ledger.KindBillPayment},
		{true, 1500, "", "Cashback", ledger.KindExpense},
	}
	for _, tt := range tests {
		got := kindOf(pluggy.Transaction{OperationType: tt.op}, tt.card, tt.cents, tt.description)
		if got != tt.want {
			t.Errorf("%q on card=%v: got %s, want %s", tt.description, tt.card, got, tt.want)
		}
	}
}

func TestWithoutBuiltBillPayments(t *testing.T) {
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	at := func(d, h, m int) time.Time { return time.Date(2026, 9, d, h, m, 0, 0, sp) }
	pay := func(id string, when time.Time, cents int64) ledger.ImportedTransaction {
		ext, _ := ledger.NewExternalID(id)
		return ledger.ImportedTransaction{ExternalID: ext, Date: when, Description: "Pagamento recebido", Amount: money.BRLCents(cents), Kind: ledger.KindBillPayment, Status: ledger.StatusPosted}
	}
	purchase := pay("p", at(1, 0, 0), -4890)
	purchase.Kind = ledger.KindExpense
	in := []ledger.ImportedTransaction{
		pay("built", at(1, 0, 0), 101515), pay("bank", at(1, 19, 7), 101515), // Pluggy's copy and the bank's
		pay("alone", at(3, 0, 0), 128666), // no twin: the only record
		pay("other", at(28, 0, 0), 115847), pay("other-bank", at(28, 17, 20), 99999),
		pay("both-dated-a", at(10, 0, 0), 5000), pay("both-dated-b", at(10, 0, 0), 5000),
		purchase,
	}
	out, dropped := withoutBuiltBillPayments(in)
	if dropped != 1 || len(out) != len(in)-1 {
		t.Fatalf("only the copy with a timed twin goes: dropped %d, kept %d", dropped, len(out))
	}
	for _, tx := range out {
		if tx.ExternalID.String() == "built" {
			t.Error("the built copy stayed")
		}
	}
}
