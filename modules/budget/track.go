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
	PaceOver  Pace = "over"  // spent more than available
)

// aheadBy is how far past the month's share spending must be to be ahead of
// pace: ten points, so a single early bill does not raise a flag.
const aheadBy = 1000

// LineStatus is one budget line in a month.
type LineStatus struct {
	Line      Line
	Carried   money.Money   // rolled over from the month before
	Available money.Money   // limit plus carried
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
	Available money.Money // every limit plus what rolled over
	Spent     money.Money // in the lines
	Left      money.Money // available minus spent
}

// SetAside is what a month's transfers to accounts outside lastro (the
// broker, the bank's own investments) put aside: what went out minus what
// came back, never below zero. Redeeming one investment to buy another puts
// nothing aside.
func SetAside(out, back money.Money) money.Money {
	if back.Cents() >= out.Cents() {
		return money.BRLCents(0)
	}
	return money.BRLCents(out.Cents() - back.Cents())
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
// since the first saved budget. saved must be in month order; spending needs
// every month from the first saved one to m. fixed is what recurring charges
// already took in m, by category, kept out of the pace; it may be nil.
func Track(saved []*MonthlyBudget, spending map[Month]Spending, m Month, today time.Time, fixed Spending) Tracking {
	t := Tracking{Month: m, Unlimited: Spending{}, Elapsed: elapsed(m, today), DaysLeft: daysLeft(m, today)}
	b, ok := Effective(saved, m)
	if !ok {
		t.Available, t.Spent, t.Left = money.BRLCents(0), money.BRLCents(0), money.BRLCents(0)
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

	carry := carried(saved, spending, m)
	var avail, spent int64
	for _, l := range b.Lines() {
		s := spending[m].on(l.Categories)
		a := l.Limit.Cents() + carry[l.ID]
		st := LineStatus{
			Line:      l,
			Carried:   money.BRLCents(carry[l.ID]),
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
		case a-f > 0 && (s-f)*10000/(a-f) > t.Elapsed.Bps()+aheadBy:
			st.Pace = PaceAhead
		}
		t.Lines = append(t.Lines, st)
		avail += a
		spent += s
	}
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
// month, a line that rolls over passes on what it has left, never a debt;
// one that does not roll over passes on nothing. What a line carries
// follows its id, whatever categories it covers.
func carried(saved []*MonthlyBudget, spending map[Month]Spending, m Month) map[LineID]int64 {
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
			left := l.Limit.Cents() + carry[l.ID] - spending[k].on(l.Categories)
			if l.Rollover && left > 0 {
				next[l.ID] = left
			}
		}
		carry = next
	}
	return carry
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
