//go:build integration

package budget

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"lastro/modules/categorize"
	"lastro/modules/ledger"
	"lastro/money"
	"lastro/platform/db/dbtest"
	"lastro/platform/events"
)

// today is late October in São Paulo.
var today = time.Date(2026, 10, 27, 15, 0, 0, 0, time.UTC)

type world struct {
	budget     *Service
	ledger     *ledger.Service
	categorize *categorize.Service
	cats       map[string]ledger.CategoryID
}

func newWorld(t *testing.T) world {
	t.Helper()
	pool := dbtest.New(t)
	bus := events.NewBus(nil)
	l := ledger.New(ledger.NewRepository(pool), bus)
	c := categorize.New(categorize.NewRepository(pool), l, nil, nil)
	events.Subscribe(bus, c.OnTransactionsImported)
	b := New(NewRepository(pool), l, c)
	b.now = func() time.Time { return today }
	cs, err := c.Categories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	w := world{budget: b, ledger: l, categorize: c, cats: map[string]ledger.CategoryID{}}
	for _, x := range cs {
		w.cats[x.Name] = x.ID
	}
	return w
}

type tx struct {
	day   string
	desc  string
	cents int64
	kind  ledger.Kind
}

// load teaches the rules first, so the import is categorized as it lands.
func (w world) load(t *testing.T, rules map[string]string, txs []tx) {
	t.Helper()
	ctx := context.Background()
	for text, cat := range rules {
		p, err := categorize.NewPattern(categorize.PatternContains, text)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.categorize.Learn(ctx, p, "", w.cats[cat]); err != nil {
			t.Fatal(err)
		}
	}
	ext, _ := ledger.NewExternalID("acc-1")
	if _, err := w.ledger.SyncAccount(ctx, ledger.AccountDetails{
		ExternalID: ext, Name: "Conta", Institution: "Nubank", Type: ledger.AccountChecking,
		Balance: money.BRLCents(100000), BalanceAt: today,
	}); err != nil {
		t.Fatal(err)
	}
	b := ledger.ImportBatch{Account: ext, From: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)}
	for i, x := range txs {
		id, _ := ledger.NewExternalID(fmt.Sprintf("tx-%d", i))
		d, _ := time.Parse(time.DateOnly, x.day)
		kind := x.kind
		if kind == "" {
			kind = ledger.KindExpense
		}
		b.Transactions = append(b.Transactions, ledger.ImportedTransaction{
			ExternalID: id, Date: d, Description: x.desc, Amount: money.BRLCents(x.cents), Kind: kind, Status: ledger.StatusPosted,
		})
	}
	if _, err := w.ledger.ImportTransactions(ctx, b); err != nil {
		t.Fatal(err)
	}
}

func TestBudgetMonth(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	w.load(t, map[string]string{"ALUGUEL": "Housing", "MERCADO": "Groceries", "SALARIO": "Income"}, []tx{
		{"2026-08-05", "ALUGUEL APTO", -300000, ""},
		{"2026-08-12", "MERCADO BOM", -60000, ""},
		{"2026-09-05", "ALUGUEL APTO", -300000, ""},
		{"2026-09-14", "MERCADO BOM", -90000, ""},
		{"2026-10-05", "SALARIO ACME", 1000000, ledger.KindIncome},
		{"2026-10-05", "ALUGUEL APTO", -300000, ""},
		{"2026-10-06", "APLICACAO CORRETORA", -200000, ledger.KindTransfer},
		{"2026-10-08", "MERCADO BOM", -50000, ""},
		{"2026-10-09", "ESTORNO MERCADO BOM", 5000, ""},
		{"2026-10-20", "PADARIA SEM REGRA", -1500, ""},
	})

	aug, _ := ParseMonth("2026-08")
	oct, _ := ParseMonth("2026-10")
	payday, _ := NewPayday(5)
	if err := w.budget.SetPayYourselfFirst(ctx, aug, money.BRLCents(200000), payday); err != nil {
		t.Fatal(err)
	}
	if err := w.budget.SetLimit(ctx, aug, w.cats["Groceries"], money.BRLCents(80000), true); err != nil {
		t.Fatal(err)
	}
	if err := w.budget.SetLimit(ctx, aug, w.cats["Housing"], money.BRLCents(310000), false); err != nil {
		t.Fatal(err)
	}
	if err := w.budget.SetLimit(ctx, aug, w.cats["Income"], money.BRLCents(1), false); !errors.Is(err, ErrNotBudgeted) {
		t.Fatalf("Income takes no limit: %v", err)
	}

	v, err := w.budget.Month(ctx, oct)
	if err != nil {
		t.Fatal(err)
	}
	if !v.HasBudget || v.Saved || *v.From != "2026-08" {
		t.Errorf("October inherits August: has %v saved %v from %v", v.HasBudget, v.Saved, v.From)
	}
	if v.IncomeCents != 1000000 || v.PayYourselfFirst.MovedCents != 200000 || v.PayYourselfFirst.DoneOn == nil || *v.PayYourselfFirst.DoneOn != "2026-10-06" {
		t.Errorf("income %d, pay yourself first %+v", v.IncomeCents, v.PayYourselfFirst)
	}
	g := category(t, v, w.cats["Groceries"])
	// August leaves 200; September has 1.000 with it, spends 900 and passes 100 on.
	if g.CarriedCents != 10000 || g.SpentCents != 45000 || g.LeftCents != 45000 || g.Count != 2 || g.FixedCents != 0 {
		t.Errorf("groceries %+v", g)
	}
	h := category(t, v, w.cats["Housing"])
	if h.FixedCents != 300000 || h.Pace != PaceOn {
		t.Errorf("the rent is a fixed charge, out of the pace: %+v", h)
	}
	// Not assigned also takes what was spent without a limit: the bakery.
	if v.NotAssignedCents != 1000000-200000-390000-1500 {
		t.Errorf("not assigned %d", v.NotAssignedCents)
	}
	if len(v.Unlimited) != 1 || v.Unlimited[0].Category != nil || v.Unlimited[0].SpentCents != 1500 {
		t.Errorf("only the uncategorized bakery is without a limit; the salary is not spending: %+v", v.Unlimited)
	}
	if v.Summary.SpentCents != 300000+45000+1500 || v.Summary.SavingsRateBp != (1000000-346500)*10000/1000000 {
		t.Errorf("summary %+v", v.Summary)
	}

	// September rolls over August's 200 into its own limit.
	sep, _ := ParseMonth("2026-09")
	sv, err := w.budget.Month(ctx, sep)
	if err != nil {
		t.Fatal(err)
	}
	if g := category(t, sv, w.cats["Groceries"]); g.CarriedCents != 20000 || g.AvailableCents != 100000 || g.LeftCents != 10000 || g.Pace != PaceOn {
		t.Errorf("September groceries %+v", g)
	}

	// A change in October saves October whole; August keeps its limit.
	if err := w.budget.SetLimit(ctx, oct, w.cats["Groceries"], money.BRLCents(70000), true); err != nil {
		t.Fatal(err)
	}
	if err := w.budget.RemoveLimit(ctx, oct, w.cats["Housing"]); err != nil {
		t.Fatal(err)
	}
	v, _ = w.budget.Month(ctx, oct)
	if !v.Saved || category(t, v, w.cats["Groceries"]).LimitCents != 70000 || len(v.Categories) != 1 || v.PayYourselfFirst.PlannedCents != 200000 {
		t.Errorf("October after the change: saved %v %+v", v.Saved, v.Categories)
	}
	av, _ := w.budget.Month(ctx, aug)
	if category(t, av, w.cats["Groceries"]).LimitCents != 80000 || len(av.Categories) != 2 {
		t.Errorf("August changed: %+v", av.Categories)
	}

	rec, err := w.budget.Recurring(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Charges) != 1 || rec.Charges[0].Name != "ALUGUEL APTO" || rec.MonthlyCents != 300000 || !rec.Charges[0].PaidThisMonth {
		t.Errorf("recurring %+v", rec)
	}
}

func TestAlertsAreSentOnce(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	w.load(t, map[string]string{"MERCADO": "Groceries"}, []tx{
		{"2026-10-08", "MERCADO BOM", -90000, ""},
	})
	oct, _ := ParseMonth("2026-10")
	if err := w.budget.SetLimit(ctx, oct, w.cats["Groceries"], money.BRLCents(50000), false); err != nil {
		t.Fatal(err)
	}
	all, err := w.budget.Alerts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Kind != AlertOver || all[0].Title != "Groceries is over its limit by R$ 400,00" {
		t.Fatalf("alerts %+v", all)
	}
	fresh, _ := w.budget.Unsent(ctx, all)
	if len(fresh) != 1 {
		t.Fatalf("unsent %d", len(fresh))
	}
	if err := w.budget.MarkSent(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	if again, _ := w.budget.Unsent(ctx, all); len(again) != 0 {
		t.Errorf("sent twice: %+v", again)
	}
}

func category(t *testing.T, v MonthView, c ledger.CategoryID) CategoryView {
	t.Helper()
	for _, x := range v.Categories {
		if x.Category == c.String() {
			return x
		}
	}
	t.Fatalf("%s has no limit in %s", c, v.Month)
	return CategoryView{}
}

func TestCopyAndReplaceLimits(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	sep, oct := month(t, "2026-09"), month(t, "2026-10")
	food, fun := w.cats["Groceries"], w.cats["Leisure"]
	if food.IsZero() || fun.IsZero() {
		t.Fatalf("seed categories missing: %v", w.cats)
	}
	if err := w.budget.SetLimit(ctx, sep, food, money.BRLCents(120000), true); err != nil {
		t.Fatal(err)
	}
	// October gets its own limits, then all of them at once.
	if err := w.budget.SetLimits(ctx, oct, []Line{{Category: fun, Limit: money.BRLCents(30000)}}); err != nil {
		t.Fatal(err)
	}
	v, err := w.budget.Month(ctx, oct)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Categories) != 1 || v.Categories[0].Category != fun.String() || !v.Saved || !v.PrevDiffers {
		t.Fatalf("after SetLimits: %d categories, saved %v, prevDiffers %v", len(v.Categories), v.Saved, v.PrevDiffers)
	}
	if err := w.budget.SetLimits(ctx, oct, []Line{{Category: w.cats["Income"], Limit: money.BRLCents(1)}}); !errors.Is(err, ErrNotBudgeted) {
		t.Errorf("a limit on Income: %v, want ErrNotBudgeted", err)
	}

	n, err := w.budget.CopyLimits(ctx, oct, sep)
	if err != nil || n != 1 {
		t.Fatalf("CopyLimits = %d, %v", n, err)
	}
	v, _ = w.budget.Month(ctx, oct)
	if len(v.Categories) != 1 || v.Categories[0].Category != food.String() || v.Categories[0].LimitCents != 120000 || !v.Categories[0].Rollover || v.PrevDiffers {
		t.Fatalf("after copying September: %+v, prevDiffers %v", v.Categories, v.PrevDiffers)
	}
	if _, err := w.budget.CopyLimits(ctx, oct, month(t, "2025-01")); !errors.Is(err, ErrInvalid) {
		t.Errorf("copying a month without a budget: %v, want ErrInvalid", err)
	}

	// The default for a new limit is off until set.
	if v.RolloverNew {
		t.Error("rolloverNew should start off")
	}
	if err := w.budget.SetPreferences(ctx, ptr(true), nil); err != nil {
		t.Fatal(err)
	}
	if v, _ = w.budget.Month(ctx, oct); !v.RolloverNew {
		t.Error("rolloverNew should be on")
	}
}

func TestWhatAMonthLeaves(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	w.load(t, map[string]string{"ALUGUEL": "Housing", "MERCADO": "Groceries", "SALARIO": "Income"}, []tx{
		{"2026-09-05", "SALARIO ACME", 1000000, ledger.KindIncome},
		{"2026-09-05", "ALUGUEL APTO", -300000, ""},
		{"2026-09-14", "MERCADO BOM", -110000, ""}, // 300 past its limit
		{"2026-09-20", "PADARIA SEM REGRA", -2000, ""},
		{"2026-10-05", "SALARIO ACME", 1000000, ledger.KindIncome},
		{"2026-10-06", "APLICACAO CORRETORA", -350000, ledger.KindTransfer},
	})
	sep, _ := ParseMonth("2026-09")
	oct, _ := ParseMonth("2026-10")
	payday, _ := NewPayday(5)
	if err := w.budget.SetPayYourselfFirst(ctx, sep, money.BRLCents(200000), payday); err != nil {
		t.Fatal(err)
	}
	if err := w.budget.SetLimit(ctx, sep, w.cats["Housing"], money.BRLCents(310000), false); err != nil {
		t.Fatal(err)
	}
	if err := w.budget.SetLimit(ctx, sep, w.cats["Groceries"], money.BRLCents(80000), false); err != nil {
		t.Fatal(err)
	}

	// September: 10.000 − 2.000 − 3.900 limits − 300 over − 20 without a
	// category + 100 Housing did not spend = 3.880.
	v, err := w.budget.Month(ctx, oct)
	if err != nil {
		t.Fatal(err)
	}
	if v.Prev == nil || !v.Prev.Closed || v.Prev.LeftCents != 388000 || !v.Prev.Pending || v.Prev.OverCents != 30000 || v.Prev.UnspentCents != 10000 {
		t.Fatalf("September's rest = %+v", v.Prev)
	}
	// October runs: nothing kept yet, so Not assigned is 10.000 − 2.000 − 3.900.
	if v.NotAssignedCents != 410000 || v.Rest == nil || v.Rest.Closed {
		t.Fatalf("October: not assigned %d, rest %+v", v.NotAssignedCents, v.Rest)
	}

	if err := w.budget.DecideRest(ctx, oct, RestInvest, money.BRLCents(0)); !errors.Is(err, ErrInvalid) {
		t.Errorf("deciding a month that runs: %v, want ErrInvalid", err)
	}
	if err := w.budget.DecideRest(ctx, sep, RestSplit, money.BRLCents(388001)); !errors.Is(err, ErrInvalid) {
		t.Errorf("investing more than was left: %v, want ErrInvalid", err)
	}
	if err := w.budget.DecideRest(ctx, sep, RestSplit, money.BRLCents(150000)); err != nil {
		t.Fatal(err)
	}
	v, _ = w.budget.Month(ctx, oct)
	if v.Prev.Pending || v.Prev.Choice != RestSplit || v.Prev.InvestCents != 150000 || v.Prev.KeepCents != 238000 {
		t.Fatalf("after the split: %+v", v.Prev)
	}
	// The kept part is October's; the invested part goes with pay yourself
	// first, and the 3.500 moved covers both.
	if v.NotAssignedCents != 410000+238000 || v.Rest.KeptCents != 238000 || v.PayYourselfFirst.FromRestCents != 150000 || v.PayYourselfFirst.DoneOn == nil {
		t.Fatalf("October after the split: not assigned %d, rest %+v, pay yourself first %+v", v.NotAssignedCents, v.Rest, v.PayYourselfFirst)
	}

	// Taking it back with "always invest" as the default invests it all.
	invest := RestInvest
	if err := w.budget.SetPreferences(ctx, nil, &invest); err != nil {
		t.Fatal(err)
	}
	if err := w.budget.ClearRest(ctx, sep); err != nil {
		t.Fatal(err)
	}
	v, _ = w.budget.Month(ctx, oct)
	if v.RestDefault != RestInvest || v.Prev.Decided || v.Prev.Choice != RestInvest || v.Prev.InvestCents != 388000 || v.NotAssignedCents != 410000 {
		t.Fatalf("with the default: %+v, not assigned %d", v.Prev, v.NotAssignedCents)
	}
	if v.PayYourselfFirst.DoneOn != nil {
		t.Error("3.500 moved does not cover 2.000 plus 3.880")
	}
	bad := RestChoice("split")
	if err := w.budget.SetPreferences(ctx, nil, &bad); !errors.Is(err, ErrInvalid) {
		t.Errorf("split as a default: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }
