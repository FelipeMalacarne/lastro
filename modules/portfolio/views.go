package portfolio

// OverviewView is the Portfolio screen.
type OverviewView struct {
	AsOf      *string  `json:"asOf"` // the newest price or report among the holdings
	Positions int      `json:"positions"`
	Where     []string `json:"where"` // institutions

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

	Holdings []HoldingView `json:"holdings"`
	Theses   []ThesisView  `json:"theses"`
}

type NetWorthView struct {
	Date           string `json:"date"`
	CashCents      int64  `json:"cashCents"`
	CardsCents     int64  `json:"cardsCents"`
	InvestedCents  int64  `json:"investedCents"`
	EmergencyCents int64  `json:"emergencyCents"`
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
	Holdings    int    `json:"holdings"`
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

type HoldingView struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Institution string  `json:"institution"`
	Kind        Kind    `json:"kind"`
	Ticker      *string `json:"ticker"`
	Class       Class   `json:"class"`
	AutoClass   Class   `json:"autoClass"` // Classify's, to go back to
	Emergency   bool    `json:"emergency"`
	Quantity    string  `json:"quantity"`
	ValueCents  int64   `json:"valueCents"`
	BasisCents  int64   `json:"basisCents"`
	ReturnBp    *int64  `json:"returnBp"` // since it was put in
	Indexer     Indexer `json:"indexer"`
	RateBp      int64   `json:"rateBp"`
	Maturity    *string `json:"maturity"`
	AsOf        string  `json:"asOf"`
	Priced      bool    `json:"priced"` // valued at a market quote newer than the bank's report
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
