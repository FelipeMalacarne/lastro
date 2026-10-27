package budget

import (
	"errors"
	"fmt"
	"time"

	"lastro/money"
)

// ErrInvalid means a value broke one of the budget's rules.
var ErrInvalid = errors.New("budget: invalid")

// Month is a calendar month, the budget's unit.
type Month struct {
	year  int
	month time.Month
}

// MonthOf is the month a moment falls in, in its own location.
func MonthOf(t time.Time) Month { return Month{year: t.Year(), month: t.Month()} }

// ParseMonth reads "2026-10".
func ParseMonth(s string) (Month, error) {
	t, err := time.Parse("2006-01", s)
	if err != nil {
		return Month{}, fmt.Errorf("%w: month %q, want YYYY-MM", ErrInvalid, s)
	}
	return MonthOf(t), nil
}

func (m Month) String() string { return fmt.Sprintf("%04d-%02d", m.year, int(m.month)) }
func (m Month) IsZero() bool   { return m.year == 0 }

// First is the month's first day, at midnight UTC.
func (m Month) First() time.Time { return time.Date(m.year, m.month, 1, 0, 0, 0, 0, time.UTC) }

// Days is how many days the month has.
func (m Month) Days() int { return m.First().AddDate(0, 1, -1).Day() }

// Add moves n months, back when n is negative.
func (m Month) Add(n int) Month { return MonthOf(m.First().AddDate(0, n, 0)) }

func (m Month) Before(o Month) bool {
	return m.year < o.year || (m.year == o.year && m.month < o.month)
}
func (m Month) After(o Month) bool { return o.Before(m) }

// Since is how many months lie from o to m: 1 from September to October.
func (m Month) Since(o Month) int { return (m.year-o.year)*12 + int(m.month) - int(o.month) }

// Payday is the day of the month the salary lands and pay yourself first is
// set aside. A day past the month's end means its last day.
type Payday struct{ d int }

// NewPayday accepts 1 to 31.
func NewPayday(d int) (Payday, error) {
	if d < 1 || d > 31 {
		return Payday{}, fmt.Errorf("%w: payday %d, want 1 to 31", ErrInvalid, d)
	}
	return Payday{d: d}, nil
}

func (p Payday) Day() int { return p.d }

// In is the payday's date in a month.
func (p Payday) In(m Month) time.Time {
	return m.First().AddDate(0, 0, min(p.d, m.Days())-1)
}

// amount checks a budget amount: in reais and never negative.
func amount(m money.Money, what string) error {
	if m.Currency() != money.BRL {
		return fmt.Errorf("%w: %s in %s, want BRL", ErrInvalid, what, m.Currency())
	}
	if m.IsNegative() {
		return fmt.Errorf("%w: negative %s", ErrInvalid, what)
	}
	return nil
}
