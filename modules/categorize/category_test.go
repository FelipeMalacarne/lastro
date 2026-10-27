package categorize

import (
	"errors"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func TestNewCategory(t *testing.T) {
	c, err := NewCategory("  Pets  ", Note5, true)
	if err != nil || c.Name != "Pets" || c.Color != Note5 || !c.Budgeted || c.ID.IsZero() || !uuidRe.MatchString(c.ID.String()) {
		t.Fatalf("got %+v, %v", c, err)
	}
	if _, err := NewCategory("", Note5, true); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty name: %v", err)
	}
	if _, err := NewCategory("Pets", "note-3", true); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad colour: %v", err)
	}
}

func TestCategoryChange(t *testing.T) {
	c, _ := NewCategory("Pets", Note5, true)
	if err := c.Change(CategoryChange{Name: ptr("Pets and vet"), Budgeted: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if c.Name != "Pets and vet" || c.Color != Note5 || c.Budgeted {
		t.Fatalf("a nil field stays as it is: %+v", c)
	}

	before := c
	if err := c.Change(CategoryChange{Name: ptr("Vet"), Color: ptr(Color("purple"))}); !errors.Is(err, ErrInvalid) || c != before {
		t.Fatalf("one bad field changes nothing: %+v, %v", c, err)
	}

	c.Archived = true
	if err := c.Change(CategoryChange{Name: ptr("Vet")}); !errors.Is(err, ErrArchived) || c.Name != "Pets and vet" {
		t.Fatalf("an archived category does not change: %+v, %v", c, err)
	}
}
