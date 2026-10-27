package ledger

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"lastro/money"
)

// ErrInvalid wraps every value object constructor error, so callers can
// answer "bad input" with errors.Is(err, ledger.ErrInvalid).
var ErrInvalid = errors.New("ledger: invalid value")

// ExternalID is the id a source system gives an account or a transaction
// (today, Pluggy). The ledger only compares it, except to tell what I typed
// in, which it ids itself with manualPrefix.
type ExternalID struct{ v string }

// manualPrefix starts the external id of what I type in: accounts outside
// Open Finance and transactions I add by hand.
const manualPrefix = "manual-"

// newManualID is a fresh external id for something I type in.
func newManualID() ExternalID { return ExternalID{v: manualPrefix + newID()} }

// NewExternalID trims s and rejects an empty, overlong or multi-line id.
func NewExternalID(s string) (ExternalID, error) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 200 || strings.ContainsFunc(s, unicode.IsControl) {
		return ExternalID{}, fmt.Errorf("%w: external id %q", ErrInvalid, s)
	}
	return ExternalID{v: s}, nil
}

func (e ExternalID) String() string { return e.v }
func (e ExternalID) IsZero() bool   { return e.v == "" }

// IsManual reports whether I typed it in, rather than a source sending it.
func (e ExternalID) IsManual() bool { return strings.HasPrefix(e.v, manualPrefix) }

// Kind says what a movement is for the reports. Only expenses and income
// count; transfers between my own accounts and card bill payments never do,
// because the money did not leave me (and the card purchases already count).
type Kind string

const (
	KindExpense     Kind = "expense"
	KindIncome      Kind = "income"
	KindTransfer    Kind = "transfer"
	KindBillPayment Kind = "bill_payment"
)

// ParseKind accepts the spellings above.
func ParseKind(s string) (Kind, error) {
	switch k := Kind(s); k {
	case KindExpense, KindIncome, KindTransfer, KindBillPayment:
		return k, nil
	}
	return "", fmt.Errorf("%w: kind %q", ErrInvalid, s)
}

func (k Kind) String() string { return string(k) }

// Counts reports whether the kind enters spending and income totals.
func (k Kind) Counts() bool { return k == KindExpense || k == KindIncome }

// Installment is "3 of 10": one installment of a purchase split across
// bills. The zero value means the transaction is not an installment.
type Installment struct{ number, total int }

// NewInstallment requires 1 ≤ number ≤ total and 2 ≤ total ≤ 999: a single
// payment is not an installment.
func NewInstallment(number, total int) (Installment, error) {
	if total < 2 || total > 999 || number < 1 || number > total {
		return Installment{}, fmt.Errorf("%w: installment %d of %d", ErrInvalid, number, total)
	}
	return Installment{number: number, total: total}, nil
}

func (i Installment) Number() int  { return i.number }
func (i Installment) Total() int   { return i.total }
func (i Installment) IsZero() bool { return i.total == 0 }

func (i Installment) String() string {
	if i.IsZero() {
		return ""
	}
	return fmt.Sprintf("%d of %d", i.number, i.total)
}

// CategoryID identifies a category of the categorize module. The ledger
// stores it with the transaction and never looks inside.
type CategoryID struct{ v string }

// NewCategoryID trims s and rejects an empty id, an id longer than 64
// characters or one with spaces inside.
func NewCategoryID(s string) (CategoryID, error) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 64 || strings.ContainsFunc(s, unicode.IsSpace) {
		return CategoryID{}, fmt.Errorf("%w: category id %q", ErrInvalid, s)
	}
	return CategoryID{v: s}, nil
}

func (c CategoryID) String() string { return c.v }
func (c CategoryID) IsZero() bool   { return c.v == "" }

// Source says who chose a transaction's category.
type Source string

const (
	SourceRule   Source = "rule"
	SourceLLM    Source = "llm"
	SourceManual Source = "manual"
)

// ParseSource accepts the spellings above.
func ParseSource(s string) (Source, error) {
	switch src := Source(s); src {
	case SourceRule, SourceLLM, SourceManual:
		return src, nil
	}
	return "", fmt.Errorf("%w: category source %q", ErrInvalid, s)
}

func (s Source) String() string { return string(s) }

// CanReplace reports whether a category chosen by s may replace one chosen
// by current. A manual choice replaces anything; automation never replaces a
// manual choice. An empty current means no category yet.
func (s Source) CanReplace(current Source) bool {
	return s == SourceManual || current != SourceManual
}

// Status says whether the bank has settled a movement. Pending ones may still
// change or disappear.
type Status string

const (
	StatusPending Status = "pending"
	StatusPosted  Status = "posted"
)

// ParseStatus accepts the spellings above.
func ParseStatus(s string) (Status, error) {
	switch st := Status(s); st {
	case StatusPending, StatusPosted:
		return st, nil
	}
	return "", fmt.Errorf("%w: status %q", ErrInvalid, s)
}

func (s Status) String() string { return string(s) }

// AccountType is what kind of account the ledger keeps. Investments are not
// accounts: they are positions in portfolio.
type AccountType string

const (
	AccountChecking   AccountType = "checking"
	AccountSavings    AccountType = "savings"
	AccountCreditCard AccountType = "credit_card"
	// A wallet and an asset are mine to type in: Open Finance has neither.
	AccountCash  AccountType = "cash"
	AccountAsset AccountType = "asset"
)

// ParseAccountType accepts the spellings above.
func ParseAccountType(s string) (AccountType, error) {
	switch t := AccountType(s); t {
	case AccountChecking, AccountSavings, AccountCreditCard, AccountCash, AccountAsset:
		return t, nil
	}
	return "", fmt.Errorf("%w: account type %q", ErrInvalid, s)
}

func (t AccountType) String() string { return string(t) }
func (t AccountType) IsCard() bool   { return t == AccountCreditCard }

// IsManual is whether the account is typed in by me, outside Open Finance.
func (t AccountType) IsManual() bool { return t == AccountCash || t == AccountAsset }

// CardDetails is what the bank says about a card's limit and its bill, as of
// the balance it came with. Banks share different parts: each one is
// optional, and the zero value knows nothing.
type CardDetails struct {
	limit, available money.Money
	hasLimit         bool
	due, closes      time.Time // calendar days; zero when not reported
}

// NewCardDetails takes the limit and what is still available (nil when the
// bank does not report them, which it does together) and the bill's due and
// closing days (zero when not reported). The limit is never negative; the
// available amount can be, on a card over its limit.
func NewCardDetails(limit, available *money.Money, due, closes time.Time) (CardDetails, error) {
	var c CardDetails
	if (limit == nil) != (available == nil) {
		return CardDetails{}, fmt.Errorf("%w: a card limit comes with what is available", ErrInvalid)
	}
	if limit != nil {
		if limit.IsNegative() || limit.Currency() != available.Currency() {
			return CardDetails{}, fmt.Errorf("%w: card limit %s, available %s", ErrInvalid, limit, available)
		}
		c.limit, c.available, c.hasLimit = *limit, *available, true
	}
	if !due.IsZero() {
		c.due = civilDate(due)
	}
	if !closes.IsZero() {
		c.closes = civilDate(closes)
	}
	return c, nil
}

// Limit returns the limit and what is available, when the bank reports them.
func (c CardDetails) Limit() (limit, available money.Money, ok bool) {
	return c.limit, c.available, c.hasLimit
}

// Due and Closes are the bill's due and closing days, zero when unknown.
func (c CardDetails) Due() time.Time    { return c.due }
func (c CardDetails) Closes() time.Time { return c.closes }
func (c CardDetails) IsZero() bool      { return c == CardDetails{} }
