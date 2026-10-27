package categorize

import (
	"cmp"
	"slices"
	"time"

	"lastro/modules/ledger"
)

// ReviewGroup is what one correction settles in the review queue: the
// transactions waiting for a category whose descriptions suggest the same
// pattern (every "PAG*IFOOD SP" and "IFOOD *RESTAURANTE" under IFOOD).
type ReviewGroup struct {
	// Pattern is the rule the correction would teach. It is zero when the
	// description is too short to suggest one; the group then holds the
	// transactions with that exact description.
	Pattern      Pattern
	Count        int
	TotalCents   int64
	Latest       time.Time
	Transactions []ledger.Categorizable // newest first
}

// GroupForReview groups the waiting transactions by the pattern each one's
// description suggests, the groups that settle the most first (then the
// most recent), each group's transactions newest first.
func GroupForReview(txs []ledger.Categorizable) []ReviewGroup {
	index := map[string]int{}
	var groups []ReviewGroup
	for _, t := range txs {
		p, ok := SuggestPattern(t.Description)
		key := "p:" + p.text
		if !ok {
			key = "d:" + t.Description
		}
		i, seen := index[key]
		if !seen {
			i = len(groups)
			index[key] = i
			groups = append(groups, ReviewGroup{Pattern: p})
		}
		g := &groups[i]
		g.Count++
		g.TotalCents += t.AmountCents
		if t.Date.After(g.Latest) {
			g.Latest = t.Date
		}
		g.Transactions = append(g.Transactions, t)
	}
	for i := range groups {
		slices.SortStableFunc(groups[i].Transactions, func(a, b ledger.Categorizable) int {
			return cmp.Or(b.Date.Compare(a.Date), cmp.Compare(a.ID, b.ID))
		})
	}
	slices.SortStableFunc(groups, func(a, b ReviewGroup) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), b.Latest.Compare(a.Latest), cmp.Compare(a.Pattern.text, b.Pattern.text))
	})
	return groups
}
