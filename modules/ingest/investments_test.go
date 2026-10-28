package ingest

import (
	"encoding/json"
	"testing"
	"time"

	"lastro/modules/ingest/internal/fixtures"
	"lastro/modules/ingest/internal/pluggy"
	"lastro/modules/portfolio"
)

func TestMapInvestments(t *testing.T) {
	at := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	s := loadFixtures(t)
	var got []portfolio.Report
	for item, invs := range s.Investments {
		for _, inv := range invs {
			r, err := mapInvestment(inv, item, "Banco", at)
			if err != nil {
				t.Fatalf("%s: %v", inv.ID, err)
			}
			got = append(got, r)
		}
	}
	byName := map[string]portfolio.Report{}
	for _, r := range got {
		byName[r.Name] = r
	}
	ipca := byName["Tesouro IPCA+ 2035"]
	if ipca.Kind != portfolio.KindTreasury || ipca.Indexer != portfolio.IndexerIPCA || ipca.Rate.Bps() != 610 ||
		ipca.Value.Cents() != 1234567 || ipca.Invested == nil || ipca.Invested.Cents() != 1100000 || ipca.Maturity.Year() != 2035 {
		t.Errorf("Tesouro IPCA+: %+v", ipca)
	}
	if c := byName["Caixinha Reserva"]; c.Indexer != portfolio.IndexerCDI || c.Rate.Bps() != 10000 || c.Value.Cents() != 500000 || !c.Active {
		t.Errorf("Caixinha: %+v", c)
	}
}

// My own Nubank: 37 active CDBs, 5 stocks and a Tesouro bond come through;
// the 72 fully withdrawn CDBs close. Two of the stocks are ETFs Nubank
// sends as stocks, which portfolio's Classify puts in their class.
func TestMapMeuPluggyInvestments(t *testing.T) {
	s, err := fixtures.Load("testdata/meupluggy")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	kinds := map[portfolio.Kind]int{}
	var active, tickers int
	for item, invs := range s.Investments {
		for _, inv := range invs {
			r, err := mapInvestment(inv, item, "Nubank", at)
			if err != nil {
				t.Fatalf("%s: %v", inv.ID, err)
			}
			if r.Active {
				active++
				kinds[r.Kind]++
			}
			if r.Kind == portfolio.KindStock && !r.Ticker.IsZero() {
				tickers++
			}
		}
	}
	if active != 43 || kinds[portfolio.KindFixedIncome] != 37 || kinds[portfolio.KindStock] != 5 || kinds[portfolio.KindETF] != 0 || kinds[portfolio.KindTreasury] != 1 {
		t.Errorf("active %d, kinds %v", active, kinds)
	}
	// The anonymizer scrambles codes keeping their shape, so they still read as tickers.
	if tickers != 5 {
		t.Errorf("%d stocks with a ticker, want 5", tickers)
	}
}

// Every investment of my Nubank maps with its movements, signs from the
// types: purchases and withdrawals, and the two transfers in from another
// broker, which come without a quantity.
func TestMapMeuPluggyHistories(t *testing.T) {
	s, err := fixtures.Load("testdata/meupluggy")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	kinds := map[portfolio.MovementKind]int{}
	var histories, moves, issued, withoutQuantity int
	for item, invs := range s.Investments {
		for _, inv := range invs {
			h, err := mapHistory(inv, s.InvestmentTransactions[inv.ID], item, "Nubank", at)
			if err != nil {
				t.Fatalf("%s: %v", inv.ID, err)
			}
			histories++
			if h.Issued != nil {
				issued++
			}
			for _, m := range h.Movements {
				moves++
				kinds[m.Kind]++
				if m.Quantity == nil {
					withoutQuantity++
				}
				if m.Amount.IsNegative() || m.Amount.IsZero() {
					t.Errorf("%s: amount %s", m.ExternalID, m.Amount)
				}
			}
		}
	}
	if histories != 115 || moves != 178 || kinds[portfolio.MoveBuy] != 88 || kinds[portfolio.MoveSell] != 88 || kinds[portfolio.MoveTransferIn] != 2 {
		t.Errorf("%d histories, %d movements, by kind %v", histories, moves, kinds)
	}
	if withoutQuantity != 2 {
		t.Errorf("only the transfers come without a quantity: %d", withoutQuantity)
	}
	// CDBs and the Tesouro bond say when they were bought; stocks do not.
	if issued != 110 {
		t.Errorf("%d with an issue date, want 110", issued)
	}
}

func TestMapMovementDirection(t *testing.T) {
	tx := func(typ, movement, amount string) pluggy.InvestmentTransaction {
		return pluggy.InvestmentTransaction{ID: "t", Type: typ, MovementType: movement, Amount: json.Number(amount), Date: "2026-03-02T00:00:00.000Z"}
	}
	for _, tt := range []struct {
		tx   pluggy.InvestmentTransaction
		want portfolio.MovementKind
	}{
		{tx("BUY", "CREDIT", "100"), portfolio.MoveBuy},
		{tx("SELL", "DEBIT", "100"), portfolio.MoveSell},
		{tx("TRANSFER", "CREDIT", "100"), portfolio.MoveTransferIn},
		{tx("TRANSFER", "DEBIT", "100"), portfolio.MoveTransferOut},
		{tx("INTEREST", "CREDIT", "12.5"), portfolio.MoveIncome},
		{tx("TAX", "DEBIT", "-3"), portfolio.MoveTax},
	} {
		m, err := mapMovement(tt.tx)
		if err != nil || m.Kind != tt.want || m.Amount.IsNegative() {
			t.Errorf("%s/%s: %+v %v, want %s", tt.tx.Type, tt.tx.MovementType, m, err, tt.want)
		}
	}
	if _, err := mapMovement(tx("SWAP", "", "1")); err == nil {
		t.Error("an unknown type is not guessed")
	}
}
