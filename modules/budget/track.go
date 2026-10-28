package budget

import (
	"sort"
	"time"

	"lastro/modules/ledger"
	"lastro/money"
)

// Spending is a month's net spending by category: expenses minus refunds,
// positive when money went out.
type Spending map[ledger.CategoryID]money.Money

// on is the spending of some categories together, in cents.
func (s Spending) on(cats []ledger.CategoryID) int64 {
	var cents int64
	for _, c := range cats {
		cents += s[c].Cents()
	}
	return cents
}

// Pace says how a category's spending compares with the share of the month
// gone.
type Pace string

const (
	PaceOn    Pace = "on_pace"
	PaceAhead Pace = "ahead" // more than aheadBy past the month's share
	PaceOver  Pace = "over"  // spent more than available: for a fund, in the red
)

// aheadBy is how far past the month's share spending must be to be ahead of
// pace: ten points, so a single early bill does not raise a flag.
const aheadBy = 1000

// LineStatus is one budget line in a month. For a fund, Carried is the
// balance it brought, debt included, Available that plus its monthly amount
// and adjustment, and Left its balance after the month's spending.
type LineStatus struct {
	Line      Line
	Carried   money.Money   // rolled over from the month before
	Adjusted  money.Money   // a fund's adjustment this month
	Available money.Money   // limit plus carried, plus a fund's adjustment
	Spent     money.Money   // its categories' together
	Left      money.Money   // negative when over
	Used      money.Percent // spent over available; zero when nothing is available
	// Fixed is what recurring charges already took this month, left out of
	// the pace: rent paid on the 5th is not spending ahead of the month.
	Fixed money.Money
	Pace  Pace
}

// Tracking is a month's budget against its spending.
type Tracking struct {
	Month Month
	// Budget is the one that applies, saved for this month or inherited;
	// nil when no budget was ever saved at or before it.
	Budget *MonthlyBudget
	// Saved reports whether the month has its own budget; otherwise it
	// follows Budget's month.
	Saved bool
	From  Month // the saved month it comes from
	// Elapsed is the share of the month gone: all of it for a past month,
	// none for a future one.
	Elapsed  money.Percent
	DaysLeft int // today included; zero for a past month
	// Lines are the budget's: what needs attention first (over, then
	// ahead of pace), then by spending.
	Lines []LineStatus
	// Unlimited is spending in categories no line covers, and with no
	// category yet under the zero id.
	Unlimited Spending
	// Available, Spent and Left are the limits', leaving the funds out: a
	// car fund filled over a year is not this month's money to spend.
	Available money.Money // every limit plus what rolled over
	Spent     money.Money
	Left      money.Money // available minus spent
	Funds     Funds
}

// Funds is the month's funds together (ADR-0011): their monthly amounts,
// what they carried and were adjusted by, what they paid and their balance
// after it. Kept is their monthly amounts minus their spending, what the
// month put away in them; it is negative when the month spent more from
// them than it put in.
type Funds struct {
	Amount, Carried, Adjusted, Spent, Balance, Kept money.Money
	Count                                           int
}

// SetAside is what a month's transfers to accounts outside lastro (the
// broker, the bank's own investments) put aside for the long term: what
// went out, minus what came back, minus what the funds kept, never below
// zero. Redeeming one investment to buy another puts nothing aside, and
// neither does filling a fund kept invested; redeeming it to pay what
// the fund was for takes nothing away (ADR-0011).
func SetAside(out, back, kept money.Money) money.Money {
	cents := out.Cents() - back.Cents() - kept.Cents()
	return money.BRLCents(max(cents, 0))
}

// Effective is the budget that applies to a month: the latest saved at or
// before it, as that month's copy. saved must be in month order.
func Effective(saved []*MonthlyBudget, m Month) (*MonthlyBudget, bool) {
	for i := len(saved) - 1; i >= 0; i-- {
		if !saved[i].month.After(m) {
			return saved[i].For(m), true
		}
	}
	return nil, false
}

// Track follows a month's lines, rolling over what was left in every month
// since the first saved budget and carrying each fund's balance. saved must
// be in month order; spending needs every month from the first saved one to
// m. adjusted are the funds' adjustments and fixed is what recurring charges
// already took in m, by category, kept out of the pace; both may be nil.
func Track(saved []*MonthlyBudget, spending map[Month]Spending, adjusted Adjustments, m Month, today time.Time, fixed Spending) Tracking {
	t := Tracking{Month: m, Unlimited: Spending{}, Elapsed: elapsed(m, today), DaysLeft: daysLeft(m, today)}
	b, ok := Effective(saved, m)
	if !ok {
		zero := money.BRLCents(0)
		t.Available, t.Spent, t.Left = zero, zero, zero
		t.Funds = Funds{Amount: zero, Carried: zero, Adjusted: zero, Spent: zero, Balance: zero, Kept: zero}
		for c, s := range spending[m] {
			t.Unlimited[c] = s
		}
		return t
	}
	t.Budget = b
	for _, s := range saved {
		if !s.month.After(m) {
			t.From = s.month
		}
	}
	t.Saved = t.From == m

	carry := carried(saved, spending, adjusted, m)
	var avail, spent int64
	var funds struct{ amount, carried, adjusted, spent, kept int64 }
	for _, l := range b.Lines() {
		s := spending[m].on(l.Categories)
		in, adj := intake(l, carry[l.ID]), int64(0)
		if l.IsFund() {
			adj = adjusted.of(m, l.ID)
		}
		a := l.Limit.Cents() + in + adj
		st := LineStatus{
			Line:      l,
			Carried:   money.BRLCents(in),
			Adjusted:  money.BRLCents(adj),
			Available: money.BRLCents(a),
			Spent:     money.BRLCents(s),
			Left:      money.BRLCents(a - s),
			Pace:      PaceOn,
		}
		if a > 0 {
			st.Used = money.FromBps(s * 10000 / a)
		}
		f := min(max(fixed.on(l.Categories), 0), s)
		st.Fixed = money.BRLCents(f)
		switch {
		case s > a:
			st.Pace = PaceOver
		case !l.IsFund() && a-f > 0 && (s-f)*10000/(a-f) > t.Elapsed.Bps()+aheadBy:
			st.Pace = PaceAhead // a fund's spending comes in lumps: it is never ahead
		}
		t.Lines = append(t.Lines, st)
		if l.IsFund() {
			funds.amount += l.Limit.Cents()
			funds.carried += in
			funds.adjusted += adj
			funds.spent += s
			funds.kept += l.Limit.Cents() - s
			t.Funds.Count++
			continue
		}
		avail += a
		spent += s
	}
	t.Funds.Amount, t.Funds.Carried, t.Funds.Adjusted = money.BRLCents(funds.amount), money.BRLCents(funds.carried), money.BRLCents(funds.adjusted)
	t.Funds.Spent, t.Funds.Kept = money.BRLCents(funds.spent), money.BRLCents(funds.kept)
	t.Funds.Balance = money.BRLCents(funds.amount + funds.carried + funds.adjusted - funds.spent)
	for c, s := range spending[m] {
		if _, covered := b.LineOf(c); !covered {
			t.Unlimited[c] = s
		}
	}
	rank := map[Pace]int{PaceOver: 0, PaceAhead: 1, PaceOn: 2}
	sort.SliceStable(t.Lines, func(i, j int) bool {
		a, b := t.Lines[i], t.Lines[j]
		if rank[a.Pace] != rank[b.Pace] {
			return rank[a.Pace] < rank[b.Pace]
		}
		if a.Spent.Cents() != b.Spent.Cents() {
			return a.Spent.Cents() > b.Spent.Cents()
		}
		return a.Line.ID.String() < b.Line.ID.String()
	})
	t.Available, t.Spent, t.Left = money.BRLCents(avail), money.BRLCents(spent), money.BRLCents(avail-spent)
	return t
}

// carried is what each line brings into m: walking from the first saved
// month, a limit that rolls over passes on what it has left, never a debt,
// and one that does not passes on nothing; a fund passes on its balance
// after its monthly amount, adjustment and spending, debt included. What a
// line carries follows its id, whatever categories it covers.
func carried(saved []*MonthlyBudget, spending map[Month]Spending, adjusted Adjustments, m Month) map[LineID]int64 {
	carry := map[LineID]int64{}
	if len(saved) == 0 {
		return carry
	}
	for k := saved[0].month; k.Before(m); k = k.Add(1) {
		b, ok := Effective(saved, k)
		if !ok {
			continue
		}
		next := map[LineID]int64{}
		for _, l := range b.Lines() {
			left := l.Limit.Cents() + intake(l, carry[l.ID]) - spending[k].on(l.Categories)
			switch {
			case l.IsFund():
				next[l.ID] = left + adjusted.of(k, l.ID)
			case l.Rollover && left > 0:
				next[l.ID] = left
			}
		}
		carry = next
	}
	return carry
}

// intake is what a line takes of what it carried: a fund all of it, debt
// included; a limit never a debt, so a fund turned into a limit leaves its
// debt behind.
func intake(l Line, carried int64) int64 {
	if l.IsFund() {
		return carried
	}
	return max(carried, 0)
}

// elapsed is the share of m gone by today, counting today as gone.
func elapsed(m Month, today time.Time) money.Percent {
	now := MonthOf(today)
	switch {
	case m.Before(now):
		return money.FromBps(10000)
	case m.After(now):
		return money.FromBps(0)
	}
	return money.FromBps(int64(today.Day()) * 10000 / int64(m.Days()))
}

func daysLeft(m Month, today time.Time) int {
	now := MonthOf(today)
	switch {
	case m.Before(now):
		return 0
	case m.After(now):
		return m.Days()
	}
	return m.Days() - today.Day() + 1
}
