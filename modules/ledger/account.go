package ledger

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"lastro/money"
)

// ErrCurrencyChanged means a value arrived in another currency than the one
// the account or transaction already has. Currencies never change.
var ErrCurrencyChanged = errors.New("ledger: currency changed")

// Account is one of my accounts at a bank: checking, savings or a credit
// card. Its balance is from my point of view: on a card it is negative, the
// amount owed on the open bill.
type Account struct {
	id          AccountID
	externalID  ExternalID
	name        string
	institution string
	typ         AccountType
	number      string
	currency    money.Currency
	balance     money.Money
	balanceAt   time.Time
	card        CardDetails
}

// AccountDetails is what a source knows about an account at a given moment.
type AccountDetails struct {
	ExternalID  ExternalID
	Name        string
	Institution string
	Type        AccountType
	Number      string // as the bank formats it; only the last four digits are kept
	Balance     money.Money
	BalanceAt   time.Time
	Card        CardDetails // credit cards only; as of BalanceAt
}

// NewAccount creates an account. Its currency is the currency of the
// balance, and from then on everything in the account is in that currency.
func NewAccount(d AccountDetails) (*Account, error) {
	if err := d.validate(); err != nil {
		return nil, err
	}
	a := &Account{
		id:         AccountID(newID()),
		externalID: d.ExternalID,
		typ:        d.Type,
		currency:   d.Balance.Currency(),
	}
	a.apply(d)
	return a, nil
}

// Refresh applies newer details from the source. Type and currency never
// change; a balance older than the one already known is ignored.
func (a *Account) Refresh(d AccountDetails) error {
	if err := d.validate(); err != nil {
		return err
	}
	switch {
	case d.ExternalID != a.externalID:
		return fmt.Errorf("%w: details of account %s applied to %s", ErrInvalid, d.ExternalID, a.externalID)
	case d.Type != a.typ:
		return fmt.Errorf("%w: account %s cannot change from %s to %s", ErrInvalid, a.externalID, a.typ, d.Type)
	case d.Balance.Currency() != a.currency:
		return fmt.Errorf("%w: account %s is in %s, got %s", ErrCurrencyChanged, a.externalID, a.currency, d.Balance.Currency())
	}
	if d.BalanceAt.Before(a.balanceAt) {
		d.Balance, d.BalanceAt, d.Card = a.balance, a.balanceAt, a.card
	}
	a.apply(d)
	return nil
}

func (a *Account) apply(d AccountDetails) {
	a.name = strings.TrimSpace(d.Name)
	a.institution = strings.TrimSpace(d.Institution)
	a.number = lastDigits(d.Number, 4)
	a.balance, a.balanceAt = d.Balance, d.BalanceAt
	a.card = d.Card
}

func (d AccountDetails) validate() error {
	if d.ExternalID.IsZero() {
		return fmt.Errorf("%w: account without an external id", ErrInvalid)
	}
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("%w: account %s without a name", ErrInvalid, d.ExternalID)
	}
	if _, err := ParseAccountType(string(d.Type)); err != nil {
		return err
	}
	if _, err := money.Zero(d.Balance.Currency()); err != nil {
		return fmt.Errorf("%w: account %s balance: %w", ErrInvalid, d.ExternalID, err)
	}
	if !d.Card.IsZero() && !d.Type.IsCard() {
		return fmt.Errorf("%w: account %s is not a card but has card details", ErrInvalid, d.ExternalID)
	}
	if limit, _, ok := d.Card.Limit(); ok && limit.Currency() != d.Balance.Currency() {
		return fmt.Errorf("%w: account %s card limit in %s, balance in %s", ErrInvalid, d.ExternalID, limit.Currency(), d.Balance.Currency())
	}
	return nil
}

// lastDigits keeps the last n digits of an account or card number: enough to
// tell accounts apart on screen, never the whole number.
func lastDigits(s string, n int) string {
	var digits []rune
	for _, r := range s {
		if unicode.IsDigit(r) {
			digits = append(digits, r)
		}
	}
	if len(digits) > n {
		digits = digits[len(digits)-n:]
	}
	return string(digits)
}

// BalanceSnapshot is an account's balance on a day: the last one known that day.
type BalanceSnapshot struct {
	AccountID AccountID
	Date      time.Time // calendar day of At, in At's location
	Balance   money.Money
	At        time.Time // when the source read the balance
}

// Snapshot is the account's balance on the day it was read.
func (a *Account) Snapshot() BalanceSnapshot {
	return BalanceSnapshot{AccountID: a.id, Date: civilDate(a.balanceAt), Balance: a.balance, At: a.balanceAt}
}

func (a *Account) ID() AccountID            { return a.id }
func (a *Account) ExternalID() ExternalID   { return a.externalID }
func (a *Account) Name() string             { return a.name }
func (a *Account) Institution() string      { return a.institution }
func (a *Account) Type() AccountType        { return a.typ }
func (a *Account) Number() string           { return a.number }
func (a *Account) Currency() money.Currency { return a.currency }
func (a *Account) Balance() money.Money     { return a.balance }
func (a *Account) BalanceAt() time.Time     { return a.balanceAt }
func (a *Account) Card() CardDetails        { return a.card }
