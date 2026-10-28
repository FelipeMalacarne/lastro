package budget

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"lastro/money"
)

var (
	// ErrInvalid means a value broke one of the budget's rules.
	ErrInvalid = errors.New("budget: invalid")
	// ErrShared means a category's own limit was asked for while it shares
	// a line with others: the line is what changes.
	ErrShared = errors.New("budget: the category shares a line")
	// ErrNoLine means the month's budget has no line with that id.
	ErrNoLine = errors.New("budget: no such line")
)

// LineID names a budget line from month to month, so what it carried
// follows it when its categories change (ADR-0012). A line saved before a
// line could cover several categories has its category's id.
type LineID struct{ s string }

// ParseLineID reads a line's id as the API and the database write it: a
// UUID.
func ParseLineID(s string) (LineID, error) {
	s = strings.ToLower(s)
	if len(s) != 36 {
		return LineID{}, fmt.Errorf("%w: line id %q, want a UUID", ErrInvalid, s)
	}
	for i, r := range s {
		hyphen := i == 8 || i == 13 || i == 18 || i == 23
		if hyphen != (r == '-') || !hyphen && !strings.ContainsRune("0123456789abcdef", r) {
			return LineID{}, fmt.Errorf("%w: line id %q, want a UUID", ErrInvalid, s)
		}
	}
	return LineID{s: s}, nil
}

// newLineID is a random (version 4) UUID.
func newLineID() LineID {
	var b [16]byte
	_, _ = rand.Read(b[:]) // never fails: crypto/rand panics instead of returning an error
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return LineID{s: fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])}
}

func (id LineID) String() string { return id.s }
func (id LineID) IsZero() bool   { return id.s == "" }

// maxLineName is how long a shared line's name may be.
const maxLineName = 40

// lineName checks a line's name: required when it covers several
// categories, at most 40 characters, without the spaces around it.
func lineName(name string, shared bool) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "" && shared:
		return "", fmt.Errorf("%w: a line with several categories needs a name", ErrInvalid)
	case utf8.RuneCountInString(name) > maxLineName:
		return "", fmt.Errorf("%w: a line's name is at most %d characters", ErrInvalid, maxLineName)
	}
	return name, nil
}

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
