//go:build integration

package categorize

import (
	"context"
	"errors"
	"testing"
	"time"

	"lastro/modules/ledger"
	"lastro/money"
)

// importOctober creates a checking account and imports the descriptions
// into it, each as an expense unless kinds says otherwise.
func importOctober(t *testing.T, s *Service, descriptions []string, kinds map[string]ledger.Kind) {
	t.Helper()
	ctx := context.Background()
	ext, _ := ledger.NewExternalID("acc-1")
	if _, err := s.ledger.SyncAccount(ctx, ledger.AccountDetails{
		ExternalID: ext, Name: "Conta", Institution: "Nubank", Type: ledger.AccountChecking,
		Balance: money.BRLCents(100000), BalanceAt: t0,
	}); err != nil {
		t.Fatal(err)
	}
	b := ledger.ImportBatch{Account: ext, From: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	for i, d := range descriptions {
		tx, _ := ledger.NewExternalID(d)
		kind := ledger.KindExpense
		if k, ok := kinds[d]; ok {
			kind = k
		}
		b.Transactions = append(b.Transactions, ledger.ImportedTransaction{
			ExternalID: tx, Date: time.Date(2026, 10, 10+i, 0, 0, 0, 0, time.UTC), Description: d,
			Amount: money.BRLCents(-1000), Kind: kind, Status: ledger.StatusPosted,
		})
	}
	if _, err := s.ledger.ImportTransactions(ctx, b); err != nil {
		t.Fatal(err)
	}
}

// categories returns each October transaction's category and source by its
// description, with an empty category for the ones still without one.
func categories(t *testing.T, s *Service) map[string]ledger.CategoryView {
	t.Helper()
	list, err := s.ledger.Transactions(context.Background(), ledger.TransactionFilter{Month: t0})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]ledger.CategoryView{}
	for _, v := range list.Transactions {
		if v.Category != nil {
			out[v.Description] = *v.Category
		} else {
			out[v.Description] = ledger.CategoryView{}
		}
	}
	return out
}

func learn(t *testing.T, s *Service, kind PatternKind, text string, c Category) {
	t.Helper()
	if err := s.Learn(context.Background(), pattern(t, kind, text), "", c.ID); err != nil {
		t.Fatal(err)
	}
}

func TestImportsAreCategorizedByTheRules(t *testing.T) {
	s := newService(t)
	cats := byName(t, s)
	learn(t, s, PatternContains, "ifood", cats["Restaurants"])
	learn(t, s, PatternContains, "transferencia", cats["Other"])

	importOctober(t, s, []string{"PAG*IFOOD SP", "Netflix.com", "Transferencia IFOOD"},
		map[string]ledger.Kind{"Transferencia IFOOD": ledger.KindTransfer})

	got := categories(t, s)
	if c := got["PAG*IFOOD SP"]; c.ID != cats["Restaurants"].ID.String() || c.Source != string(ledger.SourceRule) {
		t.Errorf("a rule meets it on import: %+v", c)
	}
	if c := got["Netflix.com"]; c.ID != "" {
		t.Errorf("no rule meets it: it waits in To review, got %+v", c)
	}
	if c := got["Transferencia IFOOD"]; c.ID != "" {
		t.Errorf("a transfer never gets a category, got %+v", c)
	}

	ctx := context.Background()
	in, _ := s.ledger.Transactions(ctx, ledger.TransactionFilter{Month: t0, Category: cats["Restaurants"].ID.String()})
	none, _ := s.ledger.Transactions(ctx, ledger.TransactionFilter{Month: t0, Category: ledger.CategoryNone})
	if len(in.Transactions) != 1 || len(none.Transactions) != 1 || none.Transactions[0].Description != "Netflix.com" {
		t.Errorf("filter by category, and by none (never a transfer): %+v / %+v", in.Transactions, none.Transactions)
	}
}

func TestCategorizeUncategorizedReachesWhatCameBefore(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	cats := byName(t, s)
	importOctober(t, s, []string{"PAG*IFOOD SP", "Netflix.com", "UBER *TRIP"}, nil)

	// Netflix is chosen by hand before any rule exists.
	list, _ := s.ledger.Transactions(ctx, ledger.TransactionFilter{Month: t0})
	for _, v := range list.Transactions {
		if v.Description == "Netflix.com" {
			if err := s.ledger.Categorize(ctx, v.ID, cats["Leisure"].ID, ledger.SourceManual); err != nil {
				t.Fatal(err)
			}
		}
	}
	learn(t, s, PatternContains, "ifood", cats["Restaurants"])
	learn(t, s, PatternContains, "netflix", cats["Other"])

	n, err := s.CategorizeUncategorized(ctx)
	if err != nil || n != 1 {
		t.Fatalf("only iFood gets a category: %d %v", n, err)
	}
	got := categories(t, s)
	if got["PAG*IFOOD SP"].ID != cats["Restaurants"].ID.String() {
		t.Errorf("the backlog meets the new rule: %+v", got["PAG*IFOOD SP"])
	}
	if c := got["Netflix.com"]; c.ID != cats["Leisure"].ID.String() || c.Source != string(ledger.SourceManual) {
		t.Errorf("a manual choice stays: %+v", c)
	}
	if got["UBER *TRIP"].ID != "" {
		t.Errorf("no rule meets Uber: %+v", got["UBER *TRIP"])
	}
	if n, err := s.CategorizeUncategorized(ctx); err != nil || n != 0 {
		t.Errorf("a second pass has nothing left to do: %d %v", n, err)
	}
}

func TestReviewQueue(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	cats := byName(t, s)
	importOctober(t, s, []string{"PAG*IFOOD SP", "PG *IFOOD", "Netflix.com", "DROGASIL 2104 SAO PAULO"}, nil)

	rv, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rv.Waiting != 4 || len(rv.Groups) != 3 || rv.Groups[0].Pattern.Text() != "IFOOD" || rv.Groups[0].Count != 2 {
		t.Fatalf("four waiting, iFood's two together first: %+v", rv)
	}

	n, err := s.Correct(ctx, rv.Groups[0].Pattern, "", cats["Restaurants"].ID)
	if err != nil || n != 2 {
		t.Fatalf("one correction settles the group: %d %v", n, err)
	}
	var netflix, drogasil ledger.TransactionID
	for _, g := range mustReview(t, s).Groups {
		for _, tx := range g.Transactions {
			switch tx.Description {
			case "Netflix.com":
				netflix = tx.ID
			case "DROGASIL 2104 SAO PAULO":
				drogasil = tx.ID
			}
		}
	}
	if n, err := s.SetCategory(ctx, netflix, cats["Leisure"].ID, nil); err != nil || n != 0 {
		t.Fatalf("a choice for one transaction: %d %v", n, err)
	}
	teach := pattern(t, PatternContains, "drogasil")
	if _, err := s.SetCategory(ctx, drogasil, cats["Health"].ID, &teach); err != nil {
		t.Fatal(err)
	}
	if rv := mustReview(t, s); rv.Waiting != 0 || rv.Groups != nil {
		t.Errorf("nothing left to review: %+v", rv)
	}
	got := categories(t, s)
	if c := got["Netflix.com"]; c.ID != cats["Leisure"].ID.String() || c.Source != string(ledger.SourceManual) {
		t.Errorf("my choice is manual: %+v", c)
	}
	if c := got["DROGASIL 2104 SAO PAULO"]; c.Source != string(ledger.SourceManual) {
		t.Errorf("the transaction I corrected stays manual even though it also taught a rule: %+v", c)
	}
	if c := got["PG *IFOOD"]; c.ID != cats["Restaurants"].ID.String() || c.Source != string(ledger.SourceRule) {
		t.Errorf("the group went by the rule: %+v", c)
	}

	if _, err := s.ArchiveCategory(ctx, cats["Shopping"].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetCategory(ctx, netflix, cats["Shopping"].ID, nil); !errors.Is(err, ErrArchived) {
		t.Errorf("never an archived category: %v", err)
	}
}

func mustReview(t *testing.T, s *Service) Review {
	t.Helper()
	rv, err := s.Review(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return rv
}
