package portfolio

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"lastro/money"
)

// Thesis is why I hold a stock, in a line, and what would make me sell it
// (F8). The next earnings report is when to read both again.
type Thesis struct {
	Ticker     Ticker
	Thesis     string
	Breaks     string
	NextReport *time.Time
	UpdatedAt  time.Time
}

const maxThesis = 280

// NewThesis checks that both lines are there and short.
func NewThesis(t Ticker, thesis, breaks string, next *time.Time, now time.Time) (Thesis, error) {
	thesis, breaks = strings.TrimSpace(thesis), strings.TrimSpace(breaks)
	switch {
	case t.IsZero():
		return Thesis{}, fmt.Errorf("%w: thesis without a ticker", ErrInvalid)
	case thesis == "" || breaks == "":
		return Thesis{}, fmt.Errorf("%w: a thesis needs both why and what would break it", ErrInvalid)
	case utf8.RuneCountInString(thesis) > maxThesis || utf8.RuneCountInString(breaks) > maxThesis:
		return Thesis{}, fmt.Errorf("%w: keep each line under %d characters", ErrInvalid, maxThesis)
	}
	return Thesis{Ticker: t, Thesis: thesis, Breaks: breaks, NextReport: next, UpdatedAt: now}, nil
}

// ReportIn is how many days until the next report, from today; false when
// none is set or it already passed.
func (t Thesis) ReportIn(today time.Time) (int, bool) {
	if t.NextReport == nil {
		return 0, false
	}
	d := int(t.NextReport.Sub(today).Hours() / 24)
	return d, d >= 0
}

// PurchaseStatus is where a planned purchase stands.
type PurchaseStatus string

const (
	Planned   PurchaseStatus = "planned"
	Done      PurchaseStatus = "done"
	Cancelled PurchaseStatus = "cancelled"
)

// ErrDecided means a purchase already done or cancelled was changed.
var ErrDecided = errors.New("portfolio: the purchase was already decided")

// Purchase is a buy I planned (F7 P1): when, what, how much and on what
// condition. lastro never buys; I mark it done or cancelled.
type Purchase struct {
	ID        string
	PlannedOn time.Time
	What      string
	Class     Class
	Amount    money.Money
	Condition string
	Status    PurchaseStatus
	Note      string
	CreatedAt time.Time
	DecidedAt *time.Time
}

// NewPurchase plans a buy.
func NewPurchase(on time.Time, what string, c Class, amount money.Money, condition string, now time.Time) (Purchase, error) {
	what = strings.TrimSpace(what)
	if what == "" {
		return Purchase{}, fmt.Errorf("%w: a purchase needs what to buy", ErrInvalid)
	}
	if _, err := ParseClass(string(c)); err != nil {
		return Purchase{}, err
	}
	if !amount.IsPositive() || amount.Currency() != money.BRL {
		return Purchase{}, fmt.Errorf("%w: purchase of %s", ErrInvalid, amount)
	}
	return Purchase{ID: newID(), PlannedOn: on, What: what, Class: c, Amount: amount, Condition: strings.TrimSpace(condition), Status: Planned, CreatedAt: now}, nil
}

// Decide marks a planned purchase done or cancelled, with a note.
func (p *Purchase) Decide(s PurchaseStatus, note string, now time.Time) error {
	if p.Status != Planned {
		return ErrDecided
	}
	if s != Done && s != Cancelled {
		return fmt.Errorf("%w: status %q", ErrInvalid, s)
	}
	p.Status, p.Note, p.DecidedAt = s, strings.TrimSpace(note), &now
	return nil
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
