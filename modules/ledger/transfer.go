package ledger

import (
	"cmp"
	"slices"
	"time"
)

const (
	// transferWindow is how many days apart the two sides of a transfer
	// between bank accounts may be dated: a Pix lands the same day, a TED by
	// the next.
	transferWindow = 1
	// billPaymentWindow is the same for a card bill payment: the card may
	// post the payment a few days after the checking account debits it.
	billPaymentWindow = 3
)

// MatchTransfers is the TransferMatcher: among unpaired transactions, it
// pairs an outflow from one of my bank accounts with an inflow of the same
// amount in another of my accounts, dated close enough:
//
//   - into a card, the inflow must be one the card itself calls a bill
//     payment, within billPaymentWindow days; refunds never pair;
//   - between bank accounts, an expense with an income, within
//     transferWindow days; a wallet is only ever the inflow (a withdrawal).
//
// Closer dates pair first. When an outflow or an inflow has two equally
// close candidates in different accounts, it is left alone: guessing would
// hide real spending or income. It returns the transactions it paired.
func MatchTransfers(candidates []*Transaction) []*Transaction {
	type option struct {
		out, in *Transaction
		days    int
	}
	var outs, ins []*Transaction
	for _, t := range candidates {
		switch {
		case t.IsPaired() || t.IsRemoved() || t.amount.IsZero():
		// Money leaving a wallet is spending, never a transfer: only a
		// withdrawal into it pairs.
		case t.amount.IsNegative() && !t.accountType.IsCard() && t.accountType != AccountCash && t.sourceKind == KindExpense:
			outs = append(outs, t)
		case t.amount.IsPositive() && t.accountType.IsCard() && t.sourceKind == KindBillPayment,
			t.amount.IsPositive() && !t.accountType.IsCard() && t.sourceKind == KindIncome:
			ins = append(ins, t)
		}
	}

	var opts []option
	for _, o := range outs {
		for _, i := range ins {
			if i.accountID == o.accountID || i.amount != o.amount.Neg() {
				continue
			}
			window := transferWindow
			if i.accountType.IsCard() {
				window = billPaymentWindow
			}
			if d := daysApart(o.date, i.date); d <= window {
				opts = append(opts, option{out: o, in: i, days: d})
			}
		}
	}
	slices.SortFunc(opts, func(a, b option) int {
		return cmp.Or(cmp.Compare(a.days, b.days), a.out.date.Compare(b.out.date),
			cmp.Compare(a.out.id, b.out.id), cmp.Compare(a.in.id, b.in.id))
	})

	// ambiguous reports whether t has another free candidate, at the same
	// distance, in a different account than the one at hand.
	used := map[TransactionID]bool{}
	ambiguous := func(o option) bool {
		for _, x := range opts {
			if x.days != o.days || x == o || used[x.out.id] || used[x.in.id] {
				continue
			}
			if (x.out == o.out && x.in.accountID != o.in.accountID) || (x.in == o.in && x.out.accountID != o.out.accountID) {
				return true
			}
		}
		return false
	}
	var paired []*Transaction
	skipped := map[TransactionID]bool{}
	for _, o := range opts {
		if used[o.out.id] || used[o.in.id] || skipped[o.out.id] || skipped[o.in.id] {
			continue
		}
		if ambiguous(o) {
			skipped[o.out.id], skipped[o.in.id] = true, true
			continue
		}
		if err := Pair(o.out, o.in); err != nil {
			continue // cannot happen with the filters above; never pair on doubt
		}
		used[o.out.id], used[o.in.id] = true, true
		paired = append(paired, o.out, o.in)
	}
	return paired
}

func daysApart(a, b time.Time) int {
	d := int(civilDate(a).Sub(civilDate(b)).Hours() / 24)
	if d < 0 {
		return -d
	}
	return d
}
