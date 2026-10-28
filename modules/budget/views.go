package budget

// MonthView is the Budget screen and the dashboard's month: the budget that
// applies, what came in, what was set aside and each category's spending.
type MonthView struct {
	Month     string  `json:"month"` // "2026-10"
	HasBudget bool    `json:"hasBudget"`
	Saved     bool    `json:"saved"` // its own budget, not inherited
	From      *string `json:"from"`  // the saved month it follows
	// PrevDiffers is whether the month before had other limits, so copying
	// them would change something.
	PrevDiffers bool `json:"prevDiffers"`
	// RolloverNew is whether a limit set for the first time rolls over by
	// default (Settings).
	RolloverNew bool  `json:"rolloverNew"`
	Days        int   `json:"days"`
	DaysLeft    int   `json:"daysLeft"`  // today included
	ElapsedBp   int64 `json:"elapsedBp"` // share of the month gone

	Payday           int     `json:"payday"`
	PayYourselfFirst PYFView `json:"payYourselfFirst"`

	IncomeCents      int64 `json:"incomeCents"`
	IncomeCount      int   `json:"incomeCount"`
	LimitsCents      int64 `json:"limitsCents"`  // every line's limit, before rollover
	CarriedCents     int64 `json:"carriedCents"` // rolled over into this month
	AvailableCents   int64 `json:"availableCents"`
	SpentCents       int64 `json:"spentCents"` // in the lines
	LeftCents        int64 `json:"leftCents"`
	NotAssignedCents int64 `json:"notAssignedCents"` // income minus pay yourself first and limits
	// UpcomingCents is what recurring charges in the lines' categories
	// should still take this month: left to spend once they come is
	// LeftCents minus it.
	UpcomingCents int64 `json:"upcomingCents"`

	// Categories are the budget's lines (ADR-0012), what needs attention
	// first.
	Categories []LineView  `json:"categories"`
	Unlimited  []SpendView `json:"unlimited"` // categories no line covers, by amount
	Summary    SummaryView `json:"summary"`
}

// PYFView is pay yourself first: what was planned and what left for
// accounts outside lastro, net of what came back from them (SetAside).
type PYFView struct {
	PlannedCents int64   `json:"plannedCents"`
	MovedCents   int64   `json:"movedCents"`
	Due          string  `json:"due"`    // the payday's date
	DoneOn       *string `json:"doneOn"` // the last transfer's day, once moved reaches planned
}

// LineView is one budget line in a month. Category is the category it
// covers or, for a line shared by several, the first of them by id, so a
// screen that shows one category per line keeps working; ByCategory says
// where its spending went.
type LineView struct {
	ID             string             `json:"id"`
	Name           *string            `json:"name"` // a shared line's; null for one that goes by its category's
	Category       string             `json:"category"`
	ByCategory     []LineCategoryView `json:"byCategory"` // every category it covers, the most spent first
	LimitCents     int64              `json:"limitCents"`
	CarriedCents   int64              `json:"carriedCents"`
	AvailableCents int64              `json:"availableCents"`
	SpentCents     int64              `json:"spentCents"`
	LeftCents      int64              `json:"leftCents"`
	Rollover       bool               `json:"rollover"`
	UsedBp         int64              `json:"usedBp"`
	Pace           Pace               `json:"pace"`
	Count          int                `json:"count"`
	FixedCents     int64              `json:"fixedCents"`    // recurring charges already paid this month
	UpcomingCents  int64              `json:"upcomingCents"` // recurring charges still to come this month
	History
}

// LineCategoryView is one of a line's categories: what it spent this month
// and, alone, in the six months before on average.
type LineCategoryView struct {
	Category     string `json:"category"`
	SpentCents   int64  `json:"spentCents"`
	Count        int    `json:"count"`
	AverageCents int64  `json:"averageCents"`
}

// SpendView is spending in a category without a limit; Category is null
// for transactions without a category yet.
type SpendView struct {
	Category   *string `json:"category"`
	SpentCents int64   `json:"spentCents"`
	Count      int     `json:"count"`
	History
}

// History is the spending in the six months before this one.
type History struct {
	History      []MonthSpend `json:"history"` // oldest first
	AverageCents int64        `json:"averageCents"`
}

type MonthSpend struct {
	Month      string `json:"month"`
	SpentCents int64  `json:"spentCents"`
}

// SummaryView is the month in four numbers (F4 P1).
type SummaryView struct {
	IncomeCents       int64 `json:"incomeCents"`
	SpentCents        int64 `json:"spentCents"` // everything that counts, limited or not
	ContributionCents int64 `json:"contributionCents"`
	SavingsRateBp     int64 `json:"savingsRateBp"` // income minus spending, over income
}

// RecurringView is the fixed costs panel (F5): every charge that comes back
// each month, most expensive first.
type RecurringView struct {
	MonthlyCents  int64             `json:"monthlyCents"`  // what they cost a month together
	PaidCents     int64             `json:"paidCents"`     // already charged this month
	UpcomingCents int64             `json:"upcomingCents"` // still to come this month
	New           int               `json:"new"`
	Changed       int               `json:"changed"`
	Charges       []RecurringCharge `json:"charges"`
}

type RecurringCharge struct {
	Name          string      `json:"name"`
	Category      *string     `json:"category"`
	Steadiness    Steadiness  `json:"steadiness"`
	MonthlyCents  int64       `json:"monthlyCents"`
	Months        int         `json:"months"`
	First         string      `json:"first"`
	Last          string      `json:"last"`
	Next          string      `json:"next"`
	PaidThisMonth bool        `json:"paidThisMonth"`
	New           bool        `json:"new"`
	Change        *ChangeView `json:"change"`
}

type ChangeView struct {
	FromCents int64  `json:"fromCents"`
	ToCents   int64  `json:"toCents"`
	Since     string `json:"since"`
}

// AlertView is an alert as the Dashboard and a push channel show it.
type AlertView struct {
	Key      string    `json:"key"`
	Kind     AlertKind `json:"kind"`
	Severity string    `json:"severity"` // "danger", "warning" or "info"
	Title    string    `json:"title"`
	Detail   string    `json:"detail"`
	Category *string   `json:"category"`
	Link     string    `json:"link"`
}
