//go:build integration

package portfolio

import (
	"context"
	"errors"
	"testing"
	"time"

	"lastro/modules/ledger"
	"lastro/modules/market"
	"lastro/money"
	"lastro/platform/db/dbtest"
)

func TestPortfolio(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)
	l := ledger.New(ledger.NewRepository(pool), nil)
	s := New(NewRepository(pool), l, market.New(market.NewRepository(pool), nil, nil, nil, nil))
	day1 := time.Date(2025, 10, 27, 15, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return day1 }

	ext, _ := ledger.NewExternalID("acc")
	if _, err := l.SyncAccount(ctx, ledger.AccountDetails{ExternalID: ext, Name: "Conta", Institution: "Nubank", Type: ledger.AccountChecking, Balance: money.BRLCents(500000), BalanceAt: day1}); err != nil {
		t.Fatal(err)
	}
	q := func(s string) Quantity { x, _ := ParseQuantity(s); return x }
	tk := func(s string) Ticker { x, _ := NewTicker(s); return x }
	inv := func(c int64) *money.Money { m := money.BRLCents(c); return &m }
	reports := func(asOf time.Time, wegeQty string, wegeValue int64) []Report {
		return []Report{
			{ExternalID: "cdb", Item: "it", Institution: "Nubank", Kind: KindFixedIncome, Name: "CDB", Indexer: IndexerCDI, Rate: money.FromBps(10000), Value: money.BRLCents(2500000), Invested: inv(2400000), AsOf: asOf, Active: true},
			{ExternalID: "ipca", Item: "it", Institution: "Nubank", Kind: KindTreasury, Name: "Tesouro IPCA+ 2035", Indexer: IndexerIPCA, Value: money.BRLCents(2000000), Invested: inv(1900000), AsOf: asOf, Active: true},
			{ExternalID: "wege", Item: "it", Institution: "Nubank", Kind: KindStock, Ticker: tk("WEGE3"), Name: "WEG", Quantity: q(wegeQty), Value: money.BRLCents(wegeValue), AsOf: asOf, Active: true},
			{ExternalID: "ivvb", Item: "it", Institution: "Nubank", Kind: KindETF, Ticker: tk("IVVB11"), Name: "IVVB11", Quantity: q("10"), Value: money.BRLCents(4500000), AsOf: asOf, Active: true},
			{ExternalID: "rdb", Item: "it", Institution: "Nubank", Kind: KindFixedIncome, Name: "Reserva", Indexer: IndexerCDI, Value: money.BRLCents(1000000), AsOf: asOf, Active: true},
			{ExternalID: "gone", Item: "it", Institution: "Nubank", Kind: KindFixedIncome, Name: "Old CDB", Value: money.BRLCents(0), AsOf: asOf, Active: false},
		}
	}
	if n, err := s.SyncPositions(ctx, "it", reports(day1, "100", 1000000)); err != nil || n != 5 {
		t.Fatalf("first sync: %d, %v (a CDB withdrawn before lastro knew it is not followed)", n, err)
	}
	if err := s.UpdatePosition(ctx, "rdb", nil, ptr(true)); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdatePosition(ctx, "nope", nil, ptr(true)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown position: %v", err)
	}
	if _, err := s.Valuate(ctx, s.Today()); err != nil {
		t.Fatal(err)
	}

	// A year later: 50 more WEGE3, at a higher price; the CDB is gone.
	day2 := day1.AddDate(1, 0, 0)
	s.now = func() time.Time { return day2 }
	r2 := reports(day2, "150", 1800000)[1:]
	if _, err := s.SyncPositions(ctx, "it", r2); err != nil {
		t.Fatal(err)
	}
	nw, err := s.Valuate(ctx, s.Today())
	if err != nil {
		t.Fatal(err)
	}
	if nw.Invested.Cents() != 2000000+1800000+4500000 || nw.Emergency.Cents() != 1000000 || nw.Cash.Cents() != 500000 {
		t.Errorf("net worth %+v", nw)
	}

	o, err := s.Overview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if o.Positions != 4 || o.InvestedCents != 8300000 || o.Since == nil || *o.Since != "2025-10-27" {
		t.Errorf("overview: %d positions, invested %d, since %v", o.Positions, o.InvestedCents, o.Since)
	}
	// Put in over the year: WEGE3's 50 at R$ 120, minus the CDB taken out at
	// its last value, R$ 25.000.
	if *o.PutIn12mCents != 600000-2500000 {
		t.Errorf("put in %d", *o.PutIn12mCents)
	}
	byClass := map[Class]ClassView{}
	for _, c := range o.Classes {
		byClass[c.Class] = c
	}
	if c := byClass[ClassGlobal]; c.Status != AboveBand || c.NowBp != 5421 {
		t.Errorf("global %+v", c)
	}
	if c := byClass[ClassCDI]; c.Status != BelowBand || c.ValueCents != 0 {
		t.Errorf("CDI %+v", c)
	}

	// Contribute R$ 3.000: CDI, far below its band, takes it all.
	p, err := s.Plan(ctx, money.BRLCents(300000))
	if err != nil {
		t.Fatal(err)
	}
	for _, sp := range p.Splits {
		if (sp.Class == ClassCDI) != (sp.AmountCents == 300000) {
			t.Errorf("split %+v", sp)
		}
	}

	next := s.Today().AddDate(0, 0, 3)
	if err := s.SaveThesis(ctx, "wege3", "Grows with electrification", "Margins below 15%", &next); err != nil {
		t.Fatal(err)
	}
	rem, err := s.Reminders(ctx, 7)
	if err != nil || len(rem) != 1 || rem[0].Ticker != "WEGE3" || rem[0].InDays != 3 {
		t.Errorf("reminders %+v, %v", rem, err)
	}

	pv, err := s.PlanPurchase(ctx, next, "WEGE3", ClassBR, money.BRLCents(40000), "after the report")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DecidePurchase(ctx, pv.ID, Done, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.DecidePurchase(ctx, pv.ID, Cancelled, ""); !errors.Is(err, ErrDecided) {
		t.Errorf("decided twice: %v", err)
	}
	// The satellite in 2026: R$ 10.000 grew to R$ 18.000 with R$ 6.000 added
	// (50 WEGE3 at R$ 120): 2.000 on 13.000, counting the 6.000 half the year.
	sb, err := s.Scoreboard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sb.Years) != 1 || sb.Years[0].Year != 2026 || sb.Years[0].SatelliteBp == nil || *sb.Years[0].SatelliteBp != 1538 || sb.Ibovespa {
		t.Errorf("scoreboard %+v", sb)
	}

	if err := s.SetTargets(ctx, map[Class]money.Percent{ClassCDI: money.FromBps(5000)}, 200); !errors.Is(err, ErrInvalid) {
		t.Errorf("targets at 50%%: %v", err)
	}
}

func TestDividendsAndPriceDrops(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)
	l := ledger.New(ledger.NewRepository(pool), nil)
	m := market.New(market.NewRepository(pool), nil, nil, nil, nil)
	s := New(NewRepository(pool), l, m)
	date := func(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }
	at := func(day string) { s.now = func() time.Time { return date(day).Add(15 * time.Hour) } }
	wege := func(qty string, asOf time.Time) []Report {
		tk, _ := NewTicker("WEGE3")
		q, _ := ParseQuantity(qty)
		return []Report{{ExternalID: "wege", Item: "it", Institution: "Nubank", Kind: KindStock, Ticker: tk, Name: "WEG", Quantity: q, Value: money.BRLCents(400000), AsOf: asOf, Active: true}}
	}
	for _, x := range []struct{ day, qty string }{{"2026-03-01", "100"}, {"2026-06-01", "150"}} {
		at(x.day)
		if _, err := s.SyncPositions(ctx, "it", wege(x.qty, date(x.day))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Valuate(ctx, s.Today()); err != nil {
			t.Fatal(err)
		}
	}
	at("2026-10-27")
	err := m.Record(ctx,
		[]market.Quote{
			{Symbol: "WEGE3", Date: date("2026-10-26"), Close: money.BRLCents(4070), Source: "test"},
			{Symbol: "WEGE3", Date: date("2026-10-27"), Close: money.BRLCents(3822), Source: "test"},
		},
		[]market.Dividend{
			{Symbol: "WEGE3", ComDate: date("2026-02-10"), Label: "JCP", RateMicro: 100_000, PaymentDate: date("2026-03-01")}, // before lastro knew
			{Symbol: "WEGE3", ComDate: date("2026-04-10"), Label: "JCP", RateMicro: 100_000, PaymentDate: date("2026-05-01")}, // 100 shares
			{Symbol: "WEGE3", ComDate: date("2026-09-30"), Label: "JCP", RateMicro: 157_843, PaymentDate: date("2026-12-12")}, // 150, not paid yet
			{Symbol: "WEGE3", ComDate: date("2026-11-20"), Label: "DIVIDENDO", RateMicro: 50_000},                             // not earned yet
			{Symbol: "WEGE3", ComDate: date("2026-08-20"), Label: "DIVIDENDO", RateMicro: 10_000},                             // earned, no payment date yet
		}, nil)
	if err != nil {
		t.Fatal(err)
	}

	v, err := s.Dividends(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v.Since == nil || *v.Since != "2026-03-01" || v.YearCents != 1000 || len(v.Upcoming) != 2 || v.Upcoming[1].Cents != 2368 || v.Upcoming[0].PaymentDate != nil {
		t.Errorf("R$ 0,10 × 100 paid; R$ 0,01 × 150 with no date and R$ 0,157843 × 150 to come: %+v", v)
	}
	if len(v.Tickers) != 1 || v.Tickers[0].TrailingMicro != 367_843 || v.Tickers[0].YieldBps != 96 {
		t.Errorf("trailing and yield: %+v", v.Tickers)
	}

	if drops, err := s.PriceDrops(ctx); err != nil || len(drops) != 0 {
		t.Fatalf("no alert, no drop: %+v %v", drops, err)
	}
	if err := s.SetPriceAlert(ctx, "WEGE3", 500); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPriceAlert(ctx, "WEGE3", 10); !errors.Is(err, ErrInvalid) {
		t.Errorf("a threshold under 0,5%%: %v", err)
	}
	drops, err := s.PriceDrops(ctx)
	if err != nil || len(drops) != 1 || drops[0].ChangeBps != -609 || drops[0].CloseCents != 3822 {
		t.Errorf("a 6,1%% fall over a 5%% alert: %+v %v", drops, err)
	}
}
