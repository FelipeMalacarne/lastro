package ledger

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// ErrNotDuplicate means the two transactions cannot be the same movement
// told twice: see PossibleDuplicate.
var ErrNotDuplicate = errors.New("ledger: not a possible duplicate")

// duplicateWindow is how many days apart a transaction I added and a synced
// one may be and still be the same movement: the bank may date it a day or
// two after I did.
const duplicateWindow = 3

// PossibleDuplicate reports whether a transaction I added in a synced
// account and one the source sent may be the same movement: same account,
// same amount, up to duplicateWindow days apart, both live. The ledger never
// decides that on its own (two Pix of the same amount in a week are common):
// it asks, and I merge them or keep them apart.
func PossibleDuplicate(manual, synced *Transaction) error {
	days := manual.date.Sub(synced.date).Hours() / 24
	switch {
	case !manual.IsManual() || manual.accountType.IsManual():
		return fmt.Errorf("%w: %s is not one I added in a synced account", ErrNotDuplicate, manual.id)
	case synced.IsManual():
		return fmt.Errorf("%w: %s is one I added, not a synced one", ErrNotDuplicate, synced.id)
	case manual.IsRemoved() || synced.IsRemoved():
		return fmt.Errorf("%w: %s or %s", ErrRemoved, manual.id, synced.id)
	case manual.accountID != synced.accountID:
		return fmt.Errorf("%w: %s and %s are in different accounts", ErrNotDuplicate, manual.id, synced.id)
	case manual.amount != synced.amount:
		return fmt.Errorf("%w: %s and %s have different amounts", ErrNotDuplicate, manual.id, synced.id)
	case days > duplicateWindow || days < -duplicateWindow:
		return fmt.Errorf("%w: %s and %s are more than %d days apart", ErrNotDuplicate, manual.id, synced.id, duplicateWindow)
	}
	return nil
}

// Annotations are what I add to a transaction outside the aggregate.
type Annotations struct {
	Note  Note
	Tags  []Tag
	Split []SplitPart
}

// Merge makes the synced transaction the one that stays: it takes over what
// I gave the one I added, which is removed. The category moves unless I
// chose the synced one's myself, the transfer mark moves, a note moves to a
// synced transaction without one, tags join, and a split moves to one that
// is not split. It returns the synced transaction's annotations after the
// merge, and the counterpart of the removed one's pair, which must leave it.
func Merge(manual, synced *Transaction, mine, theirs Annotations, at time.Time) (Annotations, TransactionID, error) {
	if err := PossibleDuplicate(manual, synced); err != nil {
		return Annotations{}, "", err
	}
	out := Annotations{Note: theirs.Note, Tags: slices.Clone(theirs.Tags), Split: theirs.Split}
	if out.Note == "" {
		out.Note = mine.Note
	}
	for _, t := range mine.Tags {
		if !slices.Contains(out.Tags, t) && len(out.Tags) < maxTags {
			out.Tags = append(out.Tags, t)
		}
	}
	slices.Sort(out.Tags)

	_, theirSrc := synced.Category()
	if c, src := manual.Category(); !c.IsZero() && theirSrc != SourceManual {
		if err := synced.Restore(c, src); err != nil {
			return Annotations{}, "", err
		}
		if len(theirs.Split) == 0 && len(mine.Split) > 0 {
			out.Split = mine.Split
		}
	}
	if manual.MarkedTransfer() && !synced.MarkedTransfer() && !synced.IsPaired() {
		// The synced one may be a kind that takes no mark (a bill payment):
		// then it already counts as no spending.
		_ = synced.MarkTransfer(true)
	}
	return out, manual.Remove(at), nil
}
