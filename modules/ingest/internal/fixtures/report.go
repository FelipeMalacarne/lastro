package fixtures

import (
	"cmp"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode"

	"lastro/modules/ingest/internal/pluggy"
)

// Report prints what a dump contains in aggregate: counts, date ranges,
// statuses, enum values and description shapes built only from banking
// vocabulary. It never prints an amount, a name, an id or a free-text word,
// so the output is safe to share.
func Report(w io.Writer, s Set, now time.Time) {
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format+"\n", args...) }
	p("Pluggy dump report: aggregates only, safe to share.")
	for i, it := range s.Items {
		of := "direct connector"
		if it.Connector.IsOpenFinance {
			of = "Open Finance"
		}
		p("")
		p("Item %d of %d: %s (%s), status %s / %s", i+1, len(s.Items), it.Connector.Name, of, it.Status, it.ExecutionStatus)
		if it.Error != nil && it.Error.Code != "" {
			p("  error code: %s", it.Error.Code)
		}
		p("  data last updated by Pluggy: %s; authorization expires: %s", day(it.LastUpdatedAt), day(it.ConsentExpiresAt))
		for j, a := range s.Accounts[it.ID] {
			reportAccount(p, j+1, a, s.Transactions[a.ID], now)
			if a.Type == "CREDIT" {
				bills, read := s.Bills[a.ID]
				reportBills(p, bills, read, s.Transactions[a.ID])
			}
		}
		invs := s.Investments[it.ID]
		kinds := map[string]int{}
		for _, inv := range invs {
			kinds[inv.Type+"/"+cmp.Or(inv.Subtype, "-")+" "+cmp.Or(inv.Status, "-")]++
		}
		p("  investments: %d, by type:%s", len(invs), histogram(kinds))
		reportInvestmentTransactions(p, invs, s.InvestmentTransactions)
	}
}

// reportInvestmentTransactions says whether the movements behind the
// investments came through: how many, of which types, over which dates,
// and whether the fully withdrawn ones carry the withdrawal.
func reportInvestmentTransactions(p func(string, ...any), invs []pluggy.Investment, byInvestment map[string][]pluggy.InvestmentTransaction) {
	types, movements := map[string]int{}, map[string]int{}
	var n, with, closed, closedWithSell, net, expenses int
	var first, last string
	for _, inv := range invs {
		txs := byInvestment[inv.ID]
		if len(txs) > 0 {
			with++
		}
		sold := false
		for _, t := range txs {
			n++
			types[cmp.Or(t.Type, "(none)")]++
			movements[cmp.Or(t.MovementType, "(none)")]++
			d := day(t.Date)
			if first == "" || d < first {
				first = d
			}
			if d > last {
				last = d
			}
			if t.NetAmount != "" {
				net++
			}
			if len(t.Expenses) > 0 && string(t.Expenses) != "null" {
				expenses++
			}
			sold = sold || t.Type == "SELL"
		}
		if inv.Status == "TOTAL_WITHDRAWAL" {
			closed++
			if sold {
				closedWithSell++
			}
		}
	}
	p("  investment transactions: %d, on %d of %d investments", n, with, len(invs))
	if n == 0 {
		return
	}
	p("    dates: %s to %s", first, last)
	p("    by type:%s; movement types:%s", histogram(types), histogram(movements))
	p("    with netAmount: %d; with expenses: %d", net, expenses)
	p("    fully withdrawn investments with a SELL: %d of %d", closedWithSell, closed)
}

func reportAccount(p func(string, ...any), n int, a pluggy.Account, txs []pluggy.Transaction, now time.Time) {
	p("  account %d: %s/%s, %s, %d transactions", n, a.Type, a.Subtype, a.CurrencyCode, len(txs))
	if len(txs) == 0 {
		return
	}
	var first, last string
	status, ops, inflowOps, shapesIn, shapesOut, currencies := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	var future, midnight, withTime, withProviderID, installments, withBill, inAccountCurrency int
	totals := map[string]int{}
	for _, t := range txs {
		d := day(t.Date)
		if first == "" || d < first {
			first = d
		}
		if d > last {
			last = d
		}
		if d > now.Format(time.DateOnly) {
			future++
		}
		if strings.HasSuffix(t.Date, "T00:00:00.000Z") || strings.HasSuffix(t.Date, "T00:00:00Z") {
			midnight++
		} else if strings.Contains(t.Date, "T") {
			withTime++
		}
		status[t.Status]++
		ops[cmp.Or(t.OperationType, "(none)")]++
		if t.ProviderID != "" {
			withProviderID++
		}
		if t.CurrencyCode != a.CurrencyCode {
			currencies[t.CurrencyCode]++
		}
		if t.AmountInAccountCurrency != "" {
			inAccountCurrency++
		}
		if m := t.CreditCardMetadata; m != nil {
			if m.TotalInstallments > 1 {
				installments++
				totals[fmt.Sprint(m.TotalInstallments)]++
			}
			if m.BillID != "" {
				withBill++
			}
		}
		if t.Type == "CREDIT" {
			inflowOps[cmp.Or(t.OperationType, "(none)")]++
			if a.Type == "CREDIT" {
				shapesIn[shape(t.Description)]++
			}
		} else if a.Type == "BANK" && (strings.Contains(shape(t.Description), "FATURA") || strings.Contains(shape(t.Description), "CARTAO")) {
			shapesOut[shape(t.Description)]++
		}
	}
	p("    dates: %s to %s; %d dated after today", first, last, future)
	p("    time of day: %d at midnight UTC, %d with a time, %d plain dates", midnight, withTime, len(txs)-midnight-withTime)
	p("    status:%s", histogram(status))
	p("    with the bank's own id (providerId): %d of %d", withProviderID, len(txs))
	p("    operation types:%s", histogram(ops))
	p("    inflows by operation type:%s", histogram(inflowOps))
	if len(currencies) > 0 || inAccountCurrency > 0 {
		p("    other currencies:%s; with amountInAccountCurrency: %d", histogram(currencies), inAccountCurrency)
	}
	if a.Type == "CREDIT" {
		p("    installments: %d (totals seen:%s); with billId: %d", installments, histogram(totals), withBill)
		p("    inflow descriptions:%s", histogram(shapesIn))
	} else if len(shapesOut) > 0 {
		p("    outflows that look like card bill payments:%s", histogram(shapesOut))
	}
}

// reportBills says whether the card's closed bills came through and how its
// transactions point to them: what the open bill is made of.
func reportBills(p func(string, ...any), bills []pluggy.Bill, read bool, txs []pluggy.Transaction) {
	if !read {
		p("    bills: not read (the endpoint failed for this card)")
		return
	}
	ids := map[string]bool{}
	var first, last string
	var minimum, charges int
	for _, b := range bills {
		ids[b.ID] = true
		d := day(b.DueDate)
		if first == "" || d < first {
			first = d
		}
		if d > last {
			last = d
		}
		if b.MinimumPaymentAmount != "" {
			minimum++
		}
		if len(b.FinanceCharges) > 0 {
			charges++
		}
	}
	p("    bills: %d, due %s to %s; with a minimum: %d; with interest or fees: %d", len(bills), cmp.Or(first, "-"), cmp.Or(last, "-"), minimum, charges)
	ref := map[string]int{}
	for _, t := range txs {
		state := "no billId"
		if m := t.CreditCardMetadata; m != nil && m.BillID != "" {
			state = "billId of an unlisted bill"
			if ids[m.BillID] {
				state = "billId of a listed bill"
			}
		}
		ref[t.Status+", "+state]++
	}
	p("    transactions by status and bill:%s", histogram(ref))
}

// shape keeps only banking vocabulary: other words become "*" and digits "#".
func shape(desc string) string {
	var out []string
	for _, w := range strings.FieldsFunc(desc, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		switch f := fold(w); {
		case isDigits(f):
			out = append(out, "#")
		case vocabulary[f]:
			out = append(out, f)
		default:
			out = append(out, "*")
		}
	}
	return strings.Join(slices.Compact(out), " ")
}

// histogram formats counts as " A 3, B 1", most frequent first, at most 8 entries.
func histogram(m map[string]int) string {
	if len(m) == 0 {
		return " none"
	}
	keys := slices.SortedFunc(maps.Keys(m), func(a, b string) int {
		return cmp.Or(cmp.Compare(m[b], m[a]), cmp.Compare(a, b))
	})
	var parts []string
	for i, k := range keys {
		if i == 8 {
			parts = append(parts, fmt.Sprintf("and %d more", len(keys)-8))
			break
		}
		parts = append(parts, fmt.Sprintf("%s %d", k, m[k]))
	}
	return " " + strings.Join(parts, ", ")
}

func day(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return cmp.Or(ts, "unknown")
}
