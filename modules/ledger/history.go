package ledger

import (
	"fmt"
	"time"

	"lastro/money"
)

// DayNet is what an account's live transactions add up to on a day.
type DayNet struct {
	Date time.Time
	Net  money.Money
}

// BalancePoint is an account's balance at the end of a day.
type BalancePoint struct {
	Date    time.Time
	Balance money.Money
}

// BalanceHistory rebuilds an account's balance at the end of each day, from
// the first day with a transaction to the day of the balance the bank
// reported. It walks back from that balance: the balance at the end of a day
// is the one at the end of the next day minus the next day's net. Days after
// asOf are left out (scheduled payments, future installments), and so is
// their money: the bank's balance does not include it yet.
func BalanceHistory(current money.Money, asOf time.Time, days []DayNet) ([]BalancePoint, error) {
	last := civilDate(asOf)
	net := map[time.Time]int64{}
	first := last
	for _, d := range days {
		if d.Net.Currency() != current.Currency() {
			return nil, fmt.Errorf("%w: balance history: %s day in a %s account", ErrCurrencyChanged, d.Net.Currency(), current.Currency())
		}
		day := civilDate(d.Date)
		if day.After(last) {
			continue
		}
		net[day] += d.Net.Cents()
		if day.Before(first) {
			first = day
		}
	}
	n := int(last.Sub(first).Hours()/24) + 1
	out := make([]BalancePoint, n)
	bal := current.Cents()
	for i := n - 1; i >= 0; i-- {
		day := first.AddDate(0, 0, i)
		b, err := money.New(bal, current.Currency())
		if err != nil {
			return nil, err
		}
		out[i] = BalancePoint{Date: day, Balance: b}
		bal -= net[day]
	}
	return out, nil
}
