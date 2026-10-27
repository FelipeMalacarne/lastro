package ledger

import (
	"errors"
	"testing"
	"time"

	"lastro/money"
)

func TestBalanceHistory(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	net := func(d int, c int64) DayNet { return DayNet{Date: day(d), Net: money.BRLCents(c)} }
	asOf := time.Date(2026, 10, 26, 3, 1, 0, 0, time.UTC)

	got, err := BalanceHistory(money.BRLCents(321055), asOf, []DayNet{
		net(20, 30000), net(22, -48990), net(5, 725000), net(26, -4890),
		net(28, -31990), // scheduled after the bank's balance: not in it
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 22 || !got[0].Date.Equal(day(5)) || !got[21].Date.Equal(day(26)) {
		t.Fatalf("one point a day from the first transaction to the balance's day: %d points, %v … %v", len(got), got[0].Date, got[len(got)-1].Date)
	}
	at := func(d int) int64 { return got[d-5].Balance.Cents() }
	tests := []struct {
		day  int
		want int64
	}{
		{26, 321055},                // the bank's balance
		{25, 321055 + 4890},         // before the 26th's spending
		{22, 321055 + 4890},         // nothing on the 23rd to the 25th
		{21, 321055 + 4890 + 48990}, // before the 22nd
		{19, 321055 + 4890 + 48990 - 30000},
		{5, 321055 + 4890 + 48990 - 30000},
	}
	for _, tt := range tests {
		if at(tt.day) != tt.want {
			t.Errorf("end of %d Oct: %d, want %d", tt.day, at(tt.day), tt.want)
		}
	}
	if only, err := BalanceHistory(money.BRLCents(100), asOf, nil); err != nil || len(only) != 1 || only[0].Balance.Cents() != 100 {
		t.Errorf("no transactions: today's balance alone, got %+v %v", only, err)
	}
	usd, _ := money.New(1, money.USD)
	if _, err := BalanceHistory(money.BRLCents(100), asOf, []DayNet{{Date: day(1), Net: usd}}); !errors.Is(err, ErrCurrencyChanged) {
		t.Errorf("a day in another currency: %v", err)
	}
}
