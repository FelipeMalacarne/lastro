package ledger

import (
	"errors"
	"testing"
	"time"

	"lastro/money"
)

func details(t *testing.T) AccountDetails {
	t.Helper()
	return AccountDetails{
		ExternalID:  mustExternalID(t, "acc-1"),
		Name:        "Conta Corrente",
		Institution: "Nubank",
		Type:        AccountChecking,
		Number:      "0001/9876543-2",
		Balance:     money.BRLCents(321055),
		BalanceAt:   time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC),
	}
}

func mustExternalID(t *testing.T, s string) ExternalID {
	t.Helper()
	id, err := NewExternalID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestNewAccount(t *testing.T) {
	a, err := NewAccount(details(t))
	if err != nil {
		t.Fatal(err)
	}
	if a.ID() == "" || a.Currency() != money.BRL || a.Balance() != money.BRLCents(321055) {
		t.Fatalf("unexpected account: %+v", a)
	}
	if a.Number() != "5432" {
		t.Errorf("only the last four digits are kept, got %q", a.Number())
	}

	d := details(t)
	d.Name = "  "
	if _, err := NewAccount(d); !errors.Is(err, ErrInvalid) {
		t.Errorf("an account needs a name, got %v", err)
	}
	d = details(t)
	d.Balance = money.Money{}
	if _, err := NewAccount(d); !errors.Is(err, ErrInvalid) {
		t.Errorf("a balance without a currency is invalid, got %v", err)
	}
}

func TestAccountRefresh(t *testing.T) {
	a, _ := NewAccount(details(t))

	d := details(t)
	d.Name = "Conta"
	d.Balance = money.BRLCents(100)
	d.BalanceAt = d.BalanceAt.Add(time.Hour)
	if err := a.Refresh(d); err != nil {
		t.Fatal(err)
	}
	if a.Name() != "Conta" || a.Balance() != money.BRLCents(100) {
		t.Fatalf("refresh not applied: %+v", a)
	}

	older := details(t)
	older.Balance = money.BRLCents(999)
	if err := a.Refresh(older); err != nil {
		t.Fatal(err)
	}
	if a.Balance() != money.BRLCents(100) {
		t.Error("an older balance must not replace a newer one")
	}

	usd := details(t)
	usd.Balance, _ = money.New(100, money.USD)
	if err := a.Refresh(usd); !errors.Is(err, ErrCurrencyChanged) {
		t.Errorf("want ErrCurrencyChanged, got %v", err)
	}
	card := details(t)
	card.Type = AccountCreditCard
	if err := a.Refresh(card); !errors.Is(err, ErrInvalid) {
		t.Errorf("an account cannot change type, got %v", err)
	}
	other := details(t)
	other.ExternalID = mustExternalID(t, "acc-2")
	if err := a.Refresh(other); !errors.Is(err, ErrInvalid) {
		t.Errorf("details of another account must be refused, got %v", err)
	}
}

func TestSnapshot(t *testing.T) {
	d := details(t)
	brt := time.FixedZone("BRT", -3*3600)
	d.BalanceAt = time.Date(2026, 10, 26, 23, 30, 0, 0, brt) // 02:30 UTC on the 27th
	a, _ := NewAccount(d)
	s := a.Snapshot()
	if !s.Date.Equal(time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)) || s.Balance != d.Balance || s.AccountID != a.ID() || !s.At.Equal(d.BalanceAt) {
		t.Errorf("the snapshot is of the day the balance was read, where it was read: %+v", s)
	}
}
