package portfolio

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	_ "time/tzdata" // the image has no zoneinfo; days turn in São Paulo

	"lastro/modules/ledger"
	"lastro/modules/market"
	"lastro/money"
)

var saoPaulo, _ = time.LoadLocation("America/Sao_Paulo")

type Service struct {
	repo   *PostgresRepository
	ledger *ledger.Service
	market *market.Service
	now    func() time.Time
}

func New(repo *PostgresRepository, l *ledger.Service, m *market.Service) *Service {
	return &Service{repo: repo, ledger: l, market: m, now: time.Now}
}

// Today is the date in São Paulo.
func (s *Service) Today() time.Time {
	y, m, d := s.now().In(saoPaulo).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// SyncPositions takes what an item's bank reports now. Positions the item
// no longer reports close; the others are updated or start being followed.
func (s *Service) SyncPositions(ctx context.Context, item string, reports []Report) (int, error) {
	all, err := s.repo.positions(ctx)
	if err != nil {
		return 0, fmt.Errorf("portfolio: sync positions: %w", err)
	}
	byID := make(map[string]*Position, len(all))
	for _, p := range all {
		byID[p.ExternalID] = p
	}
	now := s.now()
	seen := map[string]bool{}
	var touched []*Position
	for _, r := range reports {
		seen[r.ExternalID] = true
		if p, ok := byID[r.ExternalID]; ok {
			if err := p.Update(r, now); err != nil {
				return 0, fmt.Errorf("portfolio: sync positions: %w", err)
			}
			touched = append(touched, p)
			continue
		}
		if !r.Active && r.Value.IsZero() {
			continue // withdrawn before lastro knew it: nothing to follow
		}
		p, err := NewPosition(r, now)
		if err != nil {
			return 0, fmt.Errorf("portfolio: sync positions: %w", err)
		}
		touched = append(touched, p)
	}
	for _, p := range all {
		if p.Item == item && !seen[p.ExternalID] && p.Open() {
			p.Close(now)
			touched = append(touched, p)
		}
	}
	if err := s.repo.savePositions(ctx, touched, now); err != nil {
		return 0, fmt.Errorf("portfolio: sync positions: %w", err)
	}
	return len(touched), nil
}

// Tickers are the open positions' tickers, for the market to quote.
func (s *Service) Tickers(ctx context.Context) ([]string, error) {
	ps, err := s.repo.positions(ctx)
	if err != nil {
		return nil, fmt.Errorf("portfolio: tickers: %w", err)
	}
	set := map[string]bool{}
	for _, p := range ps {
		if p.Open() && !p.Ticker.IsZero() {
			set[p.Ticker.String()] = true
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out, nil
}

// value values the open positions today, at the latest quotes.
func (s *Service) value(ctx context.Context, day time.Time) (Valuation, []*Position, error) {
	ps, err := s.repo.positions(ctx)
	if err != nil {
		return Valuation{}, nil, err
	}
	var tickers []string
	for _, p := range ps {
		if p.Open() && !p.Ticker.IsZero() {
			tickers = append(tickers, p.Ticker.String())
		}
	}
	qs, err := s.market.LatestQuotes(ctx, tickers)
	if err != nil {
		return Valuation{}, nil, err
	}
	prices := make(map[string]Price, len(qs))
	for t, q := range qs {
		prices[t] = Price{Date: q.Date, Price: q.Close}
	}
	return Value(ps, prices, day), ps, nil
}

// netWorth adds the accounts to a valuation.
func (s *Service) netWorth(ctx context.Context, v Valuation) (NetWorth, error) {
	accs, err := s.ledger.Accounts(ctx)
	if err != nil {
		return NetWorth{}, err
	}
	nw := NetWorth{Date: v.Date, Invested: v.Invested().Value, Emergency: v.Emergency.Value}
	var cash, cards int64
	for _, a := range accs {
		if a.Currency != "BRL" {
			continue // net worth is in reais; an account abroad is not converted
		}
		if a.Type == string(ledger.AccountCreditCard) {
			cards += min(a.BalanceCents, 0)
		} else {
			cash += a.BalanceCents
		}
	}
	nw.Cash, nw.Cards = money.BRLCents(cash), money.BRLCents(cards)
	nw.Basis = money.BRLCents(v.Invested().Basis.Cents() + v.Emergency.Basis.Cents())
	return nw, nil
}

// Valuate stores the day's valuation and net worth, replacing an earlier
// one of the same day: the history the charts read (F6).
func (s *Service) Valuate(ctx context.Context, day time.Time) (NetWorth, error) {
	v, ps, err := s.value(ctx, day)
	if err != nil {
		return NetWorth{}, fmt.Errorf("portfolio: valuate: %w", err)
	}
	nw, err := s.netWorth(ctx, v)
	if err != nil {
		return NetWorth{}, fmt.Errorf("portfolio: valuate: %w", err)
	}
	if err := s.repo.saveValuation(ctx, v, nw, heldOf(ps, day), s.now()); err != nil {
		return NetWorth{}, fmt.Errorf("portfolio: valuate: %w", err)
	}
	return nw, nil
}

// Targets is the target allocation and band, or the defaults.
func (s *Service) Targets(ctx context.Context) (Targets, error) {
	t, err := s.repo.targets(ctx)
	if err != nil {
		return Targets{}, fmt.Errorf("portfolio: targets: %w", err)
	}
	return t, nil
}

// SetTargets changes the targets, which must add up to 100%, and the band.
func (s *Service) SetTargets(ctx context.Context, weights map[Class]money.Percent, bandBps int64) error {
	a, err := NewAllocation(weights)
	if err != nil {
		return err
	}
	b, err := NewBand(bandBps)
	if err != nil {
		return err
	}
	if err := s.repo.saveTargets(ctx, Targets{Allocation: a, Band: b}, s.now()); err != nil {
		return fmt.Errorf("portfolio: set targets: %w", err)
	}
	return nil
}

// UpdatePosition moves a position to another class (nil leaves it, "" goes
// back to the automatic one) or in and out of the emergency fund.
func (s *Service) UpdatePosition(ctx context.Context, id string, class *string, emergency *bool) error {
	ps, err := s.repo.positions(ctx)
	if err != nil {
		return fmt.Errorf("portfolio: update position: %w", err)
	}
	for _, p := range ps {
		if p.ExternalID != id {
			continue
		}
		if class != nil {
			if err := p.Reclassify(Class(*class)); err != nil {
				return err
			}
		}
		if emergency != nil {
			p.MarkEmergency(*emergency)
		}
		return s.repo.savePositions(ctx, []*Position{p}, s.now())
	}
	return ErrNotFound
}

// Plan splits a contribution between the classes at today's values.
func (s *Service) Plan(ctx context.Context, amount money.Money) (PlanView, error) {
	t, err := s.Targets(ctx)
	if err != nil {
		return PlanView{}, err
	}
	v, ps, err := s.value(ctx, s.Today())
	if err != nil {
		return PlanView{}, fmt.Errorf("portfolio: plan: %w", err)
	}
	p, err := PlanContribution(v.Values(), t.Allocation, t.Band, amount)
	if err != nil {
		return PlanView{}, err
	}
	bonds, err := s.market.Bonds(ctx)
	if err != nil {
		return PlanView{}, fmt.Errorf("portfolio: plan: %w", err)
	}
	out := PlanView{AmountCents: p.Amount.Cents(), TotalCents: p.Total.Cents(), BandBp: t.Band.Bps()}
	for _, sp := range p.Splits {
		out.Splits = append(out.Splits, SplitView{
			Class: sp.Class, AmountCents: sp.Amount.Cents(), Why: sp.Why, ValueCents: sp.Value.Cents(),
			BeforeBp: sp.Before.Bps(), AfterBp: sp.After.Bps(), TargetBp: sp.Target.Bps(), Buy: suggest(sp.Class, v, ps, bonds, s.Today()),
		})
	}
	return out, nil
}

// suggest names something to buy in a class: for IPCA+, the Tesouro IPCA+
// paying the most over inflation; otherwise the class's biggest holding.
func suggest(c Class, v Valuation, ps []*Position, bonds []market.Bond, today time.Time) *string {
	if c == ClassIPCA {
		var best *market.Bond
		for i, b := range bonds {
			if b.Name == "Tesouro IPCA+" && b.Maturity.After(today.AddDate(5, 0, 0)) && (best == nil || b.BuyRate.Bps() > best.BuyRate.Bps()) {
				best = &bonds[i]
			}
		}
		if best != nil {
			s := fmt.Sprintf("%s, IPCA + %s", best.Title(), fmtPercent(best.BuyRate))
			return &s
		}
	}
	var top *Holding
	for i, h := range v.Holdings {
		if !h.Position.Emergency() && h.Position.Class() == c && (top == nil || h.Value.Cents() > top.Value.Cents()) {
			top = &v.Holdings[i]
		}
	}
	if top == nil {
		return nil
	}
	name := top.Position.Name
	if !top.Position.Ticker.IsZero() {
		name = top.Position.Ticker.String()
	} else if top.Position.Indexer == IndexerCDI && top.Position.Rate.Bps() > 0 {
		name = fmt.Sprintf("%s, %s of CDI", top.Position.Institution+" "+kindName(top.Position.Kind), fmtPercent(top.Position.Rate))
	}
	return &name
}

func kindName(k Kind) string {
	switch k {
	case KindFixedIncome:
		return "CDB"
	case KindTreasury:
		return "Tesouro"
	}
	return string(k)
}

// fmtPercent writes 650 bps as "6,5%" and 10000 as "100%".
func fmtPercent(p money.Percent) string {
	s := fmt.Sprintf("%d,%02d", p.Bps()/100, p.Bps()%100)
	s = strings.TrimSuffix(strings.TrimSuffix(s, "0"), ",0")
	s = strings.TrimSuffix(s, ",")
	return s + "%"
}

// Invested is the allocation's value today, the emergency fund left out,
// and the target weights: what a projection starts from.
func (s *Service) Invested(ctx context.Context) (money.Money, map[Class]money.Percent, error) {
	t, err := s.Targets(ctx)
	if err != nil {
		return money.Money{}, nil, err
	}
	v, _, err := s.value(ctx, s.Today())
	if err != nil {
		return money.Money{}, nil, fmt.Errorf("portfolio: invested: %w", err)
	}
	return v.Invested().Value, t.Allocation.Weights(), nil
}

// heldOf sums the shares of each ticker in the open positions.
func heldOf(ps []*Position, day time.Time) []Held {
	by := map[string]int64{}
	for _, p := range ps {
		if p.Open() && !p.Ticker.IsZero() && p.Quantity.Micro() > 0 {
			by[p.Ticker.String()] += p.Quantity.Micro()
		}
	}
	out := make([]Held, 0, len(by))
	for t, q := range by {
		out = append(out, Held{Date: day, Ticker: t, Quantity: QuantityFromMicro(q)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ticker < out[j].Ticker })
	return out
}
