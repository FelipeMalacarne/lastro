package ledger

import (
	"errors"
	"testing"
	"time"

	"lastro/money"
)

func TestManualAccounts(t *testing.T) {
	now := time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC)
	if _, err := NewManualAccount("Conta", AccountChecking, money.BRLCents(0), now); !errors.Is(err, ErrInvalid) {
		t.Errorf("only a wallet or an asset is typed in: %v", err)
	}
	pgbl, err := NewManualAccount(" Brasilprev PGBL ", AccountAsset, money.BRLCents(4552825), now)
	if err != nil {
		t.Fatal(err)
	}
	if pgbl.Institution() != ManualInstitution || pgbl.Name() != "Brasilprev PGBL" {
		t.Errorf("account %+v", pgbl)
	}
	jul, err := pgbl.Counted(money.BRLCents(4710865), time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC), now)
	if err != nil || jul.Date.Format(time.DateOnly) != "2026-07-26" {
		t.Fatalf("a counted balance is its day's: %+v %v", jul, err)
	}
	// Typed later, a correction of an earlier day never becomes the balance;
	// the latest day's does.
	oct, _ := pgbl.Counted(money.BRLCents(4831277), time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), now.Add(-time.Hour))
	if err := pgbl.Latest([]BalanceSnapshot{oct, jul}); err != nil || pgbl.Balance().Cents() != 4831277 {
		t.Errorf("the latest day: %d %v", pgbl.Balance().Cents(), err)
	}
	if err := pgbl.Latest(nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("an asset keeps a balance: %v", err)
	}
	if err := pgbl.Move(money.BRLCents(-100), now); !errors.Is(err, ErrInvalid) {
		t.Errorf("an asset takes no entries: %v", err)
	}
	usd, _ := money.New(100, money.USD)
	if _, err := pgbl.Counted(usd, now, now); !errors.Is(err, ErrCurrencyChanged) {
		t.Errorf("same currency: %v", err)
	}

	wallet, _ := NewManualAccount("Wallet", AccountCash, money.BRLCents(30000), now)
	if err := wallet.Move(money.BRLCents(-6400), now.Add(time.Hour)); err != nil || wallet.Balance().Cents() != 23600 {
		t.Errorf("an entry moves the wallet: %d %v", wallet.Balance().Cents(), err)
	}
	if _, err := wallet.Counted(money.BRLCents(1), now, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("a wallet's balance comes from its entries: %v", err)
	}
	if _, err := checking(t).Counted(money.BRLCents(1), now, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("a synced account's balance comes from the bank: %v", err)
	}
}

func TestNewManualDay(t *testing.T) {
	// Late on 30 Oct in São Paulo, already 31 Oct in UTC: today is the 30th.
	today := civilDate(time.Date(2026, 10, 31, 1, 30, 0, 0, time.UTC).In(saoPaulo))
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	for _, c := range []struct {
		name string
		day  time.Time
		ok   bool
	}{
		{"today", day(2026, 10, 30), true},
		{"yesterday", day(2026, 10, 29), true},
		{"the first day", day(2000, 1, 1), true},
		{"the day before", day(1999, 12, 31), false},
		{"a typo for 2026", day(1900, 1, 1), false},
		{"tomorrow in São Paulo, today in UTC", day(2026, 10, 31), false},
		{"next year", day(2027, 10, 30), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := NewManualDay(c.day, today)
			if (err == nil) != c.ok {
				t.Fatalf("NewManualDay(%s) = %v, %v", c.day.Format(time.DateOnly), got, err)
			}
			if err == nil && !got.Time().Equal(c.day) {
				t.Errorf("the day kept: %v", got.Time())
			}
			var r reason
			if err != nil && (!errors.Is(err, ErrInvalid) || !errors.As(err, &r)) {
				t.Errorf("an invalid value with its reason: %v", err)
			}
		})
	}
}
