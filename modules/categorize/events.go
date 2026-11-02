package categorize

import (
	"time"

	"lastro/modules/ledger"
)

// CategoryArchived is published after a category is archived. Transactions
// already in it keep it; what plans by category (the budget) lets it go from
// the month of At on.
type CategoryArchived struct {
	Category ledger.CategoryID
	At       time.Time
}

func (CategoryArchived) EventName() string { return "categorize.category_archived" }
