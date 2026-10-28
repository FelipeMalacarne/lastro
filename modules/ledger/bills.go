package ledger

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"lastro/money"
)

// ImportedBill is what a source says about one of a card's closed bills.
type ImportedBill struct {
	ExternalID ExternalID
	Due        time.Time   // the calendar day is what counts
	Total      money.Money // what the bill asks for, positive when owed
	Minimum    *money.Money
	Charges    money.Money // interest and fees in it; zero when none
}

func (in ImportedBill) validate(currency money.Currency) error {
	sameCurrency := func(m money.Money) bool { return m.IsZero() || m.Currency() == currency }
	switch {
	case in.ExternalID.IsZero():
		return fmt.Errorf("%w: bill without an external id", ErrInvalid)
	case in.Due.IsZero():
		return fmt.Errorf("%w: bill %s without a due date", ErrInvalid, in.ExternalID)
	case in.Total.Currency() != currency || !sameCurrency(in.Charges) || (in.Minimum != nil && !sameCurrency(*in.Minimum)):
		return fmt.Errorf("%w: bill %s is not in its card's %s", ErrCurrencyChanged, in.ExternalID, currency)
	case in.Minimum != nil && in.Minimum.IsNegative(), in.Charges.IsNegative():
		return fmt.Errorf("%w: bill %s with a negative minimum or charges", ErrInvalid, in.ExternalID)
	}
	return nil
}

// Bill is one of a card's closed bills as the source last reported it, the
// input of CardBills. Ref is the source's id, which a transaction's bill
// reference points to.
type Bill struct {
	Ref     string
	Due     time.Time
	Total   money.Money // positive when owed
	Minimum *money.Money
	Charges money.Money
}

// BillItem is one of a card's live transactions as its bills read it, the
// input of CardBills.
type BillItem struct {
	ID          TransactionID
	Date        time.Time
	Description string
	Amount      money.Money // negative: a purchase; positive: a refund or a payment
	Payment     bool        // a bill payment, as the source said
	Pending     bool
	Installment Installment
	BillRef     string    // the source's bill, empty while on the open one
	From        AccountID // a payment's other side, once paired
	Lines       []CategoryLine
}

// CategoryLine is where part of a transaction went: the whole of it, or one
// part of a split.
type CategoryLine struct {
	Category CategoryID // zero: none yet
	Amount   money.Money
}

// BillState says where a bill stands.
type BillState string

const (
	BillNext       BillState = "next"        // installments already set for the bills to come
	BillOpen       BillState = "open"        // still taking purchases
	BillUnpaid     BillState = "unpaid"      // closed, nothing paid yet, not due yet
	BillOverdue    BillState = "overdue"     // closed, nothing paid, past its due date
	BillPartlyPaid BillState = "partly_paid" // closed, paid in part
	BillPaid       BillState = "paid"
)

// BillPart is one kind of movement in a bill: its amount, positive, and how
// many there were.
type BillPart struct {
	Amount money.Money
	Count  int
}

// CardBill is one bill of a card: what is in it and how it was paid.
type CardBill struct {
	Ref          string // the source's id; empty for the open and next ones
	State        BillState
	Due          time.Time   // zero when unknown
	Total        money.Money // what it asks for, positive when owed
	Minimum      *money.Money
	Charges      money.Money // interest and fees
	Carried      money.Money // what the bill before left unpaid, in this one
	Purchases    BillPart
	Installments BillPart
	Refunds      BillPart
	Paid         money.Money
	PaidOn       time.Time // the last payment's day
	PaidFrom     AccountID // the last payment's other side
	LastDay      time.Time // its latest transaction's day
	ByCategory   []CategoryLine
	Items        []BillItem // newest first, payments left out
}

// CardBills puts a card's transactions into its bills (a domain service: it
// reads the stored closed bills and every live transaction of the card).
//
//   - A transaction goes into the closed bill its bill reference names. With
//     no closed bill stored at all (a source without the bills endpoint), the
//     posted transactions sharing a reference make one, without a due date.
//   - The rest: what is dated after today goes to the next bills (the
//     installments already set); pending is the open bill, and so is a posted
//     transaction without a closed bill dated after the last closed one (one I
//     added by hand).
//   - A closed bill's total is the source's; what it holds beyond its own
//     transactions and charges is what the bill before left unpaid.
//   - A bill payment pays the closed bill before the one it is credited in,
//     or the latest closed bill while it is on the open one. Paid in full,
//     in part, nothing yet, or nothing and past due.
//
// The open bill is due a month after the last closed one, or on the card's
// due day when no closed bill is known. Bills come out next, open, then the
// closed ones, the latest first.
func CardBills(closed []Bill, items []BillItem, cardDue, today time.Time, currency money.Currency) []CardBill {
	today = civilDate(today)

	byRef := map[string]*billSums{}
	var order []*billSums // the closed bills
	for _, b := range closed {
		a := newBillSums(b.Ref, BillUnpaid)
		a.due, a.minimum, a.charges, a.total, a.known = civilDate(b.Due), b.Minimum, b.Charges.Cents(), b.Total.Cents(), true
		byRef[b.Ref], order = a, append(order, a)
	}
	if len(closed) == 0 {
		for _, it := range items {
			if it.BillRef != "" && !it.Pending && byRef[it.BillRef] == nil {
				a := newBillSums(it.BillRef, BillUnpaid)
				byRef[it.BillRef], order = a, append(order, a)
			}
		}
	}

	open, next := newBillSums("", BillOpen), newBillSums("", BillNext)
	var payments, unbilled []BillItem
	var lastClosed time.Time
	for _, it := range items {
		switch a := byRef[it.BillRef]; {
		case it.Payment:
			payments = append(payments, it)
		case it.BillRef != "" && a != nil:
			a.add(it)
			if d := civilDate(it.Date); d.After(lastClosed) {
				lastClosed = d
			}
		default:
			unbilled = append(unbilled, it)
		}
	}
	for _, it := range unbilled {
		switch d := civilDate(it.Date); {
		case d.After(today):
			next.add(it)
		case it.Pending || d.After(lastClosed):
			open.add(it)
		}
	}

	// The closed bills in the order they fell due: by due date, or by their
	// last transaction when the source gave none.
	slices.SortStableFunc(order, func(a, b *billSums) int {
		return cmp.Or(a.due.Compare(b.due), a.lastDay.Compare(b.lastDay), strings.Compare(a.ref, b.ref))
	})
	index := make(map[string]int, len(order))
	for i, a := range order {
		index[a.ref] = i
	}
	slices.SortStableFunc(payments, func(a, b BillItem) int { return a.Date.Compare(b.Date) })
	for _, p := range payments {
		target := len(order) - 1
		if i, ok := index[p.BillRef]; ok && p.BillRef != "" {
			target = i - 1 // credited in this bill: it paid the one before
		}
		if target < 0 {
			continue // it paid a bill older than the history the source keeps
		}
		a := order[target]
		a.paid += p.Amount.Cents()
		if d := civilDate(p.Date); !d.Before(a.paidOn) {
			a.paidOn, a.paidFrom = d, p.From
		}
	}

	open.due = openDue(order, cardDue, today)
	var out []CardBill
	if len(next.items) > 0 {
		out = append(out, next.bill(today, currency))
	}
	out = append(out, open.bill(today, currency))
	for i := len(order) - 1; i >= 0; i-- {
		out = append(out, order[i].bill(today, currency))
	}
	return out
}

// billSums adds up one bill while CardBills fills it, in cents.
type billSums struct {
	ref     string
	state   BillState
	due     time.Time
	known   bool // a closed bill the source reported: its total and due date are the source's
	total   int64
	minimum *money.Money
	charges int64
	spent   int64 // purchases and installments, net of refunds
	parts   [3]struct {
		cents int64
		n     int
	} // purchases, installments, refunds
	paid     int64
	paidOn   time.Time
	paidFrom AccountID
	lastDay  time.Time
	lines    map[CategoryID]int64
	items    []BillItem
}

func newBillSums(ref string, state BillState) *billSums {
	return &billSums{ref: ref, state: state, lines: map[CategoryID]int64{}}
}

const (
	partPurchases = iota
	partInstallments
	partRefunds
)

func (a *billSums) add(it BillItem) {
	cents := it.Amount.Cents()
	part := partPurchases
	switch {
	case cents > 0:
		part = partRefunds
	case !it.Installment.IsZero():
		part = partInstallments
	}
	a.parts[part].n++
	if cents < 0 {
		a.parts[part].cents -= cents
	} else {
		a.parts[part].cents += cents
	}
	a.spent -= cents
	for _, l := range it.Lines {
		a.lines[l.Category] -= l.Amount.Cents()
	}
	if d := civilDate(it.Date); d.After(a.lastDay) {
		a.lastDay = d
	}
	a.items = append(a.items, it)
}

func (a *billSums) bill(today time.Time, currency money.Currency) CardBill {
	m := func(cents int64) money.Money { v, _ := money.New(cents, currency); return v }
	total := a.spent
	b := CardBill{
		Ref: a.ref, State: a.state, Due: a.due, Minimum: a.minimum, Charges: m(a.charges),
		Purchases:    BillPart{Amount: m(a.parts[partPurchases].cents), Count: a.parts[partPurchases].n},
		Installments: BillPart{Amount: m(a.parts[partInstallments].cents), Count: a.parts[partInstallments].n},
		Refunds:      BillPart{Amount: m(a.parts[partRefunds].cents), Count: a.parts[partRefunds].n},
		Paid:         m(a.paid), PaidOn: a.paidOn, PaidFrom: a.paidFrom, LastDay: a.lastDay,
	}
	if a.known {
		total = a.total
		b.Carried = m(a.total - a.spent - a.charges)
	} else {
		b.Carried = m(0)
	}
	b.Total = m(total)
	if b.State == BillUnpaid {
		switch {
		case total <= 0 || a.paid >= total:
			b.State = BillPaid
		case a.paid > 0:
			b.State = BillPartlyPaid
		case !a.due.IsZero() && today.After(a.due):
			b.State = BillOverdue
		}
	}
	for c, cents := range a.lines {
		b.ByCategory = append(b.ByCategory, CategoryLine{Category: c, Amount: m(cents)})
	}
	slices.SortFunc(b.ByCategory, func(x, y CategoryLine) int {
		return cmp.Or(cmp.Compare(y.Amount.Cents(), x.Amount.Cents()), strings.Compare(x.Category.String(), y.Category.String()))
	})
	b.Items = slices.Clone(a.items)
	slices.SortStableFunc(b.Items, func(x, y BillItem) int {
		return cmp.Or(y.Date.Compare(x.Date), strings.Compare(string(x.ID), string(y.ID)))
	})
	return b
}

// openDue is when the open bill falls due: a month after the last closed
// bill the source dated, else the card's due day, rolled forward to today.
func openDue(order []*billSums, cardDue, today time.Time) time.Time {
	for i := len(order) - 1; i >= 0; i-- {
		if !order[i].due.IsZero() {
			return addMonth(order[i].due)
		}
	}
	if cardDue.IsZero() {
		return time.Time{}
	}
	d := civilDate(cardDue)
	for d.Before(today) {
		d = addMonth(d)
	}
	return d
}

// addMonth is the same day a month later, or the month's last day when it
// has no such day.
func addMonth(d time.Time) time.Time {
	y, m, day := d.Date()
	first := time.Date(y, m+1, 1, 0, 0, 0, 0, d.Location())
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(y, m+1, min(day, last), 0, 0, 0, 0, d.Location())
}
