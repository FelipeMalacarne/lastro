package budget

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lastro/modules/budget/internal/db"
	"lastro/modules/ledger"
	"lastro/money"
)

// PostgresRepository keeps the saved budgets in the budget schema.
type PostgresRepository struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, q: db.New(pool)}
}

// Saved is every budget saved up to a month, oldest first.
func (r *PostgresRepository) Saved(ctx context.Context, until Month) ([]*MonthlyBudget, error) {
	months, err := r.q.ListMonths(ctx, until.First())
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListLines(ctx, until.First())
	if err != nil {
		return nil, err
	}
	cats, err := r.q.ListLineCategories(ctx, until.First())
	if err != nil {
		return nil, err
	}
	type key struct {
		m  Month
		id string
	}
	covers := map[key][]ledger.CategoryID{}
	for _, row := range cats {
		c, err := ledger.NewCategoryID(row.CategoryID)
		if err != nil {
			return nil, err
		}
		k := key{MonthOf(row.Month), row.LineID}
		covers[k] = append(covers[k], c)
	}
	lines := map[Month][]Line{}
	for _, row := range rows {
		id, err := ParseLineID(row.LineID)
		if err != nil {
			return nil, err
		}
		m := MonthOf(row.Month)
		l := Line{ID: id, Categories: covers[key{m, row.LineID}], Limit: money.BRLCents(row.LimitCents), Rollover: row.Rollover, Kind: KindLimit}
		if row.Fund {
			l.Kind = KindFund
		}
		if len(l.Categories) == 0 {
			continue // Save writes a line with its categories; one without any covers nothing
		}
		if row.Name != nil {
			l.Name = *row.Name
		}
		lines[m] = append(lines[m], l)
	}
	out := make([]*MonthlyBudget, len(months))
	for i, row := range months {
		p, err := NewPayday(int(row.Payday))
		if err != nil {
			return nil, err
		}
		m := MonthOf(row.Month)
		out[i] = RestoreMonthlyBudget(m, p, money.BRLCents(row.PyfCents), lines[m], row.UpdatedAt)
	}
	return out, nil
}

// Save writes months' budgets whole, together, replacing their lines and
// the categories they cover.
func (r *PostgresRepository) Save(ctx context.Context, bs ...*MonthlyBudget) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		for _, b := range bs {
			if err := saveMonth(ctx, q, b); err != nil {
				return err
			}
		}
		return nil
	})
}

func saveMonth(ctx context.Context, q *db.Queries, b *MonthlyBudget) error {
	m := b.Month().First()
	if err := q.SaveMonth(ctx, db.SaveMonthParams{
		Month: m, Payday: int16(b.Payday().Day()), PyfCents: b.PayYourselfFirst().Cents(), UpdatedAt: b.UpdatedAt(),
	}); err != nil {
		return fmt.Errorf("save month: %w", err)
	}
	if err := q.DeleteLines(ctx, m); err != nil {
		return fmt.Errorf("delete lines: %w", err)
	}
	for _, l := range b.Lines() {
		var name *string
		if l.Name != "" {
			name = &l.Name
		}
		if err := q.InsertLine(ctx, db.InsertLineParams{
			Month: m, LineID: l.ID.String(), Name: name, LimitCents: l.Limit.Cents(), Rollover: l.Rollover, Fund: l.IsFund(),
		}); err != nil {
			return fmt.Errorf("insert line: %w", err)
		}
		for _, c := range l.Categories {
			if err := q.InsertLineCategory(ctx, db.InsertLineCategoryParams{
				Month: m, LineID: l.ID.String(), CategoryID: c.String(),
			}); err != nil {
				return fmt.Errorf("insert line category: %w", err)
			}
		}
	}
	return nil
}

// Adjustments are the funds' adjustments up to a month.
func (r *PostgresRepository) Adjustments(ctx context.Context, until Month) (Adjustments, error) {
	rows, err := r.q.ListAdjustments(ctx, until.First())
	if err != nil {
		return nil, err
	}
	out := Adjustments{}
	for _, row := range rows {
		id, err := ParseLineID(row.LineID)
		if err != nil {
			return nil, err
		}
		m := MonthOf(row.Month)
		if out[m] == nil {
			out[m] = map[LineID]money.Money{}
		}
		out[m][id] = money.BRLCents(row.Cents)
	}
	return out, nil
}

// SaveAdjustment writes a fund's adjustment for its month, replacing the
// one it had.
func (r *PostgresRepository) SaveAdjustment(ctx context.Context, a Adjustment) error {
	return r.q.SaveAdjustment(ctx, db.SaveAdjustmentParams{Month: a.Month.First(), LineID: a.Line.String(), Cents: a.Amount.Cents()})
}

// RemoveAdjustment deletes a fund's adjustment for a month and reports
// whether it had one.
func (r *PostgresRepository) RemoveAdjustment(ctx context.Context, m Month, id LineID) (bool, error) {
	n, err := r.q.DeleteAdjustment(ctx, db.DeleteAdjustmentParams{Month: m.First(), LineID: id.String()})
	return n > 0, err
}

// sent is which of the keys were already pushed.
func (r *PostgresRepository) sent(ctx context.Context, keys []string) (map[string]bool, error) {
	rows, err := r.q.ListSentAlerts(ctx, keys)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, k := range rows {
		out[k] = true
	}
	return out, nil
}

func (r *PostgresRepository) markSent(ctx context.Context, keys []string, at time.Time) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		for _, k := range keys {
			if err := q.MarkAlertSent(ctx, db.MarkAlertSentParams{Key: k, SentAt: at}); err != nil {
				return err
			}
		}
		return nil
	})
}

// rolloverNew is whether a new limit rolls over; false until it is set.
func (r *PostgresRepository) rolloverNew(ctx context.Context) (bool, error) {
	on, err := r.q.GetPreferences(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return on, err
}

func (r *PostgresRepository) setRolloverNew(ctx context.Context, on bool) error {
	return r.q.SavePreferences(ctx, on)
}
