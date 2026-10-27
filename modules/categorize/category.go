package categorize

import (
	"crypto/rand"
	"fmt"

	"lastro/modules/ledger"
)

// Category is simple CRUD: no aggregate, only the validation below. Its id
// is the one the ledger stores with each transaction.
type Category struct {
	ID    ledger.CategoryID
	Name  string
	Color Color
	// Budgeted says whether it counts toward the monthly budget: spending
	// categories do, Income does not.
	Budgeted bool
	// Archived categories leave the pickers and the LLM's list; transactions
	// already in one keep showing it.
	Archived bool
}

// NewCategory validates a new category and gives it an id.
func NewCategory(name string, color Color, budgeted bool) (Category, error) {
	c := Category{Budgeted: budgeted}
	if err := c.Change(CategoryChange{Name: &name, Color: &color}); err != nil {
		return Category{}, err
	}
	id, err := ledger.NewCategoryID(newID())
	if err != nil {
		return Category{}, err
	}
	c.ID = id
	return c, nil
}

// CategoryChange holds the fields to change; nil leaves a field as it is.
type CategoryChange struct {
	Name     *string
	Color    *Color
	Budgeted *bool
}

// Change validates and applies ch. It changes nothing when any field is
// invalid, and an archived category does not change at all (ErrArchived).
func (c *Category) Change(ch CategoryChange) error {
	if c.Archived {
		return fmt.Errorf("%w: %s", ErrArchived, c.Name)
	}
	next := *c
	if ch.Name != nil {
		n, err := cleanName(*ch.Name)
		if err != nil {
			return err
		}
		next.Name = n
	}
	if ch.Color != nil {
		col, err := ParseColor(string(*ch.Color))
		if err != nil {
			return err
		}
		next.Color = col
	}
	if ch.Budgeted != nil {
		next.Budgeted = *ch.Budgeted
	}
	*c = next
	return nil
}

// newID returns a random (version 4) UUID.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // never fails: crypto/rand panics instead of returning an error
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
