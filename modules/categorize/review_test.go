package categorize

import (
	"testing"
	"time"

	"lastro/modules/ledger"
)

func TestGroupForReview(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	tx := func(id, description string, d int, cents int64) ledger.Categorizable {
		return ledger.Categorizable{ID: ledger.TransactionID(id), Account: checking, Description: description, Date: day(d), AmountCents: cents}
	}
	groups := GroupForReview([]ledger.Categorizable{
		tx("a", "PAG*IFOOD SP", 3, -4890),
		tx("b", "DROGASIL 2104 SAO PAULO", 25, -8735),
		tx("c", "PG *IFOOD", 20, -3100),
		tx("d", "Netflix.com", 18, -4490),
		tx("e", "!!", 1, -100),
		tx("f", "Pag*iFood RJ", 12, -2500),
	})
	if len(groups) != 4 {
		t.Fatalf("want 4 groups, got %+v", groups)
	}
	ifood := groups[0]
	if ifood.Pattern.Text() != "IFOOD" || ifood.Count != 3 || ifood.TotalCents != -10490 || !ifood.Latest.Equal(day(20)) {
		t.Errorf("the biggest group first, with its total and latest day: %+v", ifood)
	}
	if ifood.Transactions[0].ID != "c" || ifood.Transactions[2].ID != "a" {
		t.Errorf("newest first: %+v", ifood.Transactions)
	}
	if groups[1].Pattern.Text() != "DROGASIL" || groups[2].Pattern.Text() != "NETFLIX" {
		t.Errorf("then the most recent: %s, %s", groups[1].Pattern.Text(), groups[2].Pattern.Text())
	}
	if !groups[3].Pattern.IsZero() || groups[3].Transactions[0].ID != "e" {
		t.Errorf("a description too short for a pattern stays alone: %+v", groups[3])
	}
	if GroupForReview(nil) != nil {
		t.Error("nothing waiting, no groups")
	}
}
