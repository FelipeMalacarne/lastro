package planning

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
)

// ErrInvalid means a scenario's numbers make no sense.
var ErrInvalid = errors.New("planning: invalid")

// Projections are estimates, so they run on float64 in reais of today; the
// service turns results back into money.Money for display (ADR-0003).

// Scenario is what a projection assumes: where it starts, what goes in every
// month (raised with inflation, so constant in today's money), for how long
// and at what real return a year.
type Scenario struct {
	Start      float64 // reais of today
	Monthly    float64
	Months     int
	RealAnnual float64 // 0.04 is 4% a year above inflation
}

func (s Scenario) validate() error {
	switch {
	case s.Start < 0 || s.Monthly < 0:
		return fmt.Errorf("%w: negative money", ErrInvalid)
	case s.Months < 1 || s.Months > 12*60:
		return fmt.Errorf("%w: horizon of %d months, want 1 to 60 years", ErrInvalid, s.Months)
	case s.RealAnnual <= -0.5 || s.RealAnnual > 0.5:
		return fmt.Errorf("%w: real return of %.1f%% a year", ErrInvalid, s.RealAnnual*100)
	}
	return nil
}

// monthly is the month's rate that compounds to the year's.
func monthly(annual float64) float64 { return math.Pow(1+annual, 1.0/12) - 1 }

// Project returns the value at the end of every month, month 0 being today:
// the month's return on what is there, then the month's contribution.
func Project(s Scenario) ([]float64, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	r := monthly(s.RealAnnual)
	out := make([]float64, s.Months+1)
	out[0] = s.Start
	for m := 1; m <= s.Months; m++ {
		out[m] = out[m-1]*(1+r) + s.Monthly
	}
	return out, nil
}

// MonthsUntil is how many months until the value reaches target, false when
// it does not within 100 years.
func MonthsUntil(start, monthlyIn, realAnnual, target float64) (int, bool) {
	if start >= target {
		return 0, true
	}
	r := monthly(realAnnual)
	v := start
	for m := 1; m <= 1200; m++ {
		v = v*(1+r) + monthlyIn
		if v >= target {
			return m, true
		}
	}
	return 0, false
}

// Asset is one part of a Monte Carlo mix: its weight and its real return a
// year, as a mean and a standard deviation.
type Asset struct {
	Weight float64
	Mean   float64
	Stdev  float64
}

// Band is the spread of simulated outcomes at one month.
type Band struct {
	Month         int
	P10, P50, P90 float64
}

// Simulation is a Monte Carlo run: the bands over time and the share of
// paths that reach a target by the end.
type Simulation struct {
	Paths   int
	Bands   []Band // one per year, and the last month
	Reached float64
}

// Simulate runs paths of monthly returns drawn from the mix (each asset
// normal and independent, rebalanced every month to its weight), with the
// same start and contributions as the scenario. The seed makes it
// repeatable: the same inputs always draw the same paths.
func Simulate(s Scenario, mix []Asset, paths int, target float64, seed uint64) (Simulation, error) {
	if err := s.validate(); err != nil {
		return Simulation{}, err
	}
	var w float64
	for _, a := range mix {
		if a.Weight < 0 || a.Stdev < 0 {
			return Simulation{}, fmt.Errorf("%w: asset %+v", ErrInvalid, a)
		}
		w += a.Weight
	}
	if len(mix) == 0 || math.Abs(w-1) > 1e-6 || paths < 100 || paths > 20000 {
		return Simulation{}, fmt.Errorf("%w: a mix adding up to 1 and 100 to 20000 paths", ErrInvalid)
	}
	type monthlyAsset struct{ w, mu, sd float64 }
	ms := make([]monthlyAsset, len(mix))
	for i, a := range mix {
		// Log-normal per year: a month draws a twelfth of the year's log
		// mean and variance.
		sigma2 := math.Log(1 + a.Stdev*a.Stdev/((1+a.Mean)*(1+a.Mean)))
		mu := math.Log(1+a.Mean) - sigma2/2
		ms[i] = monthlyAsset{w: a.Weight, mu: mu / 12, sd: math.Sqrt(sigma2 / 12)}
	}
	rng := rand.New(rand.NewPCG(seed, 0x6c617374726f))
	checkpoints := []int{}
	for m := 12; m < s.Months; m += 12 {
		checkpoints = append(checkpoints, m)
	}
	checkpoints = append(checkpoints, s.Months)
	at := make([][]float64, len(checkpoints))
	reached := 0
	for p := 0; p < paths; p++ {
		v := s.Start
		c := 0
		for m := 1; m <= s.Months; m++ {
			var growth float64
			for _, a := range ms {
				growth += a.w * math.Exp(a.mu+a.sd*rng.NormFloat64())
			}
			v = v*growth + s.Monthly
			if c < len(checkpoints) && m == checkpoints[c] {
				at[c] = append(at[c], v)
				c++
			}
		}
		if target > 0 && v >= target {
			reached++
		}
	}
	out := Simulation{Paths: paths, Reached: float64(reached) / float64(paths)}
	for i, m := range checkpoints {
		vs := at[i]
		sort.Float64s(vs)
		q := func(f float64) float64 { return vs[int(f*float64(len(vs)-1))] }
		out.Bands = append(out.Bands, Band{Month: m, P10: q(0.10), P50: q(0.50), P90: q(0.90)})
	}
	return out, nil
}
