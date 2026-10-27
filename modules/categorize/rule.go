package categorize

import (
	"errors"
	"fmt"
	"time"

	"lastro/modules/ledger"
)

// ErrArchived means an archived category was changed or a rule was pointed
// at one.
var ErrArchived = errors.New("categorize: category is archived")

// RuleID is a rule's id, a random UUID.
type RuleID string

// Rule sends the transactions whose description meets its pattern to a
// category, in every account or only in one. Each manual correction
// becomes a rule, or retargets the rule with the same pattern, so there is
// at most one rule per pattern and account.
type Rule struct {
	id        RuleID
	pattern   Pattern
	account   ledger.AccountID // empty: every account
	category  ledger.CategoryID
	createdAt time.Time
	updatedAt time.Time
}

// NewRule creates a rule. The category must be live: a rule never leads to
// an archived one.
func NewRule(p Pattern, account ledger.AccountID, c Category, now time.Time) (*Rule, error) {
	if p.IsZero() {
		return nil, fmt.Errorf("%w: rule without a pattern", ErrInvalid)
	}
	r := &Rule{id: RuleID(newID()), pattern: p, account: account, createdAt: now}
	if _, err := r.Retarget(c, now); err != nil {
		return nil, err
	}
	return r, nil
}

// Retarget sends the rule to another category, for a correction that meets
// a pattern already taught. It reports whether anything changed.
func (r *Rule) Retarget(c Category, now time.Time) (bool, error) {
	switch {
	case c.ID.IsZero():
		return false, fmt.Errorf("%w: rule without a category", ErrInvalid)
	case c.Archived:
		return false, fmt.Errorf("%w: %s", ErrArchived, c.Name)
	case c.ID == r.category:
		return false, nil
	}
	r.category, r.updatedAt = c.ID, now
	return true, nil
}

// Matches reports whether the rule applies to a transaction with this
// description in this account.
func (r *Rule) Matches(description string, account ledger.AccountID) bool {
	if r.account != "" && r.account != account {
		return false
	}
	return r.pattern.Matches(description)
}
