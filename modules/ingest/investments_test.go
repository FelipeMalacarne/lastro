package ingest

import (
	"testing"
	"time"

	"lastro/modules/ingest/internal/fixtures"
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

// My own Nubank: 37 active CDBs, 3 stocks, 2 ETFs and a Tesouro bond come
// through; the 72 fully withdrawn CDBs close.
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
	if active != 43 || kinds[portfolio.KindFixedIncome] != 37 || kinds[portfolio.KindStock] != 3 || kinds[portfolio.KindETF] != 2 || kinds[portfolio.KindTreasury] != 1 {
		t.Errorf("active %d, kinds %v", active, kinds)
	}
	// The anonymizer scrambles codes, so only the stocks' still read as tickers.
	if tickers != 3 {
		t.Errorf("%d stocks with a ticker, want 3", tickers)
	}
}
