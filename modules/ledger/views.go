package ledger

import (
	"fmt"
	"strings"
	"time"
)

// The read side (CQRS-lite): what the screens list, straight from SQL into
// these DTOs. They are also the JSON the API answers with.

// AccountView is an account as the screens show it.
type AccountView struct {
	ID           AccountID `json:"id"`
	Name         string    `json:"name"`
	Institution  string    `json:"institution"`
	Type         string    `json:"type"`
	Number       string    `json:"number"`
	Currency     string    `json:"currency"`
	BalanceCents int64     `json:"balanceCents"`
	BalanceAt    time.Time `json:"balanceAt"`
	Card         *CardView `json:"card"` // credit cards, when the bank reports any of it
}

// CardView is a card's limit and bill, each part only when the bank reports it.
type CardView struct {
	LimitCents     *int64  `json:"limitCents"`
	AvailableCents *int64  `json:"availableCents"`
	DueDate        *string `json:"dueDate"`   // "2026-11-10"
	CloseDate      *string `json:"closeDate"` // Nubank sends none
}

// TransactionView is a transaction as the Transactions screen lists it.
type TransactionView struct {
	ID          TransactionID    `json:"id"`
	Date        string           `json:"date"` // calendar day, "2026-10-26"
	Description string           `json:"description"`
	AmountCents int64            `json:"amountCents"`
	Currency    string           `json:"currency"`
	Kind        string           `json:"kind"`
	Status      string           `json:"status"`
	Installment *InstallmentView `json:"installment"`
	Category    *CategoryView    `json:"category"`
	Account     AccountRef       `json:"account"`
	// Counterpart is the other side's account, for transfers and bill
	// payments, and CounterpartID the other side itself.
	Counterpart   *AccountRef    `json:"counterpart"`
	CounterpartID *TransactionID `json:"counterpartId"`
}

type InstallmentView struct {
	Number int `json:"number"`
	Total  int `json:"total"`
}

type CategoryView struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

// Categorizable is a transaction categorize may give a category to: a live
// expense or income whose category nobody chose by hand.
type Categorizable struct {
	ID          TransactionID
	Account     AccountID
	Description string
	Date        time.Time // the calendar day
	AmountCents int64     // negative when money left the account
	Category    string    // its category's id, or empty while it has none
	Source      Source    // who chose it: a rule or the model (never by hand)
}

// DescriptionCount is how often a description came in an account, and when
// last: what categorize needs to say how many transactions a rule meets.
type DescriptionCount struct {
	Description string
	Account     AccountID
	Count       int
	Last        time.Time
}

// CategoryTotal is a range's expenses and income in one category (empty:
// still without one): money out and in, as positive cents, and how many.
type CategoryTotal struct {
	Category string `json:"category"` // empty: none yet
	OutCents int64  `json:"outCents"`
	InCents  int64  `json:"inCents"`
	Count    int    `json:"count"`
}

// CategoryMonth is one month's live expenses or income in one category
// (empty: none yet) and kind.
type CategoryMonth struct {
	Month    time.Time // the month's first day
	Category string
	Kind     string // "expense" or "income"
	OutCents int64  // positive
	InCents  int64
	Count    int
}

// MonthKind is one month's live transactions of one kind and direction,
// paired with one in another of my accounts or not.
type MonthKind struct {
	Month  time.Time // the month's first day
	Kind   string
	Inflow bool
	Paired bool
	Cents  int64 // negative for outflows
	Count  int
	Last   time.Time // the latest day among them
}

// Charge is a live expense outside installments, for recurring charges.
type Charge struct {
	ID          TransactionID
	Description string
	Date        time.Time
	AmountCents int64  // positive
	Category    string // empty: none yet
	Account     string // "Nubank Ultravioleta"
}

// AccountRef names an account inside another view.
type AccountRef struct {
	ID          AccountID `json:"id"`
	Name        string    `json:"name"`
	Institution string    `json:"institution"`
	Type        string    `json:"type"`
}

// Flow narrows a list by what the money did.
type Flow string

const (
	FlowAll      Flow = ""
	FlowIn       Flow = "in"       // money in that counts: income, refunds
	FlowOut      Flow = "out"      // money out that counts: spending
	FlowTransfer Flow = "transfer" // transfers and card bill payments
)

// ParseFlow accepts "", "in", "out" and "transfer".
func ParseFlow(s string) (Flow, error) {
	switch f := Flow(s); f {
	case FlowAll, FlowIn, FlowOut, FlowTransfer:
		return f, nil
	}
	return "", fmt.Errorf("%w: flow %q", ErrInvalid, s)
}

// maxListed is how many transactions one month's list returns at most.
const maxListed = 2000

// TransactionFilter selects a month of transactions.
type TransactionFilter struct {
	Month time.Time // any day in the month
	// AllMonths searches every month instead of Month; only with a Search.
	AllMonths bool
	Account AccountID // empty: every account
	Flow    Flow
	// Search matches descriptions, ignoring case and accents.
	Search string
	// Day narrows the month to one of its days; zero is the whole month.
	Day time.Time
	// Category is a category's id, or CategoryNone for the expenses and
	// income still without one; empty is every transaction.
	Category string
}

// CategoryNone filters the expenses and income still waiting for a category.
const CategoryNone = "none"

// MonthRange returns the first day of the filter's month and of the next.
func (f TransactionFilter) MonthRange() (from, to time.Time) {
	return monthOf(f.Month), monthOf(f.Month).AddDate(0, 1, 0)
}

// Range is the days the filter lists: its day, or its month.
func (f TransactionFilter) Range() (from, to time.Time) {
	if !f.Day.IsZero() {
		d := civilDate(f.Day)
		return d, d.AddDate(0, 0, 1)
	}
	return f.MonthRange()
}

func monthOf(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
}

// likePattern escapes the LIKE wildcards in a search term.
func likePattern(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(s))
}

// TransactionList is a month of transactions and what they add up to.
type TransactionList struct {
	Transactions []TransactionView `json:"transactions"`
	Summary      ListSummary       `json:"summary"`
	// Truncated is set when the month has more than the list returns.
	Truncated bool `json:"truncated"`
}

// ListSummary counts a list. In and out add only what counts: transfers and
// card bill payments are left out, as everywhere.
type ListSummary struct {
	Count    int   `json:"count"`
	Pending  int   `json:"pending"`
	InCents  int64 `json:"inCents"`
	OutCents int64 `json:"outCents"` // negative
}

func summarize(txs []TransactionView) ListSummary {
	s := ListSummary{Count: len(txs)}
	for _, t := range txs {
		if t.Status == string(StatusPending) {
			s.Pending++
		}
		if !Kind(t.Kind).Counts() {
			continue
		}
		if t.AmountCents > 0 {
			s.InCents += t.AmountCents
		} else {
			s.OutCents += t.AmountCents
		}
	}
	return s
}

// BalanceHistoryView is an account's balance at the end of each day, rebuilt
// from the bank's balance and the transactions (see BalanceHistory).
type BalanceHistoryView struct {
	Currency string             `json:"currency"`
	AsOf     time.Time          `json:"asOf"` // when the bank reported the balance
	Points   []BalancePointView `json:"points"`
}

type BalancePointView struct {
	Date         string `json:"date"`
	BalanceCents int64  `json:"balanceCents"`
}

// MonthView is a month of one account: everything in and out, what counts
// (spending, and refunds that lower it), and the flows it is made of.
type MonthView struct {
	Month        string     `json:"month"`  // "2026-10"
	Future       bool       `json:"future"` // after today's month: only what is already committed
	InCents      int64      `json:"inCents"`
	OutCents     int64      `json:"outCents"`   // negative
	SpentCents   int64      `json:"spentCents"` // positive: outflows that count
	RefundsCents int64      `json:"refundsCents"`
	Purchases    int        `json:"purchases"`
	Flows        []FlowView `json:"flows"`
}

// FlowView is one part of a month: a kind, a direction and, for pairs, the
// other side's account.
type FlowView struct {
	Kind        string      `json:"kind"`
	Direction   string      `json:"direction"` // "in" or "out"
	Counterpart *AccountRef `json:"counterpart"`
	Cents       int64       `json:"cents"`
	Count       int         `json:"count"`
}

// MonthFlowRow is what the repository sums for monthViews.
type MonthFlowRow struct {
	Month       time.Time
	Kind        Kind
	Inflow      bool
	Counterpart *AccountRef
	Cents       int64
	Count       int
}

// monthViews folds the rows into one view per month, oldest first. A month
// after today's is marked future.
func monthViews(rows []MonthFlowRow, today time.Time) []MonthView {
	var out []MonthView
	now := monthOf(today)
	for _, r := range rows {
		key := monthOf(r.Month).Format("2006-01")
		if len(out) == 0 || out[len(out)-1].Month != key {
			out = append(out, MonthView{Month: key, Future: monthOf(r.Month).After(now), Flows: []FlowView{}})
		}
		m := &out[len(out)-1]
		dir := "out"
		if r.Inflow {
			dir = "in"
			m.InCents += r.Cents
		} else {
			m.OutCents += r.Cents
		}
		if r.Kind.Counts() {
			if r.Inflow {
				if r.Kind == KindExpense {
					m.RefundsCents += r.Cents
				}
			} else {
				m.SpentCents -= r.Cents
				m.Purchases += r.Count
			}
		}
		m.Flows = append(m.Flows, FlowView{Kind: string(r.Kind), Direction: dir, Counterpart: r.Counterpart, Cents: r.Cents, Count: r.Count})
	}
	return out
}

// PurchaseView is a card purchase in installments (see GroupInstallments).
type PurchaseView struct {
	Description    string                    `json:"description"`
	Total          int                       `json:"total"`
	AmountCents    int64                     `json:"amountCents"` // one installment, positive
	Current        int                       `json:"current"`     // the latest installment billed by today, 0 for none yet
	RemainingCount int                       `json:"remainingCount"`
	RemainingCents int64                     `json:"remainingCents"`
	FirstDate      string                    `json:"firstDate"` // of the first installment the ledger has
	LastDate       string                    `json:"lastDate"`  // of the last one, the source's or a month apart per missing one
	Installments   []PurchaseInstallmentView `json:"installments"`
}

type PurchaseInstallmentView struct {
	ID     TransactionID `json:"id"`
	Number int           `json:"number"`
	Date   string        `json:"date"`
	Status string        `json:"status"`
}

func purchaseView(p Purchase, today time.Time) PurchaseView {
	n, left := p.Remaining(today)
	v := PurchaseView{
		Description: p.Description, Total: p.Total, AmountCents: p.Amount.Abs().Cents(),
		Current: p.Current(today), RemainingCount: n, RemainingCents: left.Cents(),
		FirstDate: p.Installments[0].Date.Format(time.DateOnly), LastDate: p.Last().Format(time.DateOnly),
		Installments: make([]PurchaseInstallmentView, len(p.Installments)),
	}
	for i, r := range p.Installments {
		v.Installments[i] = PurchaseInstallmentView{ID: r.ID, Number: r.Installment.number, Date: r.Date.Format(time.DateOnly), Status: string(r.Status)}
	}
	return v
}

// TransactionDetail is one transaction with what the detail panel shows:
// the other side of its pair, its purchase's installments and how often its
// description came up in the last six months.
type TransactionDetail struct {
	Transaction TransactionView  `json:"transaction"`
	Counterpart *TransactionView `json:"counterpartTransaction"`
	Purchase    *PurchaseView    `json:"purchase"`
	History     DescriptionStats `json:"history"`
}

// DescriptionStats counts a description, month by month, oldest first.
type DescriptionStats struct {
	Count      int          `json:"count"`
	TotalCents int64        `json:"totalCents"`
	Months     []MonthTotal `json:"months"`
}

type MonthTotal struct {
	Month string `json:"month"`
	Cents int64  `json:"cents"`
	Count int    `json:"count"`
}

// DaysView is a month's spending day by day, with the month before for
// comparison and what the month adds up to.
type DaysView struct {
	Month    string      `json:"month"`
	Days     []DayView   `json:"days"`
	Previous []DayView   `json:"previous"`
	Summary  DaysSummary `json:"summary"`
}

type DayView struct {
	Date       string `json:"date"`
	SpentCents int64  `json:"spentCents"`
	Purchases  int    `json:"purchases"`
	InCents    int64  `json:"inCents"`
	MovedCents int64  `json:"movedCents"`
}

type DaysSummary struct {
	SpentCents int64 `json:"spentCents"`
	InCents    int64 `json:"inCents"`
	MovedCents int64 `json:"movedCents"`
	// PreviousToDateCents is what the month before had spent by the same day
	// of the month as the last day listed (or by its end, when shorter).
	PreviousToDateCents int64 `json:"previousToDateCents"`
}

// descriptionStats fills the six months from from with what the rows count,
// zero for a month without any.
func descriptionStats(rows []MonthTotal, from time.Time) DescriptionStats {
	st := DescriptionStats{Months: make([]MonthTotal, 6)}
	for i := range st.Months {
		key := monthOf(from).AddDate(0, i, 0).Format("2006-01")
		st.Months[i] = MonthTotal{Month: key}
		for _, r := range rows {
			if r.Month == key {
				st.Months[i] = r
				st.Count += r.Count
				st.TotalCents += r.Cents
			}
		}
	}
	return st
}
