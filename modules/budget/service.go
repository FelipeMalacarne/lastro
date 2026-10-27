package budget

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
	_ "time/tzdata" // the image has no zoneinfo; months turn in São Paulo

	"lastro/modules/categorize"
	"lastro/modules/ledger"
	"lastro/money"
)

// ErrNotBudgeted means a limit was asked for a category that does not
// count toward the budget (Income), is archived or does not exist.
var ErrNotBudgeted = errors.New("budget: category does not count toward the budget")

// Repository keeps the saved budgets.
type Repository interface {
	Saved(ctx context.Context, until Month) ([]*MonthlyBudget, error)
	Save(ctx context.Context, b *MonthlyBudget) error
}

// historyMonths is how far back a category's average looks.
const historyMonths = 6

var saoPaulo, _ = time.LoadLocation("America/Sao_Paulo")

type Service struct {
	repo       Repository
	ledger     *ledger.Service
	categorize *categorize.Service
	now        func() time.Time
}

func New(repo Repository, l *ledger.Service, c *categorize.Service) *Service {
	return &Service{repo: repo, ledger: l, categorize: c, now: time.Now}
}

// today is the date in São Paulo, where the months turn.
func (s *Service) today() time.Time { return s.now().In(saoPaulo) }

// Month tracks a month's budget against what came in, what was set aside
// and what was spent.
func (s *Service) Month(ctx context.Context, m Month) (MonthView, error) {
	saved, err := s.repo.Saved(ctx, m)
	if err != nil {
		return MonthView{}, fmt.Errorf("budget: month: %w", err)
	}
	from := m.Add(-historyMonths)
	if len(saved) > 0 && saved[0].month.Before(from) {
		from = saved[0].month
	}
	cats, err := s.categorize.Categories(ctx)
	if err != nil {
		return MonthView{}, fmt.Errorf("budget: month: %w", err)
	}
	rows, err := s.ledger.CategoryMonths(ctx, from.First(), m.First())
	if err != nil {
		return MonthView{}, fmt.Errorf("budget: month: %w", err)
	}
	kinds, err := s.ledger.MonthKinds(ctx, m.First(), m.First())
	if err != nil {
		return MonthView{}, fmt.Errorf("budget: month: %w", err)
	}

	spending, counts := spendingByMonth(rows, budgetedSet(cats))
	// This month, recurring charges already paid stay out of the pace, and
	// the ones still to come are counted against what is left.
	var fixed, upcoming Spending
	if today := s.today(); m == MonthOf(today) {
		rec, err := s.recurring(ctx, today)
		if err != nil {
			return MonthView{}, fmt.Errorf("budget: month: %w", err)
		}
		fixed, upcoming = Spending{}, Spending{}
		end := m.Add(1).First()
		for _, r := range rec {
			if r.PaidThisMonth {
				fixed[r.Category] = money.BRLCents(fixed[r.Category].Cents() + r.PaidCents)
			} else if r.Next.Before(end) {
				upcoming[r.Category] = money.BRLCents(upcoming[r.Category].Cents() + r.Monthly.Cents())
			}
		}
	}
	t := Track(saved, spending, m, s.today(), fixed)
	v := MonthView{
		Month:          m.String(),
		HasBudget:      t.Budget != nil,
		Saved:          t.Saved,
		Days:           m.Days(),
		DaysLeft:       t.DaysLeft,
		ElapsedBp:      t.Elapsed.Bps(),
		AvailableCents: t.Available.Cents(),
		SpentCents:     t.Spent.Cents(),
		LeftCents:      t.Left.Cents(),
		Categories:     []CategoryView{},
		Unlimited:      []SpendView{},
	}
	p, _ := NewPayday(defaultPayday)
	if t.Budget != nil {
		f := t.From.String()
		v.From = &f
		p = t.Budget.Payday()
		v.PayYourselfFirst.PlannedCents = t.Budget.PayYourselfFirst().Cents()
		v.LimitsCents = t.Budget.Limits().Cents()
	}
	v.Payday = p.Day()
	v.PayYourselfFirst.Due = p.In(m).Format(time.DateOnly)

	var last time.Time
	for _, k := range kinds {
		switch {
		case k.Kind == "income" && k.Inflow:
			v.IncomeCents += k.Cents
			v.IncomeCount += k.Count
		case k.Kind == "transfer" && !k.Inflow && !k.Paired:
			v.PayYourselfFirst.MovedCents -= k.Cents
			if k.Last.After(last) {
				last = k.Last
			}
		}
	}
	if pyf := v.PayYourselfFirst; pyf.PlannedCents > 0 && pyf.MovedCents >= pyf.PlannedCents {
		d := last.Format(time.DateOnly)
		v.PayYourselfFirst.DoneOn = &d
	}
	v.NotAssignedCents = v.IncomeCents - v.PayYourselfFirst.PlannedCents - v.LimitsCents

	for _, c := range t.Categories {
		v.CarriedCents += c.Carried.Cents()
		v.Categories = append(v.Categories, CategoryView{
			Category:       c.Category.String(),
			LimitCents:     c.Limit.Cents(),
			CarriedCents:   c.Carried.Cents(),
			AvailableCents: c.Available.Cents(),
			SpentCents:     c.Spent.Cents(),
			LeftCents:      c.Left.Cents(),
			Rollover:       c.Rollover,
			UsedBp:         c.Used.Bps(),
			Pace:           c.Pace,
			Count:          counts[m][c.Category],
			FixedCents:     c.Fixed.Cents(),
			UpcomingCents:  upcoming[c.Category].Cents(),
			History:        history(spending, m, c.Category),
		})
		v.UpcomingCents += upcoming[c.Category].Cents()
	}

	// Every budgeted category without a limit shows, even with nothing
	// spent this month, when it had spending in the months before.
	unlimited := map[ledger.CategoryID]bool{}
	for c := range t.Unlimited {
		unlimited[c] = true
	}
	for k := m.Add(-historyMonths); k.Before(m); k = k.Add(1) {
		for c := range spending[k] {
			if t.Budget == nil {
				unlimited[c] = true
			} else if _, limited := t.Budget.Line(c); !limited {
				unlimited[c] = true
			}
		}
	}
	for c := range unlimited {
		sv := SpendView{SpentCents: spending[m][c].Cents(), Count: counts[m][c], History: history(spending, m, c)}
		if !c.IsZero() {
			id := c.String()
			sv.Category = &id
		}
		v.Unlimited = append(v.Unlimited, sv)
	}
	sort.Slice(v.Unlimited, func(i, j int) bool {
		a, b := v.Unlimited[i], v.Unlimited[j]
		if a.SpentCents != b.SpentCents {
			return a.SpentCents > b.SpentCents
		}
		return a.AverageCents > b.AverageCents
	})

	var spent int64
	for _, s := range spending[m] {
		spent += s.Cents()
	}
	v.Summary = SummaryView{IncomeCents: v.IncomeCents, SpentCents: spent, ContributionCents: v.PayYourselfFirst.MovedCents}
	if v.IncomeCents > 0 {
		v.Summary.SavingsRateBp = (v.IncomeCents - spent) * 10000 / v.IncomeCents
	}
	return v, nil
}

// recurringSince is how far back recurring charges are looked for.
const recurringSince = 13

func (s *Service) recurring(ctx context.Context, today time.Time) ([]Recurring, error) {
	rows, err := s.ledger.Charges(ctx, MonthOf(today).Add(-recurringSince).First())
	if err != nil {
		return nil, err
	}
	charges := make([]Charge, len(rows))
	for i, r := range rows {
		c := Charge{ID: r.ID, Description: r.Description, Date: r.Date, Amount: money.BRLCents(r.AmountCents)}
		if r.Category != "" {
			c.Category, _ = ledger.NewCategoryID(r.Category)
		}
		charges[i] = c
	}
	return FindRecurring(charges, today), nil
}

// Recurring lists the charges that come back every month, with what they
// cost together and what is still to come this month.
func (s *Service) Recurring(ctx context.Context) (RecurringView, error) {
	today := s.today()
	rec, err := s.recurring(ctx, today)
	if err != nil {
		return RecurringView{}, fmt.Errorf("budget: recurring: %w", err)
	}
	v := RecurringView{Charges: make([]RecurringCharge, 0, len(rec))}
	end := MonthOf(today).Add(1).First()
	for _, r := range rec {
		c := RecurringCharge{
			Name: r.Name, Steadiness: r.Steadiness, MonthlyCents: r.Monthly.Cents(), Months: r.Months,
			First: r.First.Format(time.DateOnly), Last: r.Last.Format(time.DateOnly), Next: r.Next.Format(time.DateOnly),
			PaidThisMonth: r.PaidThisMonth, New: r.New,
		}
		if !r.Category.IsZero() {
			id := r.Category.String()
			c.Category = &id
		}
		if r.Change != nil {
			c.Change = &ChangeView{FromCents: r.Change.From.Cents(), ToCents: r.Change.To.Cents(), Since: r.Change.Since.Format(time.DateOnly)}
			v.Changed++
		}
		if r.New {
			v.New++
		}
		v.MonthlyCents += r.Monthly.Cents()
		if r.PaidThisMonth {
			v.PaidCents += r.PaidCents
		} else if r.Next.Before(end) {
			v.UpcomingCents += r.Monthly.Cents()
		}
		v.Charges = append(v.Charges, c)
	}
	return v, nil
}

// budgetedSet is the categories whose spending counts: everything but
// Income-like ones. Archived categories still count for the months they
// had spending.
func budgetedSet(cats []categorize.Category) map[ledger.CategoryID]bool {
	out := make(map[ledger.CategoryID]bool, len(cats))
	for _, c := range cats {
		out[c.ID] = c.Budgeted
	}
	return out
}

// spendingByMonth turns the ledger's sums into spending per month and
// category: expenses minus refunds in every budgeted category, and for
// transactions without a category only expenses, so an uncategorized
// salary is never read as a refund.
func spendingByMonth(rows []ledger.CategoryMonth, budgeted map[ledger.CategoryID]bool) (map[Month]Spending, map[Month]map[ledger.CategoryID]int) {
	cents := map[Month]map[ledger.CategoryID]int64{}
	counts := map[Month]map[ledger.CategoryID]int{}
	for _, r := range rows {
		var c ledger.CategoryID
		if r.Category != "" {
			id, err := ledger.NewCategoryID(r.Category)
			if err != nil || !budgeted[id] {
				continue
			}
			c = id
		} else if r.Kind != "expense" {
			continue
		}
		m := MonthOf(r.Month)
		if cents[m] == nil {
			cents[m], counts[m] = map[ledger.CategoryID]int64{}, map[ledger.CategoryID]int{}
		}
		cents[m][c] += r.OutCents - r.InCents
		counts[m][c] += r.Count
	}
	out := make(map[Month]Spending, len(cents))
	for m, by := range cents {
		out[m] = Spending{}
		for c, v := range by {
			out[m][c] = money.BRLCents(v)
		}
	}
	return out, counts
}

// history is a category's spending in the six months before m and their
// average.
func history(spending map[Month]Spending, m Month, c ledger.CategoryID) History {
	h := History{History: make([]MonthSpend, 0, historyMonths)}
	var sum int64
	for k := m.Add(-historyMonths); k.Before(m); k = k.Add(1) {
		v := spending[k][c].Cents()
		h.History = append(h.History, MonthSpend{Month: k.String(), SpentCents: v})
		sum += v
	}
	h.AverageCents = sum / historyMonths
	return h
}

// edit loads the budget that applies to a month, lets f change it and saves
// it as that month's own.
func (s *Service) edit(ctx context.Context, m Month, f func(b *MonthlyBudget, now time.Time) error) error {
	saved, err := s.repo.Saved(ctx, m)
	if err != nil {
		return err
	}
	now := s.now()
	b, ok := Effective(saved, m)
	if !ok {
		if b, err = NewMonthlyBudget(m, now); err != nil {
			return err
		}
	}
	if err := f(b, now); err != nil {
		return err
	}
	return s.repo.Save(ctx, b)
}

// SetLimit gives a category a limit from a month on.
func (s *Service) SetLimit(ctx context.Context, m Month, c ledger.CategoryID, limit money.Money, rollover bool) error {
	cats, err := s.categorize.Categories(ctx)
	if err != nil {
		return fmt.Errorf("budget: set limit: %w", err)
	}
	ok := false
	for _, x := range cats {
		ok = ok || (x.ID == c && x.Budgeted && !x.Archived)
	}
	if !ok {
		return fmt.Errorf("budget: set limit: %w", ErrNotBudgeted)
	}
	err = s.edit(ctx, m, func(b *MonthlyBudget, now time.Time) error { return b.SetLimit(c, limit, rollover, now) })
	if err != nil {
		return fmt.Errorf("budget: set limit: %w", err)
	}
	return nil
}

// RemoveLimit leaves a category without a limit from a month on.
func (s *Service) RemoveLimit(ctx context.Context, m Month, c ledger.CategoryID) error {
	err := s.edit(ctx, m, func(b *MonthlyBudget, now time.Time) error {
		b.RemoveLimit(c, now)
		return nil
	})
	if err != nil {
		return fmt.Errorf("budget: remove limit: %w", err)
	}
	return nil
}

// SetPayYourselfFirst sets aside an amount on payday from a month on.
func (s *Service) SetPayYourselfFirst(ctx context.Context, m Month, amt money.Money, payday Payday) error {
	err := s.edit(ctx, m, func(b *MonthlyBudget, now time.Time) error { return b.SetPayYourselfFirst(amt, payday, now) })
	if err != nil {
		return fmt.Errorf("budget: pay yourself first: %w", err)
	}
	return nil
}
