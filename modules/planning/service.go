package planning

import (
	"context"
	"fmt"
	"math"

	"lastro/modules/portfolio"
	"lastro/money"
)

// The three scenarios' real returns a year (README): pessimistic, base and
// optimistic.
var scenarios = []struct {
	Name string
	Rate float64
}{{"pessimistic", 0.02}, {"base", 0.04}, {"optimistic", 0.06}}

// classAssumptions are each class's real return a year for the Monte Carlo,
// as a mean and a standard deviation: long-run figures, not forecasts.
var classAssumptions = map[portfolio.Class]Asset{
	portfolio.ClassCDI:    {Mean: 0.04, Stdev: 0.015},
	portfolio.ClassIPCA:   {Mean: 0.05, Stdev: 0.06},
	portfolio.ClassBR:     {Mean: 0.06, Stdev: 0.25},
	portfolio.ClassGlobal: {Mean: 0.06, Stdev: 0.17},
}

const simulationPaths = 4000

type Service struct {
	portfolio *portfolio.Service
}

func New(p *portfolio.Service) *Service { return &Service{portfolio: p} }

// Ask is what a projection is asked: what goes in every month, for how long,
// and the target for "how long until".
type Ask struct {
	Monthly money.Money
	Years   int
	Target  money.Money
}

// Project projects the invested money in today's money (F9).
func (s *Service) Project(ctx context.Context, a Ask) (ProjectionView, error) {
	start, weights, err := s.portfolio.Invested(ctx)
	if err != nil {
		return ProjectionView{}, fmt.Errorf("planning: %w", err)
	}
	if a.Years < 1 || a.Years > 60 {
		return ProjectionView{}, fmt.Errorf("%w: %d years, want 1 to 60", ErrInvalid, a.Years)
	}
	reais := func(m money.Money) float64 { return float64(m.Cents()) / 100 }
	cents := func(v float64) int64 { return int64(math.Round(v * 100)) }
	months := a.Years * 12
	out := ProjectionView{StartCents: start.Cents(), MonthlyCents: a.Monthly.Cents(), Months: months, TargetCents: a.Target.Cents()}

	for _, sc := range scenarios {
		path, err := Project(Scenario{Start: reais(start), Monthly: reais(a.Monthly), Months: months, RealAnnual: sc.Rate})
		if err != nil {
			return ProjectionView{}, err
		}
		v := ScenarioView{Name: sc.Name, RateBp: int64(math.Round(sc.Rate * 10000)), FinalCents: cents(path[months])}
		for m := 0; m <= months; m += 12 {
			v.YearlyCents = append(v.YearlyCents, cents(path[m]))
		}
		out.Scenarios = append(out.Scenarios, v)
		w := WhenView{Name: sc.Name, RateBp: v.RateBp}
		if a.Target.IsPositive() {
			if n, ok := MonthsUntil(reais(start), reais(a.Monthly), sc.Rate, reais(a.Target)); ok {
				w.Months = &n
			}
		}
		out.When = append(out.When, w)
	}

	// What R$ 500 more or less a month does to the date, in the base scenario.
	if a.Target.IsPositive() {
		for _, d := range []int64{50000, -50000} {
			m := a.Monthly.Cents() + d
			if m < 0 {
				continue
			}
			l := LeverView{MonthlyCents: m}
			if n, ok := MonthsUntil(reais(start), float64(m)/100, 0.04, reais(a.Target)); ok {
				l.Months = &n
			}
			out.Levers = append(out.Levers, l)
		}
	}

	// Monte Carlo over the target allocation.
	var mix []Asset
	for _, c := range portfolio.Classes {
		w := float64(weights[c].Bps()) / 10000
		if w == 0 {
			continue
		}
		as := classAssumptions[c]
		as.Weight = w
		mix = append(mix, as)
		out.Mix = append(out.Mix, MixView{Class: c, WeightBp: weights[c].Bps(), MeanBp: int64(as.Mean * 10000), StdevBp: int64(as.Stdev * 10000)})
	}
	sim, err := Simulate(Scenario{Start: reais(start), Monthly: reais(a.Monthly), Months: months}, mix, simulationPaths, reais(a.Target), uint64(start.Cents())^uint64(months))
	if err != nil {
		return ProjectionView{}, err
	}
	out.MonteCarlo = MonteCarloView{Paths: sim.Paths, ReachedBp: int64(math.Round(sim.Reached * 10000))}
	for _, b := range sim.Bands {
		out.MonteCarlo.Bands = append(out.MonteCarlo.Bands, BandView{Month: b.Month, P10Cents: cents(b.P10), P50Cents: cents(b.P50), P90Cents: cents(b.P90)})
	}
	return out, nil
}
