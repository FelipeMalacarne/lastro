package categorize

import (
	"cmp"
	"slices"
	"strings"

	"lastro/modules/ledger"
)

// Engine is the domain service that picks, among the rules a transaction
// meets, the one that decides its category: the most specific. It keeps the
// rules in that order, so the first that matches wins.
type Engine struct {
	rules []*Rule
}

// NewEngine orders a copy of the rules, most specific first:
//
//  1. a rule limited to one account before a rule for every account;
//  2. exact before contains;
//  3. more words before fewer, then more characters before fewer;
//  4. the most recently taught first, so on a full tie my latest
//     correction wins (then the id, so the order never wobbles).
func NewEngine(rules []*Rule) *Engine {
	ordered := slices.Clone(rules)
	slices.SortFunc(ordered, func(a, b *Rule) int {
		return cmp.Or(
			cmp.Compare(rank(b), rank(a)),
			cmp.Compare(words(b), words(a)),
			cmp.Compare(len(b.pattern.text), len(a.pattern.text)),
			b.updatedAt.Compare(a.updatedAt),
			cmp.Compare(a.id, b.id),
		)
	})
	return &Engine{rules: ordered}
}

// Match returns the rule that decides a transaction with this description
// in this account, or nil when no rule meets it.
func (e *Engine) Match(description string, account ledger.AccountID) *Rule {
	for _, r := range e.rules {
		if r.Matches(description, account) {
			return r
		}
	}
	return nil
}

// Decision is the category a rule gives one transaction.
type Decision struct {
	Transaction ledger.TransactionID
	Category    ledger.CategoryID
}

// Decide gives each transaction a rule meets the category of the rule that
// decides it. The ones no rule meets are left out: they wait in To review.
func (e *Engine) Decide(txs []ledger.Categorizable) []Decision {
	var out []Decision
	for _, t := range txs {
		if r := e.Match(t.Description, t.Account); r != nil {
			out = append(out, Decision{Transaction: t.ID, Category: r.category})
		}
	}
	return out
}

// rank puts the account first and the kind second: 3 for an exact rule on
// one account, 2 for contains on one account, 1 for exact everywhere and 0
// for contains everywhere.
func rank(r *Rule) int {
	n := 0
	if r.account != "" {
		n += 2
	}
	if r.pattern.kind == PatternExact {
		n++
	}
	return n
}

func words(r *Rule) int { return strings.Count(r.pattern.text, " ") + 1 }
