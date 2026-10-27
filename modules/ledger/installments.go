package ledger

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"time"

	"lastro/money"
)

// InstallmentRow is one installment as the ledger stores it, the input of
// GroupInstallments.
type InstallmentRow struct {
	ID          TransactionID
	Description string
	Installment Installment
	Amount      money.Money // negative: a purchase
	Date        time.Time
	Status      Status
}

// Purchase is one card purchase split into installments, as far as the
// ledger can tell: the source has no id for it (see GroupInstallments).
type Purchase struct {
	Description  string // without the "3/12" some banks append
	Total        int
	Amount       money.Money // of one installment, the latest one's
	Installments []InstallmentRow
}

// installmentSuffix is the " 3/12" Nubank appends to each installment.
var installmentSuffix = regexp.MustCompile(`\s*\d{1,3}/\d{1,3}\s*$`)

// GroupInstallments puts the installments of an account's purchases
// together. The source has no purchase id, and the purchase date it sends is
// each future installment's own date, so a purchase is the installments with
// the same description (without the "n/m" suffix), the same count and the
// same amount, within a cent per installment: the rounding left over when a
// price is split lands on the first one. An installment number appears once
// per purchase; a second "3 of 10" of the same shop and amount is another
// purchase. Purchases come out by their first installment's date.
func GroupInstallments(rows []InstallmentRow) []Purchase {
	sorted := slices.Clone(rows)
	slices.SortFunc(sorted, func(a, b InstallmentRow) int {
		return cmp.Or(
			strings.Compare(baseDescription(a.Description), baseDescription(b.Description)),
			cmp.Compare(a.Installment.total, b.Installment.total),
			cmp.Compare(a.Installment.number, b.Installment.number),
			a.Date.Compare(b.Date),
			strings.Compare(string(a.ID), string(b.ID)),
		)
	})
	var out []Purchase
	for _, r := range sorted {
		if r.Installment.IsZero() {
			continue
		}
		base := baseDescription(r.Description)
		i := slices.IndexFunc(out, func(p Purchase) bool {
			diff := p.Amount.Cents() - r.Amount.Cents()
			return p.Description == base && p.Total == r.Installment.total &&
				diff <= int64(p.Total) && diff >= -int64(p.Total) &&
				!slices.ContainsFunc(p.Installments, func(x InstallmentRow) bool { return x.Installment.number == r.Installment.number })
		})
		if i < 0 {
			out = append(out, Purchase{Description: base, Total: r.Installment.total})
			i = len(out) - 1
		}
		out[i].Installments = append(out[i].Installments, r)
		out[i].Amount = r.Amount
	}
	slices.SortStableFunc(out, func(a, b Purchase) int { return a.Installments[0].Date.Compare(b.Installments[0].Date) })
	return out
}

func baseDescription(s string) string {
	return strings.TrimSpace(installmentSuffix.ReplaceAllString(s, ""))
}

// Current is the number of the latest installment dated on or before today,
// zero when none is yet.
func (p Purchase) Current(today time.Time) int {
	day := civilDate(today)
	n := 0
	for _, r := range p.Installments {
		if !r.Date.After(day) && r.Installment.number > n {
			n = r.Installment.number
		}
	}
	return n
}

// Remaining is how many installments come after today's, and what they add
// up to at the latest installment's amount (as a positive amount).
func (p Purchase) Remaining(today time.Time) (int, money.Money) {
	n := p.Total - p.Current(today)
	return n, p.Amount.Abs().Times(int64(n))
}

// Last is the day of the last installment: the source's, when it sent it,
// else a month after the latest one known for each installment still
// missing.
func (p Purchase) Last() time.Time {
	latest := p.Installments[0]
	for _, r := range p.Installments {
		if r.Installment.number > latest.Installment.number {
			latest = r
		}
	}
	return latest.Date.AddDate(0, p.Total-latest.Installment.number, 0)
}

// InProgress reports whether installments of the purchase remain after today.
func (p Purchase) InProgress(today time.Time) bool {
	n, _ := p.Remaining(today)
	return n > 0
}
