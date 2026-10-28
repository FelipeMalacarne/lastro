package budget

import (
	"errors"
	"testing"

	"lastro/modules/ledger"
	"lastro/money"
)

// fund is a fund's line of one category.
func fund(c ledger.CategoryID, cents int64) Line {
	return Line{Categories: []ledger.CategoryID{c}, Limit: money.BRLCents(cents), Kind: KindFund}
}

// months builds a budget per month from the one before, as editing does,
// each month given its lines.
func months(t *testing.T, by map[string][]Line) []*MonthlyBudget {
	t.Helper()
	var out []*MonthlyBudget
	for _, k := range []string{"2026-07", "2026-08", "2026-09", "2026-10", "2026-11", "2026-12"} {
		lines, ok := by[k]
		if !ok {
			continue
		}
		b, _ := NewMonthlyBudget(month(t, k), now)
		if len(out) > 0 {
			b = out[len(out)-1].For(month(t, k))
		}
		if err := b.SetLimits(lines, now); err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

func TestFundCarriesItsDebt(t *testing.T) {
	car, food := cat(t, "car"), cat(t, "food")
	budgets := months(t, map[string][]Line{"2026-08": {fund(car, 30000), one(food, 80000, false)}})
	spending := spend(t, map[string]map[string]int64{
		"2026-08": {"food": 70000},
		"2026-09": {"car": 200000, "food": 90000}, // a repair: 300 + 300 - 2.000
		"2026-10": {"food": 80000},
		"2026-11": {"car": -90000}, // the insurer pays back, a month late
	})

	sep := find(t, Track(budgets, spending, nil, month(t, "2026-09"), now, nil), "car")
	if sep.Carried.Cents() != 30000 || sep.Available.Cents() != 60000 || sep.Left.Cents() != -140000 || sep.Pace != PaceOver {
		t.Errorf("September's car fund, in the red: %+v", sep)
	}
	oct := Track(budgets, spending, nil, month(t, "2026-10"), now, nil)
	if c := find(t, oct, "car"); c.Carried.Cents() != -140000 || c.Left.Cents() != -110000 || c.Pace != PaceOver {
		t.Errorf("October pays 300 of the debt back: %+v", c)
	}
	// The refund goes back into the fund, on top of a debt it remembers.
	nov := find(t, Track(budgets, spending, nil, month(t, "2026-11"), now, nil), "car")
	if nov.Carried.Cents() != -110000 || nov.Left.Cents() != 10000 || nov.Pace != PaceOn {
		t.Errorf("November, with the refund: %+v", nov)
	}

	// The month's totals are the limits'; the funds have their own.
	if oct.Available.Cents() != 80000 || oct.Spent.Cents() != 80000 || oct.Left.Cents() != 0 {
		t.Errorf("October's totals: %d %d %d", oct.Available.Cents(), oct.Spent.Cents(), oct.Left.Cents())
	}
	f := oct.Funds
	if f.Count != 1 || f.Amount.Cents() != 30000 || f.Carried.Cents() != -140000 || f.Spent.Cents() != 0 || f.Balance.Cents() != -110000 || f.Kept.Cents() != 30000 {
		t.Errorf("October's funds: %+v", f)
	}
	if k := Track(budgets, spending, nil, month(t, "2026-09"), now, nil).Funds.Kept.Cents(); k != 30000-200000 {
		t.Errorf("September kept %d: the repair took more than the month put in", k)
	}
}

func TestFundIsNeverAheadOfPace(t *testing.T) {
	budgets := months(t, map[string][]Line{"2026-10": {fund(cat(t, "moi"), 50000)}})
	spending := spend(t, map[string]map[string]int64{"2026-10": {"moi": 45000}})
	third := now.AddDate(0, 0, -24) // the 3rd: 90% spent, far ahead for a limit
	if p := find(t, Track(budgets, spending, nil, month(t, "2026-10"), third, nil), "moi").Pace; p != PaceOn {
		t.Errorf("a fund's lump: %s", p)
	}
}

func TestAdjustments(t *testing.T) {
	car, moi := cat(t, "car"), cat(t, "moi")
	budgets := months(t, map[string][]Line{"2026-09": {fund(car, 30000), fund(moi, 50000)}})
	carID, _ := budgets[0].LineOf(car)
	moiID, _ := budgets[0].LineOf(moi)
	// September starts the car fund with what was already saved for it;
	// October moves 200 from going out to the car.
	adjusted := Adjustments{
		month(t, "2026-09"): {carID.ID: money.BRLCents(100000)},
		month(t, "2026-10"): {carID.ID: money.BRLCents(20000), moiID.ID: money.BRLCents(-20000)},
	}
	spending := spend(t, map[string]map[string]int64{"2026-09": {"car": 10000}})
	oct := Track(budgets, spending, adjusted, month(t, "2026-10"), now, nil)
	c, m := find(t, oct, "car"), find(t, oct, "moi")
	if c.Carried.Cents() != 120000 || c.Adjusted.Cents() != 20000 || c.Left.Cents() != 170000 {
		t.Errorf("car: %+v", c)
	}
	if m.Carried.Cents() != 50000 || m.Adjusted.Cents() != -20000 || m.Left.Cents() != 80000 {
		t.Errorf("going out: %+v", m)
	}
	// An adjustment is old money: what the month kept is its amounts alone.
	if oct.Funds.Adjusted.Cents() != 0 || oct.Funds.Kept.Cents() != 80000 || oct.Funds.Balance.Cents() != 250000 {
		t.Errorf("October's funds: %+v", oct.Funds)
	}
	// A limit takes no adjustment, even one left from when it was a fund.
	limits := months(t, map[string][]Line{"2026-09": {fund(car, 30000)}, "2026-10": {{Categories: cats(car), Limit: money.BRLCents(30000), Kind: KindLimit}}})
	limitID, _ := limits[1].LineOf(car)
	if l := find(t, Track(limits, spending, Adjustments{month(t, "2026-10"): {limitID.ID: money.BRLCents(99900)}}, month(t, "2026-10"), now, nil), "car"); l.Adjusted.Cents() != 0 || l.Available.Cents() != 50000 {
		t.Errorf("a limit with a stray adjustment: %+v", l)
	}
}

func TestNewAdjustment(t *testing.T) {
	b, _ := NewMonthlyBudget(month(t, "2026-10"), now)
	car, food := cat(t, "car"), cat(t, "food")
	if err := b.SetLimits([]Line{fund(car, 30000), one(food, 80000, false)}, now); err != nil {
		t.Fatal(err)
	}
	carLine, _ := b.LineOf(car)
	foodLine, _ := b.LineOf(food)
	usd, _ := money.New(100, money.USD)
	unknown, _ := ParseLineID("0b7e4c1d-8a9f-4e2b-b3c4-d5e6f7a8b9c0")
	tests := []struct {
		name string
		id   LineID
		amt  money.Money
		want error
	}{
		{"a limit", foodLine.ID, money.BRLCents(100), ErrInvalid},
		{"nothing", carLine.ID, money.BRLCents(0), ErrInvalid},
		{"dollars", carLine.ID, usd, ErrInvalid},
		{"a line the month lacks", unknown, money.BRLCents(100), ErrNoLine},
	}
	for _, tt := range tests {
		if _, err := NewAdjustment(b, tt.id, tt.amt); !errors.Is(err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.want)
		}
	}
	a, err := NewAdjustment(b, carLine.ID, money.BRLCents(-5000))
	if err != nil || a.Month != b.Month() || a.Amount.Cents() != -5000 {
		t.Errorf("taking 50 out of the car fund: %+v, %v", a, err)
	}
}

func TestKindIsKeptUnlessGiven(t *testing.T) {
	car, food, fun, shop := cat(t, "car"), cat(t, "food"), cat(t, "fun"), cat(t, "shop")
	b, _ := NewMonthlyBudget(month(t, "2026-10"), now)
	roll := fund(car, 30000)
	roll.Rollover = true
	if err := b.SetLimits([]Line{roll, one(food, 80000, true)}, now); err != nil {
		t.Fatal(err)
	}
	if l, _ := b.LineOf(car); !l.IsFund() || l.Rollover {
		t.Errorf("a fund always carries, so it never rolls over: %+v", l)
	}
	if l, _ := b.LineOf(food); l.Kind != KindLimit {
		t.Errorf("a line without a kind is a limit: %+v", l)
	}

	// A category's own limit, and every limit at once without kinds, keep them.
	if err := b.SetLimit(car, money.BRLCents(40000), true, now); err != nil {
		t.Fatal(err)
	}
	if err := b.SetLimits([]Line{one(car, 40000, false), one(food, 80000, true)}, now); err != nil {
		t.Fatal(err)
	}
	if l, _ := b.LineOf(car); !l.IsFund() || l.Limit.Cents() != 40000 {
		t.Errorf("the car fund after its limit changed: %+v", l)
	}

	// Changing a line without a kind keeps it; a new line is a limit.
	id, _ := b.AddLine(Line{Name: "Free", Categories: cats(fun, shop), Limit: money.BRLCents(100000), Kind: KindFund}, now)
	if err := b.ChangeLine(Line{ID: id, Name: "Free", Categories: cats(fun, shop), Limit: money.BRLCents(90000)}, now); err != nil {
		t.Fatal(err)
	}
	if l, _ := b.Line(id); !l.IsFund() {
		t.Errorf("a shared fund changed without a kind: %+v", l)
	}
	if err := b.ChangeLine(Line{ID: id, Name: "Free", Categories: cats(fun, shop), Limit: money.BRLCents(90000), Kind: KindLimit}, now); err != nil {
		t.Fatal(err)
	}
	if l, _ := b.Line(id); l.IsFund() {
		t.Errorf("a fund turned into a limit: %+v", l)
	}
	if _, err := b.AddLine(Line{Categories: cats(cat(t, "gifts")), Limit: money.BRLCents(1), Kind: "envelope"}, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("an unknown kind: %v, want ErrInvalid", err)
	}
}

// A fund turned into a limit leaves its debt behind but keeps what it
// saved; a limit turned into a fund starts from what it rolled over.
func TestKindChanges(t *testing.T) {
	car, moi := cat(t, "car"), cat(t, "moi")
	budgets := months(t, map[string][]Line{
		"2026-08": {fund(car, 30000), one(moi, 50000, true)},
		"2026-10": {{Categories: cats(car), Limit: money.BRLCents(30000), Kind: KindLimit}, fund(moi, 50000)},
	})
	spending := spend(t, map[string]map[string]int64{"2026-08": {"car": 100000, "moi": 30000}})
	tr := Track(budgets, spending, nil, month(t, "2026-10"), now, nil)
	// August: car -700, moi 200 left; September: car -400, moi 700 (it rolls over).
	if c := find(t, tr, "car"); c.Carried.Cents() != 0 || c.Available.Cents() != 30000 {
		t.Errorf("the car, a limit again, without the debt: %+v", c)
	}
	if m := find(t, tr, "moi"); m.Carried.Cents() != 70000 || m.Left.Cents() != 120000 {
		t.Errorf("going out, a fund now, from what it rolled over: %+v", m)
	}
}

func TestFindAlertsForFunds(t *testing.T) {
	oct := month(t, "2026-10")
	b := money.BRLCents
	car := LineID{s: "car"}
	figures := []Figures{
		{Line: car, Category: cat(t, "car"), Limited: true, Fund: true, Spent: b(200000), Available: b(60000), Left: b(-140000), Average: b(5000), Pace: PaceOver},
		{Line: LineID{s: "moi"}, Category: cat(t, "moi"), Limited: true, Fund: true, Spent: b(90000), Available: b(100000), Left: b(10000), Average: b(20000), Pace: PaceOn},
	}
	got := FindAlerts(oct, now, figures, nil, 5)
	if len(got) != 1 || got[0].Key != "overdrawn:2026-10:car" || got[0].Kind != AlertOverdrawn || got[0].Amount.Cents() != 140000 {
		t.Errorf("a fund's one alert is going into the red, never unusual: %+v", got)
	}
}

// A fund's balance at each month's end, for its history: the months it was
// a fund among the last six, this one as it stands.
func TestFundBalances(t *testing.T) {
	car, food := cat(t, "car"), cat(t, "food")
	budgets := months(t, map[string][]Line{"2026-08": {fund(car, 30000), one(food, 80000, false)}})
	spending := spend(t, map[string]map[string]int64{
		"2026-09": {"car": 200000},
		"2026-11": {"car": -90000},
	})
	tr := Track(budgets, spending, nil, month(t, "2026-11"), now, nil)
	var got []string
	for _, b := range find(t, tr, "car").Balances {
		got = append(got, b.Month.String()+" "+b.Balance.String())
	}
	want := []string{"2026-08 R$ 300,00", "2026-09 -R$ 1.400,00", "2026-10 -R$ 1.100,00", "2026-11 R$ 100,00"}
	if len(got) != len(want) {
		t.Fatalf("balances %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("balance %d = %s, want %s", i, got[i], want[i])
		}
	}
	if find(t, tr, "food").Balances != nil {
		t.Error("a limit has no balances")
	}
}
