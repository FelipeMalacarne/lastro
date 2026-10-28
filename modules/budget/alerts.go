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
	AlertOver        AlertKind = "over"         // spent more than the line's limit
	AlertOverdrawn   AlertKind = "overdrawn"    // a fund spent past its balance (ADR-0011)
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
	Line     LineID            // zero for a recurring charge and a category no line covers
	Category ledger.CategoryID // zero for a recurring charge and a shared line
	Name     string            // a recurring charge's merchant, or a shared line's name
	Amount   money.Money       // over by, left, spent, or the new price
	Compare  money.Money       // the limit, the average, or the old price
	DaysLeft int
}

// Figures is what the alerts read of one line, or of a category no line
// covers, this month.
type Figures struct {
	Line      LineID            // zero for a category no line covers
	Name      string            // a shared line's
	Category  ledger.CategoryID // zero for a shared line
	Limited   bool
	Fund      bool
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

// FindAlerts lists what is worth telling about a month: lines over their
// limit or ahead of pace (only while the month runs), spending far above
// the usual, and recurring charges that changed price this month or are
// new. A fund's one alert is going into the red: its spending comes in
// lumps, so it is never ahead of pace nor unusual. Over and overdrawn
// before ahead before unusual; charges after. A line's key is its id, which
// for a line saved before lines could be shared is its category's, so an
// alert already sent is not sent again.
func FindAlerts(m Month, today time.Time, figures []Figures, recurring []Recurring, daysLeft int) []Alert {
	current := m == MonthOf(today)
	var out []Alert
	for _, f := range figures {
		id := f.Category.String()
		if !f.Line.IsZero() {
			id = f.Line.String()
		}
		at := Alert{Line: f.Line, Category: f.Category, Name: f.Name}
		switch {
		case f.Fund:
			if f.Pace == PaceOver {
				at.Key, at.Kind, at.Amount, at.Compare = fmt.Sprintf("overdrawn:%s:%s", m, id), AlertOverdrawn, f.Left.Neg(), f.Available
				out = append(out, at)
			}
			continue
		case f.Limited && f.Pace == PaceOver:
			at.Key, at.Kind, at.Amount, at.Compare = fmt.Sprintf("over:%s:%s", m, id), AlertOver, f.Left.Neg(), f.Available
			out = append(out, at)
			continue
		case f.Limited && f.Pace == PaceAhead && current && daysLeft > 0:
			ahead := at
			ahead.Key, ahead.Kind, ahead.Amount, ahead.Compare, ahead.DaysLeft = fmt.Sprintf("ahead:%s:%s", m, id), AlertAhead, f.Left, f.Available, daysLeft
			out = append(out, ahead)
		}
		avg := f.Average.Cents()
		if id != "" && avg >= usualMin && f.Spent.Cents()*10 >= avg*unusualTenths && f.Spent.Cents()-avg >= unusualMin {
			at.Key, at.Kind, at.Amount, at.Compare = fmt.Sprintf("unusual:%s:%s", m, id), AlertUnusual, f.Spent, f.Average
			out = append(out, at)
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
	rank := map[AlertKind]int{AlertOver: 0, AlertOverdrawn: 0, AlertAhead: 1, AlertUnusual: 2, AlertPriceChange: 3, AlertNewCharge: 4}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Kind] < rank[out[j].Kind] })
	return out
}
