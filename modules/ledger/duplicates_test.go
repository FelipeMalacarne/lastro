package ledger

import (
	"errors"
	"testing"
)

// added is a transaction I typed into a synced account.
func added(t *testing.T, acc *Account, day int, cents int64) *Transaction {
	t.Helper()
	return tx(t, acc, "manual-"+newID(), day, cents, KindExpense)
}

func TestPossibleDuplicate(t *testing.T) {
	acc, other := checking(t), account(t, "acc-2", AccountSavings)
	wallet := account(t, "manual-wallet", AccountCash)
	removed := tx(t, acc, "gone", 12, -179000, KindExpense)
	removed.Remove(oct(20))
	tests := []struct {
		name           string
		manual, synced *Transaction
		want           error
	}{
		{"same day", added(t, acc, 10, -179000), tx(t, acc, "t1", 10, -179000, KindExpense), nil},
		{"the bank dates it 3 days later", added(t, acc, 10, -179000), tx(t, acc, "t1", 13, -179000, KindExpense), nil},
		{"3 days earlier", added(t, acc, 10, -179000), tx(t, acc, "t1", 7, -179000, KindExpense), nil},
		{"4 days apart", added(t, acc, 10, -179000), tx(t, acc, "t1", 14, -179000, KindExpense), ErrNotDuplicate},
		{"another amount", added(t, acc, 10, -179000), tx(t, acc, "t1", 10, -178999, KindExpense), ErrNotDuplicate},
		{"another account", added(t, acc, 10, -179000), tx(t, other, "t1", 10, -179000, KindExpense), ErrNotDuplicate},
		{"two synced ones", tx(t, acc, "t0", 10, -179000, KindExpense), tx(t, acc, "t1", 10, -179000, KindExpense), ErrNotDuplicate},
		{"two I added", added(t, acc, 10, -179000), added(t, acc, 10, -179000), ErrNotDuplicate},
		{"a wallet entry", added(t, wallet, 10, -179000), tx(t, wallet, "t1", 10, -179000, KindExpense), ErrNotDuplicate},
		{"a removed one", added(t, acc, 12, -179000), removed, ErrRemoved},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := PossibleDuplicate(tt.manual, tt.synced); !errors.Is(err, tt.want) {
				t.Errorf("PossibleDuplicate = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestMerge(t *testing.T) {
	gifts, food := mustCategory(t, "gifts"), mustCategory(t, "food")
	acc := checking(t)

	t.Run("the synced one takes what I gave mine", func(t *testing.T) {
		m, s := added(t, acc, 10, -179000), tx(t, acc, "t1", 11, -179000, KindExpense)
		if err := m.Categorize(gifts, SourceManual); err != nil {
			t.Fatal(err)
		}
		if err := s.Categorize(food, SourceRule); err != nil {
			t.Fatal(err)
		}
		if err := m.MarkTransfer(true); err != nil {
			t.Fatal(err)
		}
		mine := Annotations{Note: "rent for Oct", Tags: []Tag{"home", "rent"}, Split: []SplitPart{{gifts, m.Amount()}}}
		theirs := Annotations{Tags: []Tag{"pix"}}
		got, broken, err := Merge(m, s, mine, theirs, oct(27))
		if err != nil {
			t.Fatal(err)
		}
		if c, src := s.Category(); c != gifts || src != SourceManual {
			t.Errorf("category %v by %s, want mine", c, src)
		}
		if !s.MarkedTransfer() || !m.IsRemoved() || broken != "" {
			t.Errorf("marked %v, mine removed %v, broken %q", s.MarkedTransfer(), m.IsRemoved(), broken)
		}
		if got.Note != "rent for Oct" || len(got.Tags) != 3 || got.Tags[0] != "home" || len(got.Split) != 1 {
			t.Errorf("annotations %+v", got)
		}
	})

	t.Run("a category I chose on the synced one stays", func(t *testing.T) {
		m, s := added(t, acc, 10, -179000), tx(t, acc, "t1", 10, -179000, KindExpense)
		_ = m.Categorize(gifts, SourceManual)
		_ = s.Categorize(food, SourceManual)
		got, _, err := Merge(m, s, Annotations{Note: "mine", Split: []SplitPart{{gifts, m.Amount()}}}, Annotations{Note: "theirs"}, oct(27))
		if err != nil {
			t.Fatal(err)
		}
		if c, _ := s.Category(); c != food || got.Note != "theirs" || len(got.Split) != 0 {
			t.Errorf("category %v, annotations %+v", c, got)
		}
	})

	t.Run("not a possible duplicate", func(t *testing.T) {
		m, s := added(t, acc, 10, -179000), tx(t, acc, "t1", 20, -179000, KindExpense)
		if _, _, err := Merge(m, s, Annotations{}, Annotations{}, oct(27)); !errors.Is(err, ErrNotDuplicate) || m.IsRemoved() {
			t.Errorf("Merge = %v, removed %v", err, m.IsRemoved())
		}
	})
}
