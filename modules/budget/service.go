package budget

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	sent(ctx context.Context, keys []string) (map[string]bool, error)
	markSent(ctx context.Context, keys []string, at time.Time) error
	rolloverNew(ctx context.Context) (bool, error)
	setRolloverNew(ctx context.Context, on bool) error
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

	budgeted := budgetedSet(cats)
	spending, counts := spendingByMonth(rows, budgeted)
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
	rollNew, err := s.repo.rolloverNew(ctx)
	if err != nil {
		return MonthView{}, fmt.Errorf("budget: month: %w", err)
	}
	prev, hasPrev := Effective(saved, m.Add(-1))
	v := MonthView{
		Month:          m.String(),
		HasBudget:      t.Budget != nil,
		Saved:          t.Saved,
		PrevDiffers:    hasPrev && t.Budget != nil && !sameLines(prev.Lines(), t.Budget.Lines()),
		RolloverNew:    rollNew,
		Days:           m.Days(),
		DaysLeft:       t.DaysLeft,
		ElapsedBp:      t.Elapsed.Bps(),
		AvailableCents: t.Available.Cents(),
		SpentCents:     t.Spent.Cents(),
		LeftCents:      t.Left.Cents(),
		Categories:     []LineView{},
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
	var out, back int64
	for _, k := range kinds {
		switch {
		case k.Kind == "income" && k.Inflow:
			v.IncomeCents += k.Cents
			v.IncomeCount += k.Count
		case k.Kind == "transfer" && !k.Paired && k.Inflow:
			back += k.Cents
		case k.Kind == "transfer" && !k.Paired:
			out -= k.Cents
			if k.Last.After(last) {
				last = k.Last
			}
		}
	}
	// What came in under a spending category is a refund, already taken
	// off that category's spending.
	refunded, n := refunds(rows, budgeted, m)
	v.IncomeCents -= refunded
	v.IncomeCount = max(v.IncomeCount-n, 0)
	v.PayYourselfFirst.MovedCents = SetAside(money.BRLCents(out), money.BRLCents(back)).Cents()
	if pyf := v.PayYourselfFirst; pyf.PlannedCents > 0 && pyf.MovedCents >= pyf.PlannedCents {
		d := last.Format(time.DateOnly)
		v.PayYourselfFirst.DoneOn = &d
	}
	v.NotAssignedCents = v.IncomeCents - v.PayYourselfFirst.PlannedCents - v.LimitsCents

	for _, st := range t.Lines {
		l := st.Line
		lv := LineView{
			ID:             l.ID.String(),
			Category:       l.Categories[0].String(),
			ByCategory:     make([]LineCategoryView, 0, len(l.Categories)),
			LimitCents:     l.Limit.Cents(),
			CarriedCents:   st.Carried.Cents(),
			AvailableCents: st.Available.Cents(),
			SpentCents:     st.Spent.Cents(),
			LeftCents:      st.Left.Cents(),
			Rollover:       l.Rollover,
			UsedBp:         st.Used.Bps(),
			Pace:           st.Pace,
			FixedCents:     st.Fixed.Cents(),
			UpcomingCents:  upcoming.on(l.Categories),
			History:        history(spending, m, l.Categories),
		}
		if l.Name != "" {
			name := l.Name
			lv.Name = &name
		}
		for _, c := range l.Categories {
			lv.ByCategory = append(lv.ByCategory, LineCategoryView{
				Category: c.String(), SpentCents: spending[m][c].Cents(), Count: counts[m][c],
				AverageCents: history(spending, m, []ledger.CategoryID{c}).AverageCents,
			})
			lv.Count += counts[m][c]
		}
		sort.SliceStable(lv.ByCategory, func(i, j int) bool { return lv.ByCategory[i].SpentCents > lv.ByCategory[j].SpentCents })
		v.CarriedCents += lv.CarriedCents
		v.UpcomingCents += lv.UpcomingCents
		v.Categories = append(v.Categories, lv)
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
			} else if _, limited := t.Budget.LineOf(c); !limited {
				unlimited[c] = true
			}
		}
	}
	for c := range unlimited {
		sv := SpendView{SpentCents: spending[m][c].Cents(), Count: counts[m][c], History: history(spending, m, []ledger.CategoryID{c})}
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

// refunds is the income of month m under budgeted categories, and how
// many transactions brought it: refunds, not income, since spendingByMonth
// already takes them off the category's spending.
func refunds(rows []ledger.CategoryMonth, budgeted map[ledger.CategoryID]bool, m Month) (int64, int) {
	var cents int64
	var count int
	for _, r := range rows {
		if r.Kind != "income" || r.Category == "" || MonthOf(r.Month) != m {
			continue
		}
		if id, err := ledger.NewCategoryID(r.Category); err == nil && budgeted[id] {
			cents += r.InCents
			count += r.Count
		}
	}
	return cents, count
}

// history is the spending of some categories together in the six months
// before m, and their average: a line's current categories, also in months
// before they shared it, so the comparison is like with like.
func history(spending map[Month]Spending, m Month, cats []ledger.CategoryID) History {
	h := History{History: make([]MonthSpend, 0, historyMonths)}
	var sum int64
	for k := m.Add(-historyMonths); k.Before(m); k = k.Add(1) {
		v := spending[k].on(cats)
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
		_, err := b.RemoveLimit(c, now)
		return err
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

// limitable is the categories that can take a limit: live ones that count
// toward the budget.
func limitable(cats []categorize.Category) map[ledger.CategoryID]bool {
	out := make(map[ledger.CategoryID]bool, len(cats))
	for _, c := range cats {
		out[c.ID] = c.Budgeted && !c.Archived
	}
	return out
}

// sameLines is whether two months have the same lines, whatever their ids:
// copying one onto the other would change nothing.
func sameLines(a, b []Line) bool {
	key := func(ls []Line) []string {
		out := make([]string, len(ls))
		for i, l := range ls {
			out[i] = fmt.Sprintf("%v|%s|%d|%v", l.Categories, l.Name, l.Limit.Cents(), l.Rollover)
		}
		slices.Sort(out)
		return out
	}
	return slices.Equal(key(a), key(b))
}

// covered checks that every category of the lines can take a limit.
func covered(cats []categorize.Category, lines ...Line) error {
	ok := limitable(cats)
	for _, l := range lines {
		for _, c := range l.Categories {
			if !ok[c] {
				return ErrNotBudgeted
			}
		}
	}
	return nil
}

// SetLimits replaces every line from a month on with the ones given.
func (s *Service) SetLimits(ctx context.Context, m Month, lines []Line) error {
	cats, err := s.categorize.Categories(ctx)
	if err != nil {
		return fmt.Errorf("budget: set limits: %w", err)
	}
	if err := covered(cats, lines...); err != nil {
		return fmt.Errorf("budget: set limits: %w", err)
	}
	err = s.edit(ctx, m, func(b *MonthlyBudget, now time.Time) error { return b.SetLimits(lines, now) })
	if err != nil {
		return fmt.Errorf("budget: set limits: %w", err)
	}
	return nil
}

// AddLine adds a line from a month on and returns its id.
func (s *Service) AddLine(ctx context.Context, m Month, l Line) (LineID, error) {
	cats, err := s.categorize.Categories(ctx)
	if err != nil {
		return LineID{}, fmt.Errorf("budget: add line: %w", err)
	}
	if err := covered(cats, l); err != nil {
		return LineID{}, fmt.Errorf("budget: add line: %w", err)
	}
	var id LineID
	err = s.edit(ctx, m, func(b *MonthlyBudget, now time.Time) error {
		id, err = b.AddLine(l, now)
		return err
	})
	if err != nil {
		return LineID{}, fmt.Errorf("budget: add line: %w", err)
	}
	return id, nil
}

// ChangeLine changes a line from a month on.
func (s *Service) ChangeLine(ctx context.Context, m Month, l Line) error {
	cats, err := s.categorize.Categories(ctx)
	if err != nil {
		return fmt.Errorf("budget: change line: %w", err)
	}
	if err := covered(cats, l); err != nil {
		return fmt.Errorf("budget: change line: %w", err)
	}
	err = s.edit(ctx, m, func(b *MonthlyBudget, now time.Time) error { return b.ChangeLine(l, now) })
	if err != nil {
		return fmt.Errorf("budget: change line: %w", err)
	}
	return nil
}

// RemoveLine takes a line out from a month on.
func (s *Service) RemoveLine(ctx context.Context, m Month, id LineID) error {
	err := s.edit(ctx, m, func(b *MonthlyBudget, now time.Time) error {
		if !b.RemoveLine(id, now) {
			return ErrNoLine
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("budget: remove line: %w", err)
	}
	return nil
}

// CopyLimits gives a month the lines another had (CopyLines), leaving out
// the categories archived since and a line left without any. It returns
// how many lines were copied.
func (s *Service) CopyLimits(ctx context.Context, to, from Month) (int, error) {
	until := from
	if to.Add(-1).After(until) {
		until = to.Add(-1)
	}
	saved, err := s.repo.Saved(ctx, until)
	if err != nil {
		return 0, fmt.Errorf("budget: copy limits: %w", err)
	}
	prev, _ := Effective(saved, to.Add(-1))
	src, ok := Effective(saved, from)
	if !ok {
		return 0, fmt.Errorf("budget: copy limits: %w: %s has no budget", ErrInvalid, from)
	}
	cats, err := s.categorize.Categories(ctx)
	if err != nil {
		return 0, fmt.Errorf("budget: copy limits: %w", err)
	}
	live := limitable(cats)
	var lines []Line
	for _, l := range src.Lines() {
		l.Categories = slices.DeleteFunc(l.Categories, func(c ledger.CategoryID) bool { return !live[c] })
		if len(l.Categories) > 0 {
			lines = append(lines, l)
		}
	}
	err = s.edit(ctx, to, func(b *MonthlyBudget, now time.Time) error { return b.CopyLines(lines, prev, now) })
	if err != nil {
		return 0, fmt.Errorf("budget: copy limits: %w", err)
	}
	return len(lines), nil
}

// SetRolloverNew is whether a category given a limit for the first time
// rolls over unless I say otherwise.
func (s *Service) SetRolloverNew(ctx context.Context, on bool) error {
	if err := s.repo.setRolloverNew(ctx, on); err != nil {
		return fmt.Errorf("budget: rollover for new limits: %w", err)
	}
	return nil
}
