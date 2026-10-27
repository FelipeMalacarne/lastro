package ledger

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"lastro/money"
	"lastro/platform/events"
)

var (
	// ErrManualCategory means automation tried to replace a category I chose.
	ErrManualCategory = errors.New("ledger: category was chosen manually")
	// ErrRemoved means the transaction no longer exists at the source.
	ErrRemoved = errors.New("ledger: transaction removed")
	// ErrPaired means the transaction already has a counterpart.
	ErrPaired = errors.New("ledger: transaction already paired")
)

const maxDescription = 500

// Transaction is one movement in one of my accounts. Its amount is negative
// when money leaves the account, on checking and on cards alike.
//
// Its kind is what the source said (an expense, an income, a card bill
// payment) until the TransferMatcher pairs it with the other side of a
// transfer or a bill payment; while paired, the pair decides the kind. If the
// pair breaks, the source's kind comes back.
type Transaction struct {
	id          TransactionID
	accountID   AccountID
	accountType AccountType
	externalID  ExternalID
	bankRef     string
	date        time.Time
	description string
	amount      money.Money
	sourceKind  Kind
	status      Status
	installment Installment
	category    CategoryID
	categorySrc Source
	counterpart TransactionID
	pairKind    Kind
	removedAt   time.Time

	events []events.Event
}

// ImportedTransaction is what a source says about a transaction.
type ImportedTransaction struct {
	ExternalID ExternalID
	// BankRef is the bank's own id for the transaction, when the source has
	// one. It survives when the source deletes and recreates a transaction
	// with a new ExternalID, so it is how the ledger recognizes it.
	BankRef     string
	Date        time.Time // the calendar day is what counts
	Description string
	Amount      money.Money
	Kind        Kind
	Status      Status
	Installment Installment
}

// NewTransaction creates a transaction in the account.
func NewTransaction(acc *Account, in ImportedTransaction) (*Transaction, error) {
	if err := in.validate(acc.currency); err != nil {
		return nil, err
	}
	t := &Transaction{
		id:          TransactionID(newID()),
		accountID:   acc.id,
		accountType: acc.typ,
	}
	t.apply(in)
	return t, nil
}

// ApplyImport applies what the source says now and reports whether anything
// changed. A removed transaction the source returns again comes back. If the
// amount changed while paired, the pair no longer holds: the transaction
// leaves it and returns its counterpart, which must leave it too.
func (t *Transaction) ApplyImport(in ImportedTransaction) (changed bool, broken TransactionID, err error) {
	if err := in.validate(t.amount.Currency()); err != nil {
		return false, "", err
	}
	before := *t
	if t.IsPaired() && in.Amount != t.amount {
		broken = t.counterpart
		t.LeavePair()
	}
	t.apply(in)
	t.removedAt = time.Time{}
	return t.differs(before), broken, nil
}

func (t *Transaction) apply(in ImportedTransaction) {
	t.externalID = in.ExternalID
	if ref := strings.TrimSpace(in.BankRef); ref != "" {
		t.bankRef = ref
	}
	t.date = civilDate(in.Date)
	t.description = truncate(strings.TrimSpace(in.Description), maxDescription)
	t.amount = in.Amount
	t.sourceKind = in.Kind
	t.status = in.Status
	t.installment = in.Installment
}

func (t *Transaction) differs(o Transaction) bool {
	return t.externalID != o.externalID || t.bankRef != o.bankRef || !t.date.Equal(o.date) ||
		t.description != o.description || t.amount != o.amount || t.sourceKind != o.sourceKind ||
		t.status != o.status || t.installment != o.installment || t.counterpart != o.counterpart ||
		!t.removedAt.Equal(o.removedAt)
}

func (in ImportedTransaction) validate(currency money.Currency) error {
	switch {
	case in.ExternalID.IsZero():
		return fmt.Errorf("%w: transaction without an external id", ErrInvalid)
	case in.Date.IsZero():
		return fmt.Errorf("%w: transaction %s without a date", ErrInvalid, in.ExternalID)
	case strings.TrimSpace(in.Description) == "":
		return fmt.Errorf("%w: transaction %s without a description", ErrInvalid, in.ExternalID)
	case in.Amount.Currency() != currency:
		return fmt.Errorf("%w: transaction %s is in %q, its account in %s", ErrCurrencyChanged, in.ExternalID, in.Amount.Currency(), currency)
	}
	if _, err := ParseKind(string(in.Kind)); err != nil {
		return err
	}
	if _, err := ParseStatus(string(in.Status)); err != nil {
		return err
	}
	return nil
}

// Remove marks the transaction as gone from the source: pending ones that
// never settled, or the old half of a delete-and-recreate. It stays stored,
// out of every list and total, in case it comes back. If it was paired, it
// leaves the pair and returns the counterpart, which must leave it too.
func (t *Transaction) Remove(at time.Time) (broken TransactionID) {
	if t.IsPaired() {
		broken = t.counterpart
		t.LeavePair()
	}
	if t.removedAt.IsZero() {
		t.removedAt = at
	}
	return broken
}

// Categorize sets the category. Automation never replaces a manual choice
// (ErrManualCategory); setting the category it already has is a no-op.
func (t *Transaction) Categorize(c CategoryID, src Source) error {
	if t.IsRemoved() {
		return fmt.Errorf("%w: %s", ErrRemoved, t.id)
	}
	if c.IsZero() {
		return fmt.Errorf("%w: empty category", ErrInvalid)
	}
	if _, err := ParseSource(string(src)); err != nil {
		return err
	}
	if !src.CanReplace(t.categorySrc) {
		return fmt.Errorf("%w: %s", ErrManualCategory, t.id)
	}
	if t.category == c && t.categorySrc == src {
		return nil
	}
	t.category, t.categorySrc = c, src
	t.events = append(t.events, TransactionCategorized{TransactionID: t.id, Category: c, Source: src})
	return nil
}

// Uncategorize takes back a category automation gave, for a rule that no
// longer meets the transaction. A manual choice is never taken back.
func (t *Transaction) Uncategorize() error {
	if t.categorySrc == SourceManual {
		return fmt.Errorf("%w: %s", ErrManualCategory, t.id)
	}
	t.category, t.categorySrc = CategoryID{}, ""
	return nil
}

// Restore puts back the category a transaction had before a choice I
// undo, whoever made it, a manual one included. An empty category leaves it
// waiting for one again.
func (t *Transaction) Restore(c CategoryID, src Source) error {
	if t.IsRemoved() {
		return fmt.Errorf("%w: %s", ErrRemoved, t.id)
	}
	if c.IsZero() {
		t.category, t.categorySrc = CategoryID{}, ""
		return nil
	}
	if _, err := ParseSource(string(src)); err != nil {
		return err
	}
	if t.category == c && t.categorySrc == src {
		return nil
	}
	t.category, t.categorySrc = c, src
	t.events = append(t.events, TransactionCategorized{TransactionID: t.id, Category: c, Source: src})
	return nil
}

// Pair links the two sides of a movement between my own accounts: out
// leaves one account and in arrives at another, with the same amount. Into
// a card it is a bill payment; between bank accounts, a transfer. While
// paired, neither counts as spending or income.
func Pair(out, in *Transaction) error {
	switch {
	case out.IsRemoved() || in.IsRemoved():
		return fmt.Errorf("%w: cannot pair %s and %s", ErrRemoved, out.id, in.id)
	case out.IsPaired() || in.IsPaired():
		return fmt.Errorf("%w: %s or %s", ErrPaired, out.id, in.id)
	case out.accountID == in.accountID:
		return fmt.Errorf("%w: %s and %s are in the same account", ErrInvalid, out.id, in.id)
	case !out.amount.IsNegative() || !in.amount.IsPositive() || out.amount.Neg() != in.amount:
		return fmt.Errorf("%w: %s (%s) and %s (%s) are not the two sides of one movement", ErrInvalid, out.id, out.amount, in.id, in.amount)
	case out.accountType.IsCard():
		return fmt.Errorf("%w: money does not move out of a card into my accounts", ErrInvalid)
	}
	kind := KindTransfer
	if in.accountType.IsCard() {
		kind = KindBillPayment
	}
	out.counterpart, out.pairKind = in.id, kind
	in.counterpart, in.pairKind = out.id, kind
	return nil
}

// LeavePair unlinks this side of a pair; the kind goes back to the source's.
func (t *Transaction) LeavePair() { t.counterpart, t.pairKind = "", "" }

// PullEvents returns the events recorded since the last call and forgets them.
func (t *Transaction) PullEvents() []events.Event {
	evs := t.events
	t.events = nil
	return evs
}

// Kind is the effective kind: the pair's while paired, the source's otherwise.
func (t *Transaction) Kind() Kind {
	if t.IsPaired() {
		return t.pairKind
	}
	return t.sourceKind
}

func (t *Transaction) ID() TransactionID              { return t.id }
func (t *Transaction) AccountID() AccountID           { return t.accountID }
func (t *Transaction) ExternalID() ExternalID         { return t.externalID }
func (t *Transaction) BankRef() string                { return t.bankRef }
func (t *Transaction) Date() time.Time                { return t.date }
func (t *Transaction) Description() string            { return t.description }
func (t *Transaction) Amount() money.Money            { return t.amount }
func (t *Transaction) SourceKind() Kind               { return t.sourceKind }
func (t *Transaction) Status() Status                 { return t.status }
func (t *Transaction) Installment() Installment       { return t.installment }
func (t *Transaction) Counterpart() TransactionID     { return t.counterpart }
func (t *Transaction) IsPaired() bool                 { return t.counterpart != "" }
func (t *Transaction) IsRemoved() bool                { return !t.removedAt.IsZero() }
func (t *Transaction) Category() (CategoryID, Source) { return t.category, t.categorySrc }

// civilDate keeps only the calendar day of d, in d's own location.
func civilDate(d time.Time) time.Time {
	y, m, day := d.Date()
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
