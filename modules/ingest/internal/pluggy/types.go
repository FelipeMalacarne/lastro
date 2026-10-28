package pluggy

import "encoding/json"

// The types below hold only the fields ingest reads. Amounts stay json.Number
// (the literal Pluggy sent) so they become cents without passing through a
// float. Dates stay strings because Pluggy mixes full timestamps and plain
// dates. Raw keeps the whole record as received, for the raw payload store.
//
// Fields that identify people (owner, taxNumber, paymentData) are left out on
// purpose: they exist only in Raw.

// Item is one connection. With Meu Pluggy, each bank connected there is an item.
type Item struct {
	ID               string          `json:"id"`
	Connector        Connector       `json:"connector"`
	Status           string          `json:"status"`
	ExecutionStatus  string          `json:"executionStatus"`
	Error            *ItemError      `json:"error"`
	LastUpdatedAt    string          `json:"lastUpdatedAt"`
	ConsentExpiresAt string          `json:"consentExpiresAt"`
	Raw              json.RawMessage `json:"-"`
}

// Connector is the institution behind an item.
type Connector struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	IsOpenFinance bool   `json:"isOpenFinance"`
}

// ItemError explains why the last update of an item failed.
type ItemError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Account is a bank account (type BANK) or a credit card (type CREDIT).
type Account struct {
	ID            string          `json:"id"`
	ItemID        string          `json:"itemId"`
	Type          string          `json:"type"`
	Subtype       string          `json:"subtype"`
	Number        string          `json:"number"`
	Name          string          `json:"name"`
	MarketingName string          `json:"marketingName"`
	Balance       json.Number     `json:"balance"`
	CurrencyCode  string          `json:"currencyCode"`
	BankData      *BankData       `json:"bankData"`
	CreditData    *CreditData     `json:"creditData"`
	Raw           json.RawMessage `json:"-"`
}

// BankData is the bank part of a BANK account. ingest reads only the bank
// code at the start of transferNumber ("260/0001/…"), never the rest.
type BankData struct {
	TransferNumber string `json:"transferNumber"`
}

// CreditData is the card part of a CREDIT account.
type CreditData struct {
	Brand                string      `json:"brand"`
	Level                string      `json:"level"`
	BalanceCloseDate     string      `json:"balanceCloseDate"`
	BalanceDueDate       string      `json:"balanceDueDate"`
	CreditLimit          json.Number `json:"creditLimit"`
	AvailableCreditLimit json.Number `json:"availableCreditLimit"`
}

// Transaction is one movement of an account. Type says the direction from
// the holder's point of view (DEBIT out, CREDIT in), on cards too: Pluggy
// normalizes card purchases to DEBIT and bill payments to CREDIT.
type Transaction struct {
	ID                      string              `json:"id"`
	AccountID               string              `json:"accountId"`
	Description             string              `json:"description"`
	DescriptionRaw          string              `json:"descriptionRaw"`
	CurrencyCode            string              `json:"currencyCode"`
	Amount                  json.Number         `json:"amount"`
	AmountInAccountCurrency json.Number         `json:"amountInAccountCurrency"`
	Date                    string              `json:"date"`
	Type                    string              `json:"type"`
	Status                  string              `json:"status"`
	OperationType           string              `json:"operationType"`
	ProviderID              string              `json:"providerId"`
	CreditCardMetadata      *CreditCardMetadata `json:"creditCardMetadata"`
	Raw                     json.RawMessage     `json:"-"`
}

// CreditCardMetadata carries the installment and bill of a card transaction.
type CreditCardMetadata struct {
	InstallmentNumber int    `json:"installmentNumber"`
	TotalInstallments int    `json:"totalInstallments"`
	PurchaseDate      string `json:"purchaseDate"`
	BillID            string `json:"billId"`
}

// Bill is one closed bill of a credit card. Pluggy lists the bills it
// collected for a card; the open one is not among them. TotalAmount is what
// the bill asks for; minimumPaymentAmount may be null.
type Bill struct {
	ID                      string          `json:"id"`
	DueDate                 string          `json:"dueDate"`
	TotalAmount             json.Number     `json:"totalAmount"`
	TotalAmountCurrencyCode string          `json:"totalAmountCurrencyCode"`
	MinimumPaymentAmount    json.Number     `json:"minimumPaymentAmount"`
	FinanceCharges          []FinanceCharge `json:"financeCharges"`
	Raw                     json.RawMessage `json:"-"`
}

// FinanceCharge is interest or a fee in a bill (late payment interest, the
// late fee, IOF and others).
type FinanceCharge struct {
	Type   string      `json:"type"`
	Amount json.Number `json:"amount"`
}

// Investment is one position of an item. M4 maps them into portfolio; until
// then ingest only stores them.
type Investment struct {
	ID           string          `json:"id"`
	ItemID       string          `json:"itemId"`
	Type         string          `json:"type"`
	Subtype      string          `json:"subtype"`
	Name         string          `json:"name"`
	Code         string          `json:"code"`
	ISIN         string          `json:"isin"`
	CurrencyCode string          `json:"currencyCode"`
	Balance      json.Number     `json:"balance"`        // net of taxes
	Amount       json.Number     `json:"amount"`         // gross
	Original     json.Number     `json:"amountOriginal"` // what was put in
	Quantity     json.Number     `json:"quantity"`
	UnitValue    json.Number     `json:"value"`
	Rate         json.Number     `json:"rate"`            // % of the index: 100 is 100% of CDI
	FixedRate    json.Number     `json:"fixedAnnualRate"` // % a year, over IPCA or on its own
	Date         string          `json:"date"`
	IssueDate    string          `json:"issueDate"` // fixed income: when it was bought
	DueDate      string          `json:"dueDate"`
	Status       string          `json:"status"`
	RateType     string          `json:"rateType"`
	Raw          json.RawMessage `json:"-"`
}

// InvestmentTransaction is one movement of an investment: Type is BUY (an
// application), SELL (a withdrawal), TAX, TRANSFER (between investments),
// INTEREST (dividends, coupons) or AMORTIZATION. Date is when it was made,
// TradeDate when it settled.
type InvestmentTransaction struct {
	ID              string          `json:"id"`
	Type            string          `json:"type"`
	MovementType    string          `json:"movementType"` // CREDIT or DEBIT, when sent
	Description     string          `json:"description"`
	Quantity        json.Number     `json:"quantity"`
	UnitValue       json.Number     `json:"value"`
	Amount          json.Number     `json:"amount"`    // gross
	NetAmount       json.Number     `json:"netAmount"` // after expenses
	Date            string          `json:"date"`
	TradeDate       string          `json:"tradeDate"`
	BrokerageNumber string          `json:"brokerageNumber"`
	Expenses        json.RawMessage `json:"expenses"` // fees and taxes of the brokerage note
	Raw             json.RawMessage `json:"-"`
}
