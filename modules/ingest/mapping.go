package ingest

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // the image has no zoneinfo; dates are read in São Paulo

	"golang.org/x/text/unicode/norm"

	"lastro/modules/ingest/internal/pluggy"
	"lastro/modules/ledger"
	"lastro/money"
)

// errUnsupported marks Pluggy records the ledger has no place for yet; sync
// skips them with a warning instead of failing.
var errUnsupported = errors.New("ingest: unsupported")

var saoPaulo = mustLocation("America/Sao_Paulo")

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err) // impossible: time/tzdata is embedded
	}
	return loc
}

// mapAccount turns a Pluggy account into the ledger's account details. at is
// when the balance was read: the item's lastUpdatedAt.
func mapAccount(a pluggy.Account, institution string, at time.Time) (ledger.AccountDetails, error) {
	ext, err := ledger.NewExternalID(a.ID)
	if err != nil {
		return ledger.AccountDetails{}, err
	}
	var typ ledger.AccountType
	switch {
	case a.Type == "BANK" && a.Subtype == "CHECKING_ACCOUNT":
		typ = ledger.AccountChecking
	case a.Type == "BANK" && a.Subtype == "SAVINGS_ACCOUNT":
		typ = ledger.AccountSavings
	case a.Type == "CREDIT" && (a.Subtype == "CREDIT_CARD" || a.Subtype == ""):
		typ = ledger.AccountCreditCard
	default:
		return ledger.AccountDetails{}, fmt.Errorf("%w: account %s of type %s/%s", errUnsupported, a.ID, a.Type, a.Subtype)
	}
	cents, err := parseCents(a.Balance)
	if err != nil {
		return ledger.AccountDetails{}, fmt.Errorf("ingest: account %s balance: %w", a.ID, err)
	}
	if typ.IsCard() {
		cents = -cents // Pluggy reports what I owe; the ledger, what I have
	}
	balance, err := money.New(cents, money.Currency(a.CurrencyCode))
	if err != nil {
		return ledger.AccountDetails{}, fmt.Errorf("%w: account %s in %q", errUnsupported, a.ID, a.CurrencyCode)
	}
	return ledger.AccountDetails{
		ExternalID:  ext,
		Name:        cmp.Or(strings.TrimSpace(a.Name), strings.TrimSpace(a.MarketingName), typeName(typ)),
		Institution: institution,
		Type:        typ,
		Number:      a.Number,
		Balance:     balance,
		BalanceAt:   at.In(saoPaulo), // the balance's day is São Paulo's
	}, nil
}

func typeName(t ledger.AccountType) string {
	switch t {
	case ledger.AccountSavings:
		return "Savings"
	case ledger.AccountCreditCard:
		return "Credit card"
	}
	return "Checking"
}

// mapTransaction turns a Pluggy transaction into a ledger import for its
// account (see docs/pluggy.md for the reasoning behind each rule).
func mapTransaction(t pluggy.Transaction, acc pluggy.Account) (ledger.ImportedTransaction, error) {
	ext, err := ledger.NewExternalID(t.ID)
	if err != nil {
		return ledger.ImportedTransaction{}, err
	}
	date, err := parseDate(t.Date)
	if err != nil {
		return ledger.ImportedTransaction{}, fmt.Errorf("ingest: transaction %s: %w", t.ID, err)
	}

	// Purchases in another currency come with the amount in the account's.
	literal := t.Amount
	if t.CurrencyCode != acc.CurrencyCode && t.AmountInAccountCurrency != "" {
		literal = t.AmountInAccountCurrency
	}
	cents, err := parseCents(literal)
	if err != nil {
		return ledger.ImportedTransaction{}, fmt.Errorf("ingest: transaction %s amount: %w", t.ID, err)
	}
	card := acc.Type == "CREDIT"
	cents = abs(cents)
	switch {
	case t.Type == "DEBIT":
		cents = -cents
	case t.Type == "CREDIT":
	case card:
		// No direction: on cards a positive amount is a purchase.
		if p, _ := parseCents(literal); p > 0 {
			cents = -cents
		}
	default:
		if p, _ := parseCents(literal); p < 0 {
			cents = -cents
		}
	}
	amount, err := money.New(cents, money.Currency(acc.CurrencyCode))
	if err != nil {
		return ledger.ImportedTransaction{}, fmt.Errorf("%w: transaction %s in %q", errUnsupported, t.ID, acc.CurrencyCode)
	}

	status := ledger.StatusPosted
	if t.Status == "PENDING" {
		status = ledger.StatusPending
	}
	var inst ledger.Installment
	if m := t.CreditCardMetadata; m != nil && m.TotalInstallments > 1 {
		inst, _ = ledger.NewInstallment(m.InstallmentNumber, m.TotalInstallments) // a broken one is just not shown
	}
	description := cmp.Or(strings.TrimSpace(t.Description), strings.TrimSpace(t.DescriptionRaw), "No description")

	return ledger.ImportedTransaction{
		ExternalID:  ext,
		BankRef:     t.ProviderID,
		Date:        date,
		Description: description,
		Amount:      amount,
		Kind:        kindOf(t, card, cents, description),
		Status:      status,
		Installment: inst,
	}, nil
}

var (
	// What the card side of a bill payment looks like when the bank does not
	// set PAGAMENTO_FATURA: "Pagamento recebido", "Pagamento de fatura".
	billPaymentRe = regexp.MustCompile(`^PAG(AMENTO|TO)?\b.*\b(RECEBIDO|FATURA)\b`)
	// Money moving between a checking account and its own investment
	// (Itaú's automatic investment, Banco do Brasil's Rende Fácil).
	investmentMoveRe = regexp.MustCompile(`^(APLICACAO|APLIC|RESGATE)\b`)
)

// kindOf is the kind the source data supports on its own. Transfers between
// my accounts and the checking side of bill payments look like ordinary
// expenses and income here; the TransferMatcher pairs them later.
func kindOf(t pluggy.Transaction, card bool, cents int64, description string) ledger.Kind {
	words := fold(description)
	inflow := cents > 0
	switch {
	case card && inflow && (t.OperationType == "PAGAMENTO_FATURA" || billPaymentRe.MatchString(words)):
		return ledger.KindBillPayment
	case card:
		return ledger.KindExpense // purchases, and refunds or cashback that reduce spending
	case t.OperationType == "RESGATE_APLIC_FINANCEIRA" || investmentMoveRe.MatchString(words):
		return ledger.KindTransfer
	case inflow:
		return ledger.KindIncome
	}
	return ledger.KindExpense
}

// parseDate reads Pluggy's dates. A timestamp at midnight UTC (or a plain
// date) is a calendar day; any other time is converted to São Paulo first.
func parseDate(s string) (time.Time, error) {
	if len(s) == len(time.DateOnly) {
		return time.Parse(time.DateOnly, s)
	}
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("date %q: %w", s, err)
	}
	if ts.UTC().Format("15:04:05.000000000") == "00:00:00.000000000" {
		y, m, d := ts.UTC().Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC), nil
	}
	return ts.In(saoPaulo), nil
}

// parseCents converts a JSON number literal to cents without a float,
// rounding anything past two places (floating-point residue such as
// 33.330000000000005) half away from zero.
func parseCents(n json.Number) (int64, error) {
	r, ok := new(big.Rat).SetString(string(n))
	if !ok {
		return 0, fmt.Errorf("%w: %q", money.ErrInvalidAmount, n)
	}
	r.Mul(r, big.NewRat(100, 1))
	q, rem := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if twice := new(big.Int).Mul(new(big.Int).Abs(rem), big.NewInt(2)); twice.Cmp(r.Denom()) >= 0 {
		q.Add(q, big.NewInt(int64(r.Sign())))
	}
	if !q.IsInt64() {
		return 0, fmt.Errorf("%w: %q is too large", money.ErrInvalidAmount, n)
	}
	return q.Int64(), nil
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// fold upper-cases and strips accents: "Cartão" and "CARTAO" read the same.
func fold(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if r >= 0x300 && r <= 0x36f { // combining marks
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToUpper(b.String())
}
