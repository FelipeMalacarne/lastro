package ledger

import (
	"fmt"
	"strings"
	"time"
)

// The read side (CQRS-lite): what the screens list, straight from SQL into
// these DTOs. They are also the JSON the API answers with.

// AccountView is an account as the screens show it.
type AccountView struct {
	ID           AccountID `json:"id"`
	Name         string    `json:"name"`
	Institution  string    `json:"institution"`
	Type         string    `json:"type"`
	Number       string    `json:"number"`
	Currency     string    `json:"currency"`
	BalanceCents int64     `json:"balanceCents"`
	BalanceAt    time.Time `json:"balanceAt"`
}

// TransactionView is a transaction as the Transactions screen lists it.
type TransactionView struct {
	ID          TransactionID    `json:"id"`
	Date        string           `json:"date"` // calendar day, "2026-10-26"
	Description string           `json:"description"`
	AmountCents int64            `json:"amountCents"`
	Currency    string           `json:"currency"`
	Kind        string           `json:"kind"`
	Status      string           `json:"status"`
	Installment *InstallmentView `json:"installment"`
	Category    *CategoryView    `json:"category"`
	Account     AccountRef       `json:"account"`
	// Counterpart is the other side's account, for transfers and bill payments.
	Counterpart *AccountRef `json:"counterpart"`
}

type InstallmentView struct {
	Number int `json:"number"`
	Total  int `json:"total"`
}

type CategoryView struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

// AccountRef names an account inside another view.
type AccountRef struct {
	ID          AccountID `json:"id"`
	Name        string    `json:"name"`
	Institution string    `json:"institution"`
	Type        string    `json:"type"`
}

// Flow narrows a list by what the money did.
type Flow string

const (
	FlowAll      Flow = ""
	FlowIn       Flow = "in"       // money in that counts: income, refunds
	FlowOut      Flow = "out"      // money out that counts: spending
	FlowTransfer Flow = "transfer" // transfers and card bill payments
)

// ParseFlow accepts "", "in", "out" and "transfer".
func ParseFlow(s string) (Flow, error) {
	switch f := Flow(s); f {
	case FlowAll, FlowIn, FlowOut, FlowTransfer:
		return f, nil
	}
	return "", fmt.Errorf("%w: flow %q", ErrInvalid, s)
}

// maxListed is how many transactions one month's list returns at most.
const maxListed = 2000

// TransactionFilter selects a month of transactions.
type TransactionFilter struct {
	Month   time.Time // any day in the month
	Account AccountID // empty: every account
	Flow    Flow
	// Search matches descriptions, ignoring case and accents.
	Search string
}

// MonthRange returns the first day of the filter's month and of the next.
func (f TransactionFilter) MonthRange() (from, to time.Time) {
	y, m, _ := f.Month.Date()
	from = time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	return from, from.AddDate(0, 1, 0)
}

// likePattern escapes the LIKE wildcards in a search term.
func likePattern(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(s))
}

// TransactionList is a month of transactions and what they add up to.
type TransactionList struct {
	Transactions []TransactionView `json:"transactions"`
	Summary      ListSummary       `json:"summary"`
	// Truncated is set when the month has more than the list returns.
	Truncated bool `json:"truncated"`
}

// ListSummary counts a list. In and out add only what counts: transfers and
// card bill payments are left out, as everywhere.
type ListSummary struct {
	Count    int   `json:"count"`
	Pending  int   `json:"pending"`
	InCents  int64 `json:"inCents"`
	OutCents int64 `json:"outCents"` // negative
}

func summarize(txs []TransactionView) ListSummary {
	s := ListSummary{Count: len(txs)}
	for _, t := range txs {
		if t.Status == string(StatusPending) {
			s.Pending++
		}
		if !Kind(t.Kind).Counts() {
			continue
		}
		if t.AmountCents > 0 {
			s.InCents += t.AmountCents
		} else {
			s.OutCents += t.AmountCents
		}
	}
	return s
}
