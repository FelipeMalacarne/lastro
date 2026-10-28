package budget

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"lastro/modules/ledger"
	"lastro/money"
)

// Line is one amount for one or more categories (ADR-0012), and what kind
// of amount it is (ADR-0011): a limit, of which what is left may roll over
// to the next month, or a fund's monthly amount, which saves up. A line
// with one category goes by its category's name; a line shared by several
// has a name of its own.
type Line struct {
	ID         LineID
	Name       string
	Categories []ledger.CategoryID // in id order
	Limit      money.Money         // a fund's monthly amount
	Rollover   bool                // a limit's; a fund always carries its balance
	Kind       Kind
}

// Kind is what a line's amount is. A line given without one keeps the kind
// it had, and a new line is a limit.
type Kind string

const (
	KindLimit Kind = "limit"
	KindFund  Kind = "fund"
)

// Shared reports whether the line covers more than one category.
func (l Line) Shared() bool { return len(l.Categories) > 1 }

// IsFund reports whether the line is a fund, which carries its balance,
// debt included, instead of a limit.
func (l Line) IsFund() bool { return l.Kind == KindFund }

// as is the line with its kind settled: the one given, else the one it had
// (had, when ok), else a limit. A fund does not roll over: it always
// carries.
func (l Line) as(had Line, ok bool) Line {
	switch {
	case l.Kind != "":
	case ok:
		l.Kind = had.Kind
	default:
		l.Kind = KindLimit
	}
	if l.IsFund() {
		l.Rollover = false
	}
	return l
}

// Covers reports whether a category is one of the line's.
func (l Line) Covers(c ledger.CategoryID) bool { return slices.Contains(l.Categories, c) }

func (l Line) clone() Line {
	l.Categories = slices.Clone(l.Categories)
	return l
}

// MonthlyBudget is the budget saved for one month: pay yourself first and
// its lines. A month without one inherits the latest saved before it
// (ADR-0006), so changing a month saves it whole from then on.
type MonthlyBudget struct {
	month     Month
	payday    Payday
	pyf       money.Money // pay yourself first
	lines     map[LineID]Line
	updatedAt time.Time
}

// defaultPayday is the 5th, when most Brazilian salaries land.
const defaultPayday = 5

// NewMonthlyBudget starts an empty budget: nothing set aside and no limits.
func NewMonthlyBudget(m Month, now time.Time) (*MonthlyBudget, error) {
	if m.IsZero() {
		return nil, fmt.Errorf("%w: budget without a month", ErrInvalid)
	}
	p, _ := NewPayday(defaultPayday)
	return &MonthlyBudget{month: m, payday: p, pyf: money.BRLCents(0), lines: map[LineID]Line{}, updatedAt: now}, nil
}

// RestoreMonthlyBudget rebuilds a saved budget, for the repository.
func RestoreMonthlyBudget(m Month, payday Payday, pyf money.Money, lines []Line, updatedAt time.Time) *MonthlyBudget {
	b := &MonthlyBudget{month: m, payday: payday, pyf: pyf, lines: make(map[LineID]Line, len(lines)), updatedAt: updatedAt}
	for _, l := range lines {
		l = l.clone()
		slices.SortFunc(l.Categories, byID)
		b.lines[l.ID] = l.as(Line{}, false)
	}
	return b
}

// For is the budget a month starts from: a copy of this one, which is the
// latest saved at or before it. The copy is saved only once it changes.
func (b *MonthlyBudget) For(m Month) *MonthlyBudget {
	c := &MonthlyBudget{month: m, payday: b.payday, pyf: b.pyf, lines: make(map[LineID]Line, len(b.lines)), updatedAt: b.updatedAt}
	for k, v := range b.lines {
		c.lines[k] = v.clone()
	}
	return c
}

func (b *MonthlyBudget) Month() Month                  { return b.month }
func (b *MonthlyBudget) Payday() Payday                { return b.payday }
func (b *MonthlyBudget) PayYourselfFirst() money.Money { return b.pyf }
func (b *MonthlyBudget) UpdatedAt() time.Time          { return b.updatedAt }

// Lines are the budget's lines, in a stable order.
func (b *MonthlyBudget) Lines() []Line {
	out := make([]Line, 0, len(b.lines))
	for _, l := range b.lines {
		out = append(out, l.clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out
}

// Line is the line with an id, if the budget has it.
func (b *MonthlyBudget) Line(id LineID) (Line, bool) {
	l, ok := b.lines[id]
	return l.clone(), ok
}

// LineOf is the line that covers a category, if one does.
func (b *MonthlyBudget) LineOf(c ledger.CategoryID) (Line, bool) {
	for _, l := range b.lines {
		if l.Covers(c) {
			return l.clone(), true
		}
	}
	return Line{}, false
}

// SetPayYourselfFirst sets aside an amount on payday, before any limit.
func (b *MonthlyBudget) SetPayYourselfFirst(amt money.Money, payday Payday, now time.Time) error {
	if err := amount(amt, "pay yourself first"); err != nil {
		return err
	}
	b.pyf, b.payday, b.updatedAt = amt, payday, now
	return nil
}

// SetLimit gives a category a limit of its own, or changes it: the line
// that covers only that category. Zero is a limit too: the category should
// see no spending. A category that shares a line changes with the line.
func (b *MonthlyBudget) SetLimit(c ledger.CategoryID, limit money.Money, rollover bool, now time.Time) error {
	if c.IsZero() {
		return fmt.Errorf("%w: limit without a category", ErrInvalid)
	}
	if err := amount(limit, "limit"); err != nil {
		return err
	}
	l, ok := b.LineOf(c)
	switch {
	case ok && l.Shared():
		return fmt.Errorf("%w: change the line %q", ErrShared, l.Name)
	case !ok:
		l = Line{ID: newLineID(), Categories: []ledger.CategoryID{c}, Kind: KindLimit}
	}
	l.Limit, l.Rollover = limit, rollover
	b.lines[l.ID] = l.as(l, true)
	b.updatedAt = now
	return nil
}

// RemoveLimit leaves a category without a limit. It reports whether it had
// one; a category that shares a line leaves it through the line.
func (b *MonthlyBudget) RemoveLimit(c ledger.CategoryID, now time.Time) (bool, error) {
	l, ok := b.LineOf(c)
	if !ok {
		return false, nil
	}
	if l.Shared() {
		return false, fmt.Errorf("%w: change the line %q", ErrShared, l.Name)
	}
	delete(b.lines, l.ID)
	b.updatedAt = now
	return true, nil
}

// AddLine adds a line with a new id and returns it. The categories it
// takes leave the lines they were in, and a line left without any goes.
func (b *MonthlyBudget) AddLine(l Line, now time.Time) (LineID, error) {
	l.ID = newLineID()
	if err := b.put(l, now); err != nil {
		return LineID{}, err
	}
	return l.ID, nil
}

// ChangeLine changes a line by its id: its name, categories, limit,
// rollover and kind. What it carried stays with it, also when it turns from
// a limit into a fund or back; the categories it takes leave the lines they
// were in, and a line left without any goes.
func (b *MonthlyBudget) ChangeLine(l Line, now time.Time) error {
	if _, ok := b.lines[l.ID]; !ok {
		return ErrNoLine
	}
	return b.put(l, now)
}

func (b *MonthlyBudget) put(l Line, now time.Time) error {
	l, err := checked(l)
	if err != nil {
		return err
	}
	had, ok := b.lines[l.ID]
	l = l.as(had, ok)
	if l.Name != "" {
		for _, o := range b.lines {
			if o.ID != l.ID && strings.EqualFold(o.Name, l.Name) {
				return fmt.Errorf("%w: another line is named %q", ErrInvalid, o.Name)
			}
		}
	}
	for id, o := range b.lines {
		if id == l.ID {
			continue
		}
		o.Categories = slices.DeleteFunc(o.Categories, l.Covers)
		if len(o.Categories) == 0 {
			delete(b.lines, id)
		} else {
			b.lines[id] = o
		}
	}
	b.lines[l.ID] = l
	b.updatedAt = now
	return nil
}

// RemoveLine takes a line out of the budget. It reports whether it was there.
func (b *MonthlyBudget) RemoveLine(id LineID, now time.Time) bool {
	if _, ok := b.lines[id]; !ok {
		return false
	}
	delete(b.lines, id)
	b.updatedAt = now
	return true
}

// Limits is the sum of every line's limit, before rollover.
func (b *MonthlyBudget) Limits() money.Money {
	var cents int64
	for _, l := range b.lines {
		cents += l.Limit.Cents()
	}
	return money.BRLCents(cents)
}

// SetLimits replaces every line at once: the limits typed in together, or
// another month's copied over. A line given without an id keeps the id of
// the line that covered the same categories, so what it carried follows
// it; otherwise it gets a new one. A line given without a kind keeps the
// one it had. A category left out has no limit.
func (b *MonthlyBudget) SetLimits(lines []Line, now time.Time) error {
	next := make(map[LineID]Line, len(lines))
	seen := map[ledger.CategoryID]bool{}
	names := map[string]bool{}
	for _, l := range lines {
		l, err := checked(l)
		if err != nil {
			return err
		}
		for _, c := range l.Categories {
			if seen[c] {
				return fmt.Errorf("%w: two limits for one category", ErrInvalid)
			}
			seen[c] = true
		}
		if k := strings.ToLower(l.Name); k != "" {
			if names[k] {
				return fmt.Errorf("%w: two lines named %q", ErrInvalid, l.Name)
			}
			names[k] = true
		}
		if l.ID.IsZero() {
			var ok bool
			if l.ID, ok = b.idOf(l.Categories); !ok {
				l.ID = newLineID()
			}
		}
		had, ok := b.lines[l.ID]
		l = l.as(had, ok)
		if _, dup := next[l.ID]; dup {
			return fmt.Errorf("%w: two lines with one id", ErrInvalid)
		}
		next[l.ID] = l
	}
	b.lines, b.updatedAt = next, now
	return nil
}

// CopyLines replaces the lines with another month's. A copied line takes
// the id of the line that covered the same categories in prev, the month
// before this one (nil when it has no budget), so what that line carried
// follows it; otherwise it keeps its own, or gets a new one when that is
// taken.
func (b *MonthlyBudget) CopyLines(lines []Line, prev *MonthlyBudget, now time.Time) error {
	given := make([]Line, len(lines))
	used := map[LineID]bool{}
	var keep []int
	for i, l := range lines {
		l = l.clone()
		slices.SortFunc(l.Categories, byID)
		if id, ok := prev.idOf(l.Categories); ok {
			l.ID, used[id] = id, true
		} else {
			keep = append(keep, i)
		}
		given[i] = l
	}
	for _, i := range keep {
		if given[i].ID.IsZero() || used[given[i].ID] {
			given[i].ID = newLineID()
		}
		used[given[i].ID] = true
	}
	return b.SetLimits(given, now)
}

// idOf is the id of the line that covers exactly these categories, given
// in id order; none in a nil budget.
func (b *MonthlyBudget) idOf(cats []ledger.CategoryID) (LineID, bool) {
	if b == nil {
		return LineID{}, false
	}
	for id, l := range b.lines {
		if slices.Equal(l.Categories, cats) {
			return id, true
		}
	}
	return LineID{}, false
}

// checked is a line that keeps the rules: at least one category, each
// once, a name when it covers several, a known kind and a limit in reais,
// never negative.
func checked(l Line) (Line, error) {
	if len(l.Categories) == 0 {
		return Line{}, fmt.Errorf("%w: limit without a category", ErrInvalid)
	}
	if l.Kind != "" && l.Kind != KindLimit && l.Kind != KindFund {
		return Line{}, fmt.Errorf("%w: a line is a limit or a fund, not %q", ErrInvalid, l.Kind)
	}
	l = l.clone()
	slices.SortFunc(l.Categories, byID)
	for i, c := range l.Categories {
		if c.IsZero() {
			return Line{}, fmt.Errorf("%w: limit without a category", ErrInvalid)
		}
		if i > 0 && c == l.Categories[i-1] {
			return Line{}, fmt.Errorf("%w: a category twice in one line", ErrInvalid)
		}
	}
	name, err := lineName(l.Name, l.Shared())
	if err != nil {
		return Line{}, err
	}
	l.Name = name
	if err := amount(l.Limit, "limit"); err != nil {
		return Line{}, err
	}
	return l, nil
}

func byID(a, b ledger.CategoryID) int { return strings.Compare(a.String(), b.String()) }
