//go:build integration

package budget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"lastro/migrations"
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
	if v.NotAssignedCents != 1000000-200000-390000 {
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
	if !v.Saved || category(t, v, w.cats["Groceries"]).LimitCents != 70000 || len(v.Lines) != 1 || v.PayYourselfFirst.PlannedCents != 200000 {
		t.Errorf("October after the change: saved %v %+v", v.Saved, v.Lines)
	}
	av, _ := w.budget.Month(ctx, aug)
	if category(t, av, w.cats["Groceries"]).LimitCents != 80000 || len(av.Lines) != 2 {
		t.Errorf("August changed: %+v", av.Lines)
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

// category is the line that covers a category.
func category(t *testing.T, v MonthView, c ledger.CategoryID) LineView {
	t.Helper()
	for _, x := range v.Lines {
		for _, p := range x.ByCategory {
			if p.Category == c.String() {
				return x
			}
		}
	}
	t.Fatalf("%s has no limit in %s", c, v.Month)
	return LineView{}
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
	if err := w.budget.SetLimits(ctx, oct, []Line{one(fun, 30000, false)}); err != nil {
		t.Fatal(err)
	}
	v, err := w.budget.Month(ctx, oct)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Lines) != 1 || v.Lines[0].ByCategory[0].Category != fun.String() || !v.Saved || !v.PrevDiffers {
		t.Fatalf("after SetLimits: %d categories, saved %v, prevDiffers %v", len(v.Lines), v.Saved, v.PrevDiffers)
	}
	if err := w.budget.SetLimits(ctx, oct, []Line{one(w.cats["Income"], 1, false)}); !errors.Is(err, ErrNotBudgeted) {
		t.Errorf("a limit on Income: %v, want ErrNotBudgeted", err)
	}

	n, err := w.budget.CopyLimits(ctx, oct, sep)
	if err != nil || n != 1 {
		t.Fatalf("CopyLimits = %d, %v", n, err)
	}
	v, _ = w.budget.Month(ctx, oct)
	if len(v.Lines) != 1 || v.Lines[0].ByCategory[0].Category != food.String() || v.Lines[0].LimitCents != 120000 || !v.Lines[0].Rollover || v.PrevDiffers {
		t.Fatalf("after copying September: %+v, prevDiffers %v", v.Lines, v.PrevDiffers)
	}
	if _, err := w.budget.CopyLimits(ctx, oct, month(t, "2025-01")); !errors.Is(err, ErrInvalid) {
		t.Errorf("copying a month without a budget: %v, want ErrInvalid", err)
	}

	// The default for a new limit is off until set.
	if v.RolloverNew {
		t.Error("rolloverNew should start off")
	}
	if err := w.budget.SetRolloverNew(ctx, true); err != nil {
		t.Fatal(err)
	}
	if v, _ = w.budget.Month(ctx, oct); !v.RolloverNew {
		t.Error("rolloverNew should be on")
	}
}

func TestPayYourselfFirstIsNetOfRedemptions(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	w.load(t, map[string]string{"SALARIO": "Income"}, []tx{
		{"2026-10-01", "SALARIO ACME", 1000000, ledger.KindIncome},
		{"2026-10-01", "APLICACAO RDB", -300000, ledger.KindTransfer},
		// Moving from one investment to another puts nothing aside.
		{"2026-10-20", "RESGATE RDB", 500000, ledger.KindTransfer},
		{"2026-10-20", "COMPRA DE RENDA VARIAVEL", -500000, ledger.KindTransfer},
		{"2026-10-22", "RESGATE RDB", 50000, ledger.KindTransfer},
	})
	oct, _ := ParseMonth("2026-10")
	payday, _ := NewPayday(1)
	if err := w.budget.SetPayYourselfFirst(ctx, oct, money.BRLCents(250000), payday); err != nil {
		t.Fatal(err)
	}
	v, err := w.budget.Month(ctx, oct)
	if err != nil {
		t.Fatal(err)
	}
	if v.PayYourselfFirst.MovedCents != 250000 || v.PayYourselfFirst.DoneOn == nil || *v.PayYourselfFirst.DoneOn != "2026-10-20" {
		t.Errorf("pay yourself first %+v, want 250.000 moved, done on the 20th", v.PayYourselfFirst)
	}
	if v.Summary.ContributionCents != 250000 {
		t.Errorf("contribution %d", v.Summary.ContributionCents)
	}
}

// Money that comes in under a spending category is a refund: it lowers
// that category's spending and is not income, or it would count twice.
func TestRefundIsNotIncome(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	w.load(t, map[string]string{"SALARIO": "Income", "SEGURO": "Transport"}, []tx{
		{"2026-10-01", "SALARIO ACME", 1000000, ledger.KindIncome},
		{"2026-10-03", "SEGURO AUTO", -100000, ""},
		{"2026-10-06", "TRANSFERENCIA RECEBIDA SEGURO AUTO", 40000, ledger.KindIncome},
		{"2026-10-07", "PIX RECEBIDO SEM REGRA", 20000, ledger.KindIncome},
	})
	oct, _ := ParseMonth("2026-10")
	v, err := w.budget.Month(ctx, oct)
	if err != nil {
		t.Fatal(err)
	}
	// The uncategorized Pix stays income: nothing says it is a refund.
	if v.IncomeCents != 1020000 || v.IncomeCount != 2 {
		t.Errorf("income %d from %d, want 1.020.000 from 2", v.IncomeCents, v.IncomeCount)
	}
	if v.Summary.SpentCents != 60000 || v.Summary.SavingsRateBp != (1020000-60000)*10000/1020000 {
		t.Errorf("summary %+v", v.Summary)
	}
}

// Leisure and Shopping share one amount; each still says where its share
// went, and the line keeps what it carried when Restaurants joins it.
func TestSharedLine(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	w.load(t, map[string]string{"CINEMA": "Leisure", "LOJA": "Shopping", "BAR": "Restaurants", "MERCADO": "Groceries"}, []tx{
		{"2026-09-10", "CINEMA CENTRO", -20000, ""},
		{"2026-09-12", "LOJA PECAS PC", -50000, ""},
		{"2026-10-03", "LOJA PECAS PC", -90000, ""},
		{"2026-10-04", "LOJA PECAS PC", -10000, ""},
		{"2026-10-08", "BAR DO ZE", -15000, ""},
		{"2026-10-09", "MERCADO BOM", -30000, ""},
	})
	sep, oct := month(t, "2026-09"), month(t, "2026-10")
	fun, shop, bar := w.cats["Leisure"], w.cats["Shopping"], w.cats["Restaurants"]
	if err := w.budget.SetLimit(ctx, sep, bar, money.BRLCents(20000), false); err != nil {
		t.Fatal(err)
	}
	if _, err := w.budget.AddLine(ctx, sep, Line{Name: "Free to spend", Categories: cats(fun, w.cats["Income"]), Limit: money.BRLCents(1)}); !errors.Is(err, ErrNotBudgeted) {
		t.Errorf("a line with Income: %v, want ErrNotBudgeted", err)
	}
	free, err := w.budget.AddLine(ctx, sep, Line{Name: "Free to spend", Categories: cats(fun, shop), Limit: money.BRLCents(100000), Rollover: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.budget.SetLimit(ctx, sep, shop, money.BRLCents(1), false); !errors.Is(err, ErrShared) {
		t.Errorf("Shopping's own limit while it shares a line: %v, want ErrShared", err)
	}

	// October: Restaurants leaves its own limit for the line.
	if err := w.budget.ChangeLine(ctx, oct, Line{ID: free, Name: "Free to spend", Categories: cats(fun, shop, bar), Limit: money.BRLCents(100000), Rollover: true}); err != nil {
		t.Fatal(err)
	}
	v, err := w.budget.Month(ctx, oct)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Lines) != 1 {
		t.Fatalf("October's lines: %+v", v.Lines)
	}
	l := v.Lines[0]
	// September left 300 of 1.000; October spends 1.000 and 150.
	if l.ID != free.String() || l.Name == nil || *l.Name != "Free to spend" || l.CarriedCents != 30000 || l.SpentCents != 115000 || l.LeftCents != 15000 || l.Count != 3 {
		t.Errorf("the line: %+v", l)
	}
	if len(l.ByCategory) != 3 || l.ByCategory[0].Category != shop.String() || l.ByCategory[0].SpentCents != 100000 || l.ByCategory[0].Count != 2 || l.ByCategory[2].SpentCents != 0 {
		t.Errorf("by category: %+v", l.ByCategory)
	}
	// The six months before are its three categories' together.
	if l.History.History[5].SpentCents != 70000 {
		t.Errorf("September, like with like: %+v", l.History.History)
	}
	if len(v.Unlimited) != 1 || *v.Unlimited[0].Category != w.cats["Groceries"].String() {
		t.Errorf("unlimited: %+v", v.Unlimited)
	}
	sv, _ := w.budget.Month(ctx, sep)
	if len(sv.Lines) != 2 || category(t, sv, bar).LimitCents != 20000 {
		t.Errorf("September keeps Restaurants' own limit: %+v", sv.Lines)
	}

	// Copying September back gives Restaurants its limit again, and the
	// line keeps its id.
	if _, err := w.budget.CopyLimits(ctx, oct, sep); err != nil {
		t.Fatal(err)
	}
	v, _ = w.budget.Month(ctx, oct)
	if l := category(t, v, fun); l.ID != free.String() || len(l.ByCategory) != 2 || category(t, v, bar).LimitCents != 20000 {
		t.Errorf("after copying September: %+v", v.Lines)
	}

	if err := w.budget.RemoveLine(ctx, oct, free); err != nil {
		t.Fatal(err)
	}
	if err := w.budget.RemoveLine(ctx, oct, free); !errors.Is(err, ErrNoLine) {
		t.Errorf("removing it twice: %v, want ErrNoLine", err)
	}
}

// The migration to lines turns each category's limit into a line of that
// one category, with the category's id, so rollover keeps walking from the
// months saved before it; rolled back, a shared line keeps one category.
func TestLinesFromCategoryLimits(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)
	const before = 20261109100100
	if err := migrations.DownTo(ctx, pool, before); err != nil {
		t.Fatal(err)
	}
	food, fun := "6f1c9a52-3d2e-4b7a-9c1d-0a1b2c3d4e5f", "0b7e4c1d-8a9f-4e2b-b3c4-d5e6f7a8b9c0"
	if _, err := pool.Exec(ctx, `
		INSERT INTO budget.months (month, payday, pyf_cents, updated_at) VALUES
		    ('2026-08-01', 5, 200000, now()), ('2026-10-01', 5, 200000, now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO budget.lines (month, category_id, limit_cents, rollover) VALUES
		    ('2026-08-01', $1, 50000, true), ('2026-08-01', $2, 20000, false),
		    ('2026-10-01', $1, 60000, true)`, food, fun); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(pool)
	saved, err := repo.Saved(ctx, month(t, "2026-10"))
	if err != nil || len(saved) != 2 {
		t.Fatalf("saved %d, %v", len(saved), err)
	}
	f, ok := saved[0].LineOf(cat(t, food))
	if !ok || f.ID.String() != food || f.Name != "" || len(f.Categories) != 1 || f.Limit.Cents() != 50000 || !f.Rollover {
		t.Errorf("August's food: %+v", f)
	}
	if o, _ := saved[1].LineOf(cat(t, food)); o.ID != f.ID || len(saved[1].Lines()) != 1 {
		t.Errorf("October's food keeps the id that carries: %+v", saved[1].Lines())
	}
	spending := spend(t, map[string]map[string]int64{"2026-08": {food: 40000}, "2026-09": {food: 45000}})
	if got := find(t, Track(saved, spending, nil, month(t, "2026-10"), now, nil), food).Carried.Cents(); got != 15000 {
		t.Errorf("October carried %d, want 15000", got)
	}

	// Rolled back, a shared line keeps the first of its categories.
	oct := saved[1]
	if _, err := oct.AddLine(Line{Name: "Free", Categories: cats(cat(t, fun), cat(t, food)), Limit: money.BRLCents(90000)}, now); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, oct); err != nil {
		t.Fatal(err)
	}
	if err := migrations.DownTo(ctx, pool, before); err != nil {
		t.Fatal(err)
	}
	var got string
	var cents int64
	if err := pool.QueryRow(ctx, `SELECT category_id::text, limit_cents FROM budget.lines WHERE month = '2026-10-01'`).Scan(&got, &cents); err != nil {
		t.Fatal(err)
	}
	if got != fun || cents != 90000 {
		t.Errorf("rolled back: %s %d, want %s 90000", got, cents, fun)
	}
}

// The line endpoints, through the budget's routes: a shared line is made,
// changed and removed by its id, and a category in it has no limit of its
// own to change.
func TestLineRoutes(t *testing.T) {
	w := newWorld(t)
	mux := http.NewServeMux()
	w.budget.RegisterRoutes(mux, slog.New(slog.NewTextHandler(io.Discard, nil)))
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}
	fun, shop, bar := w.cats["Leisure"].String(), w.cats["Shopping"].String(), w.cats["Restaurants"].String()

	rec := do(http.MethodPost, "/api/v1/budget/2026-10/lines", `{"name": "Free to spend", "categories": ["`+fun+`", "`+shop+`"], "limitCents": 100000, "rollover": true}`)
	var made struct{ ID string }
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &made) != nil || made.ID == "" {
		t.Fatalf("POST lines: %d %s", rec.Code, rec.Body.String())
	}
	line := "/api/v1/budget/2026-10/lines/" + made.ID
	if rec := do(http.MethodPut, "/api/v1/budget/2026-10/limits/"+fun, `{"limitCents": 1, "rollover": false}`); rec.Code != http.StatusConflict {
		t.Errorf("Leisure's own limit: %d %s, want 409", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPut, line, `{"name": "Free", "categories": ["`+fun+`", "`+shop+`", "`+bar+`"], "limitCents": 120000}`); rec.Code != http.StatusNoContent {
		t.Errorf("PUT line: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPut, "/api/v1/budget/2026-10/lines/0b7e4c1d-8a9f-4e2b-b3c4-d5e6f7a8b9c0", `{"categories": ["`+bar+`"], "limitCents": 1}`); rec.Code != http.StatusNotFound {
		t.Errorf("PUT a line the month lacks: %d, want 404", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/budget/2026-10/lines", `{"categories": ["`+fun+`", "`+bar+`"], "limitCents": 1}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "needs a name") {
		t.Errorf("a shared line without a name: %d %s", rec.Code, rec.Body.String())
	}

	v, err := w.budget.Month(context.Background(), month(t, "2026-10"))
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Lines) != 1 || v.Lines[0].ID != made.ID || *v.Lines[0].Name != "Free" || len(v.Lines[0].ByCategory) != 3 || v.LimitsCents != 120000 {
		t.Errorf("October: %+v", v.Lines)
	}

	// Every line at once, in the shape that takes several categories.
	body := `{"limits": [{"id": "` + made.ID + `", "name": "Free", "categories": ["` + fun + `", "` + shop + `"], "limitCents": 90000}, {"category": "` + bar + `", "limitCents": 20000}]}`
	if rec := do(http.MethodPut, "/api/v1/budget/2026-10/limits", body); rec.Code != http.StatusNoContent {
		t.Errorf("PUT limits: %d %s", rec.Code, rec.Body.String())
	}
	v, _ = w.budget.Month(context.Background(), month(t, "2026-10"))
	if len(v.Lines) != 2 || category(t, v, w.cats["Leisure"]).ID != made.ID || category(t, v, w.cats["Restaurants"]).LimitCents != 20000 {
		t.Errorf("after PUT limits: %+v", v.Lines)
	}

	if rec := do(http.MethodDelete, line, ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE line: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodDelete, line, ""); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE it again: %d, want 404", rec.Code)
	}
}

// ADR-0011's month: a car fund of 300 kept in a caixinha pays a repair of
// 1.600 redeemed from it, and pay yourself first is still done; the fund is
// in the red until an adjustment covers it from money already invested.
func TestFund(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	w.load(t, map[string]string{"SALARIO": "Income", "OFICINA": "Transport", "MERCADO": "Groceries"}, []tx{
		{"2026-10-01", "SALARIO ACME", 1000000, ledger.KindIncome},
		{"2026-10-01", "APLICACAO RDB", -30000, ledger.KindTransfer},
		{"2026-10-02", "COMPRA DE RENDA VARIAVEL", -400000, ledger.KindTransfer},
		{"2026-10-09", "RESGATE RDB", 160000, ledger.KindTransfer},
		{"2026-10-10", "OFICINA DO CARRO", -160000, ""},
		{"2026-10-12", "MERCADO BOM", -50000, ""},
	})
	oct := month(t, "2026-10")
	payday, _ := NewPayday(1)
	if err := w.budget.SetPayYourselfFirst(ctx, oct, money.BRLCents(400000), payday); err != nil {
		t.Fatal(err)
	}
	car, err := w.budget.AddLine(ctx, oct, Line{Categories: cats(w.cats["Transport"]), Limit: money.BRLCents(30000), Kind: KindFund})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.budget.SetLimit(ctx, oct, w.cats["Groceries"], money.BRLCents(80000), false); err != nil {
		t.Fatal(err)
	}

	v, err := w.budget.Month(ctx, oct)
	if err != nil {
		t.Fatal(err)
	}
	if v.PayYourselfFirst.MovedCents != 400000 || v.PayYourselfFirst.DoneOn == nil || v.Summary.ContributionCents != 400000 {
		t.Errorf("pay yourself first %+v: the repair came from the fund, not from it", v.PayYourselfFirst)
	}
	c := category(t, v, w.cats["Transport"])
	if !c.Fund || c.Rollover || c.LeftCents != -130000 || c.Pace != PaceOver {
		t.Errorf("the car fund: %+v", c)
	}
	if f := v.Funds; f.Count != 1 || f.AmountCents != 30000 || f.SpentCents != 160000 || f.BalanceCents != -130000 || f.KeptCents != -130000 {
		t.Errorf("funds %+v", f)
	}
	if v.AvailableCents != 80000 || v.SpentCents != 50000 || v.LimitsCents != 110000 || v.NotAssignedCents != 1000000-400000-110000 {
		t.Errorf("the month's own totals leave the fund out, Not assigned does not: %+v", v)
	}
	alerts, err := w.budget.Alerts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 || alerts[0].Kind != AlertOverdrawn || alerts[0].Title != "Transport is R$ 1.300,00 in the red" {
		t.Errorf("alerts %+v", alerts)
	}

	// What was already saved for the car covers it, without touching pay
	// yourself first.
	if err := w.budget.Adjust(ctx, oct, car, money.BRLCents(200000)); err != nil {
		t.Fatal(err)
	}
	v, _ = w.budget.Month(ctx, oct)
	if c := category(t, v, w.cats["Transport"]); c.AdjustedCents != 200000 || c.LeftCents != 70000 || c.Pace != PaceOn || v.PayYourselfFirst.MovedCents != 400000 {
		t.Errorf("after the adjustment: %+v, moved %d", c, v.PayYourselfFirst.MovedCents)
	}
	nov, _ := w.budget.Month(ctx, month(t, "2026-11"))
	if c := category(t, nov, w.cats["Transport"]); c.CarriedCents != 70000 || c.LeftCents != 100000 {
		t.Errorf("November carries the balance: %+v", c)
	}
	groceries, _ := ParseLineID(category(t, v, w.cats["Groceries"]).ID)
	if err := w.budget.Adjust(ctx, oct, groceries, money.BRLCents(100)); !errors.Is(err, ErrInvalid) {
		t.Errorf("adjusting a limit: %v, want ErrInvalid", err)
	}
	if err := w.budget.RemoveAdjustment(ctx, oct, car); err != nil {
		t.Fatal(err)
	}
	if err := w.budget.RemoveAdjustment(ctx, oct, car); !errors.Is(err, ErrNoAdjustment) {
		t.Errorf("removing it twice: %v, want ErrNoAdjustment", err)
	}
}

func TestFundRoutes(t *testing.T) {
	w := newWorld(t)
	mux := http.NewServeMux()
	w.budget.RegisterRoutes(mux, slog.New(slog.NewTextHandler(io.Discard, nil)))
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}
	moi, food := w.cats["Leisure"].String(), w.cats["Groceries"].String()
	rec := do(http.MethodPut, "/api/v1/budget/2026-10/limits", `{"limits": [{"category": "`+moi+`", "limitCents": 50000, "fund": true}, {"category": "`+food+`", "limitCents": 80000}]}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT limits with a fund: %d %s", rec.Code, rec.Body.String())
	}
	v, _ := w.budget.Month(context.Background(), month(t, "2026-10"))
	fund, limit := category(t, v, w.cats["Leisure"]), category(t, v, w.cats["Groceries"])
	if !fund.Fund || limit.Fund {
		t.Fatalf("kinds: %+v %+v", fund, limit)
	}
	adj := "/api/v1/budget/2026-10/lines/" + fund.ID + "/adjustment"
	if rec := do(http.MethodPut, adj, `{"amountCents": 100000}`); rec.Code != http.StatusNoContent {
		t.Errorf("PUT adjustment: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPut, "/api/v1/budget/2026-10/lines/"+limit.ID+"/adjustment", `{"amountCents": 100}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "only a fund") {
		t.Errorf("adjusting a limit: %d %s", rec.Code, rec.Body.String())
	}
	// The category's own limit changes the amount and keeps the fund.
	if rec := do(http.MethodPut, "/api/v1/budget/2026-10/limits/"+moi, `{"limitCents": 60000, "rollover": false}`); rec.Code != http.StatusNoContent {
		t.Errorf("PUT a fund's own limit: %d %s", rec.Code, rec.Body.String())
	}
	v, _ = w.budget.Month(context.Background(), month(t, "2026-10"))
	if f := category(t, v, w.cats["Leisure"]); !f.Fund || f.LimitCents != 60000 || f.AdjustedCents != 100000 {
		t.Errorf("after its limit changed: %+v", f)
	}
	if rec := do(http.MethodDelete, adj, ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE adjustment: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodDelete, adj, ""); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE it again: %d, want 404", rec.Code)
	}
}
