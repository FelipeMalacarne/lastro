package portfolio

import (
	"context"
	"fmt"
	"sort"
	"time"

	"lastro/modules/market"
	"lastro/money"
)

func day(t time.Time) string { return t.Format(time.DateOnly) }

func ptr[T any](v T) *T { return &v }

// Overview is the Portfolio screen: today's valuation against the targets,
// the last twelve months from the stored valuations, the holdings and the
// theses.
func (s *Service) Overview(ctx context.Context) (OverviewView, error) {
	today := s.Today()
	t, err := s.Targets(ctx)
	if err != nil {
		return OverviewView{}, err
	}
	v, _, err := s.value(ctx, today)
	if err != nil {
		return OverviewView{}, fmt.Errorf("portfolio: overview: %w", err)
	}
	nw, err := s.netWorth(ctx, v)
	if err != nil {
		return OverviewView{}, fmt.Errorf("portfolio: overview: %w", err)
	}
	hist, err := s.repo.valuations(ctx, today.AddDate(-1, -1, 0))
	if err != nil {
		return OverviewView{}, fmt.Errorf("portfolio: overview: %w", err)
	}
	inv := v.Invested()
	out := OverviewView{
		InvestedCents: inv.Value.Cents(), BasisCents: inv.Basis.Cents(), BandBp: t.Band.Bps(),
		Targets:  map[Class]int64{},
		NetWorth: netWorthView(nw),
		Classes:  []ClassView{}, Growth: []GrowthPoint{}, Groups: []GroupView{}, Theses: []ThesisView{},
	}
	for c, p := range t.Allocation.Weights() {
		out.Targets[c] = p.Bps()
	}

	// Per day: each class and the invested total, from the stored history.
	type dayAmounts struct {
		classes  map[string]Amounts
		invested Amounts
	}
	byDay := map[time.Time]*dayAmounts{}
	var days []time.Time
	for _, h := range hist {
		d, ok := byDay[h.Date]
		if !ok {
			d = &dayAmounts{classes: map[string]Amounts{}, invested: Amounts{Value: money.BRLCents(0), Basis: money.BRLCents(0)}}
			byDay[h.Date] = d
			days = append(days, h.Date)
		}
		d.classes[h.Class] = h.Amounts
		if h.Class != "emergency" {
			d.invested = Amounts{Value: money.BRLCents(d.invested.Value.Cents() + h.Value.Cents()), Basis: money.BRLCents(d.invested.Basis.Cents() + h.Basis.Cents())}
		}
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	// on is the latest stored day at or before a day.
	on := func(at time.Time) (time.Time, bool) {
		var found time.Time
		for _, d := range days {
			if d.After(at) {
				break
			}
			found = d
		}
		return found, !found.IsZero()
	}
	// The twelve months: from a year ago, or from the first valuation.
	var start time.Time
	if d, ok := on(today.AddDate(-1, 0, 0)); ok {
		start = d
	} else if len(days) > 0 && days[0].Before(today) {
		start = days[0]
	}
	if !start.IsZero() {
		s0 := byDay[start].invested
		out.Since = ptr(day(start))
		out.PutIn12mCents = ptr(inv.Basis.Cents() - s0.Basis.Cents())
		out.Returns12mCents = ptr(inv.Value.Cents() - s0.Value.Cents() - *out.PutIn12mCents)
		if r, ok := Return(s0, inv); ok {
			out.Return12mBp = ptr(r.Bps())
		}
	}
	if d, ok := on(today.AddDate(0, -1, 0)); ok {
		// What the money earned since then, not what was put in.
		s := byDay[d].invested
		out.MonthDeltaCents = ptr(inv.Value.Cents() - s.Value.Cents() - (inv.Basis.Cents() - s.Basis.Cents()))
		out.MonthFrom = ptr(day(d))
	}

	// Each class against its target.
	counts := map[Class]int{}
	for _, g := range v.Groups {
		if g.Amounts.Value.Cents() > g.Emergency.Value.Cents() {
			counts[g.Group.Class()]++
		}
	}
	for _, c := range Classes {
		a := v.Classes[c]
		cv := ClassView{Class: c, ValueCents: a.Value.Cents(), BasisCents: a.Basis.Cents(), TargetBp: t.Allocation.Of(c).Bps(), Holdings: counts[c]}
		if inv.Value.Cents() > 0 {
			cv.NowBp = a.Value.Cents() * 10000 / inv.Value.Cents()
		}
		cv.Status = t.StatusOf(c, money.FromBps(cv.NowBp))
		if !start.IsZero() {
			if r, ok := Return(byDay[start].classes[string(c)], a); ok && byDay[start].classes[string(c)].Value.Cents() > 0 {
				cv.Return12mBp = ptr(r.Bps())
			}
		}
		out.Classes = append(out.Classes, cv)
	}

	// Growth: the last valuation of each month, and today.
	lastOfMonth := map[string]time.Time{}
	for _, d := range days {
		lastOfMonth[d.Format("2006-01")] = d
	}
	var months []string
	for m := range lastOfMonth {
		months = append(months, m)
	}
	sort.Strings(months)
	for _, m := range months {
		d := lastOfMonth[m]
		// From the start of the twelve months on, so the chart and the hero
		// count the same money.
		if m == today.Format("2006-01") || (!start.IsZero() && d.Before(start)) {
			continue
		}
		a := byDay[d].invested
		out.Growth = append(out.Growth, GrowthPoint{Date: day(d), ValueCents: a.Value.Cents(), BasisCents: a.Basis.Cents()})
	}
	out.Growth = append(out.Growth, GrowthPoint{Date: day(today), ValueCents: inv.Value.Cents(), BasisCents: inv.Basis.Cents()})

	// CDI and IPCA over the same months.
	if !start.IsZero() {
		out.Benchmarks.From = ptr(day(start))
		if r, last, err := s.market.Accumulated(ctx, market.CDI, start.AddDate(0, 0, 1), today); err == nil && !last.IsZero() {
			out.Benchmarks.CDIBp, out.Benchmarks.CDIUntil = ptr(r.Percent().Bps()), ptr(day(last))
		}
		// IPCA is monthly: the months after the start's.
		first := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
		if r, last, err := s.market.Accumulated(ctx, market.IPCA, first, today); err == nil && !last.IsZero() {
			out.Benchmarks.IPCABp, out.Benchmarks.IPCAUntil = ptr(r.Percent().Bps()), ptr(last.Format("2006-01"))
		}
	}

	// Groups, biggest first, and their lots, biggest first.
	where := map[string]bool{}
	var newest time.Time
	held := map[string]int64{}
	for _, g := range v.Groups {
		out.Groups = append(out.Groups, groupView(g))
		for _, h := range g.Holdings {
			where[h.Position.Institution] = true
			if h.At.After(newest) {
				newest = h.At
			}
			if !h.Position.Ticker.IsZero() {
				held[h.Position.Ticker.String()] += h.Value.Cents()
			}
		}
	}
	sort.SliceStable(out.Groups, func(i, j int) bool { return out.Groups[i].ValueCents > out.Groups[j].ValueCents })
	out.Positions = len(out.Groups)
	for w := range where {
		out.Where = append(out.Where, w)
	}
	sort.Strings(out.Where)
	if !newest.IsZero() {
		out.AsOf = ptr(day(newest))
	}

	theses, err := s.repo.theses(ctx)
	if err != nil {
		return OverviewView{}, fmt.Errorf("portfolio: overview: %w", err)
	}
	for _, th := range theses {
		tv := ThesisView{Ticker: th.Ticker.String(), Thesis: th.Thesis, Breaks: th.Breaks}
		if v, ok := held[tv.Ticker]; ok {
			tv.Held, tv.ValueCents = true, v
		}
		if th.NextReport != nil {
			tv.NextReport = ptr(day(*th.NextReport))
			if d, ok := th.ReportIn(today); ok {
				tv.ReportInDays = ptr(d)
			}
		}
		out.Theses = append(out.Theses, tv)
	}
	return out, nil
}

func groupView(g GroupValue) GroupView {
	p := g.Holdings[0].Position
	gv := GroupView{
		ID: g.Group.ID, Name: p.Name, Institution: p.Institution, Kind: p.Kind, Class: g.Group.Class(), AutoClass: g.Group.AutoClass(),
		Indexer: p.Indexer, RateBp: p.Rate.Bps(), Reserve: g.Group.Reserve.Mode(), ReserveCents: g.Group.Reserve.Amount().Cents(),
		EmergencyCents: g.Emergency.Value.Cents(), ShortCents: g.Group.Reserve.Short(g.Amounts.Value).Cents(),
		ValueCents: g.Amounts.Value.Cents(), BasisCents: g.Cost().Cents(), Lots: []LotView{},
	}
	if !p.Ticker.IsZero() {
		gv.Ticker = ptr(p.Ticker.String())
	}
	if b := gv.BasisCents; b > 0 {
		gv.ReturnBp = ptr((gv.ValueCents - b) * 10000 / b)
	}
	var newest time.Time
	for _, h := range g.Holdings {
		lp := h.Position
		lv := LotView{ID: lp.ExternalID, Quantity: lp.Quantity.String(), ValueCents: h.Value.Cents(), BasisCents: lp.Basis().Cents(), AsOf: day(h.At), Priced: h.Priced}
		if b := lp.Basis().Cents(); b > 0 {
			lv.ReturnBp = ptr((h.Value.Cents() - b) * 10000 / b)
		}
		if lp.Maturity != nil {
			m := day(*lp.Maturity)
			lv.Maturity = &m
			if gv.FirstMaturity == nil || m < *gv.FirstMaturity {
				gv.FirstMaturity = ptr(m)
			}
			if gv.LastMaturity == nil || m > *gv.LastMaturity {
				gv.LastMaturity = ptr(m)
			}
		}
		if h.At.After(newest) {
			newest = h.At
		}
		gv.Priced = gv.Priced || h.Priced
		gv.Lots = append(gv.Lots, lv)
	}
	gv.AsOf = day(newest)
	sort.SliceStable(gv.Lots, func(i, j int) bool { return gv.Lots[i].ValueCents > gv.Lots[j].ValueCents })
	return gv
}

func netWorthView(n NetWorth) NetWorthView {
	return NetWorthView{
		Date: day(n.Date), CashCents: n.Cash.Cents(), CardsCents: n.Cards.Cents(), InvestedCents: n.Invested.Cents(),
		EmergencyCents: n.Emergency.Cents(), OtherCents: n.Other.Cents(), BasisCents: n.Basis.Cents(), TotalCents: n.Total().Cents(),
	}
}

// NetWorthHistory is the stored net worth since a day, and today's as it
// stands now.
func (s *Service) NetWorthHistory(ctx context.Context, from time.Time) ([]NetWorthView, error) {
	rows, err := s.repo.netWorth(ctx, from)
	if err != nil {
		return nil, fmt.Errorf("portfolio: net worth: %w", err)
	}
	today := s.Today()
	out := make([]NetWorthView, 0, len(rows)+1)
	for _, r := range rows {
		if !r.Date.Equal(today) {
			out = append(out, netWorthView(r))
		}
	}
	v, _, err := s.value(ctx, today)
	if err != nil {
		return nil, fmt.Errorf("portfolio: net worth: %w", err)
	}
	nw, err := s.netWorth(ctx, v)
	if err != nil {
		return nil, fmt.Errorf("portfolio: net worth: %w", err)
	}
	return append(out, netWorthView(nw)), nil
}

// SaveThesis writes a stock's thesis, what would break it and its next report.
func (s *Service) SaveThesis(ctx context.Context, ticker, thesis, breaks string, next *time.Time) error {
	t, err := NewTicker(ticker)
	if err != nil {
		return err
	}
	th, err := NewThesis(t, thesis, breaks, next, s.now())
	if err != nil {
		return err
	}
	if err := s.repo.saveThesis(ctx, th); err != nil {
		return fmt.Errorf("portfolio: save thesis: %w", err)
	}
	return nil
}

func (s *Service) DeleteThesis(ctx context.Context, ticker string) error {
	t, err := NewTicker(ticker)
	if err != nil {
		return err
	}
	if err := s.repo.deleteThesis(ctx, t); err != nil {
		return fmt.Errorf("portfolio: delete thesis: %w", err)
	}
	return nil
}

// Reminders are the earnings reports in the next week for stocks held,
// each with its thesis (F8): read them before the report, not after.
func (s *Service) Reminders(ctx context.Context, within int) ([]Reminder, error) {
	theses, err := s.repo.theses(ctx)
	if err != nil {
		return nil, fmt.Errorf("portfolio: reminders: %w", err)
	}
	tickers, err := s.Tickers(ctx)
	if err != nil {
		return nil, err
	}
	held := map[string]bool{}
	for _, t := range tickers {
		held[t] = true
	}
	today := s.Today()
	var out []Reminder
	for _, th := range theses {
		if d, ok := th.ReportIn(today); ok && d <= within && held[th.Ticker.String()] {
			out = append(out, Reminder{Ticker: th.Ticker.String(), Date: day(*th.NextReport), InDays: d, Thesis: th.Thesis, Breaks: th.Breaks})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InDays < out[j].InDays })
	return out, nil
}

func purchaseView(p Purchase) PurchaseView {
	v := PurchaseView{ID: p.ID, PlannedOn: day(p.PlannedOn), What: p.What, Class: p.Class, AmountCents: p.Amount.Cents(), Condition: p.Condition, Status: p.Status, Note: p.Note}
	if p.DecidedAt != nil {
		v.DecidedAt = ptr(day(*p.DecidedAt))
	}
	return v
}

// Purchases is the purchase schedule, newest first.
func (s *Service) Purchases(ctx context.Context) ([]PurchaseView, error) {
	ps, err := s.repo.purchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("portfolio: purchases: %w", err)
	}
	out := make([]PurchaseView, len(ps))
	for i, p := range ps {
		out[i] = purchaseView(p)
	}
	return out, nil
}

// PlanPurchase adds a buy to the schedule.
func (s *Service) PlanPurchase(ctx context.Context, on time.Time, what string, c Class, amount money.Money, condition string) (PurchaseView, error) {
	p, err := NewPurchase(on, what, c, amount, condition, s.now())
	if err != nil {
		return PurchaseView{}, err
	}
	if err := s.repo.savePurchase(ctx, p); err != nil {
		return PurchaseView{}, fmt.Errorf("portfolio: plan purchase: %w", err)
	}
	return purchaseView(p), nil
}

// DecidePurchase marks a planned buy done or cancelled.
func (s *Service) DecidePurchase(ctx context.Context, id string, status PurchaseStatus, note string) error {
	p, err := s.repo.purchase(ctx, id)
	if err != nil {
		return err
	}
	if err := p.Decide(status, note, s.now()); err != nil {
		return err
	}
	if err := s.repo.savePurchase(ctx, p); err != nil {
		return fmt.Errorf("portfolio: decide purchase: %w", err)
	}
	return nil
}
