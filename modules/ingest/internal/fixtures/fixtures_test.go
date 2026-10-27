package fixtures

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testdata = "../../testdata/pluggy"

func TestAnonymizeKeepsRelationsAndHidesValues(t *testing.T) {
	a := newAnonymizer([]byte("test key"), 150)
	in := []byte(`[
	  {"id":"c0000000-0000-4000-8000-000000000001","accountId":"b0000000-0000-4000-8000-000000000011",
	   "description":"Pix enviado Joana Prado 12345678","amount":-1000.5,"type":"DEBIT","status":"POSTED",
	   "date":"2026-10-06T00:00:00.000Z","operationType":"PIX",
	   "paymentData":{"receiver":{"name":"Joana Prado","documentNumber":{"type":"CPF","value":"123.456.789-00"}}},
	   "creditCardMetadata":{"installmentNumber":3,"totalInstallments":10}},
	  {"id":"c0000000-0000-4000-8000-000000000002","accountId":"b0000000-0000-4000-8000-000000000011",
	   "description":"PIX RECEBIDO JOANA PRADO","amount":1000.5,"owner":"Joana Prado","taxNumber":"***.456.789-**"}
	]`)
	out, err := a.JSON(in)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, leaked := range []string{"Joana", "JOANA", "Prado", "123.456.789-00", "12345678", "1000.5", "c0000000-0000-4000-8000-000000000001", "456.789"} {
		if strings.Contains(s, leaked) {
			t.Errorf("anonymized output still contains %q:\n%s", leaked, s)
		}
	}
	var txs []map[string]any
	if err := json.Unmarshal(out, &txs); err != nil {
		t.Fatal(err)
	}
	if txs[0]["accountId"] != txs[1]["accountId"] {
		t.Error("the same account id must map to the same pseudonym")
	}
	if txs[0]["amount"] != -1500.75 || txs[1]["amount"] != 1500.75 {
		t.Errorf("amounts must be scaled by the same factor: %v %v", txs[0]["amount"], txs[1]["amount"])
	}
	for _, kept := range []string{"Pix enviado", "PIX RECEBIDO", "2026-10-06T00:00:00.000Z", `"PIX"`, `"installmentNumber": 3`, `"totalInstallments": 10`} {
		if !strings.Contains(s, kept) {
			t.Errorf("anonymized output lost %q:\n%s", kept, s)
		}
	}
	d0 := strings.Fields(txs[0]["description"].(string))
	d1 := strings.Fields(txs[1]["description"].(string))
	if !strings.EqualFold(d0[2], d1[2]) || !strings.EqualFold(d0[3], d1[3]) {
		t.Errorf("the same name must become the same pseudoword: %q and %q", d0, d1)
	}
}

func TestAnonymizeIsShapePreserving(t *testing.T) {
	a := newAnonymizer([]byte("k"), 100)
	if got := a.pseudo("c0000000-0000-4000-8000-000000000001"); !uuidRe.MatchString(got) {
		t.Errorf("a UUID must stay a UUID, got %q", got)
	}
	if got := a.pseudo("0001/9876543-2"); len(got) != len("0001/9876543-2") || got[4] != '/' || got[12] != '-' {
		t.Errorf("an account number must keep its shape, got %q", got)
	}
	if got := a.scale("0.1"); got != "0.1" {
		t.Errorf("factor 1 must keep the amount, got %q", got)
	}
}

func TestLoadAndReportFixtures(t *testing.T) {
	s, err := Load(testdata)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(s.Items))
	}
	var b bytes.Buffer
	Report(&b, s, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	out := b.String()
	for _, want := range []string{
		"Item 1 of 2: Nubank (Open Finance), status UPDATED / SUCCESS",
		"account 2: CREDIT/CREDIT_CARD, BRL, 7 transactions",
		"installments: 2 (totals seen: 10 2)",
		"inflow descriptions: ESTORNO * 1, PAGAMENTO RECEBIDO 1",
		"outflows that look like card bill payments: PAGAMENTO DE FATURA 1",
		"time of day: 2 at midnight UTC, 2 with a time",
		"investments: 1, by type: FIXED_INCOME/TREASURY ACTIVE 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report misses %q:\n%s", want, out)
		}
	}
	for _, leaked := range []string{"1482", "Fulano", "iFood", "c0000000"} {
		if strings.Contains(out, leaked) {
			t.Errorf("report leaks %q", leaked)
		}
	}
}

func TestAnonymizeDir(t *testing.T) {
	a := newAnonymizer([]byte("k"), 120)
	dst := t.TempDir()
	if err := a.Dir(testdata, dst); err != nil {
		t.Fatal(err)
	}
	s, err := Load(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(s.Items))
	}
	// Account files are named after the item: the names must follow the ids inside.
	for _, it := range s.Items {
		if len(s.Accounts[it.ID]) == 0 {
			t.Errorf("no accounts file for anonymized item %s", it.ID)
		}
		for _, acc := range s.Accounts[it.ID] {
			if _, ok := s.Transactions[acc.ID]; !ok {
				t.Errorf("no transactions file for anonymized account %s", acc.ID)
			}
		}
	}
	matches, _ := filepath.Glob(filepath.Join(dst, "*", "a0000000-*.json"))
	if len(matches) > 0 {
		t.Errorf("file names must be pseudonymized too: %v", matches)
	}
}
