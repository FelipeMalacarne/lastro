package planning

import "lastro/modules/portfolio"

// ProjectionView is the Planning screen: everything in today's money.
type ProjectionView struct {
	StartCents   int64          `json:"startCents"`
	MonthlyCents int64          `json:"monthlyCents"`
	Months       int            `json:"months"`
	TargetCents  int64          `json:"targetCents"`
	Scenarios    []ScenarioView `json:"scenarios"`
	When         []WhenView     `json:"when"`
	Levers       []LeverView    `json:"levers"`
	Mix          []MixView      `json:"mix"`
	MonteCarlo   MonteCarloView `json:"monteCarlo"`
}

type ScenarioView struct {
	Name        string  `json:"name"`
	RateBp      int64   `json:"rateBp"`
	FinalCents  int64   `json:"finalCents"`
	YearlyCents []int64 `json:"yearlyCents"` // today, then each year
}

// WhenView is when the target is reached in a scenario; Months is null past 100 years.
type WhenView struct {
	Name   string `json:"name"`
	RateBp int64  `json:"rateBp"`
	Months *int   `json:"months"`
}

type LeverView struct {
	MonthlyCents int64 `json:"monthlyCents"`
	Months       *int  `json:"months"`
}

type MixView struct {
	Class    portfolio.Class `json:"class"`
	WeightBp int64           `json:"weightBp"`
	MeanBp   int64           `json:"meanBp"`
	StdevBp  int64           `json:"stdevBp"`
}

type MonteCarloView struct {
	Paths     int        `json:"paths"`
	ReachedBp int64      `json:"reachedBp"` // share of paths at the target by the end
	Bands     []BandView `json:"bands"`
}

type BandView struct {
	Month    int   `json:"month"`
	P10Cents int64 `json:"p10Cents"`
	P50Cents int64 `json:"p50Cents"`
	P90Cents int64 `json:"p90Cents"`
}
