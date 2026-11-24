package portfolio

// OverviewView is the Portfolio screen.
type OverviewView struct {
	AsOf      *string  `json:"asOf"`      // the newest price or report among the holdings
	Positions int      `json:"positions"` // groups held: a product bought in several lots is one
	Where     []string `json:"where"`     // institutions

	InvestedCents   int64   `json:"investedCents"`
	BasisCents      int64   `json:"basisCents"`
	MonthDeltaCents *int64  `json:"monthDeltaCents"` // returns since MonthFrom (about a month ago), money put in left out
	MonthFrom       *string `json:"monthFrom"`
	PutIn12mCents   *int64  `json:"putIn12mCents"`
	Returns12mCents *int64  `json:"returns12mCents"`
	Return12mBp     *int64  `json:"return12mBp"`
	Since           *string `json:"since"` // the start of those twelve months, or the first valuation

	NetWorth NetWorthView `json:"netWorth"`

	Targets map[Class]int64 `json:"targets"`
	BandBp  int64           `json:"bandBp"`
	Classes []ClassView     `json:"classes"`

	Growth     []GrowthPoint  `json:"growth"`
	Benchmarks BenchmarksView `json:"benchmarks"`

	// CostEstimated is whether some open stock or ETF's cost is still
	// lastro's estimate, so the returns built on it are too.
	CostEstimated bool `json:"costEstimated"`

	Groups []GroupView  `json:"groups"`
	Pots   PotsView     `json:"pots"`
	Theses []ThesisView `json:"theses"`
}

type NetWorthView struct {
	Date           string `json:"date"`
	CashCents      int64  `json:"cashCents"`
	CardsCents     int64  `json:"cardsCents"`
	InvestedCents  int64  `json:"investedCents"`
	PotsCents      int64  `json:"potsCents"`  // set apart for the pots
	OtherCents     int64  `json:"otherCents"` // outside Open Finance
	BasisCents     int64  `json:"basisCents"`
	TotalCents     int64  `json:"totalCents"`
}

type ClassView struct {
	Class       Class  `json:"class"`
	ValueCents  int64  `json:"valueCents"`
	BasisCents  int64  `json:"basisCents"`
	NowBp       int64  `json:"nowBp"`
	TargetBp    int64  `json:"targetBp"`
	Status      Status `json:"status"`
	Return12mBp *int64 `json:"return12mBp"`
	Holdings    int    `json:"holdings"` // groups with money in the class
	// CostEstimated is whether a group in the class costs lastro's estimate.
	CostEstimated bool `json:"costEstimated"`
}

type GrowthPoint struct {
	Date       string `json:"date"`
	ValueCents int64  `json:"valueCents"`
	BasisCents int64  `json:"basisCents"`
}

type BenchmarksView struct {
	From      *string `json:"from"`
	CDIBp     *int64  `json:"cdiBp"`
	IPCABp    *int64  `json:"ipcaBp"`
	CDIUntil  *string `json:"cdiUntil"`
	IPCAUntil *string `json:"ipcaUntil"`
}

// GroupView is a product held, in one or more lots, and how much of it is
// set apart for the pots.
type GroupView struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Institution string  `json:"institution"`
	Kind        Kind    `json:"kind"`
	Ticker      *string `json:"ticker"`
	Class       Class   `json:"class"`
	AutoClass   Class   `json:"autoClass"` // Classify's, to go back to
	Indexer     Indexer `json:"indexer"`
	RateBp      int64   `json:"rateBp"`
	// HoldsPots is whether the group is in the pool; PotsCents is what of
	// it is set apart for the pots today.
	HoldsPots      bool      `json:"holdsPots"`
	PotsCents      int64     `json:"potsCents"`
	ValueCents     int64     `json:"valueCents"`
	BasisCents     int64     `json:"basisCents"` // the open lots, at cost
	ReturnBp       *int64    `json:"returnBp"`   // since bought
	// Priceable is whether its cost can be typed (a stock or ETF whose bank
	// does not send it); CostEstimated whether it still is lastro's estimate,
	// the value on FirstSeen; AveragePriceCents the cost of one unit, and
	// CostTypedOn the day it was typed.
	Priceable         bool    `json:"priceable"`
	CostEstimated     bool    `json:"costEstimated"`
	AveragePriceCents *int64  `json:"averagePriceCents"`
	CostTypedOn       *string `json:"costTypedOn"`
	FirstSeen         string  `json:"firstSeen"`
	FirstMaturity  *string   `json:"firstMaturity"`
	LastMaturity   *string   `json:"lastMaturity"`
	AsOf           string    `json:"asOf"`
	Priced         bool      `json:"priced"`
	Lots           []LotView `json:"lots"` // biggest first
}

// LotView is one lot of a group: a single purchase or contribution, as the
// bank reports it.
type LotView struct {
	ID         string  `json:"id"`
	Quantity   string  `json:"quantity"`
	ValueCents int64   `json:"valueCents"`
	BasisCents int64   `json:"basisCents"`
	ReturnBp   *int64  `json:"returnBp"` // since it was put in
	Maturity   *string `json:"maturity"`
	AsOf       string  `json:"asOf"`
	Priced     bool    `json:"priced"` // valued at a market quote newer than the bank's report
}

type ThesisView struct {
	Ticker       string  `json:"ticker"`
	Thesis       string  `json:"thesis"`
	Breaks       string  `json:"breaks"`
	NextReport   *string `json:"nextReport"`
	ReportInDays *int    `json:"reportInDays"`
	Held         bool    `json:"held"`
	ValueCents   int64   `json:"valueCents"`
}

// PlanView is the Contribute screen and lastro contribute.
type PlanView struct {
	AmountCents int64       `json:"amountCents"`
	TotalCents  int64       `json:"totalCents"`
	BandBp      int64       `json:"bandBp"`
	Splits      []SplitView `json:"splits"`
}

type SplitView struct {
	Class       Class   `json:"class"`
	AmountCents int64   `json:"amountCents"`
	Why         Why     `json:"why"`
	ValueCents  int64   `json:"valueCents"`
	BeforeBp    int64   `json:"beforeBp"`
	AfterBp     int64   `json:"afterBp"`
	TargetBp    int64   `json:"targetBp"`
	Buy         *string `json:"buy"` // a suggestion of what to buy in the class
}

type PurchaseView struct {
	ID          string         `json:"id"`
	PlannedOn   string         `json:"plannedOn"`
	What        string         `json:"what"`
	Class       Class          `json:"class"`
	AmountCents int64          `json:"amountCents"`
	Condition   string         `json:"condition"`
	Status      PurchaseStatus `json:"status"`
	Note        string         `json:"note"`
	DecidedAt   *string        `json:"decidedAt"`
}

// Reminder is an earnings report coming soon for a stock I hold, with its thesis.
type Reminder struct {
	Ticker string `json:"ticker"`
	Date   string `json:"date"`
	InDays int    `json:"inDays"`
	Thesis string `json:"thesis"`
	Breaks string `json:"breaks"`
}

// ScoreView is one calendar year of the stock picks (the Brazilian stocks
// class, the satellite) against the Ibovespa and CDI (F8).
type ScoreView struct {
	Year        int    `json:"year"`
	From        string `json:"from"`
	To          string `json:"to"`
	Partial     bool   `json:"partial"` // the year is not over, or the history starts in it
	SatelliteBp *int64 `json:"satelliteBp"`
	IbovespaBp  *int64 `json:"ibovespaBp"`
	CDIBp       *int64 `json:"cdiBp"`
}

type ScoreboardView struct {
	Years []ScoreView `json:"years"`
	// Ibovespa is false when there are no Ibovespa quotes (brapi needs
	// LASTRO_BRAPI_TOKEN for the index).
	Ibovespa bool `json:"ibovespa"`
}

// PotsView is the pool that holds the pots against what they claim
// (ADR-0014): what the pool holds, what the pots claim (their positive
// balances), what it sets apart, and what is left over (counted in its
// groups' classes) or short. Groups is how many groups are in the pool;
// with none, nothing is set apart and the pots' money counts in its class.
type PotsView struct {
	PoolCents     int64     `json:"poolCents"`
	ClaimCents    int64     `json:"claimCents"`
	SetApartCents int64     `json:"setApartCents"`
	LeftCents     int64     `json:"leftCents"`
	ShortCents    int64     `json:"shortCents"`
	Groups        int       `json:"groups"`
	Pots          []PotView `json:"pots"` // by name
}

// PotView is one pot as Budget has it.
type PotView struct {
	Name         string `json:"name"`
	BalanceCents int64  `json:"balanceCents"`
	TargetCents  *int64 `json:"targetCents"`
}
