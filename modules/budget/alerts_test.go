package budget

import (
	"testing"
	"time"

	"lastro/money"
)

func TestFindAlerts(t *testing.T) {
	oct := month(t, "2026-10")
	today := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	b := money.BRLCents
	figures := []Figures{
		{Category: cat(t, "fun"), Limited: true, Spent: b(70000), Available: b(50000), Left: b(-20000), Average: b(30000), Pace: PaceOver},
		{Category: cat(t, "food"), Limited: true, Spent: b(76000), Available: b(80000), Left: b(4000), Average: b(70000), Pace: PaceAhead},
		{Category: cat(t, "gifts"), Spent: b(45000), Average: b(20000)},     // unusual, no limit
		{Category: cat(t, "pharmacy"), Spent: b(9000), Average: b(4000)},    // usually small: never unusual
		{Category: cat(t, "transport"), Spent: b(70000), Average: b(60000)}, // only 17% over
		{Spent: b(90000), Average: b(10000)},                                // without a category: the review queue handles it
	}
	since := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	recurring := []Recurring{
		{Name: "SPOTIFY", Monthly: b(2390), Change: &PriceChange{From: b(2190), To: b(2390), Since: since}},
		{Name: "DUOLINGO", Monthly: b(3490), New: true},
		{Name: "NETFLIX", Monthly: b(4490)},
	}
	got := FindAlerts(oct, today, figures, recurring, 5)
	want := []string{"over:2026-10:fun", "ahead:2026-10:food", "unusual:2026-10:gifts", "price:SPOTIFY:2026-10-21", "new:DUOLINGO"}
	if len(got) != len(want) {
		t.Fatalf("got %d alerts: %+v", len(got), got)
	}
	for i, a := range got {
		if a.Key != want[i] {
			t.Errorf("alert %d = %s, want %s", i, a.Key, want[i])
		}
	}
	if got[0].Amount.Cents() != 20000 || got[1].DaysLeft != 5 {
		t.Errorf("over by %d, %d days left", got[0].Amount.Cents(), got[1].DaysLeft)
	}

	// A closed month: still over, never ahead, and no new charges.
	sep := FindAlerts(month(t, "2026-09"), today, figures[:2], recurring[1:], 0)
	if len(sep) != 1 || sep[0].Kind != AlertOver {
		t.Errorf("September: %+v", sep)
	}
}

func TestRestAlert(t *testing.T) {
	sep := month(t, "2026-09")
	tests := []struct {
		name string
		r    Rest
		want AlertKind
	}{
		{"waits for a choice", Rest{Month: sep, Closed: true, Left: money.BRLCents(1000)}, AlertRest},
		{"decided", Rest{Month: sep, Closed: true, Left: money.BRLCents(1000), Choice: RestKeep}, ""},
		{"went past what came in", Rest{Month: sep, Closed: true, Left: money.BRLCents(-4500)}, AlertShort},
		{"left exactly nothing", Rest{Month: sep, Closed: true, Left: money.BRLCents(0)}, ""},
		{"still running", Rest{Month: sep, Left: money.BRLCents(1000)}, ""},
	}
	for _, tt := range tests {
		a, ok := RestAlert(tt.r)
		if ok != (tt.want != "") || a.Kind != tt.want {
			t.Errorf("%s: %v %v, want %q", tt.name, a.Kind, ok, tt.want)
		}
	}
	if a, _ := RestAlert(Rest{Month: sep, Closed: true, Left: money.BRLCents(-4500)}); a.Amount.Cents() != 4500 || a.Key != "short:2026-09" {
		t.Errorf("short = %+v", a)
	}
}
