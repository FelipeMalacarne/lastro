//go:build integration

package market

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"lastro/modules/market/internal/brapi"
	"lastro/modules/market/internal/sgs"
	"lastro/modules/market/internal/tesouro"
	"lastro/platform/db/dbtest"
)

type fakeSGS struct{ calls []time.Time }

func (f *fakeSGS) Series(_ context.Context, code int, from, to time.Time) ([]sgs.Value, error) {
	f.calls = append(f.calls, from)
	if code == sgs.IPCA {
		return []sgs.Value{{Date: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Value: "0.82"}}, nil
	}
	var out []sgs.Value
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday && d.After(to.AddDate(0, 0, -10)) {
			out = append(out, sgs.Value{Date: d, Value: "0.050788"})
		}
	}
	return out, nil
}

type fakeBrapi struct{ ranges []string }

func (f *fakeBrapi) Quote(_ context.Context, symbol, rng string) (brapi.Quote, error) {
	f.ranges = append(f.ranges, rng)
	if symbol == "XXXX3" {
		return brapi.Quote{}, errors.New("not found")
	}
	if symbol == "BBAS3" && rng == "2y" {
		return brapi.Quote{}, fmt.Errorf("%w: BBAS3", brapi.ErrRange)
	}
	return brapi.Quote{
		Symbol: symbol, Currency: "BRL", Price: json.Number("53.875"), Time: time.Date(2026, 10, 27, 13, 0, 0, 0, time.UTC),
		History: []brapi.Price{{Date: time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC), Close: json.Number("58.68")}},
	}, nil
}

func (f *fakeBrapi) Fundamentals(_ context.Context, symbol string) (brapi.Fundamentals, error) {
	return brapi.Fundamentals{
		Symbol: symbol, EPS: json.Number("1.345"), PE: json.Number("40"),
		Dividends: []brapi.CashDividend{
			{Label: "JCP", Rate: json.Number("0.157843"), ComDate: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), PaymentDate: time.Date(2026, 12, 12, 0, 0, 0, 0, time.UTC)},
			{Label: "DIVIDENDO", Rate: json.Number("0.08"), ComDate: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)},
		},
	}, nil
}

type fakeTesouro struct {
	rows  []tesouro.Row
	calls int
}

func (f *fakeTesouro) Latest(context.Context) ([]tesouro.Row, error) {
	f.calls++
	if f.rows == nil {
		return nil, errors.New("status 429")
	}
	return f.rows, nil
}

func TestRefresh(t *testing.T) {
	ctx := context.Background()
	f, b := &fakeSGS{}, &fakeBrapi{}
	s := New(NewRepository(dbtest.New(t)), f, b, &fakeTesouro{}, nil)
	s.now = func() time.Time { return time.Date(2026, 10, 27, 15, 0, 0, 0, time.UTC) }

	rep, err := s.Refresh(ctx, []string{"PETR4", "XXXX3"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Rates != 8 || rep.Quotes != 2 || len(rep.Failed) != 2 {
		t.Errorf("report %+v: one Tesouro and one ticker failure never stop the rest", rep)
	}
	q, err := s.LatestQuotes(ctx, []string{"PETR4", "XXXX3"})
	if err != nil || len(q) != 1 || q["PETR4"].Close.Cents() != 5388 || q["PETR4"].Date.Day() != 27 {
		t.Errorf("latest %+v, %v (53,875 rounds half up)", q, err)
	}
	if h, _ := s.Quotes(ctx, "PETR4", time.Time{}); len(h) != 2 || h[0].Date.Day() != 23 {
		t.Errorf("history %+v", h)
	}

	// The second time, only what is new is asked for.
	if _, err := s.Refresh(ctx, []string{"PETR4"}); err != nil {
		t.Fatal(err)
	}
	if b.ranges[0] != "2y" || b.ranges[2] != "5d" {
		t.Errorf("ranges %v: a year of history the first time, a few days after", b.ranges)
	}
	// CDI is current, so only IPCA is asked again, from the day after its last value.
	if len(f.calls) != 3 || f.calls[2].Format(time.DateOnly) != "2026-09-02" {
		t.Errorf("calls %v", f.calls)
	}

	// Fundamentals and dividends came with the first refresh, once that day.
	divs, err := s.Dividends(ctx, []string{"PETR4"})
	if err != nil || len(divs["PETR4"]) != 2 || divs["PETR4"][1].RateMicro != 157_843 || divs["PETR4"][1].PaymentDate.Day() != 12 {
		t.Errorf("dividends %+v %v", divs, err)
	}
	vs, err := s.Valuations(ctx, "PETR4", time.Time{})
	if err != nil || len(vs) != 1 {
		t.Fatalf("valuations %+v %v", vs, err)
	}
	// R$ 53,88 over R$ 1,345 of earnings (not the source's 40), and
	// R$ 0,237843 of dividends in twelve months over R$ 53,88.
	if v := vs[0]; v.PEMilli == nil || *v.PEMilli != 40_059 || v.PBMilli != nil || v.YieldBps != 44 {
		t.Errorf("valuation %+v", v)
	}

	acc, last, err := s.Accumulated(ctx, CDI, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	if err != nil || last.Format(time.DateOnly) != "2026-10-27" || acc.Percent().Bps() != 36 {
		t.Errorf("seven days of CDI: %d bps, last %s, %v", acc.Percent().Bps(), last, err)
	}
}

func TestRefreshFallsBackToTheFreeRange(t *testing.T) {
	b := &fakeBrapi{}
	s := New(NewRepository(dbtest.New(t)), &fakeSGS{}, b, &fakeTesouro{}, nil)
	s.now = func() time.Time { return time.Date(2026, 10, 27, 15, 0, 0, 0, time.UTC) }
	rep, err := s.Refresh(context.Background(), []string{"BBAS3"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Quotes != 2 || len(b.ranges) != 2 || b.ranges[1] != shortRange {
		t.Errorf("report %+v, ranges %v: a plan without two years gets three months", rep, b.ranges)
	}
}

func TestRefreshReadsTesouroOncePerDay(t *testing.T) {
	ctx := context.Background()
	tes := &fakeTesouro{rows: []tesouro.Row{{
		Name: "Tesouro Selic", Maturity: time.Date(2031, 3, 1, 0, 0, 0, 0, time.UTC), Date: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
		BuyRate: "0,0450", BuyPrice: "17950,12",
	}}}
	s := New(NewRepository(dbtest.New(t)), &fakeSGS{}, &fakeBrapi{}, tes, nil)
	s.now = func() time.Time { return time.Date(2026, 10, 27, 15, 0, 0, 0, time.UTC) }
	for range 2 {
		if _, err := s.Refresh(ctx, nil); err != nil {
			t.Fatal(err)
		}
	}
	if tes.calls != 1 {
		t.Errorf("%d reads: Monday's titles on a Tuesday are current", tes.calls)
	}
	// On Wednesday, Tuesday's titles are due.
	s.now = func() time.Time { return time.Date(2026, 10, 28, 15, 0, 0, 0, time.UTC) }
	if _, err := s.Refresh(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if tes.calls != 2 {
		t.Errorf("%d reads, want 2", tes.calls)
	}
}
