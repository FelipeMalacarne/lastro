package portfolio

import (
	"fmt"
	"math/big"
	"sort"
	"time"

	"lastro/modules/market"
)

// Held is how many shares of a ticker were held on a valued day.
type Held struct {
	Date     time.Time
	Ticker   string
	Quantity Quantity
}

// HeldHistory answers how many shares were held on a day: the quantity of the
// latest valued day on or before it, zero when the ticker was not among
// that day's holdings. Before the first valued day it does not know.
type HeldHistory struct {
	days []time.Time                     // every valued day, oldest first
	by   map[string]map[time.Time]int64 // ticker → day → quantity in millionths
}

func NewHeldHistory(hs []Held) HeldHistory {
	h := HeldHistory{by: map[string]map[time.Time]int64{}}
	seen := map[time.Time]bool{}
	for _, x := range hs {
		if !seen[x.Date] {
			seen[x.Date] = true
			h.days = append(h.days, x.Date)
		}
		if h.by[x.Ticker] == nil {
			h.by[x.Ticker] = map[time.Time]int64{}
		}
		h.by[x.Ticker][x.Date] += x.Quantity.Micro()
	}
	sort.Slice(h.days, func(i, j int) bool { return h.days[i].Before(h.days[j]) })
	return h
}

// Since is the first valued day; zero when there is none.
func (h HeldHistory) Since() time.Time {
	if len(h.days) == 0 {
		return time.Time{}
	}
	return h.days[0]
}

// On is the quantity of a ticker held at the end of a day, and whether it
// is known (the day is not before the first valued day).
func (h HeldHistory) On(ticker string, day time.Time) (Quantity, bool) {
	i := sort.Search(len(h.days), func(i int) bool { return h.days[i].After(day) })
	if i == 0 {
		return Quantity{}, false
	}
	return QuantityFromMicro(h.by[ticker][h.days[i-1]]), true
}

// Payment is a dividend counted at the quantity held on its com date.
type Payment struct {
	Ticker      string
	Label       string
	ComDate     time.Time
	PaymentDate time.Time // zero while not set
	RateMicro   int64
	Cents       int64
}

// Day is when the payment counts: when it is paid, or its com date while
// no payment date is set (it is then still to be paid).
func (p Payment) Day() time.Time {
	if p.PaymentDate.IsZero() {
		return p.ComDate
	}
	return p.PaymentDate
}

// Earned counts the dividends each ticker paid on the shares held on its com
// date, up to asOf. A dividend whose com date is before lastro knew the
// holdings, or when none were held, is left out; one whose com date has not
// come yet is not earned.
func Earned(divs map[string][]market.Dividend, h HeldHistory, asOf time.Time) []Payment {
	var out []Payment
	for ticker, ds := range divs {
		for _, d := range ds {
			if d.ComDate.After(asOf) {
				continue
			}
			q, known := h.On(ticker, d.ComDate)
			if !known || q.IsZero() {
				continue
			}
			out = append(out, Payment{
				Ticker: ticker, Label: d.Label, ComDate: d.ComDate, PaymentDate: d.PaymentDate,
				RateMicro: d.RateMicro, Cents: perShare(d.RateMicro, q),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Day().Equal(out[j].Day()) {
			return out[i].Day().Before(out[j].Day())
		}
		return out[i].Ticker < out[j].Ticker
	})
	return out
}

// perShare is a per-share amount in millionths of a real times a quantity
// in millionths, in cents, rounded half up.
func perShare(rateMicro int64, q Quantity) int64 {
	n := new(big.Int).Mul(big.NewInt(rateMicro), big.NewInt(q.Micro()))
	d := big.NewInt(10_000_000_000) // 1e6 × 1e6 / 100
	quo, rem := new(big.Int).QuoRem(n, d, new(big.Int))
	if rem.Mul(rem, big.NewInt(2)).Cmp(d) >= 0 {
		quo.Add(quo, big.NewInt(1))
	}
	return quo.Int64()
}

// Dropped is a day's change from one close to the next, in basis points, and
// whether it is a fall of at least the threshold.
func Dropped(prevCents, lastCents, thresholdBps int64) (int64, bool) {
	if prevCents <= 0 {
		return 0, false
	}
	change := (lastCents - prevCents) * 10_000 / prevCents
	return change, -change >= thresholdBps
}

// NewDropThreshold validates a price alert: from 0,5% to 50% in a day.
func NewDropThreshold(bps int64) (int64, error) {
	if bps < 50 || bps > 5000 {
		return 0, fmt.Errorf("%w: a price alert is between 0,5%% and 50%% a day", ErrInvalid)
	}
	return bps, nil
}
