package ledger

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"lastro/money"
)

func TestNewTag(t *testing.T) {
	for _, c := range []struct {
		in, want string
		ok       bool
	}{
		{"trip-rio", "trip-rio", true},
		{"  #Trip Rio ", "trip-rio", true},
		{"2026", "2026", true},
		{"", "", false},
		{"#", "", false},
		{"-rio", "", false},
		{"rio-", "", false},
		{"são-paulo", "são-paulo", true},
		{"#Educação", "educação", true},
		{"educac\u0327a\u0303o", "educação", true}, // typed decomposed, kept composed
		{"Ελλάδα-2026", "ελλάδα-2026", true},
		{"東京", "東京", true},
		{"viagem ✈️", "", false},
		{"🎉", "", false},
		{"a_b", "", false},
		{"a.b", "", false},
		{strings.Repeat("ç", 32), strings.Repeat("ç", 32), true},
		{strings.Repeat("a", 33), "", false},
	} {
		got, err := NewTag(c.in)
		if (err == nil) != c.ok || string(got) != c.want {
			t.Errorf("NewTag(%q) = %q, %v", c.in, got, err)
		}
		if err != nil && !errors.Is(err, ErrInvalid) {
			t.Errorf("NewTag(%q): want ErrInvalid, got %v", c.in, err)
		}
		var r reason
		if err != nil && (!errors.As(err, &r) || strings.Contains(string(r), "ledger")) {
			t.Errorf("NewTag(%q): the reason reads as a clause: %v", c.in, err)
		}
	}
}

func TestNewTags(t *testing.T) {
	got, err := NewTags([]string{"work", "#trip-rio", "Work"})
	if err != nil || !slices.Equal(got, []Tag{"trip-rio", "work"}) {
		t.Errorf("each once, sorted: %v %v", got, err)
	}
	many := make([]string, 11)
	for i := range many {
		many[i] = strings.Repeat("a", i+1)
	}
	if _, err := NewTags(many); !errors.Is(err, ErrInvalid) {
		t.Errorf("at most 10 tags: %v", err)
	}
}

func TestNewNote(t *testing.T) {
	if n, err := NewNote("  warranty until 2027  "); err != nil || n != "warranty until 2027" {
		t.Errorf("trimmed: %q %v", n, err)
	}
	if _, err := NewNote(strings.Repeat("é", 501)); !errors.Is(err, ErrInvalid) {
		t.Errorf("at most 500 characters: %v", err)
	}
}

func TestSplitInto(t *testing.T) {
	groceries, shopping, housing := mustCategory(t, "groceries"), mustCategory(t, "shopping"), mustCategory(t, "housing")
	part := func(c CategoryID, cents int64) SplitPart {
		return SplitPart{Category: c, Amount: money.BRLCents(cents)}
	}
	for _, c := range []struct {
		name  string
		parts []SplitPart
		ok    bool
	}{
		{"adds up", []SplitPart{part(groceries, -41230), part(shopping, -38990), part(housing, -5460)}, true},
		{"one part", []SplitPart{part(groceries, -85680)}, false},
		{"short by a cent", []SplitPart{part(groceries, -41230), part(shopping, -44449)}, false},
		{"a category twice", []SplitPart{part(groceries, -40000), part(groceries, -45680)}, false},
		{"a part the other way", []SplitPart{part(groceries, -95680), part(shopping, 10000)}, false},
		{"a zero part", []SplitPart{part(groceries, -85680), part(shopping, 0)}, false},
		{"no category", []SplitPart{part(groceries, -40000), part(CategoryID{}, -45680)}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			tx := newTx(t, checking(t), imported(t, "t1", -85680))
			s, err := tx.SplitInto(c.parts)
			if (err == nil) != c.ok {
				t.Fatalf("got %v", err)
			}
			if !c.ok {
				if !errors.Is(err, ErrInvalid) {
					t.Errorf("want ErrInvalid, got %v", err)
				}
				return
			}
			if len(s.Parts()) != 3 {
				t.Errorf("parts: %+v", s.Parts())
			}
			if cat, src := tx.Category(); cat != groceries || src != SourceManual {
				t.Errorf("the largest part is the category, chosen by me: %q %q", cat, src)
			}
		})
	}

	in := imported(t, "t2", 148237)
	in.Kind = KindBillPayment
	bill := newTx(t, checking(t), in)
	if _, err := bill.SplitInto([]SplitPart{part(groceries, 100000), part(shopping, 48237)}); !errors.Is(err, ErrInvalid) {
		t.Errorf("a card bill is never split: %v", err)
	}
}
