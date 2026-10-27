package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"time"

	"lastro/app"
	"lastro/modules/budget"
	"lastro/modules/categorize"
	"lastro/modules/ledger"
	"lastro/money"
	"lastro/platform/config"
)

// runSeed fills a dev database with fourteen months of made-up money, through
// the ledger's own commands, so every screen has something to show. It
// refuses to run outside LASTRO_ENV=dev. Running it again changes nothing.
func runSeed(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	months := fs.Int("months", 14, "how many months of history, ending today")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.IsDev() {
		return errors.New("seed: only with LASTRO_ENV=dev; it writes made-up transactions")
	}
	a, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer a.Close()

	now := time.Now()
	s := newSeeder(now, *months)
	s.plan()
	for _, acc := range s.accounts {
		if _, err := a.Ledger.SyncAccount(ctx, acc.details); err != nil {
			return fmt.Errorf("seed: %w", err)
		}
		b := ledger.ImportBatch{Account: acc.details.ExternalID, From: s.start, Transactions: acc.txs}
		res, err := a.Ledger.ImportTransactions(ctx, b)
		if err != nil {
			return fmt.Errorf("seed: %w", err)
		}
		fmt.Printf("%s %s: %d new, %d updated, %d unchanged\n", acc.details.Institution, acc.details.Name, res.Created, res.Updated, res.Unchanged)
	}
	n, err := a.Ledger.MatchTransfers(ctx, time.Time{})
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	fmt.Printf("Paired %d transfers and card bill payments\n", n)
	if err := seedRules(ctx, a); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	if err := seedBudget(ctx, a, s.start, now); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	return nil
}

// seedRules teaches the rules a few weeks of corrections would, leaving
// some merchants for the review queue.
func seedRules(ctx context.Context, a *app.App) error {
	cats, err := a.Categorize.Categories(ctx)
	if err != nil {
		return err
	}
	byName := map[string]ledger.CategoryID{}
	for _, c := range cats {
		byName[c.Name] = c.ID
	}
	rules := []struct{ text, category string }{
		{"IFOOD", "Restaurants"}, {"UBER EATS", "Restaurants"}, {"OUTBACK STEAKHOUSE", "Restaurants"},
		{"UBER TRIP", "Transport"}, {"POSTO IPIRANGA", "Transport"},
		{"PAO DE ACUCAR", "Groceries"},
		{"ALUGUEL", "Housing"}, {"CONDOMINIO", "Housing"}, {"CLARO INTERNET", "Housing"}, {"ENEL ENERGIA", "Housing"},
		{"SMART FIT", "Health"},
		{"NETFLIX COM", "Leisure"}, {"SPOTIFY", "Leisure"},
		{"AMAZON MARKETPLACE", "Shopping"}, {"FAST SHOP", "Shopping"}, {"MAGAZINE LUIZA", "Shopping"},
		{"SALARIO", "Income"},
	}
	for _, r := range rules {
		p, err := categorize.NewPattern(categorize.PatternContains, r.text)
		if err != nil {
			return err
		}
		c, ok := byName[r.category]
		if !ok {
			continue // renamed in this database
		}
		if err := a.Categorize.Learn(ctx, p, "", c); err != nil {
			return err
		}
	}
	n, err := a.Categorize.CategorizeUncategorized(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Taught %d rules, which categorized %d transactions\n", len(rules), n)
	return nil
}

// seedBudget saves a budget five months back, with pay yourself first and
// the usual limits, and raises one limit two months ago, so months inherit
// and roll over.
func seedBudget(ctx context.Context, a *app.App, start, now time.Time) error {
	cats, err := a.Categorize.Categories(ctx)
	if err != nil {
		return err
	}
	first := budget.MonthOf(now).Add(-5)
	if first.Before(budget.MonthOf(start)) {
		first = budget.MonthOf(start)
	}
	payday, _ := budget.NewPayday(5)
	if err := a.Budget.SetPayYourselfFirst(ctx, first, money.BRLCents(300000), payday); err != nil {
		return err
	}
	limits := map[string]struct {
		cents    int64
		rollover bool
	}{
		"Housing": {420000, false}, "Groceries": {80000, true}, "Restaurants": {70000, true}, "Transport": {70000, false},
		"Health": {15000, false}, "Leisure": {15000, true}, "Shopping": {50000, false}, "Other": {10000, false},
	}
	for _, c := range cats {
		l, ok := limits[c.Name]
		if !ok || c.Archived || !c.Budgeted {
			continue
		}
		if err := a.Budget.SetLimit(ctx, first, c.ID, money.BRLCents(l.cents), l.rollover); err != nil {
			return err
		}
	}
	for _, c := range cats {
		if c.Name == "Restaurants" && !c.Archived {
			if err := a.Budget.SetLimit(ctx, budget.MonthOf(now).Add(-2), c.ID, money.BRLCents(75000), true); err != nil {
				return err
			}
		}
	}
	fmt.Printf("Saved a budget from %s\n", first)
	return nil
}

type seedAccount struct {
	details ledger.AccountDetails
	txs     []ledger.ImportedTransaction
	n       int
}

type seeder struct {
	now, start                  time.Time
	r                           *rand.Rand
	accounts                    []*seedAccount
	checking, card, itau, inter *seedAccount
	cardByMonth                 map[string]int64 // card spending by purchase month, for the bills
}

func newSeeder(now time.Time, months int) *seeder {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -(months - 1), 0)
	s := &seeder{now: now, start: start, r: rand.New(rand.NewPCG(2026, 10)), cardByMonth: map[string]int64{}}
	acc := func(ext, name, inst string, typ ledger.AccountType, number string, balance int64) *seedAccount {
		id, _ := ledger.NewExternalID(ext)
		a := &seedAccount{details: ledger.AccountDetails{
			ExternalID: id, Name: name, Institution: inst, Type: typ, Number: number,
			Balance: money.BRLCents(balance), BalanceAt: now,
		}}
		s.accounts = append(s.accounts, a)
		return a
	}
	s.checking = acc("seed-nubank-checking", "Conta", "Nubank", ledger.AccountChecking, "0001/9876543-2", 321055)
	s.card = acc("seed-nubank-card", "Ultravioleta", "Nubank", ledger.AccountCreditCard, "**** 4321", -153020)
	s.itau = acc("seed-itau-checking", "Conta Corrente", "Itaú", ledger.AccountChecking, "1234/56789-0", 81240)
	s.inter = acc("seed-inter-checking", "Conta Digital", "Banco Inter", ledger.AccountChecking, "0001/1234567-8", 976353)
	limit, available := money.BRLCents(1500000), money.BRLCents(1500000-153020)
	due := time.Date(now.Year(), now.Month(), 10, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	if c, err := ledger.NewCardDetails(&limit, &available, due, due.AddDate(0, 0, -7)); err == nil {
		s.card.details.Card = c
	}
	return s
}

func (s *seeder) add(a *seedAccount, day time.Time, description string, cents int64, kind ledger.Kind, inst ledger.Installment) {
	if day.After(s.now) {
		return
	}
	a.n++
	ext, _ := ledger.NewExternalID(fmt.Sprintf("%s-%s-%d", a.details.ExternalID, day.Format("20060102"), a.n))
	status := ledger.StatusPosted
	if a == s.card && s.now.Sub(day) < 72*time.Hour {
		status = ledger.StatusPending
	}
	a.txs = append(a.txs, ledger.ImportedTransaction{
		ExternalID: ext, BankRef: "ref-" + ext.String(), Date: day, Description: description,
		Amount: money.BRLCents(cents), Kind: kind, Status: status, Installment: inst,
	})
	if a == s.card && cents < 0 && kind == ledger.KindExpense {
		s.cardByMonth[day.Format("2006-01")] -= cents
	}
}

// between is a whole number of cents in [lo, hi] reais.
func (s *seeder) between(lo, hi int) int64 { return int64(lo*100 + s.r.IntN((hi-lo)*100+1)) }

func (s *seeder) day(month time.Time, d int) time.Time {
	last := month.AddDate(0, 1, -1).Day()
	return month.AddDate(0, 0, min(d, last)-1)
}

func (s *seeder) times(month time.Time, n int, f func(day time.Time)) {
	last := month.AddDate(0, 1, -1).Day()
	for range n {
		f(s.day(month, 1+s.r.IntN(last)))
	}
}

func (s *seeder) plan() {
	none := ledger.Installment{}
	exp, inc, xfer := ledger.KindExpense, ledger.KindIncome, ledger.KindTransfer
	for m := s.start; !m.After(s.now); m = m.AddDate(0, 1, 0) {
		i := int(m.Sub(s.start).Hours() / 24 / 30)
		// Payday: the salary lands at Itaú and most of it moves to Nubank.
		s.add(s.itau, s.day(m, 5), "SALARIO ACME TECNOLOGIA LTDA", 1288000, inc, none)
		s.add(s.itau, s.day(m, 5), "PIX ENVIADO NU PAGAMENTOS", -900000, exp, none)
		s.add(s.checking, s.day(m, 5), "PIX RECEBIDO ITAU UNIBANCO", 900000, inc, none)
		// Pay yourself first, to the broker: a transfer to my own investments.
		s.add(s.checking, s.day(m, 6), "APLICACAO XP INVESTIMENTOS", -300000, xfer, none)
		// Fixed costs.
		s.add(s.checking, s.day(m, 5), "PIX ENVIADO JOAO IMOVEIS ALUGUEL", -320000, exp, none)
		s.add(s.checking, s.day(m, 5), "CONDOMINIO EDIFICIO SOLAR", -64000, exp, none)
		s.add(s.checking, s.day(m, 12), "CLARO INTERNET", -10990, exp, none)
		s.add(s.itau, s.day(m, 20), "ENEL ENERGIA SP", -s.between(140, 260), exp, none)
		s.add(s.card, s.day(m, 15), "SMART FIT", -11990, exp, none)
		s.add(s.card, s.day(m, 18), "NETFLIX.COM", -4490, exp, none)
		spotify := int64(2190)
		if !m.Before(s.now.AddDate(0, -1, 0)) {
			spotify = 2390
		}
		s.add(s.card, s.day(m, 21), "SPOTIFY", -spotify, exp, none)
		if i >= 8 {
			s.add(s.card, s.day(m, 9), "DUOLINGO", -3490, exp, none)
		}
		// Everyday spending on the card.
		s.times(m, 8+s.r.IntN(4), func(d time.Time) {
			s.add(s.card, d, []string{"PAG*IFOOD SP", "IFOOD *IFOOD", "PG *IFOOD"}[s.r.IntN(3)], -s.between(28, 82), exp, none)
		})
		s.times(m, 7+s.r.IntN(6), func(d time.Time) { s.add(s.card, d, "UBER *TRIP HELP.UBER.COM", -s.between(14, 48), exp, none) })
		s.times(m, 2+s.r.IntN(3), func(d time.Time) { s.add(s.card, d, "UBER *EATS PENDING", -s.between(25, 70), exp, none) })
		s.times(m, 3+s.r.IntN(3), func(d time.Time) { s.add(s.card, d, "PAO DE ACUCAR 1234", -s.between(90, 320), exp, none) })
		s.times(m, 2+s.r.IntN(3), func(d time.Time) { s.add(s.card, d, "MERCADO PAGO *FEIRA", -s.between(35, 95), exp, none) })
		s.times(m, 1+s.r.IntN(2), func(d time.Time) { s.add(s.card, d, "DROGASIL 2104 SAO PAULO", -s.between(30, 140), exp, none) })
		s.times(m, 2, func(d time.Time) { s.add(s.card, d, "POSTO IPIRANGA", -s.between(150, 260), exp, none) })
		s.times(m, s.r.IntN(3), func(d time.Time) { s.add(s.card, d, "AMAZON MARKETPLACE", -s.between(60, 320), exp, none) })
		s.times(m, s.r.IntN(2), func(d time.Time) { s.add(s.card, d, "CINEMARK", -s.between(50, 110), exp, none) })
		s.times(m, s.r.IntN(2), func(d time.Time) { s.add(s.card, d, "OUTBACK STEAKHOUSE", -s.between(150, 280), exp, none) })
		if s.r.IntN(3) == 0 {
			s.add(s.checking, s.day(m, 24), "PIX RECEBIDO ANA L", 48000, inc, none)
		}
		if s.r.IntN(4) == 0 {
			s.add(s.card, s.day(m, 22), "ESTORNO NETFLIX.COM", 4490, exp, none)
		}
	}
	// Two purchases in installments: the bank lists each one in its month.
	s.installments("FAST SHOP", 319900, 10, s.now.AddDate(0, -2, 0))
	s.installments("MAGAZINE LUIZA", 179400, 6, s.start.AddDate(0, 3, 0))
	// Each month's card bill is paid from Nubank checking on the 10th of the next.
	for m := s.start; !m.After(s.now); m = m.AddDate(0, 1, 0) {
		total := s.cardByMonth[m.Format("2006-01")]
		pay := s.day(m.AddDate(0, 1, 0), 10)
		if total == 0 || pay.After(s.now) {
			continue
		}
		s.add(s.checking, pay, "PAGAMENTO DE FATURA", -total, ledger.KindExpense, ledger.Installment{})
		s.add(s.card, pay, "PAGAMENTO RECEBIDO", total, ledger.KindBillPayment, ledger.Installment{})
	}
}

func (s *seeder) installments(description string, totalCents int64, n int, first time.Time) {
	each := totalCents / int64(n)
	for k := 1; k <= n; k++ {
		inst, err := ledger.NewInstallment(k, n)
		if err != nil {
			continue
		}
		d := time.Date(first.Year(), first.Month(), 14, 0, 0, 0, 0, time.UTC).AddDate(0, k-1, 0)
		s.add(s.card, d, description, -each, ledger.KindExpense, inst)
	}
}
