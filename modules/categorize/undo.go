package categorize

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"lastro/modules/ledger"
)

// Undo puts a choice back the way things were: the rule it taught as it was
// (removed, when the choice created it) and every transaction it changed
// back to its earlier category, whoever had chosen it.
type Undo struct {
	Rule    RuleID            // empty when the choice taught no rule
	WasRule ledger.CategoryID // the rule's category before; zero when the choice created it
	Before  []Before
}

// Before is one transaction's category before a choice. An empty category
// means it was waiting in the review queue.
type Before struct {
	Transaction ledger.TransactionID
	Category    ledger.CategoryID
	Source      ledger.Source
}

// snapshot is the category of every transaction automation may change.
type snapshot map[ledger.TransactionID]Before

func (s *Service) snapshot(ctx context.Context) (snapshot, error) {
	txs, err := s.ledger.Automatable(ctx)
	if err != nil {
		return nil, err
	}
	out := make(snapshot, len(txs))
	for _, t := range txs {
		b := Before{Transaction: t.ID, Source: t.Source}
		if t.Category != "" {
			if b.Category, err = ledger.NewCategoryID(t.Category); err != nil {
				return nil, err
			}
		}
		out[t.ID] = b
	}
	return out, nil
}

func beforeOf(id ledger.TransactionID, c *ledger.CategoryView) Before {
	b := Before{Transaction: id}
	if c == nil {
		return b
	}
	if cat, err := ledger.NewCategoryID(c.ID); err == nil {
		b.Category, b.Source = cat, ledger.Source(c.Source)
	}
	return b
}

// changed lists, as they were before, the transactions whose category is not
// the same after: changed by a rule, or taken out of automation's reach by
// a manual choice.
func changed(before, after snapshot) []Before {
	var out []Before
	for id, b := range before {
		if a, ok := after[id]; !ok || a != b {
			out = append(out, b)
		}
	}
	slices.SortFunc(out, func(a, b Before) int { return cmp.Compare(a.Transaction, b.Transaction) })
	return out
}

// Undo takes a choice back (see the Undo type). It returns how many
// transactions went back to their earlier category. A transaction the bank
// removed since is skipped, so an undo never stops halfway.
func (s *Service) Undo(ctx context.Context, u Undo) (int, error) {
	n := 0
	for _, b := range u.Before {
		err := s.ledger.Restore(ctx, b.Transaction, b.Category, b.Source)
		switch {
		case errors.Is(err, ledger.ErrRemoved), errors.Is(err, ledger.ErrNotFound):
		case err != nil:
			return n, fmt.Errorf("categorize: undo: %w", err)
		default:
			n++
		}
	}
	if u.Rule != "" {
		if u.WasRule.IsZero() {
			if err := s.repo.deleteRule(ctx, u.Rule); err != nil && !errors.Is(err, ErrRuleNotFound) {
				return 0, fmt.Errorf("categorize: undo: %w", err)
			}
		} else if err := s.retarget(ctx, u.Rule, u.WasRule); err != nil {
			return 0, fmt.Errorf("categorize: undo: %w", err)
		}
	}
	return n, nil
}

func (s *Service) retarget(ctx context.Context, id RuleID, to ledger.CategoryID) error {
	r, err := s.repo.rule(ctx, id)
	if errors.Is(err, ErrRuleNotFound) {
		return nil // deleted since: nothing to put back
	}
	if err != nil {
		return err
	}
	c, err := s.repo.category(ctx, to)
	if err != nil {
		return err
	}
	if changed, err := r.Retarget(c, s.now()); err != nil || !changed {
		return err
	}
	return s.repo.saveRule(ctx, r)
}
