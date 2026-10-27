package budget

import (
	"sort"
	"strings"
	"time"

	"lastro/modules/categorize"
	"lastro/modules/ledger"
	"lastro/money"
)

// Charge is one expense, as recurring charges are found in.
type Charge struct {
	ID          ledger.TransactionID
	Description string
	Date        time.Time
	Amount      money.Money // positive
	Category    ledger.CategoryID
}

// Steadiness says how much a recurring charge's amount moves.
type Steadiness string

const (
	Fixed  Steadiness = "fixed"  // within 10% of its usual amount: a subscription, rent
	Varies Steadiness = "varies" // within 60%: electricity, water
)

// PriceChange is a fixed charge whose latest amount differs from the one
// before it.
type PriceChange struct {
	From  money.Money
	To    money.Money
	Since time.Time
}

// Recurring is a charge that comes back every month (F5).
type Recurring struct {
	Name       string // the merchant, as a rule would match it
	Category   ledger.CategoryID
	Steadiness Steadiness
	Monthly    money.Money // the latest amount, or the last three's average when it varies
	Months     int         // months it was charged in
	First      time.Time
	Last       time.Time
	Next       time.Time // a month after Last, same day
	// PaidThisMonth reports whether it was already charged in today's month.
	PaidThisMonth bool
	PaidCents     int64 // what it charged in today's month
	New           bool  // first seen in the last three months
	Change        *PriceChange
	Charges       []ledger.TransactionID
}

// A series is recurring when it is charged about once a month, on about the
// same day, for at least minMonths months (two, for a new one with a steady
// amount; four, when the amount varies), in most of the months it spans,
// and stopped no more than lapse ago.
const (
	minMonths = 3
	lapse     = 45 * 24 * time.Hour
	fixedBy   = 1000 // basis points from the median
	variesBy  = 6000
	changedBy = 100 // a price change: one percent
	dayBy     = 6   // days a charge may move from its usual day: weekends, holidays
)

// FindRecurring groups charges by merchant and keeps the ones that come back
// every month. today decides what is still active, new and paid this month.
// Most expensive first.
func FindRecurring(charges []Charge, today time.Time) []Recurring {
	series := map[string][]Charge{}
	for _, c := range charges {
		k := merchant(c.Description)
		series[k] = append(series[k], c)
	}
	var out []Recurring
	for name, cs := range series {
		if r, ok := recurring(name, cs, today); ok {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Monthly.Cents() != out[j].Monthly.Cents() {
			return out[i].Monthly.Cents() > out[j].Monthly.Cents()
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// merchant is the name a series goes by: the pattern a correction would
// teach, so "PAG*IFOOD SP" and "PG *IFOOD" are one merchant.
func merchant(description string) string {
	if p, ok := categorize.SuggestPattern(description); ok {
		return p.Text()
	}
	return strings.ToUpper(strings.TrimSpace(description))
}

func recurring(name string, cs []Charge, today time.Time) (Recurring, bool) {
	sort.Slice(cs, func(i, j int) bool { return cs[i].Date.Before(cs[j].Date) })
	first, last := cs[0], cs[len(cs)-1]
	if today.Sub(last.Date) > lapse {
		return Recurring{}, false // stopped
	}

	// One charge a month, in most of the months the series spans.
	perMonth := map[Month]int64{}
	for _, c := range cs {
		perMonth[MonthOf(c.Date)] += c.Amount.Cents()
	}
	months := len(perMonth)
	span := MonthOf(last.Date).Since(MonthOf(first.Date)) + 1
	if len(cs) > months+months/4 || months*4 < span*3 {
		return Recurring{}, false
	}

	// Each charge's own amount, oldest first: an early payment that lands two
	// charges in one month is not a price change.
	amounts := make([]int64, len(cs))
	for i, c := range cs {
		amounts[i] = c.Amount.Cents()
	}
	steady, ok := steadiness(amounts)
	if !ok {
		return Recurring{}, false
	}
	isNew := today.Sub(first.Date) < 92*24*time.Hour
	earlyNew := isNew && months >= 2 && steady == Fixed // a new subscription, two months in
	switch {
	case months < minMonths && !earlyNew:
		return Recurring{}, false
	case steady == Varies && months < minMonths+1:
		return Recurring{}, false // a varying amount needs more months to tell a bill from a habit
	case !sameDay(cs):
		return Recurring{}, false
	}

	r := Recurring{
		Name: name, Category: last.Category, Steadiness: steady, Months: months,
		First: first.Date, Last: last.Date, Next: nextMonthly(last.Date), New: isNew,
	}
	for _, c := range cs {
		r.Charges = append(r.Charges, c.ID)
		if MonthOf(c.Date) == MonthOf(today) {
			r.PaidThisMonth = true
			r.PaidCents += c.Amount.Cents()
		}
	}
	if steady == Fixed {
		r.Monthly = last.Amount
		// The change: the latest charge at another price, and the first one
		// at today's.
		i := len(cs) - 1
		for i > 0 && cs[i-1].Amount == last.Amount {
			i--
		}
		if i > 0 {
			prev := cs[i-1].Amount
			if d := abs(last.Amount.Cents() - prev.Cents()); prev.Cents() > 0 && d*10000/prev.Cents() >= changedBy {
				r.Change = &PriceChange{From: prev, To: last.Amount, Since: cs[i].Date}
			}
		}
	} else {
		// The average of the last three charges.
		recent := amounts[max(0, len(amounts)-3):]
		var sum int64
		for _, v := range recent {
			sum += v
		}
		r.Monthly = money.BRLCents(sum / int64(len(recent)))
	}
	return r, true
}

// nextMonthly is a month after a day, the day clamped to that month's end:
// a charge on 31 Oct comes back on 30 Nov, not on 1 Dec.
func nextMonthly(d time.Time) time.Time {
	m := MonthOf(d).Add(1)
	return time.Date(m.year, m.month, min(d.Day(), m.Days()), 0, 0, 0, 0, d.Location())
}

// sameDay reports whether every charge falls within dayBy days of the usual
// day of the month, counting around the month's turn. Days past the 28th
// count as the 28th, so a charge at the end of the month stays at the end.
func sameDay(cs []Charge) bool {
	days := make([]int64, len(cs))
	for i, c := range cs {
		days[i] = int64(min(c.Date.Day(), 28))
	}
	usual := median(days)
	for _, d := range days {
		// Around the month: the 28th and the 1st are a few days apart.
		if dist := abs(d - usual); min(dist, 28-dist) > dayBy {
			return false
		}
	}
	return true
}

// steadiness classifies monthly amounts, oldest first. A single change of
// price keeps a charge fixed: steady before it and the same since.
func steadiness(amounts []int64) (Steadiness, bool) {
	all := spread(amounts)
	if all <= fixedBy {
		return Fixed, true
	}
	n := len(amounts)
	j := n - 1
	for j > 0 && amounts[j-1] == amounts[n-1] {
		j--
	}
	if j > 0 && spread(amounts[:j]) <= fixedBy {
		return Fixed, true
	}
	if all <= variesBy {
		return Varies, true
	}
	return "", false
}

// spread is the furthest an amount is from the median, in basis points.
func spread(amounts []int64) int64 {
	med := median(amounts)
	if med <= 0 {
		return 1 << 62
	}
	var out int64
	for _, a := range amounts {
		out = max(out, abs(a-med)*10000/med)
	}
	return out
}

func median(v []int64) int64 {
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
