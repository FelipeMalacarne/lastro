package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"lastro/app"
	"lastro/modules/budget"
	"lastro/modules/categorize"
	"lastro/modules/ledger"
	"lastro/modules/market"
	"lastro/modules/portfolio"
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
	if err := seedPortfolio(ctx, a, s.start, now); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	return nil
}

// seedPortfolio invests R$ 3.000 every month since start, split by the
// planner itself, with made-up monthly returns per class, and values each
// month's end, so the portfolio and net worth have a history. Two stock
// picks get a thesis and the schedule a few purchases.
func seedPortfolio(ctx context.Context, a *app.App, start, now time.Time) error {
	r := rand.New(rand.NewPCG(7, 2026))
	targets, err := a.Portfolio.Targets(ctx)
	if err != nil {
		return err
	}
	type holding struct {
		id, item, inst, name string
		kind                 portfolio.Kind
		class                portfolio.Class
		ticker               string
		idx                  portfolio.Indexer
		rate                 int64
		price                float64 // per unit, for stocks and ETFs
		qty                  float64
		value, basis         int64
	}
	hs := []*holding{
		{id: "seed-cdb", item: "seed-nubank", inst: "Nubank", name: "CDB Nubank 100% CDI", kind: portfolio.KindFixedIncome, class: portfolio.ClassCDI, idx: portfolio.IndexerCDI, rate: 10000},
		{id: "seed-ipca", item: "seed-xp", inst: "XP Investimentos", name: "Tesouro IPCA+ 2035", kind: portfolio.KindTreasury, class: portfolio.ClassIPCA, idx: portfolio.IndexerIPCA, rate: 640},
		{id: "seed-ivvb", item: "seed-xp", inst: "XP Investimentos", name: "iShares S&P 500", kind: portfolio.KindETF, class: portfolio.ClassGlobal, ticker: "IVVB11", price: 310},
		{id: "seed-wege", item: "seed-xp", inst: "XP Investimentos", name: "WEG ON", kind: portfolio.KindStock, class: portfolio.ClassBR, ticker: "WEGE3", price: 38},
		{id: "seed-itub", item: "seed-xp", inst: "XP Investimentos", name: "Itaú Unibanco PN", kind: portfolio.KindStock, class: portfolio.ClassBR, ticker: "ITUB4", price: 31},
	}
	// A head start, as if investing began before lastro.
	for _, h := range hs {
		h.value = map[portfolio.Class]int64{portfolio.ClassCDI: 1800000, portfolio.ClassIPCA: 1500000, portfolio.ClassGlobal: 2600000, portfolio.ClassBR: 450000}[h.class]
		if h.price > 0 {
			h.qty = float64(h.value) / 100 / h.price
		}
		h.basis = h.value
	}
	// Another lot of the same CDB, as each Caixinha contribution is: one
	// group, with R$ 15.000 of it set apart as the emergency fund.
	reserve := &holding{id: "seed-reserva", item: "seed-nubank", inst: "Nubank", name: "CDB Nubank 100% CDI", kind: portfolio.KindFixedIncome, class: portfolio.ClassCDI, idx: portfolio.IndexerCDI, rate: 10000, value: 1500000, basis: 1500000}
	monthly := map[portfolio.Class][2]float64{ // mean and spread of a month's return
		portfolio.ClassCDI: {0.0095, 0}, portfolio.ClassIPCA: {0.009, 0.006}, portfolio.ClassGlobal: {0.013, 0.035}, portfolio.ClassBR: {0.008, 0.05},
	}
	report := func(h *holding, asOf time.Time) portfolio.Report {
		rep := portfolio.Report{
			ExternalID: h.id, Item: h.item, Institution: h.inst, Kind: h.kind, Name: h.name, Indexer: h.idx,
			Rate: money.FromBps(h.rate), Value: money.BRLCents(h.value), AsOf: asOf, Active: true,
		}
		if h.ticker != "" {
			rep.Ticker, _ = portfolio.NewTicker(h.ticker)
			rep.Quantity, _ = portfolio.ParseQuantity(fmt.Sprintf("%.6f", h.qty))
		} else {
			b := money.BRLCents(h.basis)
			rep.Invested = &b
		}
		if h.kind == portfolio.KindTreasury {
			m := time.Date(2035, 5, 15, 0, 0, 0, 0, time.UTC)
			rep.Maturity = &m
		}
		return rep
	}
	for m := start; !m.After(now); m = m.AddDate(0, 1, 0) {
		day := m.AddDate(0, 1, -1)
		if day.After(now) {
			day = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		}
		// The month's returns.
		for _, h := range hs {
			mr := monthly[h.class]
			ret := mr[0] + (r.Float64()*2-1)*mr[1]
			if h.price > 0 {
				h.price *= 1 + ret
				h.value = int64(h.qty * h.price * 100)
			} else {
				h.value = int64(float64(h.value) * (1 + ret))
			}
		}
		reserve.value = int64(float64(reserve.value) * 1.0095)
		// The month's R$ 3.000, where the planner says.
		values := map[portfolio.Class]money.Money{}
		for _, h := range hs {
			values[h.class] = money.BRLCents(values[h.class].Cents() + h.value)
		}
		plan, err := portfolio.PlanContribution(values, targets.Allocation, targets.Band, money.BRLCents(300000))
		if err != nil {
			return err
		}
		for _, sp := range plan.Splits {
			// The class's first holding takes it; in Brazilian stocks, alternate.
			var pick *holding
			for _, h := range hs {
				if h.class == sp.Class && (pick == nil || (sp.Class == portfolio.ClassBR && m.Month()%2 == 0)) {
					pick = h
				}
			}
			if pick == nil || sp.Amount.IsZero() {
				continue
			}
			pick.value += sp.Amount.Cents()
			pick.basis += sp.Amount.Cents()
			if pick.price > 0 {
				pick.qty += float64(sp.Amount.Cents()) / 100 / pick.price
			}
		}
		for _, item := range []string{"seed-nubank", "seed-xp"} {
			var reps []portfolio.Report
			for _, h := range append(hs, reserve) {
				if h.item == item {
					reps = append(reps, report(h, day))
				}
			}
			if _, err := a.Portfolio.SyncPositions(ctx, item, reps); err != nil {
				return err
			}
		}
		fund, err := portfolio.AmountReserve(money.BRLCents(1500000))
		if err != nil {
			return err
		}
		if err := a.Portfolio.UpdateGroup(ctx, portfolio.GroupID(report(reserve, day)), nil, &fund); err != nil {
			return err
		}
		if _, err := a.Portfolio.Valuate(ctx, day); err != nil {
			return err
		}
	}

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	prices := map[string]float64{}
	for _, h := range hs {
		if h.ticker != "" {
			prices[h.ticker] = h.price
		}
	}
	if err := seedMarket(ctx, a, today, prices); err != nil {
		return err
	}
	wege, itub := today.AddDate(0, 0, 2), today.AddDate(0, 0, 9)
	if err := a.Portfolio.SaveThesis(ctx, "WEGE3", "Compounds at high returns on capital as electrification spreads, at home and abroad.", "Return on capital below 20% for two years, or a large acquisition paid in debt.", &wege); err != nil {
		return err
	}
	if err := a.Portfolio.SaveThesis(ctx, "ITUB4", "The most profitable large bank, paying out most of what it earns.", "Return on equity below 18%, or bad loans above 4% of the book.", &itub); err != nil {
		return err
	}
	if ps, err := a.Portfolio.Purchases(ctx); err == nil && len(ps) == 0 {
		next := time.Date(now.Year(), now.Month(), 5, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
		planned := []struct {
			on        time.Time
			what      string
			class     portfolio.Class
			cents     int64
			condition string
			decide    portfolio.PurchaseStatus
			note      string
		}{
			{next, "Tesouro IPCA+ 2035", portfolio.ClassIPCA, 120000, "On payday", "", ""},
			{next, "CDB Nubank 100% CDI", portfolio.ClassCDI, 100000, "On payday", "", ""},
			{wege, "WEGE3", portfolio.ClassBR, 40000, "Only after the earnings report, and only if the thesis still holds", "", ""},
			{today.AddDate(0, -2, 8), "ITUB4", portfolio.ClassBR, 50000, "After the report", portfolio.Done, ""},
			{today.AddDate(0, -1, -3), "BOVA11", portfolio.ClassBR, 30000, "If the index falls 5%", portfolio.Cancelled, "It did not fall."},
		}
		for _, p := range planned {
			v, err := a.Portfolio.PlanPurchase(ctx, p.on, p.what, p.class, money.BRLCents(p.cents), p.condition)
			if err != nil {
				return err
			}
			if p.decide != "" {
				if err := a.Portfolio.DecidePurchase(ctx, v.ID, p.decide, p.note); err != nil {
					return err
				}
			}
		}
	}
	fmt.Println("Invested R$ 3.000 a month since", start.Format("Jan 2006"))
	return nil
}

func ptrTo[T any](v T) *T { return &v }

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

// seedMarket makes up a year of closes ending at each ticker's price (WEGE3
// falls 6% on the last day, so its price alert fires), dividends as each
// company pays them, and a monthly copy of earnings and book value per
// share. Brapi needs a token for these tickers; the dev data should not.
func seedMarket(ctx context.Context, a *app.App, today time.Time, prices map[string]float64) error {
	r := rand.New(rand.NewPCG(11, 2026))
	var quotes []market.Quote
	tickers := make([]string, 0, len(prices))
	for t := range prices {
		tickers = append(tickers, t)
	}
	slices.Sort(tickers)
	for _, ticker := range tickers {
		end := prices[ticker]
		var days []time.Time
		for d := today.AddDate(-1, 0, -10); !d.After(today); d = d.AddDate(0, 0, 1) {
			if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
				days = append(days, d)
			}
		}
		closes := make([]float64, len(days))
		p := end
		if ticker == "WEGE3" {
			p = end / 0.939 // the day before the fall
		}
		for i := len(days) - 1; i >= 0; i-- {
			closes[i] = p
			if i == len(days)-1 && ticker == "WEGE3" {
				closes[i] = end
				continue
			}
			p /= 1 + (r.Float64()*2-1)*0.018 + 0.0003
		}
		for i, d := range days {
			quotes = append(quotes, market.Quote{Symbol: ticker, Date: d, Close: money.BRLCents(int64(closes[i]*100 + 0.5)), Source: "seed"})
		}
	}
	var divs []market.Dividend
	var funds []market.Fundamental
	month := func(n int) time.Time {
		return time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, n, 0)
	}
	for n := -15; n <= 1; n++ {
		m := month(n)
		divs = append(divs, market.Dividend{Symbol: "ITUB4", Label: "JCP", ComDate: m.AddDate(0, 0, -1), PaymentDate: m.AddDate(0, 1, 0), RateMicro: 17_600})
		if n%6 == 0 {
			divs = append(divs, market.Dividend{Symbol: "ITUB4", Label: "DIVIDENDO", ComDate: m.AddDate(0, 0, 14), PaymentDate: m.AddDate(0, 2, 0), RateMicro: 1_250_000})
		}
		if n%3 == 0 {
			divs = append(divs, market.Dividend{Symbol: "WEGE3", Label: "JCP", ComDate: m.AddDate(0, 0, 20), PaymentDate: m.AddDate(0, 2, 10), RateMicro: 157_843})
		}
	}
	for n := -12; n <= 0; n++ {
		d := month(n).AddDate(0, 0, 14)
		if n == 0 {
			d = today
		}
		if d.After(today) {
			continue
		}
		g := float64(n+12) / 12
		funds = append(funds,
			market.Fundamental{Symbol: "WEGE3", Date: d, EPSMicro: ptrTo(int64(1_150_000 + 195_000*g)), BookMicro: ptrTo(int64(4_400_000 + 435_000*g))},
			market.Fundamental{Symbol: "ITUB4", Date: d, EPSMicro: ptrTo(int64(3_700_000 + 420_000*g)), BookMicro: ptrTo(int64(17_200_000 + 1_300_000*g))},
		)
	}
	if err := a.Market.Record(ctx, quotes, divs, funds); err != nil {
		return err
	}
	if err := a.Portfolio.SetPriceAlert(ctx, "WEGE3", 500); err != nil {
		return err
	}
	_, err := a.Portfolio.Valuate(ctx, today)
	return err
}
