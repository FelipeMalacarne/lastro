package portfolio

import (
	"testing"
	"time"

	"lastro/money"
)

func TestClassify(t *testing.T) {
	tk := func(s string) Ticker { x, _ := NewTicker(s); return x }
	tests := []struct {
		kind Kind
		idx  Indexer
		t    Ticker
		name string
		want Class
	}{
		{KindFixedIncome, IndexerCDI, Ticker{}, "CDB Nubank", ClassCDI},
		{KindFixedIncome, IndexerIPCA, Ticker{}, "CDB IPCA+ 6%", ClassIPCA},
		{KindTreasury, IndexerNone, Ticker{}, "Tesouro IPCA+ 2035", ClassIPCA},
		{KindTreasury, IndexerNone, Ticker{}, "Tesouro Selic 2029", ClassCDI},
		{KindTreasury, IndexerNone, Ticker{}, "Tesouro Renda+ 2065", ClassIPCA},
		{KindETF, IndexerNone, tk("IVVB11"), "", ClassGlobal},
		{KindETF, IndexerNone, tk("BOVA11"), "", ClassBR},
		{KindETF, IndexerNone, tk("B5P211"), "", ClassIPCA},
		{KindETF, IndexerNone, tk("IMAB11"), "", ClassIPCA},
		{KindETF, IndexerNone, Ticker{}, "IT NOW IMA-B5 P2 FUNDO DE INDICE", ClassIPCA},
		{KindETF, IndexerNone, tk("LFTS11"), "", ClassCDI},
		{KindStock, IndexerNone, tk("WEGE3"), "", ClassBR},
		{KindStock, IndexerNone, tk("AAPL34"), "", ClassGlobal},
		// Banks send some ETFs as stocks.
		{KindStock, IndexerNone, tk("B5P211"), "B5P211", ClassIPCA},
		{KindStock, IndexerNone, tk("IVVB11"), "IVVB11", ClassGlobal},
		{KindStock, IndexerNone, tk("LFTS11"), "LFTS11", ClassCDI},
		{KindStock, IndexerNone, tk("BOVA11"), "BOVA11", ClassBR},
		{KindBDR, IndexerNone, tk("MSFT34"), "", ClassGlobal},
		{KindFII, IndexerNone, tk("HGLG11"), "", ClassBR},
	}
	for _, tt := range tests {
		if got := Classify(tt.kind, tt.idx, tt.t, tt.name); got != tt.want {
			t.Errorf("%s %s %s %q = %s, want %s", tt.kind, tt.idx, tt.t, tt.name, got, tt.want)
		}
	}
}

func TestPositionBasis(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	q := func(s string) Quantity { x, _ := ParseQuantity(s); return x }
	invested := money.BRLCents(472000)
	// A CDB: the bank says what was put in.
	cdb, _ := NewPosition(Report{ExternalID: "cdb", Kind: KindFixedIncome, Value: money.BRLCents(528524), Invested: &invested, Active: true}, now)
	if cdb.Basis().Cents() != 472000 {
		t.Errorf("CDB basis %d", cdb.Basis().Cents())
	}

	// A stock: no figure from the bank. 100 at R$ 20, then 150 at R$ 22, then 75.
	s, _ := NewPosition(Report{ExternalID: "s", Kind: KindStock, Quantity: q("100"), Value: money.BRLCents(200000), Active: true}, now)
	_ = s.Update(Report{ExternalID: "s", Kind: KindStock, Quantity: q("150"), Value: money.BRLCents(330000), Active: true}, now)
	if s.Basis().Cents() != 200000+110000 {
		t.Errorf("after buying 50 at R$ 22: basis %d", s.Basis().Cents())
	}
	_ = s.Update(Report{ExternalID: "s", Kind: KindStock, Quantity: q("75"), Value: money.BRLCents(150000), Active: true}, now)
	if s.Basis().Cents() != 155000 {
		t.Errorf("after selling half: basis %d", s.Basis().Cents())
	}
	// A price move alone changes nothing.
	_ = s.Update(Report{ExternalID: "s", Kind: KindStock, Quantity: q("75"), Value: money.BRLCents(180000), Active: true}, now)
	if s.Basis().Cents() != 155000 || !s.Open() {
		t.Errorf("a price move: basis %d", s.Basis().Cents())
	}
	_ = s.Update(Report{ExternalID: "s", Kind: KindStock, Quantity: q("75"), Value: money.BRLCents(180000), Active: false}, now)
	if s.Open() {
		t.Error("an inactive report closes it")
	}
}
