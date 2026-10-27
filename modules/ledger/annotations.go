package ledger

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"lastro/money"
)

// Note is what I write on a transaction, up to 500 characters.
type Note string

// NewNote trims the text; an empty note clears it.
func NewNote(s string) (Note, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > 500 {
		return "", fmt.Errorf("%w: a note has at most 500 characters", ErrInvalid)
	}
	return Note(s), nil
}

// Tag cuts across categories ("trip-rio" for a trip's flights, hotel and
// meals): lowercase letters and digits joined by dashes, up to 32.
type Tag string

// NewTag lowercases the text and drops a leading "#"; spaces become dashes.
func NewTag(s string) (Tag, error) {
	s = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#")))
	s = strings.Join(strings.Fields(s), "-")
	if s == "" || len(s) > 32 {
		return "", fmt.Errorf("%w: a tag has 1 to 32 characters", ErrInvalid)
	}
	for i, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && (r != '-' || i == 0 || i == len(s)-1) {
			return "", fmt.Errorf("%w: tag %q: use letters, digits and dashes", ErrInvalid, s)
		}
	}
	return Tag(s), nil
}

// maxTags is how many tags a transaction carries at most.
const maxTags = 10

// NewTags validates a transaction's tags: at most 10, each once, sorted.
func NewTags(in []string) ([]Tag, error) {
	out := make([]Tag, 0, len(in))
	for _, s := range in {
		t, err := NewTag(s)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	if len(out) > maxTags {
		return nil, fmt.Errorf("%w: at most %d tags", ErrInvalid, maxTags)
	}
	slices.Sort(out)
	return out, nil
}

// SplitPart is one category's share of a split transaction.
type SplitPart struct {
	Category CategoryID
	Amount   money.Money
}

// Split divides an expense or income across categories: from 2 to 10 parts,
// each in a different category, each on the same side as the transaction,
// adding up to its amount exactly. Totals by category count the parts.
type Split struct {
	parts []SplitPart
}

// Parts returns the parts in the order given.
func (s Split) Parts() []SplitPart { return slices.Clone(s.parts) }

// SplitInto validates a split of this transaction and makes the largest
// part its category, chosen by me: automation never touches a split
// transaction, and screens that show one category show the main one.
func (t *Transaction) SplitInto(parts []SplitPart) (Split, error) {
	switch {
	case t.IsRemoved():
		return Split{}, fmt.Errorf("%w: %s", ErrRemoved, t.id)
	case t.Kind() != KindExpense && t.Kind() != KindIncome:
		return Split{}, fmt.Errorf("%w: only expenses and income are split", ErrInvalid)
	case len(parts) < 2 || len(parts) > 10:
		return Split{}, fmt.Errorf("%w: a split has 2 to 10 parts", ErrInvalid)
	}
	sum, err := money.New(0, t.amount.Currency())
	if err != nil {
		return Split{}, err
	}
	main := 0
	seen := map[CategoryID]bool{}
	for i, p := range parts {
		switch {
		case p.Category.IsZero():
			return Split{}, fmt.Errorf("%w: a part without a category", ErrInvalid)
		case seen[p.Category]:
			return Split{}, fmt.Errorf("%w: a category appears twice in the split", ErrInvalid)
		case p.Amount.Currency() != t.amount.Currency():
			return Split{}, fmt.Errorf("%w: a part in another currency", ErrInvalid)
		case p.Amount.IsZero() || p.Amount.IsNegative() != t.amount.IsNegative():
			return Split{}, fmt.Errorf("%w: every part goes the same way as the transaction", ErrInvalid)
		}
		seen[p.Category] = true
		if sum, err = sum.Add(p.Amount); err != nil {
			return Split{}, err
		}
		if p.Amount.Abs().Cents() > parts[main].Amount.Abs().Cents() {
			main = i
		}
	}
	if sum != t.amount {
		return Split{}, fmt.Errorf("%w: the parts add up to %s, not %s", ErrInvalid, sum, t.amount)
	}
	if err := t.Categorize(parts[main].Category, SourceManual); err != nil {
		return Split{}, err
	}
	return Split{parts: slices.Clone(parts)}, nil
}
