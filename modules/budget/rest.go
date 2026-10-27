package budget

import (
	"fmt"
	"time"

	"lastro/modules/ledger"
	"lastro/money"
)

// RestChoice is what happens to what a month leaves after pay yourself
// first and its spending: invested, kept to spend in the next month, or
// split between the two. As a default it may also be "ask".
type RestChoice string

const (
	RestAsk    RestChoice = "ask"
	RestInvest RestChoice = "invest"
	RestKeep   RestChoice = "keep"
	RestSplit  RestChoice = "split"
)

// ParseRestDefault reads the default for months I did not decide: ask,
// invest or keep.
func ParseRestDefault(s string) (RestChoice, error) {
	switch c := RestChoice(s); c {
	case RestAsk, RestInvest, RestKeep:
		return c, nil
	}
	return "", fmt.Errorf("%w: what is left defaults to ask, invest or keep, not %q", ErrInvalid, s)
}

// Decision is what I chose for one closed month's rest. With a split,
// Invest is how much goes to investments and the rest is kept.
type Decision struct {
	Month     Month
	Choice    RestChoice
	Invest    money.Money // only for a split
	DecidedAt time.Time
}

// Decide records a choice for a month that is over and left something.
// A split invests between nothing and all of it.
func Decide(m Month, left money.Money, choice RestChoice, invest money.Money, today, now time.Time) (Decision, error) {
	if !m.Before(MonthOf(today)) {
		return Decision{}, fmt.Errorf("%w: %s is not over yet", ErrInvalid, m)
	}
	if left.Cents() <= 0 {
		return Decision{}, fmt.Errorf("%w: %s left nothing", ErrInvalid, m)
	}
	d := Decision{Month: m, Choice: choice, Invest: money.BRLCents(0), DecidedAt: now}
	switch choice {
	case RestInvest, RestKeep:
	case RestSplit:
		if err := amount(invest, "the amount invested"); err != nil {
			return Decision{}, err
		}
		if invest.Cents() > left.Cents() {
			return Decision{}, fmt.Errorf("%w: cannot invest more than the %s left", ErrInvalid, m)
		}
		d.Invest = invest
	default:
		return Decision{}, fmt.Errorf("%w: what is left is invested, kept or split, not %q", ErrInvalid, choice)
	}
	return d, nil
}

// Rest is what a month leaves and where it goes. While the month runs,
// Left counts every limit as spent (more where a category went over): it
// is what no limit holds, Not assigned. Once the month is over, a category
// that does not roll over counts only what it spent, so what it did not
// spend is left too.
type Rest struct {
	Month  Month
	Closed bool

	Income      money.Money
	Kept        money.Money // brought in from the month before
	PayFirst    money.Money // pay yourself first
	Limits      money.Money // every limit, before rollover
	Over        money.Money // spent past what limited categories had
	Unlimited   money.Money // spent without a limit, uncategorized included
	Unspent     money.Money // left in limits that do not roll over, once closed
	Left        money.Money
	Choice      RestChoice // "" while undecided or with nothing left
	Decided     bool       // chosen for this month, not by the default
	Invest      money.Money
	KeepForNext money.Money
}

// Rests walks every month from the first saved budget to m, carrying what
// each kept into the next. income is what came in each month; decisions
// are the months I decided, and def applies to the closed ones I did not.
func Rests(saved []*MonthlyBudget, spending map[Month]Spending, income map[Month]money.Money, decisions map[Month]Decision, def RestChoice, m Month, today time.Time) map[Month]Rest {
	out := map[Month]Rest{}
	if len(saved) == 0 {
		return out
	}
	now := MonthOf(today)
	carry := map[ledger.CategoryID]int64{}
	var kept int64
	for k := saved[0].month; !k.After(m); k = k.Add(1) {
		b, _ := Effective(saved, k)
		closed := k.Before(now)
		r := Rest{Month: k, Closed: closed, Kept: money.BRLCents(kept), Income: income[k], PayFirst: b.PayYourselfFirst(), Limits: b.Limits()}
		var over, unlimited, unspent int64
		next := map[ledger.CategoryID]int64{}
		for c, s := range spending[k] {
			if _, limited := b.Line(c); !limited {
				unlimited += s.Cents()
			}
		}
		for _, l := range b.Lines() {
			s := spending[k][l.Category].Cents()
			avail := l.Limit.Cents() + carry[l.Category]
			over += max(s-avail, 0)
			left := avail - s
			if l.Rollover && left > 0 {
				next[l.Category] = left
			}
			// What rolled in pays for spending first, and a category that
			// stops rolling over frees it too.
			if closed && !l.Rollover && left > 0 {
				unspent += left
			}
		}
		r.Over, r.Unlimited, r.Unspent = money.BRLCents(over), money.BRLCents(unlimited), money.BRLCents(unspent)
		left := r.Income.Cents() + kept - r.PayFirst.Cents() - r.Limits.Cents() - over - unlimited + unspent
		r.Left = money.BRLCents(left)
		r.Invest, r.KeepForNext = money.BRLCents(0), money.BRLCents(0)

		kept = 0
		if closed && left > 0 {
			d, own := decisions[k]
			choice := d.Choice
			if !own && def != RestAsk {
				choice = def
			}
			r.Choice, r.Decided = choice, own
			switch choice {
			case RestInvest:
				r.Invest = r.Left
			case RestKeep:
				r.KeepForNext = r.Left
			case RestSplit:
				// What came in later may leave less than was invested:
				// the investment stands and nothing is kept.
				inv := min(d.Invest.Cents(), left)
				r.Invest, r.KeepForNext = money.BRLCents(inv), money.BRLCents(left-inv)
			}
			kept = r.KeepForNext.Cents()
		}
		out[k] = r
		carry = next
	}
	return out
}
