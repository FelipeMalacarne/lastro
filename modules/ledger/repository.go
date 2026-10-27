package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lastro/modules/ledger/internal/db"
	"lastro/money"
)

// PostgresRepository implements Repository on the ledger schema.
type PostgresRepository struct {
	pool *pgxpool.Pool
	q    *db.Queries
	tx   pgx.Tx // set inside Tx
}

// NewRepository creates the Postgres repository.
func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, q: db.New(pool)}
}

// Tx runs fn in a database transaction. Inside one, it reuses it.
func (r *PostgresRepository) Tx(ctx context.Context, fn func(Repository) error) error {
	if r.tx != nil {
		return fn(r)
	}
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		return fn(&PostgresRepository{pool: r.pool, q: r.q.WithTx(tx), tx: tx})
	})
}

func (r *PostgresRepository) AccountByExternalID(ctx context.Context, id ExternalID) (*Account, error) {
	row, err := r.q.GetAccountByExternalID(ctx, id.String())
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: account %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	return accountFromRow(row)
}

func (r *PostgresRepository) Accounts(ctx context.Context) ([]*Account, error) {
	rows, err := r.q.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*Account, 0, len(rows))
	for _, row := range rows {
		a, err := accountFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (r *PostgresRepository) SaveAccount(ctx context.Context, a *Account) error {
	p := db.SaveAccountParams{
		ID:            string(a.id),
		ExternalID:    a.externalID.String(),
		Name:          a.name,
		Institution:   a.institution,
		Type:          string(a.typ),
		Number:        a.number,
		Currency:      string(a.currency),
		BalanceCents:  a.balance.Cents(),
		BalanceAt:     a.balanceAt,
		BillDueDate:   datePtr(a.card.due),
		BillCloseDate: datePtr(a.card.closes),
	}
	if limit, available, ok := a.card.Limit(); ok {
		l, av := limit.Cents(), available.Cents()
		p.CreditLimitCents, p.AvailableCreditCents = &l, &av
	}
	return r.q.SaveAccount(ctx, p)
}

func (r *PostgresRepository) SaveSnapshot(ctx context.Context, s BalanceSnapshot) error {
	return r.q.SaveBalanceSnapshot(ctx, db.SaveBalanceSnapshotParams{
		AccountID:    string(s.AccountID),
		Date:         s.Date,
		BalanceCents: s.Balance.Cents(),
		Currency:     string(s.Balance.Currency()),
		TakenAt:      s.At,
	})
}

func (r *PostgresRepository) Snapshots(ctx context.Context, account AccountID) ([]BalanceSnapshot, error) {
	rows, err := r.q.ListBalanceSnapshots(ctx, string(account))
	if err != nil {
		return nil, err
	}
	out := make([]BalanceSnapshot, 0, len(rows))
	for _, row := range rows {
		bal, err := money.New(row.BalanceCents, money.Currency(row.Currency))
		if err != nil {
			return nil, err
		}
		out = append(out, BalanceSnapshot{AccountID: account, Date: civilDate(row.Date), Balance: bal, At: row.TakenAt})
	}
	return out, nil
}

func (r *PostgresRepository) TransactionsForImport(ctx context.Context, account AccountID, from time.Time, ids []ExternalID, bankRefs []string) ([]*Transaction, error) {
	var window *time.Time
	if !from.IsZero() {
		d := civilDate(from)
		window = &d
	}
	ext := make([]string, len(ids))
	for i, id := range ids {
		ext[i] = id.String()
	}
	rows, err := r.q.ListTransactionsForImport(ctx, db.ListTransactionsForImportParams{
		AccountID:   string(account),
		WindowFrom:  window,
		ExternalIds: ext,
		BankRefs:    append([]string{}, bankRefs...),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*Transaction, 0, len(rows))
	for _, row := range rows {
		t, err := transactionFromRow(row.LedgerTransaction, row.AccountType)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *PostgresRepository) Transactions(ctx context.Context, ids []TransactionID) ([]*Transaction, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = string(id)
	}
	rows, err := r.q.GetTransactions(ctx, s)
	if err != nil {
		return nil, err
	}
	out := make([]*Transaction, 0, len(rows))
	for _, row := range rows {
		t, err := transactionFromRow(row.LedgerTransaction, row.AccountType)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *PostgresRepository) PairCandidates(ctx context.Context, from time.Time) ([]*Transaction, error) {
	var window *time.Time
	if !from.IsZero() {
		d := civilDate(from)
		window = &d
	}
	rows, err := r.q.ListPairCandidates(ctx, window)
	if err != nil {
		return nil, err
	}
	out := make([]*Transaction, 0, len(rows))
	for _, row := range rows {
		t, err := transactionFromRow(row.LedgerTransaction, row.AccountType)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *PostgresRepository) SaveTransactions(ctx context.Context, txs []*Transaction) error {
	if len(txs) == 0 {
		return nil
	}
	params := make([]db.SaveTransactionParams, len(txs))
	for i, t := range txs {
		params[i] = transactionParams(t)
	}
	var errs []error
	r.q.SaveTransaction(ctx, params).Exec(func(i int, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("save transaction %s: %w", txs[i].id, err))
		}
	})
	return errors.Join(errs...)
}

func accountFromRow(row db.LedgerAccount) (*Account, error) {
	ext, err := NewExternalID(row.ExternalID)
	if err != nil {
		return nil, err
	}
	typ, err := ParseAccountType(row.Type)
	if err != nil {
		return nil, err
	}
	bal, err := money.New(row.BalanceCents, money.Currency(row.Currency))
	if err != nil {
		return nil, err
	}
	card, err := cardFromRow(bal.Currency(), row.CreditLimitCents, row.AvailableCreditCents, row.BillDueDate, row.BillCloseDate)
	if err != nil {
		return nil, err
	}
	return &Account{
		id:          AccountID(row.ID),
		externalID:  ext,
		name:        row.Name,
		institution: row.Institution,
		typ:         typ,
		number:      row.Number,
		currency:    bal.Currency(),
		balance:     bal,
		balanceAt:   row.BalanceAt,
		card:        card,
	}, nil
}

func cardFromRow(c money.Currency, limit, available *int64, due, closes *time.Time) (CardDetails, error) {
	var l, av *money.Money
	if limit != nil && available != nil {
		lm, err := money.New(*limit, c)
		if err != nil {
			return CardDetails{}, err
		}
		am, err := money.New(*available, c)
		if err != nil {
			return CardDetails{}, err
		}
		l, av = &lm, &am
	}
	return NewCardDetails(l, av, derefTime(due), derefTime(closes))
}

func datePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func transactionFromRow(row db.LedgerTransaction, accountType string) (*Transaction, error) {
	ext, err := NewExternalID(row.ExternalID)
	if err != nil {
		return nil, err
	}
	amount, err := money.New(row.AmountCents, money.Currency(row.Currency))
	if err != nil {
		return nil, err
	}
	at, err := ParseAccountType(accountType)
	if err != nil {
		return nil, err
	}
	kind, err := ParseKind(row.SourceKind)
	if err != nil {
		return nil, err
	}
	status, err := ParseStatus(row.Status)
	if err != nil {
		return nil, err
	}
	t := &Transaction{
		id:          TransactionID(row.ID),
		accountID:   AccountID(row.AccountID),
		accountType: at,
		externalID:  ext,
		bankRef:     row.BankRef,
		date:        civilDate(row.Date),
		description: row.Description,
		amount:      amount,
		sourceKind:  kind,
		status:      status,
	}
	if row.InstallmentNumber != nil && row.InstallmentTotal != nil {
		if t.installment, err = NewInstallment(int(*row.InstallmentNumber), int(*row.InstallmentTotal)); err != nil {
			return nil, err
		}
	}
	if row.CategoryID != nil && row.CategorySource != nil {
		if t.category, err = NewCategoryID(*row.CategoryID); err != nil {
			return nil, err
		}
		if t.categorySrc, err = ParseSource(*row.CategorySource); err != nil {
			return nil, err
		}
	}
	if row.CounterpartID != nil && row.PairKind != nil {
		t.counterpart = TransactionID(*row.CounterpartID)
		if t.pairKind, err = ParseKind(*row.PairKind); err != nil {
			return nil, err
		}
	}
	if row.RemovedAt != nil {
		t.removedAt = *row.RemovedAt
	}
	return t, nil
}

func transactionParams(t *Transaction) db.SaveTransactionParams {
	p := db.SaveTransactionParams{
		ID:          string(t.id),
		AccountID:   string(t.accountID),
		ExternalID:  t.externalID.String(),
		BankRef:     t.bankRef,
		Date:        t.date,
		Description: t.description,
		AmountCents: t.amount.Cents(),
		Currency:    string(t.amount.Currency()),
		SourceKind:  string(t.sourceKind),
		Status:      string(t.status),
	}
	if !t.installment.IsZero() {
		n, m := int16(t.installment.number), int16(t.installment.total)
		p.InstallmentNumber, p.InstallmentTotal = &n, &m
	}
	if !t.category.IsZero() {
		c, s := t.category.String(), string(t.categorySrc)
		p.CategoryID, p.CategorySource = &c, &s
	}
	if t.counterpart != "" {
		c, k := string(t.counterpart), string(t.pairKind)
		p.CounterpartID, p.PairKind = &c, &k
	}
	if !t.removedAt.IsZero() {
		at := t.removedAt
		p.RemovedAt = &at
	}
	return p
}

func (r *PostgresRepository) AccountViews(ctx context.Context) ([]AccountView, error) {
	rows, err := r.q.ListAccountViews(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AccountView, len(rows))
	for i, row := range rows {
		out[i] = accountView(row)
	}
	return out, nil
}

func (r *PostgresRepository) AccountView(ctx context.Context, id AccountID) (AccountView, error) {
	row, err := r.q.GetAccountView(ctx, string(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountView{}, fmt.Errorf("%w: account %s", ErrNotFound, id)
	}
	if err != nil {
		return AccountView{}, err
	}
	return accountView(row), nil
}

func accountView(row db.LedgerAccount) AccountView {
	v := AccountView{
		ID:           AccountID(row.ID),
		Name:         row.Name,
		Institution:  row.Institution,
		Type:         row.Type,
		Number:       row.Number,
		Currency:     row.Currency,
		BalanceCents: row.BalanceCents,
		BalanceAt:    row.BalanceAt,
	}
	if row.CreditLimitCents != nil || row.BillDueDate != nil || row.BillCloseDate != nil {
		v.Card = &CardView{
			LimitCents:     row.CreditLimitCents,
			AvailableCents: row.AvailableCreditCents,
			DueDate:        dateString(row.BillDueDate),
			CloseDate:      dateString(row.BillCloseDate),
		}
	}
	return v
}

func (r *PostgresRepository) TransactionViews(ctx context.Context, f TransactionFilter, limit int) ([]TransactionView, error) {
	from, to := f.Range()
	p := db.ListTransactionViewsParams{DateFrom: &from, DateTo: &to, MaxRows: int32(limit)}
	if f.Account != "" {
		a := string(f.Account)
		p.AccountID = &a
	}
	if f.Flow != FlowAll {
		fl := string(f.Flow)
		p.Flow = &fl
	}
	if q := likePattern(f.Search); q != "" {
		p.Search = &q
	}
	rows, err := r.q.ListTransactionViews(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]TransactionView, len(rows))
	for i, row := range rows {
		out[i] = transactionView(row)
	}
	return out, nil
}

func (r *PostgresRepository) TransactionView(ctx context.Context, id TransactionID) (TransactionView, error) {
	s := string(id)
	rows, err := r.q.ListTransactionViews(ctx, db.ListTransactionViewsParams{ID: &s, MaxRows: 1})
	if err != nil {
		return TransactionView{}, err
	}
	if len(rows) == 0 {
		return TransactionView{}, fmt.Errorf("%w: transaction %s", ErrNotFound, id)
	}
	return transactionView(rows[0]), nil
}

func transactionView(row db.ListTransactionViewsRow) TransactionView {
	v := TransactionView{
		ID:          TransactionID(row.ID),
		Date:        row.Date.Format(time.DateOnly),
		Description: row.Description,
		AmountCents: row.AmountCents,
		Currency:    row.Currency,
		Kind:        row.Kind,
		Status:      row.Status,
		Account:     AccountRef{ID: AccountID(row.AccountID), Name: row.AccountName, Institution: row.AccountInstitution, Type: row.AccountType},
	}
	if row.InstallmentNumber != nil && row.InstallmentTotal != nil {
		v.Installment = &InstallmentView{Number: int(*row.InstallmentNumber), Total: int(*row.InstallmentTotal)}
	}
	if row.CategoryID != nil && row.CategorySource != nil {
		v.Category = &CategoryView{ID: *row.CategoryID, Source: *row.CategorySource}
	}
	if row.CounterpartID != nil {
		id := TransactionID(*row.CounterpartID)
		v.CounterpartID = &id
	}
	if row.CounterpartAccountID != nil {
		v.Counterpart = &AccountRef{
			ID:          AccountID(*row.CounterpartAccountID),
			Name:        deref(row.CounterpartAccountName),
			Institution: deref(row.CounterpartInstitution),
			Type:        deref(row.CounterpartAccountType),
		}
	}
	return v
}

func (r *PostgresRepository) DailyNet(ctx context.Context, account AccountID, until time.Time) ([]DayNet, error) {
	acc, err := r.q.GetAccountView(ctx, string(account))
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListDailyNet(ctx, db.ListDailyNetParams{AccountID: string(account), Until: civilDate(until)})
	if err != nil {
		return nil, err
	}
	out := make([]DayNet, len(rows))
	for i, row := range rows {
		net, err := money.New(row.NetCents, money.Currency(acc.Currency))
		if err != nil {
			return nil, err
		}
		out[i] = DayNet{Date: civilDate(row.Date), Net: net}
	}
	return out, nil
}

func (r *PostgresRepository) MonthFlows(ctx context.Context, account AccountID) ([]MonthFlowRow, error) {
	rows, err := r.q.ListMonthFlows(ctx, string(account))
	if err != nil {
		return nil, err
	}
	out := make([]MonthFlowRow, len(rows))
	for i, row := range rows {
		out[i] = MonthFlowRow{Month: civilDate(row.Month), Kind: Kind(row.Kind), Inflow: row.Inflow, Cents: row.Cents, Count: int(row.N)}
		if row.CounterpartAccountID != nil {
			out[i].Counterpart = &AccountRef{
				ID:          AccountID(*row.CounterpartAccountID),
				Name:        deref(row.CounterpartAccountName),
				Institution: deref(row.CounterpartInstitution),
				Type:        deref(row.CounterpartAccountType),
			}
		}
	}
	return out, nil
}

func (r *PostgresRepository) Installments(ctx context.Context, account AccountID) ([]InstallmentRow, error) {
	rows, err := r.q.ListInstallments(ctx, string(account))
	if err != nil {
		return nil, err
	}
	out := make([]InstallmentRow, 0, len(rows))
	for _, row := range rows {
		if row.InstallmentNumber == nil || row.InstallmentTotal == nil {
			continue
		}
		inst, err := NewInstallment(int(*row.InstallmentNumber), int(*row.InstallmentTotal))
		if err != nil {
			return nil, err
		}
		amount, err := money.New(row.AmountCents, money.Currency(row.Currency))
		if err != nil {
			return nil, err
		}
		status, err := ParseStatus(row.Status)
		if err != nil {
			return nil, err
		}
		out = append(out, InstallmentRow{ID: TransactionID(row.ID), Description: row.Description, Installment: inst, Amount: amount, Date: civilDate(row.Date), Status: status})
	}
	return out, nil
}

func (r *PostgresRepository) DescriptionMonths(ctx context.Context, description string, from, to time.Time) ([]MonthTotal, error) {
	rows, err := r.q.ListDescriptionMonths(ctx, db.ListDescriptionMonthsParams{Description: description, DateFrom: civilDate(from), DateTo: civilDate(to)})
	if err != nil {
		return nil, err
	}
	out := make([]MonthTotal, len(rows))
	for i, row := range rows {
		out[i] = MonthTotal{Month: row.Month.Format("2006-01"), Cents: row.Cents, Count: int(row.N)}
	}
	return out, nil
}

func (r *PostgresRepository) DaySpending(ctx context.Context, from, to time.Time) ([]DayView, error) {
	rows, err := r.q.ListDaySpending(ctx, db.ListDaySpendingParams{DateFrom: civilDate(from), DateTo: civilDate(to)})
	if err != nil {
		return nil, err
	}
	out := make([]DayView, len(rows))
	for i, row := range rows {
		out[i] = DayView{Date: row.Date.Format(time.DateOnly), SpentCents: row.SpentCents, Purchases: int(row.Purchases), InCents: row.InCents, MovedCents: row.MovedCents}
	}
	return out, nil
}

func dateString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.DateOnly)
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
