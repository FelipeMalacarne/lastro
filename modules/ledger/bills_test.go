package ledger

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"lastro/money"
)

func billDay(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }

// item builds a card transaction for the bills: negative cents a purchase,
// positive a refund; ref its bill.
func item(id string, day time.Time, cents int64, ref string, opts ...func(*BillItem)) BillItem {
	it := BillItem{ID: TransactionID(id), Date: day, Description: id, Amount: money.BRLCents(cents), BillRef: ref,
		Lines: []CategoryLine{{Category: CategoryID{v: "cat-" + id}, Amount: money.BRLCents(cents)}}}
	for _, o := range opts {
		o(&it)
	}
	return it
}

func pending(it *BillItem) { it.Pending = true }

func inCategory(c string) func(*BillItem) {
	return func(it *BillItem) { it.Lines = []CategoryLine{{Category: CategoryID{v: c}, Amount: it.Amount}} }
}

func installment(n, of int) func(*BillItem) {
	return func(it *BillItem) { it.Installment, _ = NewInstallment(n, of) }
}

// payment builds a bill payment from checking, credited in the bill ref.
func payment(id string, day time.Time, cents int64, ref string) BillItem {
	return BillItem{ID: TransactionID(id), Date: day, Description: "Pagamento recebido", Amount: money.BRLCents(cents), Payment: true, BillRef: ref, From: "checking"}
}

func closedBill(ref string, due time.Time, total, charges int64) Bill {
	return Bill{Ref: ref, Due: due, Total: money.BRLCents(total), Charges: money.BRLCents(charges)}
}

// summary is a bill in one line, for comparing.
func summary(b CardBill) string {
	due := "-"
	if !b.Due.IsZero() {
		due = b.Due.Format("01-02")
	}
	return fmt.Sprintf("%s %s due %s total %d paid %d carried %d charges %d buy %d/%d inst %d/%d back %d/%d",
		cmpOr(b.Ref, "·"), b.State, due, b.Total.Cents(), b.Paid.Cents(), b.Carried.Cents(), b.Charges.Cents(),
		b.Purchases.Amount.Cents(), b.Purchases.Count, b.Installments.Amount.Cents(), b.Installments.Count, b.Refunds.Amount.Cents(), b.Refunds.Count)
}

func cmpOr(s, or string) string {
	if s == "" {
		return or
	}
	return s
}

func TestCardBills(t *testing.T) {
	today := billDay(10, 28)
	tests := []struct {
		name    string
		closed  []Bill
		items   []BillItem
		cardDue time.Time
		want    []string
	}{
		{
			name:   "the open bill is what is pending up to today; later installments are the next ones",
			closed: []Bill{closedBill("sep", billDay(10, 5), 98175, 0)},
			items: []BillItem{
				item("aliexpress", billDay(9, 29), -186633, "", pending),
				item("adjustment", billDay(9, 27), 3006, "", pending),
				item("gym", billDay(10, 3), -10710, "", pending, installment(3, 12)),
				item("gym-4", billDay(11, 3), -10710, "", pending, installment(4, 12)),
				item("shopee-3", billDay(11, 15), -24924, "", pending, installment(3, 12)),
				item("steam", billDay(9, 10), -4889, "sep"),
				item("insurance", billDay(9, 11), -93286, "sep"),
				payment("paid-sep", billDay(9, 28), 98175, ""),
			},
			want: []string{
				"· next due - total 35634 paid 0 carried 0 charges 0 buy 0/0 inst 35634/2 back 0/0",
				"· open due 11-05 total 194337 paid 0 carried 0 charges 0 buy 186633/1 inst 10710/1 back 3006/1",
				"sep paid due 10-05 total 98175 paid 98175 carried 0 charges 0 buy 98175/2 inst 0/0 back 0/0",
			},
		},
		{
			name: "a payment credited in a bill paid the one before; one left unpaid comes back with interest",
			closed: []Bill{
				closedBill("aug", billDay(8, 10), 175525, 0),
				closedBill("sep", billDay(9, 10), 210450, 1840),
				closedBill("nov", billDay(11, 10), 148237, 0),
			},
			items: []BillItem{
				item("a1", billDay(7, 20), -175525, "aug"),
				payment("p-aug", billDay(8, 10), 150000, "sep"),
				item("s1", billDay(8, 20), -183085, "sep"),
				payment("p-sep", billDay(9, 9), 210450, "nov"),
				item("o1", billDay(10, 20), -148237, "nov"),
			},
			cardDue: billDay(10, 10),
			want: []string{
				"· open due 12-10 total 0 paid 0 carried 0 charges 0 buy 0/0 inst 0/0 back 0/0",
				"nov unpaid due 11-10 total 148237 paid 0 carried 0 charges 0 buy 148237/1 inst 0/0 back 0/0",
				"sep paid due 09-10 total 210450 paid 210450 carried 25525 charges 1840 buy 183085/1 inst 0/0 back 0/0",
				"aug partly_paid due 08-10 total 175525 paid 150000 carried 0 charges 0 buy 175525/1 inst 0/0 back 0/0",
			},
		},
		{
			name:   "an early payment, once the closed bill is paid, goes to the open bill; after it closes, its total is net of it",
			closed: []Bill{closedBill("sep", billDay(10, 5), 98175, 0)},
			items: []BillItem{
				item("s1", billDay(9, 20), -98175, "sep"),
				payment("paid-sep", billDay(9, 28), 98175, ""),
				item("coffee", billDay(10, 2), -186633, "", pending),
				payment("early", billDay(10, 28), 150000, ""),
			},
			want: []string{
				"· open due 11-05 total 186633 paid 150000 carried 0 charges 0 buy 186633/1 inst 0/0 back 0/0",
				"sep paid due 10-05 total 98175 paid 98175 carried 0 charges 0 buy 98175/1 inst 0/0 back 0/0",
			},
		},
		{
			name: "after the early-paid bill closes, its payment, credited in it, pays it",
			closed: []Bill{
				closedBill("sep", billDay(10, 5), 98175, 0),
				closedBill("oct", billDay(11, 5), 36633, 0), // 1.866,33 less the 1.500,00 paid early
			},
			items: []BillItem{
				item("s1", billDay(9, 20), -98175, "sep"),
				payment("paid-sep", billDay(9, 28), 98175, "oct"),
				item("coffee", billDay(10, 2), -186633, "oct"),
				payment("early", billDay(10, 25), 150000, "oct"),
			},
			cardDue: billDay(11, 5),
			want: []string{
				"· open due 12-05 total 0 paid 0 carried 0 charges 0 buy 0/0 inst 0/0 back 0/0",
				"oct unpaid due 11-05 total 36633 paid 0 carried -150000 charges 0 buy 186633/1 inst 0/0 back 0/0",
				"sep paid due 10-05 total 98175 paid 98175 carried 0 charges 0 buy 98175/1 inst 0/0 back 0/0",
			},
		},
		{
			name:   "a closed bill not paid by its due date is overdue",
			closed: []Bill{closedBill("oct", billDay(10, 5), 50000, 0)},
			items:  []BillItem{item("o1", billDay(9, 20), -50000, "oct")},
			want: []string{
				"· open due 11-05 total 0 paid 0 carried 0 charges 0 buy 0/0 inst 0/0 back 0/0",
				"oct overdue due 10-05 total 50000 paid 0 carried 0 charges 0 buy 50000/1 inst 0/0 back 0/0",
			},
		},
		{
			name: "without the bills endpoint, posted transactions sharing a reference make a bill; the open one falls due on the card's day",
			items: []BillItem{
				item("x1", billDay(8, 20), -30000, "r-aug"),
				item("x2", billDay(8, 25), -20000, "r-aug"),
				item("y1", billDay(9, 20), -40000, "r-sep"),
				payment("p1", billDay(9, 5), 50000, "r-sep"),
				item("open1", billDay(10, 20), -12000, "", pending),
			},
			cardDue: billDay(10, 5),
			want: []string{
				"· open due 11-05 total 12000 paid 0 carried 0 charges 0 buy 12000/1 inst 0/0 back 0/0",
				"r-sep unpaid due - total 40000 paid 0 carried 0 charges 0 buy 40000/1 inst 0/0 back 0/0",
				"r-aug paid due - total 50000 paid 50000 carried 0 charges 0 buy 50000/2 inst 0/0 back 0/0",
			},
		},
		{
			name:   "one I added by hand after the last closed bill is on the open one; an older one is in no bill",
			closed: []Bill{closedBill("sep", billDay(10, 5), 10000, 0)},
			items: []BillItem{
				item("s1", billDay(9, 20), -10000, "sep"),
				item("manual-new", billDay(10, 12), -2500, ""),
				item("manual-old", billDay(9, 1), -900, ""),
				payment("p", billDay(10, 5), 10000, ""),
			},
			want: []string{
				"· open due 11-05 total 2500 paid 0 carried 0 charges 0 buy 2500/1 inst 0/0 back 0/0",
				"sep paid due 10-05 total 10000 paid 10000 carried 0 charges 0 buy 10000/1 inst 0/0 back 0/0",
			},
		},
		{
			name:    "no closed bill and no due day: the open bill has no due date",
			items:   []BillItem{item("p1", billDay(10, 2), -1000, "", pending)},
			want:    []string{"· open due - total 1000 paid 0 carried 0 charges 0 buy 1000/1 inst 0/0 back 0/0"},
			cardDue: time.Time{},
		},
	}
	for _, tt := range tests {
		got := CardBills(tt.closed, tt.items, tt.cardDue, today, money.BRL)
		var lines []string
		for _, b := range got {
			lines = append(lines, summary(b))
		}
		if strings.Join(lines, "\n") != strings.Join(tt.want, "\n") {
			t.Errorf("%s:\ngot\n  %s\nwant\n  %s", tt.name, strings.Join(lines, "\n  "), strings.Join(tt.want, "\n  "))
		}
	}
}

func TestCardBillsByCategoryAndItems(t *testing.T) {
	split := item("split", billDay(10, 10), -10000, "", pending)
	split.Lines = []CategoryLine{{Category: CategoryID{v: "food"}, Amount: money.BRLCents(-7000)}, {Category: CategoryID{v: "fun"}, Amount: money.BRLCents(-3000)}}
	got := CardBills(nil, []BillItem{
		item("a", billDay(10, 2), -5000, "", pending, inCategory("fun")),
		split,
		item("refund", billDay(10, 15), 1000, "", pending, inCategory("fun")),
		item("none", billDay(10, 20), -400, "", pending, inCategory("")),
	}, time.Time{}, billDay(10, 28), money.BRL)
	open := got[0]
	var cats []string
	for _, c := range open.ByCategory {
		cats = append(cats, fmt.Sprintf("%s %d", cmpOr(c.Category.String(), "none"), c.Amount.Cents()))
	}
	if strings.Join(cats, ", ") != "food 7000, fun 7000, none 400" {
		t.Errorf("by category, a split by its parts and a refund netted: %v", cats)
	}
	var ids []string
	for _, it := range open.Items {
		ids = append(ids, string(it.ID))
	}
	if strings.Join(ids, " ") != "none refund split a" {
		t.Errorf("items newest first: %v", ids)
	}
}

func TestOpenDueKeepsTheDayOrTheMonthsLast(t *testing.T) {
	if got := addMonth(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)); !got.Equal(time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("31 Jan + a month = %v, want 28 Feb", got)
	}
	if got := addMonth(time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC)); !got.Equal(time.Date(2027, 1, 5, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("5 Dec + a month = %v, want 5 Jan", got)
	}
}

func TestImportedBillValidates(t *testing.T) {
	ext, _ := NewExternalID("b1")
	usd, _ := money.New(100, money.USD)
	neg := money.BRLCents(-1)
	tests := []struct {
		name string
		in   ImportedBill
		err  error
	}{
		{"valid", ImportedBill{ExternalID: ext, Due: billDay(10, 5), Total: money.BRLCents(100)}, nil},
		{"no id", ImportedBill{Due: billDay(10, 5), Total: money.BRLCents(100)}, ErrInvalid},
		{"no due date", ImportedBill{ExternalID: ext, Total: money.BRLCents(100)}, ErrInvalid},
		{"another currency", ImportedBill{ExternalID: ext, Due: billDay(10, 5), Total: usd}, ErrCurrencyChanged},
		{"a negative minimum", ImportedBill{ExternalID: ext, Due: billDay(10, 5), Total: money.BRLCents(100), Minimum: &neg}, ErrInvalid},
	}
	for _, tt := range tests {
		err := tt.in.validate(money.BRL)
		if (tt.err == nil) != (err == nil) || (tt.err != nil && !errors.Is(err, tt.err)) {
			t.Errorf("%s: got %v, want %v", tt.name, err, tt.err)
		}
	}
}
