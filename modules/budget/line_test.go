package budget

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"lastro/modules/ledger"
	"lastro/money"
)

func cats(cs ...ledger.CategoryID) []ledger.CategoryID { return cs }

func TestSharedLineRules(t *testing.T) {
	b, _ := NewMonthlyBudget(month(t, "2026-10"), now)
	fun, shop, dev, food := cat(t, "fun"), cat(t, "shop"), cat(t, "dev"), cat(t, "food")
	_ = b.SetLimit(dev, money.BRLCents(10000), false, now)
	_ = b.SetLimit(food, money.BRLCents(80000), false, now)

	bad := []struct {
		name string
		l    Line
	}{
		{"without a name", Line{Categories: cats(fun, shop), Limit: money.BRLCents(100000)}},
		{"a long name", Line{Name: strings.Repeat("a", 41), Categories: cats(fun, shop), Limit: money.BRLCents(100000)}},
		{"a category twice", Line{Name: "Free", Categories: cats(fun, fun), Limit: money.BRLCents(100000)}},
		{"no category", Line{Name: "Free", Limit: money.BRLCents(100000)}},
		{"negative", Line{Name: "Free", Categories: cats(fun, shop), Limit: money.BRLCents(-1)}},
	}
	for _, tt := range bad {
		if _, err := b.AddLine(tt.l, now); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", tt.name, err)
		}
	}
	if len(b.Lines()) != 2 {
		t.Fatalf("a refused line changed the budget: %+v", b.Lines())
	}

	// Dev leaves its own limit for the shared line.
	id, err := b.AddLine(Line{Name: " Free to spend ", Categories: cats(shop, fun, dev), Limit: money.BRLCents(100000), Rollover: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	l, ok := b.Line(id)
	if !ok || l.Name != "Free to spend" || !l.Shared() || len(l.Categories) != 3 || l.Categories[0] != dev || !l.Rollover {
		t.Errorf("the new line: %+v", l)
	}
	if got, _ := b.LineOf(dev); got.ID != id || len(b.Lines()) != 2 || b.Limits().Cents() != 180000 {
		t.Errorf("dev is in %s, %d lines, limits %d", got.ID, len(b.Lines()), b.Limits().Cents())
	}

	// A category that shares a line changes through it.
	if err := b.SetLimit(fun, money.BRLCents(1), false, now); !errors.Is(err, ErrShared) {
		t.Errorf("SetLimit on a shared category: %v, want ErrShared", err)
	}
	if _, err := b.RemoveLimit(fun, now); !errors.Is(err, ErrShared) {
		t.Errorf("RemoveLimit on a shared category: %v, want ErrShared", err)
	}
	if _, err := b.AddLine(Line{Name: "free TO SPEND", Categories: cats(cat(t, "gifts")), Limit: money.BRLCents(1)}, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("a second line with the name: %v, want ErrInvalid", err)
	}

	// Changing the line keeps its id; a line left without categories goes.
	if err := b.ChangeLine(Line{ID: id, Name: "Free", Categories: cats(fun, shop, food), Limit: money.BRLCents(120000)}, now); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.LineOf(food); got.ID != id || len(b.Lines()) != 1 {
		t.Errorf("food joined %s, %d lines left", got.ID, len(b.Lines()))
	}
	if _, ok := b.LineOf(dev); ok {
		t.Error("dev left the line and has no limit")
	}
	unknown, _ := ParseLineID("0b7e4c1d-8a9f-4e2b-b3c4-d5e6f7a8b9c0")
	if err := b.ChangeLine(Line{ID: unknown, Categories: cats(dev), Limit: money.BRLCents(1)}, now); !errors.Is(err, ErrNoLine) {
		t.Errorf("changing a line the month lacks: %v, want ErrNoLine", err)
	}
	if !b.RemoveLine(id, now) || b.RemoveLine(id, now) {
		t.Error("RemoveLine should report it once")
	}
}

func TestCopyLinesKeepsTheIDsThatCarry(t *testing.T) {
	fun, shop, food, dev := cat(t, "fun"), cat(t, "shop"), cat(t, "food"), cat(t, "dev")
	jan, _ := NewMonthlyBudget(month(t, "2026-01"), now)
	_, _ = jan.AddLine(Line{Name: "Free", Categories: cats(fun, shop), Limit: money.BRLCents(100000), Rollover: true}, now)
	_ = jan.SetLimit(dev, money.BRLCents(10000), false, now)
	janDev, _ := jan.LineOf(dev)

	// September, the month before, has the free line under its own id, and
	// October has its own lines again: food, and the free line under a third.
	sep, _ := NewMonthlyBudget(month(t, "2026-09"), now)
	sepFree, _ := sep.AddLine(Line{Name: "Free", Categories: cats(shop, fun), Limit: money.BRLCents(90000), Rollover: true}, now)
	oct, _ := NewMonthlyBudget(month(t, "2026-10"), now)
	_ = oct.SetLimit(food, money.BRLCents(80000), true, now)
	_, _ = oct.AddLine(Line{Name: "Free", Categories: cats(fun, shop), Limit: money.BRLCents(90000)}, now)

	if err := oct.CopyLines(jan.Lines(), sep, now); err != nil {
		t.Fatal(err)
	}
	if l, _ := oct.LineOf(fun); l.ID != sepFree || l.Limit.Cents() != 100000 || !l.Rollover {
		t.Errorf("the free line copied: %+v, want September's id %s, which carries", l, sepFree)
	}
	if _, ok := oct.LineOf(food); ok {
		t.Error("food had no limit in January")
	}
	if l, _ := oct.LineOf(dev); l.ID != janDev.ID {
		t.Errorf("dev's line, which September lacks, keeps its own id: %+v", l)
	}
	if err := oct.CopyLines(jan.Lines(), nil, now); err != nil {
		t.Errorf("copying with no month before: %v", err)
	}
}

func TestTrackSharedLine(t *testing.T) {
	fun, shop, dev := cat(t, "fun"), cat(t, "shop"), cat(t, "dev")
	// August: fun and shop share R$ 1.000 that rolls over; dev has R$ 100.
	aug, _ := NewMonthlyBudget(month(t, "2026-08"), now)
	free, _ := aug.AddLine(Line{Name: "Free", Categories: cats(fun, shop), Limit: money.BRLCents(100000), Rollover: true}, now)
	_ = aug.SetLimit(dev, money.BRLCents(10000), false, now)
	// October: dev joins the line, which keeps what it carried.
	oct := aug.For(month(t, "2026-10"))
	if err := oct.ChangeLine(Line{ID: free, Name: "Free", Categories: cats(fun, shop, dev), Limit: money.BRLCents(100000), Rollover: true}, now); err != nil {
		t.Fatal(err)
	}
	budgets := []*MonthlyBudget{aug, oct}
	spending := spend(t, map[string]map[string]int64{
		"2026-08": {"fun": 30000, "shop": 50000, "dev": 2000}, // free leaves 200
		"2026-09": {"shop": 110000},                           // 1.000 and 200 carried: 100 left
		"2026-10": {"fun": 20000, "shop": 60000, "dev": 15000, "food": 9000},
	})

	sep := Track(budgets, spending, nil, month(t, "2026-09"), now, nil)
	f := find(t, sep, "fun")
	if f.Line.ID != free || f.Carried.Cents() != 20000 || f.Spent.Cents() != 110000 || f.Left.Cents() != 10000 || f.Pace != PaceOn {
		t.Errorf("September's free line, where shop alone spent more than the limit: %+v", f)
	}

	tr := Track(budgets, spending, nil, month(t, "2026-10"), now, nil)
	f = find(t, tr, "dev")
	if f.Line.ID != free || f.Carried.Cents() != 10000 || f.Available.Cents() != 110000 || f.Spent.Cents() != 95000 || f.Left.Cents() != 15000 {
		t.Errorf("October's free line, with dev: %+v", f)
	}
	if len(tr.Lines) != 1 || len(tr.Unlimited) != 1 || tr.Unlimited[cat(t, "food")].Cents() != 9000 {
		t.Errorf("lines %d, unlimited %v", len(tr.Lines), tr.Unlimited)
	}
	if tr.Available.Cents() != 110000 || tr.Spent.Cents() != 95000 || tr.Left.Cents() != 15000 {
		t.Errorf("totals: %d %d %d", tr.Available.Cents(), tr.Spent.Cents(), tr.Left.Cents())
	}
}

func TestHistoryIsLikeWithLike(t *testing.T) {
	spending := spend(t, map[string]map[string]int64{
		"2026-08": {"fun": 30000, "shop": 50000, "food": 70000},
		"2026-09": {"shop": 12000},
	})
	h := history(spending, month(t, "2026-10"), cats(cat(t, "fun"), cat(t, "shop")))
	if len(h.History) != 6 || h.History[0].Month != "2026-04" || h.History[4].SpentCents != 80000 || h.History[5].SpentCents != 12000 {
		t.Errorf("history %+v", h.History)
	}
	if h.AverageCents != 92000/6 {
		t.Errorf("average %d", h.AverageCents)
	}
}

func TestFindAlertsForLines(t *testing.T) {
	oct := month(t, "2026-10")
	b := money.BRLCents
	free, _ := ParseLineID("a1b2c3d4-0000-4000-8000-000000000000")
	saved := LineID{s: "food"} // a line saved before lines could be shared: its category's id
	figures := []Figures{
		{Line: free, Name: "Free", Limited: true, Spent: b(130000), Available: b(100000), Left: b(-30000), Pace: PaceOver},
		{Line: saved, Category: cat(t, "food"), Limited: true, Spent: b(90000), Available: b(80000), Left: b(-10000), Pace: PaceOver},
	}
	got := FindAlerts(oct, now, figures, nil, 5)
	if len(got) != 2 || got[0].Key != "over:2026-10:"+free.String() || got[0].Name != "Free" || !got[0].Category.IsZero() {
		t.Fatalf("alerts %+v", got)
	}
	if got[1].Key != "over:2026-10:food" || got[1].Category != cat(t, "food") {
		t.Errorf("a saved line keeps its key: %+v", got[1])
	}
}

func TestParseLineID(t *testing.T) {
	if id, err := ParseLineID("0B7E4C1D-8A9F-4E2B-B3C4-D5E6F7A8B9C0"); err != nil || id.String() != "0b7e4c1d-8a9f-4e2b-b3c4-d5e6f7a8b9c0" {
		t.Errorf("an upper-case UUID: %v %v", id, err)
	}
	for _, s := range []string{"", "nope", "0b7e4c1d-8a9f-4e2b-b3c4-d5e6f7a8b9cz", "0b7e4c1d8a9f-4e2b-b3c4-d5e6f7a8b9c0a"} {
		if _, err := ParseLineID(s); !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseLineID(%q) = %v, want ErrInvalid", s, err)
		}
	}
	if !newLineID().IsZero() {
		if _, err := ParseLineID(newLineID().String()); err != nil {
			t.Errorf("a new id does not parse: %v", err)
		}
	}
}

// Undo on the Budget screen removes a limit and sets it again: the line it
// comes back as is the one that carried, as long as its category's id is
// free.
func TestLimitSetAgainCarriesWhatItDid(t *testing.T) {
	food, fun, shop := cat(t, "food"), cat(t, "fun"), cat(t, "shop")
	aug, _ := NewMonthlyBudget(month(t, "2026-08"), now)
	_ = aug.SetLimit(food, money.BRLCents(50000), true, now)
	if l, _ := aug.LineOf(food); l.ID.String() != "food" {
		t.Fatalf("a new limit of one category has its id: %s", l.ID)
	}
	oct := aug.For(month(t, "2026-10"))
	if _, err := oct.RemoveLimit(food, now); err != nil {
		t.Fatal(err)
	}
	_ = oct.SetLimit(food, money.BRLCents(50000), true, now)
	spending := spend(t, map[string]map[string]int64{"2026-08": {"food": 30000}, "2026-09": {"food": 40000}})
	if c := find(t, Track([]*MonthlyBudget{aug, oct}, spending, nil, month(t, "2026-10"), now, nil), "food").Carried.Cents(); c != 30000 {
		t.Errorf("October carried %d after the limit was removed and set again, want 30000", c)
	}

	// A shared line that grew from food's own keeps food's id; food on its
	// own again gets a new one, and nothing of the shared line.
	b, _ := NewMonthlyBudget(month(t, "2026-10"), now)
	_ = b.SetLimit(food, money.BRLCents(50000), true, now)
	grown, _ := b.LineOf(food)
	if err := b.ChangeLine(Line{ID: grown.ID, Name: "Food and fun", Categories: cats(food, fun), Limit: money.BRLCents(80000)}, now); err != nil {
		t.Fatal(err)
	}
	if err := b.ChangeLine(Line{ID: grown.ID, Name: "Food and fun", Categories: cats(fun, shop), Limit: money.BRLCents(80000)}, now); err != nil {
		t.Fatal(err)
	}
	_ = b.SetLimit(food, money.BRLCents(50000), false, now)
	if l, _ := b.LineOf(food); l.ID == grown.ID || l.ID.IsZero() {
		t.Errorf("food's own line took the shared line's id: %s", l.ID)
	}
	if err := b.SetLimits([]Line{{ID: grown.ID, Name: "Food and fun", Categories: cats(fun, shop), Limit: money.BRLCents(80000)}, one(food, 1, false)}, now); err != nil {
		t.Errorf("every line at once, food's id given to the shared line: %v", err)
	}
}

// Archiving a category takes it out of the budget from the month it was
// archived on: out of its line, which keeps its other categories and its id,
// and a line left without any goes. The months before keep it.
func TestDropFrom(t *testing.T) {
	food, fun, shop, pet := cat(t, "food"), cat(t, "fun"), cat(t, "shop"), cat(t, "pet")
	aug, _ := NewMonthlyBudget(month(t, "2026-08"), now)
	_ = aug.SetLimit(food, money.BRLCents(50000), true, now)
	free, _ := aug.AddLine(Line{Name: "Free", Categories: cats(fun, shop), Limit: money.BRLCents(100000), Rollover: true}, now)
	oct := aug.For(month(t, "2026-10"))
	dec := aug.For(month(t, "2026-12"))
	_, _ = dec.RemoveLimit(food, now)
	_ = dec.ChangeLine(Line{ID: free, Name: "Free", Categories: cats(fun, shop, pet), Limit: money.BRLCents(120000), Rollover: true}, now)
	saved := []*MonthlyBudget{aug, oct, dec}
	later := now.Add(time.Hour)

	tests := []struct {
		name     string
		archived ledger.CategoryID
		month    string
		want     []string // the months saved again
	}{
		{"its own line goes", food, "2026-10", []string{"2026-10"}},
		{"a shared line keeps the others", shop, "2026-10", []string{"2026-10", "2026-12"}},
		{"a month that inherits is saved as its own", food, "2026-09", []string{"2026-09", "2026-10"}},
		{"only from that month on", pet, "2026-10", []string{"2026-12"}},
		{"before the first budget", food, "2026-07", []string{"2026-08", "2026-10"}},
		{"a category no line covers", cat(t, "gifts"), "2026-10", nil},
	}
	for _, tt := range tests {
		got := DropFrom(saved, month(t, tt.month), tt.archived, later)
		var months []string
		for _, b := range got {
			months = append(months, b.Month().String())
			if _, ok := b.LineOf(tt.archived); ok || !b.UpdatedAt().Equal(later) {
				t.Errorf("%s: %s still covers it, or was not changed now", tt.name, b.Month())
			}
			// Every other category stays in the line it was in.
			was, _ := Effective(saved, b.Month())
			for _, l := range was.Lines() {
				for _, c := range l.Categories {
					if got, ok := b.LineOf(c); c != tt.archived && (!ok || got.ID != l.ID || got.Name != l.Name) {
						t.Errorf("%s: in %s, %s left line %s: %+v", tt.name, b.Month(), c, l.ID, got)
					}
				}
			}
		}
		if !slices.Equal(months, tt.want) {
			t.Errorf("%s: saved %v again, want %v", tt.name, months, tt.want)
		}
	}
	if _, ok := aug.LineOf(food); !ok || len(aug.Lines()) != 2 || len(oct.Lines()) != 2 {
		t.Error("the budgets read changed: the months before must keep their figures")
	}
}
