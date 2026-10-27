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

// preferences are how new limits and closed months start; a new limit
// does not roll over and a closed month asks until they are set.
func (r *PostgresRepository) preferences(ctx context.Context) (Preferences, error) {
	row, err := r.q.GetPreferences(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return Preferences{Rest: RestAsk}, nil
	}
	if err != nil {
		return Preferences{}, err
	}
	rest, err := ParseRestDefault(row.RestDefault)
	if err != nil {
		return Preferences{}, err
	}
	return Preferences{RolloverNew: row.RolloverNew, Rest: rest}, nil
}

func (r *PostgresRepository) savePreferences(ctx context.Context, p Preferences) error {
	return r.q.SavePreferences(ctx, db.SavePreferencesParams{RolloverNew: p.RolloverNew, RestDefault: string(p.Rest)})
}

// decisions are what I chose for each closed month up to until.
func (r *PostgresRepository) decisions(ctx context.Context, until Month) (map[Month]Decision, error) {
	rows, err := r.q.ListRests(ctx, until.First())
	if err != nil {
		return nil, err
	}
	out := make(map[Month]Decision, len(rows))
	for _, row := range rows {
		m := MonthOf(row.Month)
		out[m] = Decision{Month: m, Choice: RestChoice(row.Choice), Invest: money.BRLCents(row.InvestCents), DecidedAt: row.DecidedAt}
	}
	return out, nil
}

func (r *PostgresRepository) saveDecision(ctx context.Context, d Decision) error {
	return r.q.SaveRest(ctx, db.SaveRestParams{Month: d.Month.First(), Choice: string(d.Choice), InvestCents: d.Invest.Cents(), DecidedAt: d.DecidedAt})
}

func (r *PostgresRepository) deleteDecision(ctx context.Context, m Month) error {
	return r.q.DeleteRest(ctx, m.First())
}
