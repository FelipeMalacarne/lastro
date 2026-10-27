package budget

import (
	"errors"
	"testing"
	"time"

	"lastro/modules/ledger"
	"lastro/money"
)

func month(t *testing.T, s string) Month {
	t.Helper()
	m, err := ParseMonth(s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func cat(t *testing.T, s string) ledger.CategoryID {
	t.Helper()
	c, err := ledger.NewCategoryID(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var now = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

func TestMonth(t *testing.T) {
	if _, err := ParseMonth("2026-13"); !errors.Is(err, ErrInvalid) {
		t.Errorf("2026-13: %v", err)
	}
	m := month(t, "2026-01")
	if got := m.Add(-1).String(); got != "2025-12" {
		t.Errorf("Add(-1) = %s", got)
	}
	if got := month(t, "2026-10").Since(month(t, "2025-12")); got != 10 {
		t.Errorf("Since = %d", got)
	}
	if month(t, "2026-02").Days() != 28 {
		t.Error("February 2026 has 28 days")
	}
}

func TestPayday(t *testing.T) {
	tests := []struct {
		day   int
		month string
		want  string
		err   bool
	}{
		{day: 5, month: "2026-10", want: "2026-10-05"},
		{day: 31, month: "2026-02", want: "2026-02-28"},
		{day: 0, err: true},
		{day: 32, err: true},
	}
	for _, tt := range tests {
		p, err := NewPayday(tt.day)
		if tt.err {
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("NewPayday(%d) = %v, want ErrInvalid", tt.day, err)
			}
			continue
		}
		if got := p.In(month(t, tt.month)).Format("2006-01-02"); got != tt.want {
			t.Errorf("payday %d in %s = %s, want %s", tt.day, tt.month, got, tt.want)
		}
	}
}

func TestMonthlyBudgetRules(t *testing.T) {
	b, err := NewMonthlyBudget(month(t, "2026-10"), now)
	if err != nil {
		t.Fatal(err)
	}
	food := cat(t, "food")
	usd, _ := money.New(100, money.USD)
	tests := []struct {
		name  string
		c     ledger.CategoryID
		limit money.Money
	}{
		{"negative", food, money.BRLCents(-1)},
		{"dollars", food, usd},
		{"no category", ledger.CategoryID{}, money.BRLCents(100)},
	}
	for _, tt := range tests {
		if err := b.SetLimit(tt.c, tt.limit, false, now); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", tt.name, err)
		}
	}
	if err := b.SetLimit(food, money.BRLCents(0), false, now); err != nil {
		t.Errorf("a zero limit is a limit: %v", err)
	}
	if err := b.SetPayYourselfFirst(money.BRLCents(-5), b.Payday(), now); !errors.Is(err, ErrInvalid) {
		t.Errorf("negative pay yourself first: %v", err)
	}

	// A month's copy changes on its own.
	next := b.For(month(t, "2026-11"))
	_ = next.SetLimit(food, money.BRLCents(500), true, now)
	if l, _ := b.Line(food); l.Limit.Cents() != 0 {
		t.Errorf("the copy changed the original: %v", l.Limit)
	}
	if !next.RemoveLimit(food, now) || next.RemoveLimit(food, now) {
		t.Error("RemoveLimit should report it once")
	}
}

func TestSetLimitsReplacesEveryLimit(t *testing.T) {
	b, _ := NewMonthlyBudget(month(t, "2026-10"), now)
	food, fun, home := cat(t, "food"), cat(t, "fun"), cat(t, "home")
	_ = b.SetLimit(home, money.BRLCents(300000), false, now)

	bad := []struct {
		name  string
		lines []Line
	}{
		{"twice", []Line{{Category: food, Limit: money.BRLCents(1)}, {Category: food, Limit: money.BRLCents(2)}}},
		{"negative", []Line{{Category: food, Limit: money.BRLCents(-1)}}},
		{"no category", []Line{{Limit: money.BRLCents(1)}}},
	}
	for _, tt := range bad {
		if err := b.SetLimits(tt.lines, now); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", tt.name, err)
		}
	}
	if _, ok := b.Line(home); !ok {
		t.Fatal("a refused change must keep the limits there were")
	}

	if err := b.SetLimits([]Line{{Category: food, Limit: money.BRLCents(80000)}, {Category: fun, Limit: money.BRLCents(20000), Rollover: true}}, now); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.Line(home); ok {
		t.Error("a category left out should lose its limit")
	}
	if l, _ := b.Line(fun); !l.Rollover || l.Limit.Cents() != 20000 {
		t.Errorf("fun = %+v", l)
	}
	if got := b.Limits().Cents(); got != 100000 {
		t.Errorf("Limits = %d, want 100000", got)
	}
}

// saved saves a budget per month given, each a map of category to limit
// in cents; a category in rolls rolls over.
func saved(t *testing.T, rolls map[string]bool, months map[string]map[string]int64) []*MonthlyBudget {
	t.Helper()
	var out []*MonthlyBudget
	for _, k := range []string{"2026-07", "2026-08", "2026-09", "2026-10", "2026-11"} {
		lines, ok := months[k]
		if !ok {
			continue
		}
		b, _ := NewMonthlyBudget(month(t, k), now)
		for c, cents := range lines {
			if err := b.SetLimit(cat(t, c), money.BRLCents(cents), rolls[c], now); err != nil {
				t.Fatal(err)
			}
		}
		out = append(out, b)
	}
	return out
}

func spend(t *testing.T, months map[string]map[string]int64) map[Month]Spending {
	t.Helper()
	out := map[Month]Spending{}
	for k, by := range months {
		s := Spending{}
		for c, cents := range by {
			id := ledger.CategoryID{}
			if c != "" {
				id = cat(t, c)
			}
			s[id] = money.BRLCents(cents)
		}
		out[month(t, k)] = s
	}
	return out
}

func TestTrackInheritsAndRollsOver(t *testing.T) {
	budgets := saved(t, map[string]bool{"food": true}, map[string]map[string]int64{
		"2026-08": {"food": 50000, "fun": 20000},
		"2026-10": {"food": 60000, "fun": 20000},
	})
	spending := spend(t, map[string]map[string]int64{
		"2026-08": {"food": 40000, "fun": 25000}, // food leaves 100, fun is over
		"2026-09": {"food": 52000},               // 500 + 100 carried: 80 left
		"2026-10": {"food": 61000, "fun": 5000, "gifts": 3000, "": 1200},
	})

	sep := Track(budgets, spending, month(t, "2026-09"), now, nil)
	if sep.Saved || sep.From.String() != "2026-08" {
		t.Errorf("September follows August: saved=%v from=%s", sep.Saved, sep.From)
	}
	if got := find(t, sep, "food").Carried.Cents(); got != 10000 {
		t.Errorf("September carried %d, want 10000", got)
	}
	if got := find(t, sep, "fun").Carried.Cents(); got != 0 {
		t.Errorf("fun does not roll over, and never carries a debt: %d", got)
	}

	oct := Track(budgets, spending, month(t, "2026-10"), now, nil)
	food := find(t, oct, "food")
	if food.Carried.Cents() != 8000 || food.Available.Cents() != 68000 || food.Left.Cents() != 7000 {
		t.Errorf("October food: carried %d, available %d, left %d", food.Carried.Cents(), food.Available.Cents(), food.Left.Cents())
	}
	if !oct.Saved {
		t.Error("October has its own budget")
	}
	if len(oct.Unlimited) != 2 || oct.Unlimited[cat(t, "gifts")].Cents() != 3000 || oct.Unlimited[ledger.CategoryID{}].Cents() != 1200 {
		t.Errorf("unlimited = %v", oct.Unlimited)
	}
	if oct.Available.Cents() != 88000 || oct.Spent.Cents() != 66000 || oct.Left.Cents() != 22000 {
		t.Errorf("totals: %d %d %d", oct.Available.Cents(), oct.Spent.Cents(), oct.Left.Cents())
	}

	none := Track(budgets, spending, month(t, "2026-07"), now, nil)
	if none.Budget != nil || none.Unlimited[cat(t, "food")].Cents() != 0 {
		t.Errorf("July has no budget: %+v", none)
	}
}

func TestTrackPace(t *testing.T) {
	budgets := saved(t, nil, map[string]map[string]int64{
		"2026-10": {"over": 10000, "ahead": 10000, "on": 10000, "early": 10000, "zero": 0},
	})
	spending := spend(t, map[string]map[string]int64{
		"2026-10": {"over": 10001, "ahead": 9800, "on": 3000, "early": 9500},
	})
	// The 27th of 31: 87% of the month gone.
	tr := Track(budgets, spending, month(t, "2026-10"), now, nil)
	if tr.Elapsed.Bps() != 8709 || tr.DaysLeft != 5 {
		t.Errorf("elapsed %d, days left %d", tr.Elapsed.Bps(), tr.DaysLeft)
	}
	want := map[string]Pace{"over": PaceOver, "ahead": PaceAhead, "on": PaceOn, "early": PaceOn, "zero": PaceOn}
	for c, p := range want {
		if got := find(t, tr, c).Pace; got != p {
			t.Errorf("%s: %s, want %s", c, got, p)
		}
	}
	order := []string{}
	for _, c := range tr.Categories {
		order = append(order, c.Category.String())
	}
	if got := order[0] + "," + order[1] + "," + order[2]; got != "over,ahead,early" {
		t.Errorf("order = %v", order)
	}

	// On the 3rd, 98% used is far ahead; a past month is never ahead.
	if got := find(t, Track(budgets, spending, month(t, "2026-10"), time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC), nil), "early").Pace; got != PaceAhead {
		t.Errorf("early on the 3rd: %s", got)
	}
	if got := find(t, Track(budgets, spending, month(t, "2026-10"), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), nil), "ahead").Pace; got != PaceOn {
		t.Errorf("a past month is on pace or over: %s", got)
	}
}

func TestTrackLeavesFixedChargesOutOfThePace(t *testing.T) {
	budgets := saved(t, nil, map[string]map[string]int64{"2026-10": {"housing": 420000}})
	spending := spend(t, map[string]map[string]int64{"2026-10": {"housing": 390000}})
	sixth := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	if got := find(t, Track(budgets, spending, month(t, "2026-10"), sixth, nil), "housing").Pace; got != PaceAhead {
		t.Errorf("without the rent known as fixed: %s", got)
	}
	fixed := Spending{cat(t, "housing"): money.BRLCents(384000)}
	st := find(t, Track(budgets, spending, month(t, "2026-10"), sixth, fixed), "housing")
	if st.Pace != PaceOn || st.Fixed.Cents() != 384000 {
		t.Errorf("with the rent known as fixed: %s, fixed %d", st.Pace, st.Fixed.Cents())
	}
}

func find(t *testing.T, tr Tracking, c string) CategoryStatus {
	t.Helper()
	for _, s := range tr.Categories {
		if s.Category.String() == c {
			return s
		}
	}
	t.Fatalf("%s not tracked in %s", c, tr.Month)
	return CategoryStatus{}
}
