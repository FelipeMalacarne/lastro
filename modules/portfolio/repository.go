package portfolio

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lastro/modules/portfolio/internal/db"
	"lastro/money"
)

// ErrNotFound means no position, thesis or purchase has that id.
var ErrNotFound = errors.New("portfolio: not found")

// PostgresRepository keeps the portfolio in its schema.
type PostgresRepository struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, q: db.New(pool)}
}

func (r *PostgresRepository) positions(ctx context.Context) ([]*Position, error) {
	rows, err := r.q.ListPositions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*Position, 0, len(rows))
	for _, row := range rows {
		rep := Report{
			ExternalID: row.ExternalID, Item: row.ItemID, Institution: row.Institution, Kind: Kind(row.Kind), Name: row.Name,
			Quantity: QuantityFromMicro(row.QuantityMicro), Value: money.BRLCents(row.ValueCents), Indexer: Indexer(row.Indexer),
			Rate: money.FromBps(int64(row.RateBps)), Maturity: row.Maturity, AsOf: row.AsOf, Active: row.ClosedAt == nil,
		}
		if row.Ticker != nil {
			rep.Ticker, _ = NewTicker(*row.Ticker)
		}
		if row.InvestedCents != nil {
			m := money.BRLCents(*row.InvestedCents)
			rep.Invested = &m
		}
		var override Class
		if row.ClassOverride != nil {
			override = Class(*row.ClassOverride)
		}
		out = append(out, RestorePosition(rep, money.BRLCents(row.BasisCents), money.BRLCents(row.RealizedCents), override, row.Emergency, row.FirstSeen, row.ClosedAt))
	}
	return out, nil
}

func (r *PostgresRepository) savePositions(ctx context.Context, ps []*Position, now time.Time) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		for _, p := range ps {
			if err := q.SavePosition(ctx, positionParams(p, now)); err != nil {
				return err
			}
		}
		return nil
	})
}

func positionParams(p *Position, now time.Time) db.SavePositionParams {
	arg := db.SavePositionParams{
		ExternalID: p.ExternalID, ItemID: p.Item, Institution: p.Institution, Kind: string(p.Kind), Name: p.Name,
		QuantityMicro: p.Quantity.Micro(), ValueCents: p.Value.Cents(), BasisCents: p.Basis().Cents(),
		Indexer: string(p.Indexer), RateBps: int32(p.Rate.Bps()), Maturity: p.Maturity, AsOf: p.AsOf,
		Emergency: p.Emergency(), FirstSeen: p.FirstSeen(), ClosedAt: p.ClosedAt(), UpdatedAt: now,
		RealizedCents: p.Realized().Cents(),
	}
	if !p.Ticker.IsZero() {
		t := p.Ticker.String()
		arg.Ticker = &t
	}
	if p.Invested != nil {
		c := p.Invested.Cents()
		arg.InvestedCents = &c
	}
	if c := p.Override(); c != "" {
		s := string(c)
		arg.ClassOverride = &s
	}
	return arg
}

func (r *PostgresRepository) targets(ctx context.Context) (Targets, error) {
	row, err := r.q.GetSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultTargets(), nil
	}
	if err != nil {
		return Targets{}, err
	}
	var w map[Class]int64
	if err := json.Unmarshal(row.Targets, &w); err != nil {
		return Targets{}, err
	}
	ps := make(map[Class]money.Percent, len(w))
	for c, bps := range w {
		ps[c] = money.FromBps(bps)
	}
	a, err := NewAllocation(ps)
	if err != nil {
		return Targets{}, err
	}
	b, err := NewBand(int64(row.BandBps))
	if err != nil {
		return Targets{}, err
	}
	return Targets{Allocation: a, Band: b}, nil
}

func (r *PostgresRepository) saveTargets(ctx context.Context, t Targets, now time.Time) error {
	w := map[Class]int64{}
	for c, p := range t.Allocation.Weights() {
		w[c] = p.Bps()
	}
	raw, err := json.Marshal(w)
	if err != nil {
		return err
	}
	return r.q.SaveSettings(ctx, db.SaveSettingsParams{Targets: raw, BandBps: int32(t.Band.Bps()), UpdatedAt: now})
}

// saveValuation replaces a day's valuation and its net worth.
func (r *PostgresRepository) saveValuation(ctx context.Context, v Valuation, nw NetWorth, held []Held, now time.Time) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		if err := q.DeleteValuations(ctx, v.Date); err != nil {
			return err
		}
		if err := q.DeleteHoldings(ctx, v.Date); err != nil {
			return err
		}
		for _, h := range held {
			if err := q.SaveHolding(ctx, db.SaveHoldingParams{Date: v.Date, Ticker: h.Ticker, QuantityMicro: h.Quantity.Micro()}); err != nil {
				return err
			}
		}
		rows := map[string]Amounts{"emergency": v.Emergency}
		for c, a := range v.Classes {
			rows[string(c)] = a
		}
		for c, a := range rows {
			if err := q.SaveValuation(ctx, db.SaveValuationParams{Date: v.Date, Class: c, ValueCents: a.Value.Cents(), BasisCents: a.Basis.Cents()}); err != nil {
				return err
			}
		}
		return q.SaveNetWorth(ctx, db.SaveNetWorthParams{
			Date: v.Date, CashCents: nw.Cash.Cents(), CardsCents: nw.Cards.Cents(), InvestedCents: nw.Invested.Cents(),
			EmergencyCents: nw.Emergency.Cents(), BasisCents: nw.Basis.Cents(), UpdatedAt: now,
		})
	})
}

// DayClass is one day's value and basis of a class ("emergency" too).
type DayClass struct {
	Date  time.Time
	Class string
	Amounts
}

func (r *PostgresRepository) valuations(ctx context.Context, from time.Time) ([]DayClass, error) {
	rows, err := r.q.ListValuations(ctx, from)
	if err != nil {
		return nil, err
	}
	out := make([]DayClass, len(rows))
	for i, row := range rows {
		out[i] = DayClass{Date: row.Date, Class: row.Class, Amounts: Amounts{Value: money.BRLCents(row.ValueCents), Basis: money.BRLCents(row.BasisCents)}}
	}
	return out, nil
}

func (r *PostgresRepository) netWorth(ctx context.Context, from time.Time) ([]NetWorth, error) {
	rows, err := r.q.ListNetWorth(ctx, from)
	if err != nil {
		return nil, err
	}
	out := make([]NetWorth, len(rows))
	for i, row := range rows {
		out[i] = NetWorth{
			Date: row.Date, Cash: money.BRLCents(row.CashCents), Cards: money.BRLCents(row.CardsCents),
			Invested: money.BRLCents(row.InvestedCents), Emergency: money.BRLCents(row.EmergencyCents), Basis: money.BRLCents(row.BasisCents),
		}
	}
	return out, nil
}

func (r *PostgresRepository) theses(ctx context.Context) ([]Thesis, error) {
	rows, err := r.q.ListTheses(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Thesis, 0, len(rows))
	for _, row := range rows {
		t, err := NewTicker(row.Ticker)
		if err != nil {
			continue
		}
		out = append(out, Thesis{Ticker: t, Thesis: row.Thesis, Breaks: row.Breaks, NextReport: row.NextReport, UpdatedAt: row.UpdatedAt})
	}
	return out, nil
}

func (r *PostgresRepository) saveThesis(ctx context.Context, t Thesis) error {
	return r.q.SaveThesis(ctx, db.SaveThesisParams{Ticker: t.Ticker.String(), Thesis: t.Thesis, Breaks: t.Breaks, NextReport: t.NextReport, UpdatedAt: t.UpdatedAt})
}

func (r *PostgresRepository) deleteThesis(ctx context.Context, t Ticker) error {
	return r.q.DeleteThesis(ctx, t.String())
}

func purchaseOf(row db.PortfolioPurchase) Purchase {
	return Purchase{
		ID: row.ID, PlannedOn: row.PlannedOn, What: row.What, Class: Class(row.Class), Amount: money.BRLCents(row.AmountCents),
		Condition: row.Condition, Status: PurchaseStatus(row.Status), Note: row.Note, CreatedAt: row.CreatedAt, DecidedAt: row.DecidedAt,
	}
}

func (r *PostgresRepository) purchases(ctx context.Context) ([]Purchase, error) {
	rows, err := r.q.ListPurchases(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Purchase, len(rows))
	for i, row := range rows {
		out[i] = purchaseOf(db.PortfolioPurchase(row))
	}
	return out, nil
}

func (r *PostgresRepository) purchase(ctx context.Context, id string) (Purchase, error) {
	row, err := r.q.GetPurchase(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Purchase{}, ErrNotFound
	}
	if err != nil {
		return Purchase{}, err
	}
	return purchaseOf(db.PortfolioPurchase(row)), nil
}

func (r *PostgresRepository) savePurchase(ctx context.Context, p Purchase) error {
	return r.q.SavePurchase(ctx, db.SavePurchaseParams{
		ID: p.ID, PlannedOn: p.PlannedOn, What: p.What, Class: string(p.Class), AmountCents: p.Amount.Cents(),
		Condition: p.Condition, Status: string(p.Status), Note: p.Note, CreatedAt: p.CreatedAt, DecidedAt: p.DecidedAt,
	})
}

func (r *PostgresRepository) held(ctx context.Context) ([]Held, error) {
	rows, err := r.q.ListHoldings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Held, len(rows))
	for i, row := range rows {
		out[i] = Held{Date: row.Date, Ticker: row.Ticker, Quantity: QuantityFromMicro(row.QuantityMicro)}
	}
	return out, nil
}

func (r *PostgresRepository) priceAlerts(ctx context.Context) (map[string]int64, error) {
	rows, err := r.q.ListPriceAlerts(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		out[row.Ticker] = int64(row.DropBps)
	}
	return out, nil
}

func (r *PostgresRepository) savePriceAlert(ctx context.Context, t Ticker, bps int64, now time.Time) error {
	return r.q.SavePriceAlert(ctx, db.SavePriceAlertParams{Ticker: t.String(), DropBps: int32(bps), UpdatedAt: now})
}

func (r *PostgresRepository) deletePriceAlert(ctx context.Context, t Ticker) error {
	return r.q.DeletePriceAlert(ctx, t.String())
}
