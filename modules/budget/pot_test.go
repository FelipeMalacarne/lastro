package budget

import (
	"errors"
	"testing"

	"lastro/modules/ledger"
	"lastro/money"
)

func lineID(t *testing.T, s string) LineID {
	t.Helper()
	id, err := ParseLineID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// pot is a pot with an id of its own, since one without categories has no
// category's id to keep from month to month.
func pot(id LineID, name string, cents, target int64, cats ...ledger.CategoryID) Line {
	l := Line{ID: id, Name: name, Categories: cats, Limit: money.BRLCents(cents), Kind: KindFund}
	if target > 0 {
		l.Target = money.BRLCents(target)
	}
	return l
}

func findID(t *testing.T, tr Tracking, id LineID) LineStatus {
	t.Helper()
	for _, s := range tr.Lines {
		if s.Line.ID == id {
			return s
		}
	}
	t.Fatalf("%s not tracked in %s", id, tr.Month)
	return LineStatus{}
}

func TestPotRules(t *testing.T) {
	food := cat(t, "food")
	id := lineID(t, "0b5a3c1e-1111-4a2b-8c3d-000000000001")
	usd, err := money.New(500, money.USD)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		line Line
		want error
	}{
		{"a pot without categories", pot(id, "Emergency fund", 0, 1500000), nil},
		{"a pot without categories needs a name", pot(id, "", 0, 0), ErrInvalid},
		{"a limit without categories", Line{ID: id, Name: "Nothing", Limit: money.BRLCents(100), Kind: KindLimit}, ErrInvalid},
		{"a new line without a kind is a limit", Line{ID: id, Name: "Nothing", Limit: money.BRLCents(100)}, ErrInvalid},
		{"a target on a limit", Line{Categories: []ledger.CategoryID{food}, Limit: money.BRLCents(100), Kind: KindLimit, Target: money.BRLCents(500)}, ErrInvalid},
		{"a pot without a target", pot(id, "Trip", 100, 0, food), nil},
		{"a negative target", Line{ID: id, Name: "Trip", Limit: money.BRLCents(100), Kind: KindFund, Target: money.BRLCents(-500)}, ErrInvalid},
		{"a target in dollars", Line{ID: id, Name: "Trip", Limit: money.BRLCents(100), Kind: KindFund, Target: usd}, ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := NewMonthlyBudget(month(t, "2026-10"), now)
			err := b.SetLimits([]Line{tt.line}, now)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestPotWithoutCategoriesStays(t *testing.T) {
	food, fun := cat(t, "food"), cat(t, "fun")
	b, _ := NewMonthlyBudget(month(t, "2026-10"), now)
	id, err := b.AddLine(Line{Name: "Emergency fund", Kind: KindFund, Limit: money.BRLCents(0), Target: money.BRLCents(1500000)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddLine(one(food, 80000, false), now); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddLine(Line{Name: "Free to spend", Categories: []ledger.CategoryID{food, fun}, Limit: money.BRLCents(100000)}, now); err != nil {
		t.Fatal(err)
	}
	l, ok := b.Line(id)
	if !ok || !l.IsFund() || !l.HasTarget() || len(b.Lines()) != 2 {
		t.Fatalf("the emergency fund went with the food limit: %+v, %d lines", l, len(b.Lines()))
	}
	// Changed without a kind, it stays a pot; without a target, it has none.
	if err := b.ChangeLine(Line{ID: id, Name: "Emergency fund", Limit: money.BRLCents(50000)}, now); err != nil {
		t.Fatal(err)
	}
	if l, _ := b.Line(id); !l.IsFund() || l.HasTarget() || l.Limit.Cents() != 50000 {
		t.Errorf("changed: %+v", l)
	}
	// Copied to another month, it keeps its id and what it carries.
	c, _ := NewMonthlyBudget(month(t, "2026-11"), now)
	if err := c.CopyLines(b.Lines(), b, now); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Line(id); !ok {
		t.Error("the copy lost the emergency fund")
	}
}

func TestPotTakesWhatItLacksOfItsTarget(t *testing.T) {
	trip := cat(t, "trip")
	id := lineID(t, "0b5a3c1e-1111-4a2b-8c3d-000000000002")
	budgets := months(t, map[string][]Line{"2026-07": {pot(id, "Trip", 40000, 100000, trip)}})
	spending := spend(t, map[string]map[string]int64{"2026-11": {"trip": 30000}})
	tests := []struct {
		month              string
		amount, left, kept int64
	}{
		{"2026-07", 40000, 40000, 40000},
		{"2026-08", 40000, 80000, 40000},
		{"2026-09", 20000, 100000, 20000}, // only what it lacks
		{"2026-10", 0, 100000, 0},         // reached: Not assigned keeps the 400
		{"2026-11", 0, 70000, -30000},     // spent from it, at the target when the month began
		{"2026-12", 30000, 100000, 30000}, // below it again: takes what it lacks
	}
	for _, tt := range tests {
		tr := Track(budgets, spending, nil, month(t, tt.month), now, nil)
		st := findID(t, tr, id)
		if st.Amount.Cents() != tt.amount || st.Left.Cents() != tt.left || tr.Funds.Kept.Cents() != tt.kept || tr.Assigned.Cents() != tt.amount || tr.Funds.Amount.Cents() != tt.amount {
			t.Errorf("%s: amount %d, left %d, kept %d, assigned %d; want %d, %d, %d", tt.month, st.Amount.Cents(), st.Left.Cents(), tr.Funds.Kept.Cents(), tr.Assigned.Cents(), tt.amount, tt.left, tt.kept)
		}
	}
}

func TestAdjustmentCountsTowardsTheTarget(t *testing.T) {
	id := lineID(t, "0b5a3c1e-1111-4a2b-8c3d-000000000003")
	budgets := months(t, map[string][]Line{"2026-09": {pot(id, "Emergency fund", 40000, 1500000)}})
	adjusted := Adjustments{month(t, "2026-09"): {id: money.BRLCents(1400000)}}
	sep := findID(t, Track(budgets, nil, adjusted, month(t, "2026-09"), now, nil), id)
	if sep.Amount.Cents() != 40000 || sep.Left.Cents() != 1440000 {
		t.Errorf("September: %+v", sep)
	}
	oct := findID(t, Track(budgets, nil, adjusted, month(t, "2026-10"), now, nil), id)
	if oct.Amount.Cents() != 40000 || oct.Left.Cents() != 1480000 {
		t.Errorf("October: %+v", oct)
	}
	nov := findID(t, Track(budgets, nil, adjusted, month(t, "2026-11"), now, nil), id)
	if nov.Amount.Cents() != 20000 || nov.Left.Cents() != 1500000 {
		t.Errorf("November takes only the 200 it lacks: %+v", nov)
	}
}
