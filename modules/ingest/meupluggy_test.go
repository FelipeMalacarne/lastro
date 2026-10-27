package ingest

import (
	"testing"
	"time"

	"lastro/modules/ingest/internal/fixtures"
	"lastro/modules/ledger"
)

// The anonymized dump of a real MeuPluggy connection (lastro fixtures):
// whatever the mapping and the matcher do with it is what they will do with
// my accounts.
func TestMeuPluggyFixtures(t *testing.T) {
	s, err := fixtures.Load("testdata/meupluggy")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Items) != 1 {
		t.Fatalf("want 1 item, got %d", len(s.Items))
	}
	it := s.Items[0]
	if got := institutionOf(it, s.Accounts[it.ID]); got != "Nubank" {
		t.Errorf("the MeuPluggy item is named after its bank: got %q", got)
	}

	at := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	var all []*ledger.Transaction
	kinds := map[ledger.Kind]int{}
	var pending, installments int
	for _, pa := range s.Accounts[it.ID] {
		d, err := mapAccount(pa, "Nubank", at)
		if err != nil {
			t.Fatalf("account %s: %v", pa.ID, err)
		}
		acc, err := ledger.NewAccount(d)
		if err != nil {
			t.Fatalf("account %s: %v", pa.ID, err)
		}
		// Nubank reports the card's limit and due day, and no closing day.
		_, _, hasLimit := d.Card.Limit()
		if d.Type.IsCard() && (!hasLimit || d.Card.Due().IsZero() || !d.Card.Closes().IsZero()) {
			t.Errorf("card details: %+v", d.Card)
		}
		var ins []ledger.ImportedTransaction
		for _, pt := range s.Transactions[pa.ID] {
			in, err := mapTransaction(pt, pa)
			if err != nil {
				t.Fatalf("transaction %s: %v", pt.ID, err)
			}
			ins = append(ins, in)
		}
		// Pluggy builds a date-only copy of 12 of the card's payments, which
		// Nubank reported with their time; one older copy has no twin.
		var dropped int
		ins, dropped = withoutBuiltBillPayments(ins)
		if want := map[bool]int{true: 12, false: 0}[d.Type.IsCard()]; dropped != want {
			t.Errorf("%s: dropped %d built bill payments, want %d", d.Type, dropped, want)
		}
		for _, in := range ins {
			tx, err := ledger.NewTransaction(acc, in)
			if err != nil {
				t.Fatalf("the ledger refuses %s: %v", in.ExternalID, err)
			}
			kinds[in.Kind]++
			if in.Status == ledger.StatusPending {
				pending++
			}
			if !in.Installment.IsZero() {
				installments++
			}
			all = append(all, tx)
		}
	}
	if len(all) != 549 || pending != 39 || installments != 59 {
		t.Errorf("got %d transactions, %d pending, %d installments", len(all), pending, installments)
	}
	// Every "Pagamento recebido" on the card is a bill payment, even those
	// without PAGAMENTO_FATURA: 23 once Pluggy's copies are left out. Moves to
	// and from my own investments are transfers: 131 by operation type and 22
	// more "Valor recebido de Investimentos" by description only. "Resgate de
	// Cashback" is income.
	if kinds[ledger.KindBillPayment] != 23 {
		t.Errorf("bill payments on the card: %d, want 23", kinds[ledger.KindBillPayment])
	}
	if kinds[ledger.KindTransfer] != 153 {
		t.Errorf("moves to and from my own investments: %d, want 153", kinds[ledger.KindTransfer])
	}

	// The 59 installments are 10 purchases, each a run of installments with
	// no number twice (see ledger.GroupInstallments): one of them has its last
	// installment written in capitals.
	var rows []ledger.InstallmentRow
	for _, tx := range all {
		if !tx.Installment().IsZero() {
			rows = append(rows, ledger.InstallmentRow{ID: tx.ID(), Description: tx.Description(), Installment: tx.Installment(), Amount: tx.Amount(), Date: tx.Date(), Status: tx.Status()})
		}
	}
	purchases := ledger.GroupInstallments(rows)
	if len(purchases) != 10 {
		t.Errorf("installments group into %d purchases, want 10", len(purchases))
	}
	for _, p := range purchases {
		first := p.Installments[0].Installment.Number()
		for i, r := range p.Installments {
			if r.Installment.Number() != first+i {
				t.Errorf("%s: installments out of order or with gaps: %+v", p.Description, p.Installments)
				break
			}
		}
	}

	paired := ledger.MatchTransfers(all) // the domain service, as the sync runs it
	bills := 0
	for _, tx := range paired {
		if tx.Kind() == ledger.KindBillPayment {
			bills++
		}
	}
	// The 22 bills paid from checking all pair with the card's payment, and
	// nothing else pairs (there is only one bank). The other 13 card payments
	// came from elsewhere and stay unpaired bill payments, never counted.
	if len(paired) != 44 || bills != 44 {
		t.Errorf("want the 22 bill payments paired on both sides, got %d paired, %d bill payments", len(paired), bills)
	}
}
