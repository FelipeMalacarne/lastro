package planning

import (
	"errors"
	"math"
	"testing"
)

func TestProject(t *testing.T) {
	// No return: what was there plus what went in.
	p, err := Project(Scenario{Start: 1000, Monthly: 100, Months: 12})
	if err != nil || p[12] != 2200 {
		t.Fatalf("flat: %v, %v", p[12], err)
	}
	// 12% a year, nothing added: exactly 12% after twelve months.
	p, _ = Project(Scenario{Start: 1000, Months: 12, RealAnnual: 0.12})
	if math.Abs(p[12]-1120) > 1e-6 {
		t.Errorf("compounding: %v", p[12])
	}
	// R$ 3.000 a month for 15 years at 4%, from R$ 120.000.
	p, _ = Project(Scenario{Start: 120000, Monthly: 3000, Months: 180, RealAnnual: 0.04})
	if p[180] < 950000 || p[180] > 980000 {
		t.Errorf("15 years: %.0f", p[180])
	}
	for _, bad := range []Scenario{{Start: -1, Months: 12}, {Months: 0}, {Months: 12, RealAnnual: 0.9}} {
		if _, err := Project(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func TestMonthsUntil(t *testing.T) {
	tests := []struct {
		start, monthly, rate, target float64
		want                         int
		ok                           bool
	}{
		{1000, 100, 0, 2000, 10, true},
		{5000, 0, 0, 1000, 0, true},
		{0, 0, 0.04, 1000, 0, false},
	}
	for _, tt := range tests {
		got, ok := MonthsUntil(tt.start, tt.monthly, tt.rate, tt.target)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%+v = %d %v", tt, got, ok)
		}
	}
	// A higher monthly amount always gets there sooner.
	a, _ := MonthsUntil(120000, 3000, 0.04, 1e6)
	b, _ := MonthsUntil(120000, 3500, 0.04, 1e6)
	if b >= a {
		t.Errorf("3.500 a month (%d) is not sooner than 3.000 (%d)", b, a)
	}
}

func TestSimulate(t *testing.T) {
	s := Scenario{Start: 100000, Monthly: 3000, Months: 120}
	mix := []Asset{{Weight: 0.5, Mean: 0.04, Stdev: 0.02}, {Weight: 0.5, Mean: 0.06, Stdev: 0.16}}
	a, err := Simulate(s, mix, 2000, 600000, 42)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Simulate(s, mix, 2000, 600000, 42)
	if a.Bands[len(a.Bands)-1] != b.Bands[len(b.Bands)-1] {
		t.Error("the same seed draws the same paths")
	}
	last := a.Bands[len(a.Bands)-1]
	if last.Month != 120 || !(last.P10 < last.P50 && last.P50 < last.P90) {
		t.Errorf("bands %+v", last)
	}
	// The median lands near the deterministic projection at the mix's mean.
	det, _ := Project(Scenario{Start: 100000, Monthly: 3000, Months: 120, RealAnnual: 0.05})
	if math.Abs(last.P50-det[120])/det[120] > 0.05 {
		t.Errorf("median %.0f, deterministic %.0f", last.P50, det[120])
	}
	if a.Reached <= 0 || a.Reached >= 1 {
		t.Errorf("reached %.2f", a.Reached)
	}
	if _, err := Simulate(s, []Asset{{Weight: 0.7, Mean: 0.04}}, 2000, 0, 1); !errors.Is(err, ErrInvalid) {
		t.Errorf("weights adding up to 0,7: %v", err)
	}
}
