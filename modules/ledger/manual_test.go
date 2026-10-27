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
	pgbl, err := NewManualAccount(" Brasilprev PGBL ", AccountAsset, money.BRLCents(4552825), now.AddDate(0, -6, 0))
	if err != nil {
		t.Fatal(err)
	}
	if pgbl.Institution() != ManualInstitution || pgbl.Name() != "Brasilprev PGBL" {
		t.Errorf("account %+v", pgbl)
	}
	if _, err := pgbl.SetBalance(money.BRLCents(4831277), now); err != nil {
		t.Fatal(err)
	}
	snap, err := pgbl.SetBalance(money.BRLCents(4710865), now.AddDate(0, -3, 0))
	if err != nil || snap.Date.Format(time.DateOnly) != "2026-07-26" {
		t.Fatalf("an earlier balance is a snapshot of its day: %+v %v", snap, err)
	}
	if pgbl.Balance().Cents() != 4831277 {
		t.Errorf("an earlier balance never replaces the latest: %d", pgbl.Balance().Cents())
	}
	if err := pgbl.Move(money.BRLCents(-100), now); !errors.Is(err, ErrInvalid) {
		t.Errorf("an asset takes no entries: %v", err)
	}
	usd, _ := money.New(100, money.USD)
	if _, err := pgbl.SetBalance(usd, now); !errors.Is(err, ErrCurrencyChanged) {
		t.Errorf("same currency: %v", err)
	}

	wallet, _ := NewManualAccount("Wallet", AccountCash, money.BRLCents(30000), now)
	if err := wallet.Move(money.BRLCents(-6400), now.Add(time.Hour)); err != nil || wallet.Balance().Cents() != 23600 {
		t.Errorf("an entry moves the wallet: %d %v", wallet.Balance().Cents(), err)
	}
	bank := checking(t)
	if _, err := bank.SetBalance(money.BRLCents(1), now); !errors.Is(err, ErrInvalid) {
		t.Errorf("a synced account's balance comes from the bank: %v", err)
	}
}
