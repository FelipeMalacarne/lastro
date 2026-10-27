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

// CategoryStatus is one limited category in a month.
type CategoryStatus struct {
	Category  ledger.CategoryID
	Limit     money.Money
	Carried   money.Money // rolled over from the month before
	Available money.Money // limit plus carried
	Spent     money.Money
	Left      money.Money // negative when over
	Rollover  bool
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
	// Categories are the limited ones: what needs attention first (over,
	// then ahead of pace), then by spending.
	Categories []CategoryStatus
	// Unlimited is spending in categories without a limit, and with no
	// category yet under the zero id.
	Unlimited Spending
	Available money.Money // every limit plus what rolled over
	Spent     money.Money // in limited categories
	Left      money.Money // available minus spent
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

// Track follows a month's limits, rolling over what was left in every month
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
		c := l.Category
		s := spending[m][c].Cents()
		a := l.Limit.Cents() + carry[c]
		st := CategoryStatus{
			Category:  c,
			Limit:     l.Limit,
			Carried:   money.BRLCents(carry[c]),
			Available: money.BRLCents(a),
			Spent:     money.BRLCents(s),
			Left:      money.BRLCents(a - s),
			Rollover:  l.Rollover,
			Pace:      PaceOn,
		}
		if a > 0 {
			st.Used = money.FromBps(s * 10000 / a)
		}
		f := min(max(fixed[c].Cents(), 0), s)
		st.Fixed = money.BRLCents(f)
		switch {
		case s > a:
			st.Pace = PaceOver
		case a-f > 0 && (s-f)*10000/(a-f) > t.Elapsed.Bps()+aheadBy:
			st.Pace = PaceAhead
		}
		t.Categories = append(t.Categories, st)
		avail += a
		spent += s
	}
	for c, s := range spending[m] {
		if _, limited := b.Line(c); !limited {
			t.Unlimited[c] = s
		}
	}
	rank := map[Pace]int{PaceOver: 0, PaceAhead: 1, PaceOn: 2}
	sort.SliceStable(t.Categories, func(i, j int) bool {
		a, b := t.Categories[i], t.Categories[j]
		if rank[a.Pace] != rank[b.Pace] {
			return rank[a.Pace] < rank[b.Pace]
		}
		if a.Spent.Cents() != b.Spent.Cents() {
			return a.Spent.Cents() > b.Spent.Cents()
		}
		return a.Category.String() < b.Category.String()
	})
	t.Available, t.Spent, t.Left = money.BRLCents(avail), money.BRLCents(spent), money.BRLCents(avail-spent)
	return t
}

// carried is what each category brings into m: walking from the first saved
// month, a category that rolls over passes on what it has left, never a
// debt; one without a limit, or that does not roll over, passes on nothing.
func carried(saved []*MonthlyBudget, spending map[Month]Spending, m Month) map[ledger.CategoryID]int64 {
	carry := map[ledger.CategoryID]int64{}
	if len(saved) == 0 {
		return carry
	}
	for k := saved[0].month; k.Before(m); k = k.Add(1) {
		b, ok := Effective(saved, k)
		if !ok {
			continue
		}
		next := map[ledger.CategoryID]int64{}
		for _, l := range b.Lines() {
			left := l.Limit.Cents() + carry[l.Category] - spending[k][l.Category].Cents()
			if l.Rollover && left > 0 {
				next[l.Category] = left
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
