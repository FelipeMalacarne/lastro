package market

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr(v int64) *int64 { return &v }

func TestTrailingAndYield(t *testing.T) {
	divs := []Dividend{
		{ComDate: day("2025-10-27"), RateMicro: 500_000}, // exactly a year before: out
		{ComDate: day("2026-03-02"), RateMicro: 80_000},
		{ComDate: day("2026-09-30"), RateMicro: 157_843},
		{ComDate: day("2026-11-03"), RateMicro: 20_000}, // after: out
	}
	if got := TrailingPerShare(divs, day("2026-10-27")); got != 237_843 {
		t.Errorf("trailing = %d", got)
	}
	// R$ 0,237843 a share over R$ 38,20 is 0,62%.
	if got := YieldBps(237_843, 3820); got != 62 {
		t.Errorf("yield = %d bps", got)
	}
	if YieldBps(100, 0) != 0 || YieldBps(0, 3820) != 0 {
		t.Error("no price or no dividends yield nothing")
	}
}

func TestMultipleAndValueOn(t *testing.T) {
	// R$ 38,20 over R$ 1,345 of earnings is 28,4 times.
	if m := Multiple(3820, 1_345_000); m == nil || *m != 28_401 {
		t.Errorf("P/E = %v", m)
	}
	if Multiple(3820, -200_000) != nil || Multiple(3820, 0) != nil {
		t.Error("a loss or no earnings has no P/E")
	}
	f := Fundamental{Date: day("2026-10-27"), EPSMicro: ptr(-200_000), PEMilli: ptr(-19_100), PBMilli: ptr(7_900)}
	v := ValueOn(f, 3820, nil)
	if v.PEMilli != nil {
		t.Errorf("a loss-making company shows no P/E, not the source's negative one: %d", *v.PEMilli)
	}
	if v.PBMilli == nil || *v.PBMilli != 7_900 {
		t.Errorf("without book value, the source's P/B: %v", v.PBMilli)
	}
	f = Fundamental{Date: day("2026-10-27"), BookMicro: ptr(4_835_000), PEMilli: ptr(30_000)}
	v = ValueOn(f, 3820, nil)
	if v.PEMilli == nil || *v.PEMilli != 30_000 || v.PBMilli == nil || *v.PBMilli != 7_901 {
		t.Errorf("source P/E, computed P/B: %+v", v)
	}
}

func TestScaled(t *testing.T) {
	for _, c := range []struct {
		in   string
		by   int64
		want int64
	}{{"0.157843", 1_000_000, 157_843}, {"0.0000005", 1_000_000, 1}, {"28.4", 1000, 28_400}, {"-1,5", 1000, -1500}} {
		if got, err := scaled(c.in, c.by); err != nil || got != c.want {
			t.Errorf("scaled(%q) = %d %v", c.in, got, err)
		}
	}
	if _, err := scaled("n/a", 1000); err == nil {
		t.Error("not a number")
	}
	if p, err := optional("", 1000); p != nil || err != nil {
		t.Error("empty is missing")
	}
}
