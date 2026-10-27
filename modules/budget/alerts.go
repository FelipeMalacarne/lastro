package budget

import (
	"fmt"
	"sort"
	"time"

	"lastro/modules/ledger"
	"lastro/money"
)

// AlertKind is what an alert is about (F10).
type AlertKind string

const (
	AlertOver        AlertKind = "over"         // spent more than the limit
	AlertAhead       AlertKind = "ahead"        // likely to run out before the month ends
	AlertUnusual     AlertKind = "unusual"      // far above the category's usual month
	AlertPriceChange AlertKind = "price_change" // a recurring charge changed price this month
	AlertNewCharge   AlertKind = "new_charge"   // a recurring charge appeared
)

// Alert is one thing worth telling. Key is stable, so a channel sends it
// once.
type Alert struct {
	Key      string
	Kind     AlertKind
	Category ledger.CategoryID // zero for a recurring charge
	Name     string            // a recurring charge's merchant
	Amount   money.Money       // over by, left, spent, or the new price
	Compare  money.Money       // the limit, the average, or the old price
	DaysLeft int
}

// Figures is what the alerts read of one category this month.
type Figures struct {
	Category  ledger.CategoryID
	Limited   bool
	Spent     money.Money
	Available money.Money
	Left      money.Money
	Average   money.Money // the six months before
	Pace      Pace
}

// Unusual spending: past the whole month's average by a third and by
// R$ 100 at least, on a category that usually sees R$ 50 or more.
const (
	unusualTenths = 13 // 1,3 times the average
	unusualMin    = 10000
	usualMin      = 5000
)

// FindAlerts lists what is worth telling about a month: categories over
// their limit or ahead of pace (only while the month runs), spending far
// above the usual, and recurring charges that changed price this month or
// are new. Over before ahead before unusual; charges after.
func FindAlerts(m Month, today time.Time, figures []Figures, recurring []Recurring, daysLeft int) []Alert {
	current := m == MonthOf(today)
	var out []Alert
	for _, f := range figures {
		id := f.Category.String()
		switch {
		case f.Limited && f.Pace == PaceOver:
			out = append(out, Alert{Key: fmt.Sprintf("over:%s:%s", m, id), Kind: AlertOver, Category: f.Category, Amount: f.Left.Neg(), Compare: f.Available})
			continue
		case f.Limited && f.Pace == PaceAhead && current && daysLeft > 0:
			out = append(out, Alert{Key: fmt.Sprintf("ahead:%s:%s", m, id), Kind: AlertAhead, Category: f.Category, Amount: f.Left, Compare: f.Available, DaysLeft: daysLeft})
		}
		avg := f.Average.Cents()
		if !f.Category.IsZero() && avg >= usualMin && f.Spent.Cents()*10 >= avg*unusualTenths && f.Spent.Cents()-avg >= unusualMin {
			out = append(out, Alert{Key: fmt.Sprintf("unusual:%s:%s", m, id), Kind: AlertUnusual, Category: f.Category, Amount: f.Spent, Compare: f.Average})
		}
	}
	for _, r := range recurring {
		switch {
		case r.Change != nil && MonthOf(r.Change.Since) == m:
			out = append(out, Alert{Key: fmt.Sprintf("price:%s:%s", r.Name, r.Change.Since.Format(time.DateOnly)), Kind: AlertPriceChange, Name: r.Name, Amount: r.Change.To, Compare: r.Change.From})
		case r.New && current:
			out = append(out, Alert{Key: "new:" + r.Name, Kind: AlertNewCharge, Name: r.Name, Amount: r.Monthly})
		}
	}
	rank := map[AlertKind]int{AlertOver: 0, AlertAhead: 1, AlertUnusual: 2, AlertPriceChange: 3, AlertNewCharge: 4}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Kind] < rank[out[j].Kind] })
	return out
}
