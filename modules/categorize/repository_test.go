//go:build integration

package categorize

import (
	"context"
	"errors"
	"testing"
	"time"

	"lastro/modules/ledger"
	"lastro/platform/db/dbtest"
	"lastro/platform/events"
)

// newService wires categorize to a ledger on the same database, with the
// rules subscribed to its imports as app does.
func newService(t *testing.T) *Service {
	t.Helper()
	pool := dbtest.New(t)
	bus := events.NewBus(nil)
	s := New(NewRepository(pool), ledger.New(ledger.NewRepository(pool), bus), nil)
	events.Subscribe(bus, s.OnTransactionsImported)
	s.now = func() time.Time { return t0 }
	return s
}

func byName(t *testing.T, s *Service) map[string]Category {
	t.Helper()
	cs, err := s.Categories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Category{}
	for _, c := range cs {
		if !c.Archived {
			out[c.Name] = c
		}
	}
	return out
}

func TestSeededCategories(t *testing.T) {
	s := newService(t)
	cs, err := s.Categories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Color{
		"Transport": Note1, "Health": Note2, "Leisure": Note5, "Restaurants": Note10,
		"Shopping": Note20, "Groceries": Note50, "Housing": Note100, "Other": Note200, "Income": Note2,
	}
	if len(cs) != len(want) {
		t.Fatalf("want the %d categories of the canvas, got %+v", len(want), cs)
	}
	for _, c := range cs {
		if want[c.Name] != c.Color || c.Budgeted != (c.Name != "Income") || c.Archived {
			t.Errorf("seed %+v", c)
		}
	}
	if cs[0].Name != "Groceries" || cs[len(cs)-1].Name != "Transport" {
		t.Errorf("by name: %s … %s", cs[0].Name, cs[len(cs)-1].Name)
	}
}

func TestCategoryCRUD(t *testing.T) {
	ctx := context.Background()
	s := newService(t)

	pets, err := s.CreateCategory(ctx, "Pets", Note5, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCategory(ctx, "pets", Note10, true); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("names are unique, whatever the case: %v", err)
	}
	if _, err := s.UpdateCategory(ctx, pets.ID, CategoryChange{Name: ptr("Health")}); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("a rename cannot take a live name: %v", err)
	}

	pets, err = s.UpdateCategory(ctx, pets.ID, CategoryChange{Name: ptr("Pets and vet"), Color: ptr(Note20), Budgeted: ptr(false)})
	if err != nil {
		t.Fatal(err)
	}
	if got := byName(t, s)["Pets and vet"]; got != pets || got.Color != Note20 || got.Budgeted {
		t.Fatalf("stored %+v, want %+v", got, pets)
	}

	missing, _ := ledger.NewCategoryID("6f1c2d3e-4a5b-4c6d-8e7f-901a2b3c4d5e")
	if _, err := s.UpdateCategory(ctx, missing, CategoryChange{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
	if _, err := s.ArchiveCategory(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
}

func TestArchiveCategory(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	cats := byName(t, s)
	shopping := cats["Shopping"]

	if err := s.Learn(ctx, pattern(t, PatternContains, "amazon"), "", shopping.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Learn(ctx, pattern(t, PatternContains, "ifood"), "", cats["Restaurants"].ID); err != nil {
		t.Fatal(err)
	}

	n, err := s.ArchiveCategory(ctx, shopping.ID)
	if err != nil || n != 1 {
		t.Fatalf("archiving takes its rule with it: %d, %v", n, err)
	}
	rules, err := s.repo.rules(ctx)
	if err != nil || len(rules) != 1 || rules[0].pattern.Text() != "IFOOD" {
		t.Fatalf("the other rules stay: %+v, %v", rules, err)
	}

	all, _ := s.Categories(ctx)
	if last := all[len(all)-1]; last.ID != shopping.ID || !last.Archived {
		t.Fatalf("an archived category is still listed, last: %+v", last)
	}
	if _, ok := byName(t, s)["Shopping"]; ok {
		t.Fatal("and is no longer live")
	}
	if _, err := s.ArchiveCategory(ctx, shopping.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("archiving twice: %v", err)
	}
	if _, err := s.UpdateCategory(ctx, shopping.ID, CategoryChange{Name: ptr("Stuff")}); !errors.Is(err, ErrArchived) {
		t.Errorf("an archived category does not change: %v", err)
	}
	if err := s.Learn(ctx, pattern(t, PatternContains, "amazon"), "", shopping.ID); !errors.Is(err, ErrArchived) {
		t.Errorf("no rule leads to it: %v", err)
	}
	if _, err := s.CreateCategory(ctx, "Shopping", Note20, true); err != nil {
		t.Errorf("its name is free again: %v", err)
	}
}

func TestLearn(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	cats := byName(t, s)
	ifood := pattern(t, PatternContains, "iFood")

	if err := s.Learn(ctx, ifood, "", cats["Groceries"].ID); err != nil {
		t.Fatal(err)
	}
	// I fix it: iFood is restaurants. The same pattern is retargeted.
	if err := s.Learn(ctx, pattern(t, PatternContains, "IFOOD"), "", cats["Restaurants"].ID); err != nil {
		t.Fatal(err)
	}
	// Only on the card, the same words mean something else.
	if err := s.Learn(ctx, ifood, card, cats["Leisure"].ID); err != nil {
		t.Fatal(err)
	}
	// An exact pattern is a different rule from a contains one.
	if err := s.Learn(ctx, pattern(t, PatternExact, "iFood"), "", cats["Other"].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Learn(ctx, ifood, "", cats["Restaurants"].ID); err != nil {
		t.Fatal(err) // nothing to change
	}

	rules, err := s.repo.rules(ctx)
	if err != nil || len(rules) != 3 {
		t.Fatalf("one rule per pattern and account: %+v, %v", rules, err)
	}
	got := map[string]ledger.CategoryID{}
	for _, r := range rules {
		got[string(r.pattern.Kind())+" "+r.pattern.Text()+" "+string(r.account)] = r.category
	}
	if got["contains IFOOD "] != cats["Restaurants"].ID || got["contains IFOOD "+string(card)] != cats["Leisure"].ID || got["exact IFOOD "] != cats["Other"].ID {
		t.Fatalf("rules: %v", got)
	}
	if !rules[0].Matches("PAG*IFOOD", checking) {
		t.Error("a stored rule still matches")
	}

	missing, _ := ledger.NewCategoryID("6f1c2d3e-4a5b-4c6d-8e7f-901a2b3c4d5e")
	if err := s.Learn(ctx, ifood, "", missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown category: %v", err)
	}
}
