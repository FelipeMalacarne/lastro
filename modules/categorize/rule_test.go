package categorize

import (
	"errors"
	"testing"
	"time"

	"lastro/modules/ledger"
)

var (
	checking = ledger.AccountID("0b7f2c58-1d3e-4a4b-9c55-0e6f1f2a3b4c")
	card     = ledger.AccountID("5d0e9a7c-8f61-4b2a-a3d4-6c1b2e9f0a17")
	t0       = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
)

func category(t *testing.T, name string) Category {
	t.Helper()
	c, err := NewCategory(name, Note10, true)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func pattern(t *testing.T, kind PatternKind, text string) Pattern {
	t.Helper()
	p, err := NewPattern(kind, text)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNewRule(t *testing.T) {
	restaurants := category(t, "Restaurants")
	r, err := NewRule(pattern(t, PatternContains, "ifood"), "", restaurants, OriginCorrection, t0)
	if err != nil || r.category != restaurants.ID || r.id == "" || !r.createdAt.Equal(t0) {
		t.Fatalf("got %+v, %v", r, err)
	}
	if _, err := NewRule(Pattern{}, "", restaurants, OriginCorrection, t0); !errors.Is(err, ErrInvalid) {
		t.Errorf("no pattern: %v", err)
	}
	if _, err := NewRule(pattern(t, PatternContains, "ifood"), "", Category{}, OriginCorrection, t0); !errors.Is(err, ErrInvalid) {
		t.Errorf("no category: %v", err)
	}
	restaurants.Archived = true
	if _, err := NewRule(pattern(t, PatternContains, "ifood"), "", restaurants, OriginCorrection, t0); !errors.Is(err, ErrArchived) {
		t.Errorf("an archived category: %v", err)
	}
}

func TestRuleMatches(t *testing.T) {
	c := category(t, "Transport")
	everywhere, _ := NewRule(pattern(t, PatternContains, "uber"), "", c, OriginCorrection, t0)
	onCard, _ := NewRule(pattern(t, PatternExact, "Pix enviado"), card, c, OriginCorrection, t0)
	tests := []struct {
		rule        *Rule
		description string
		account     ledger.AccountID
		want        bool
	}{
		{everywhere, "UBER *TRIP", checking, true},
		{everywhere, "UBER *TRIP", card, true},
		{everywhere, "Uberlândia Shopping", card, false},
		{onCard, "Pix enviado", card, true},
		{onCard, "Pix enviado", checking, false}, // limited to the card
		{onCard, "Pix enviado Fulano", card, false},
	}
	for _, tt := range tests {
		if got := tt.rule.Matches(tt.description, tt.account); got != tt.want {
			t.Errorf("%q in %s: got %v", tt.description, tt.account, got)
		}
	}
}

func TestRuleRetarget(t *testing.T) {
	shopping, groceries := category(t, "Shopping"), category(t, "Groceries")
	r, _ := NewRule(pattern(t, PatternContains, "Pão de Açúcar"), "", shopping, OriginCorrection, t0)

	later := t0.Add(time.Hour)
	if changed, err := r.Retarget(groceries, later); err != nil || !changed || r.category != groceries.ID || !r.updatedAt.Equal(later) {
		t.Fatalf("a correction retargets the rule: %v %v %+v", changed, err, r)
	}
	if changed, err := r.Retarget(groceries, later.Add(time.Hour)); err != nil || changed || !r.updatedAt.Equal(later) {
		t.Fatalf("the same category changes nothing: %v %v %+v", changed, err, r)
	}
	shopping.Archived = true
	if _, err := r.Retarget(shopping, later); !errors.Is(err, ErrArchived) || r.category != groceries.ID {
		t.Fatalf("never to an archived category: %v %+v", err, r)
	}
}

func TestRuleOrigin(t *testing.T) {
	restaurants := category(t, "Restaurants")
	r, err := NewRule(pattern(t, PatternContains, "ifood"), "", restaurants, OriginHand, t0)
	if err != nil || r.Origin() != OriginHand {
		t.Fatalf("by hand: %v %v", r, err)
	}
	if _, err := NewRule(pattern(t, PatternContains, "ifood"), "", restaurants, Origin("model"), t0); !errors.Is(err, ErrInvalid) {
		t.Errorf("an unknown origin: %v", err)
	}
}

// A correction that meets a taught pattern retargets that rule; a rule
// written by hand never takes another over: "zz-test-rule!" is the rule
// ZZ TEST RULE, so writing it again is refused.
func TestTeach(t *testing.T) {
	other, groceries := category(t, "Other"), category(t, "Groceries")
	p := pattern(t, PatternContains, "zz-test-rule!")
	taught := func() *Rule {
		r, _ := NewRule(pattern(t, PatternContains, "ZZ TEST RULE"), "", other, OriginHand, t0)
		return r
	}
	later := t0.Add(time.Hour)
	tests := []struct {
		name    string
		had     *Rule
		c       Category
		origin  Origin
		err     error
		changed bool
		want    ledger.CategoryID
	}{
		{"a new rule by hand", nil, groceries, OriginHand, nil, true, groceries.ID},
		{"a new rule from a correction", nil, groceries, OriginCorrection, nil, true, groceries.ID},
		{"a correction retargets it", taught(), groceries, OriginCorrection, nil, true, groceries.ID},
		{"a correction to where it leads", taught(), other, OriginCorrection, nil, false, other.ID},
		{"by hand, its pattern is taken", taught(), groceries, OriginHand, ErrRuleTaken, false, ledger.CategoryID{}},
		{"by hand, even to where it leads", taught(), other, OriginHand, ErrRuleTaken, false, ledger.CategoryID{}},
	}
	for _, tt := range tests {
		var was ledger.CategoryID
		if tt.had != nil {
			was = tt.had.Category()
		}
		r, changed, err := Teach(tt.had, p, "", tt.c, tt.origin, later)
		if !errors.Is(err, tt.err) || changed != tt.changed {
			t.Errorf("%s: changed %v, %v; want %v, %v", tt.name, changed, err, tt.changed, tt.err)
			continue
		}
		if tt.err != nil {
			if r != nil || tt.had.Category() != was {
				t.Errorf("%s: a refused rule changed: %+v %+v", tt.name, r, tt.had)
			}
			continue
		}
		if r.Category() != tt.want || (tt.had != nil && r != tt.had) {
			t.Errorf("%s: %+v", tt.name, r)
		}
	}
}
