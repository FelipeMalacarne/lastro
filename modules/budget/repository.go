package budget

import (
	"context"
	"fmt"

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
	lines := map[Month][]Line{}
	for _, row := range rows {
		c, err := ledger.NewCategoryID(row.CategoryID)
		if err != nil {
			return nil, err
		}
		m := MonthOf(row.Month)
		lines[m] = append(lines[m], Line{Category: c, Limit: money.BRLCents(row.LimitCents), Rollover: row.Rollover})
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

// Save writes a month's budget whole, replacing its lines.
func (r *PostgresRepository) Save(ctx context.Context, b *MonthlyBudget) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
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
			if err := q.InsertLine(ctx, db.InsertLineParams{
				Month: m, CategoryID: l.Category.String(), LimitCents: l.Limit.Cents(), Rollover: l.Rollover,
			}); err != nil {
				return fmt.Errorf("insert line: %w", err)
			}
		}
		return nil
	})
}
