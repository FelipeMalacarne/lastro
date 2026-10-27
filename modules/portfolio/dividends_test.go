package portfolio

import (
	"errors"
	"testing"
	"time"

	"lastro/modules/market"
)

func d(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestHoldingsAndEarned(t *testing.T) {
	h := NewHeldHistory([]Held{
		{Date: d("2026-03-01"), Ticker: "ITSA4", Quantity: QuantityFromMicro(100_000_000)},
		{Date: d("2026-03-01"), Ticker: "WEGE3", Quantity: QuantityFromMicro(50_000_000)},
		{Date: d("2026-06-01"), Ticker: "ITSA4", Quantity: QuantityFromMicro(300_000_000)},
		// WEGE3 sold by June.
	})
	if q, ok := h.On("ITSA4", d("2026-02-28")); ok || !q.IsZero() {
		t.Error("before the first valued day it is not known")
	}
	if q, _ := h.On("ITSA4", d("2026-05-31")); q.Micro() != 100_000_000 {
		t.Errorf("the latest valued day before: %d", q.Micro())
	}
	if q, ok := h.On("WEGE3", d("2026-07-01")); !ok || !q.IsZero() {
		t.Errorf("sold: zero, and known: %d %v", q.Micro(), ok)
	}

	divs := map[string][]market.Dividend{
		"ITSA4": {
			{ComDate: d("2026-01-15"), Label: "JCP", RateMicro: 20_000},                                     // before lastro knew
			{ComDate: d("2026-04-10"), Label: "DIVIDENDO", RateMicro: 25_000, PaymentDate: d("2026-05-02")}, // 100 shares
			{ComDate: d("2026-08-20"), Label: "JCP", RateMicro: 33_333, PaymentDate: d("2026-09-01")},       // 300 shares
		},
		"WEGE3": {{ComDate: d("2026-07-10"), Label: "JCP", RateMicro: 157_843}}, // none held
	}
	divs["ITSA4"] = append(divs["ITSA4"], market.Dividend{ComDate: d("2026-11-03"), Label: "JCP", RateMicro: 20_000}) // not yet
	got := Earned(divs, h, d("2026-10-27"))
	if len(got) != 2 {
		t.Fatalf("two counted: %+v", got)
	}
	if got[0].Cents != 250 || got[1].Cents != 1000 || !got[1].Day().Equal(d("2026-09-01")) {
		t.Errorf("R$ 0,025 × 100 and R$ 0,033333 × 300 (9,9999 rounds to 10,00): %+v", got)
	}
}

func TestDropped(t *testing.T) {
	if c, hit := Dropped(4070, 3822, 500); c != -609 || !hit {
		t.Errorf("a 6,1%% fall reaches 5%%: %d %v", c, hit)
	}
	if c, hit := Dropped(4070, 3900, 500); c != -417 || hit {
		t.Errorf("4,2%% does not: %d %v", c, hit)
	}
	if _, hit := Dropped(4070, 4400, 500); hit {
		t.Error("a rise is never a drop")
	}
	if _, err := NewDropThreshold(20); !errors.Is(err, ErrInvalid) {
		t.Error("too small")
	}
}
