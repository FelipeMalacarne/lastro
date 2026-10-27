package market

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"lastro/modules/market/internal/db"
	"lastro/money"
)

// PostgresRepository keeps the series in the market schema.
type PostgresRepository struct{ q *db.Queries }

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{q: db.New(pool)}
}

func (r *PostgresRepository) addRate(ctx context.Context, p Point, source string, now time.Time) error {
	return r.q.InsertRate(ctx, db.InsertRateParams{Series: string(p.Series), Date: p.Date, RatePpb: p.Rate.PPB(), Source: source, FetchedAt: now})
}

func (r *PostgresRepository) lastRate(ctx context.Context, s Series) (time.Time, error) {
	return r.q.LastRateDate(ctx, string(s))
}

func (r *PostgresRepository) rates(ctx context.Context, s Series, from, to time.Time) ([]Point, error) {
	rows, err := r.q.ListRates(ctx, db.ListRatesParams{Series: string(s), DateFrom: from, DateTo: to})
	if err != nil {
		return nil, err
	}
	out := make([]Point, len(rows))
	for i, row := range rows {
		out[i] = Point{Series: Series(row.Series), Date: row.Date, Rate: RateFromPPB(row.RatePpb)}
	}
	return out, nil
}

func (r *PostgresRepository) saveQuote(ctx context.Context, q Quote, now, today time.Time) error {
	return r.q.SaveQuote(ctx, db.SaveQuoteParams{
		Symbol: q.Symbol, Date: q.Date, CloseCents: q.Close.Cents(), Currency: string(q.Close.Currency()),
		Source: q.Source, FetchedAt: now, Today: today,
	})
}

func (r *PostgresRepository) quoteCount(ctx context.Context, symbol string) (int, error) {
	n, err := r.q.CountQuotes(ctx, symbol)
	return int(n), err
}

func (r *PostgresRepository) latestQuotes(ctx context.Context, symbols []string) ([]Quote, error) {
	rows, err := r.q.LatestQuotes(ctx, symbols)
	if err != nil {
		return nil, err
	}
	out := make([]Quote, len(rows))
	for i, row := range rows {
		out[i] = Quote{Symbol: row.Symbol, Date: row.Date, Close: money.BRLCents(row.CloseCents), Source: row.Source}
	}
	return out, nil
}

func (r *PostgresRepository) quotes(ctx context.Context, symbol string, from time.Time) ([]Quote, error) {
	rows, err := r.q.ListQuotes(ctx, db.ListQuotesParams{Symbol: symbol, DateFrom: from})
	if err != nil {
		return nil, err
	}
	out := make([]Quote, len(rows))
	for i, row := range rows {
		out[i] = Quote{Symbol: row.Symbol, Date: row.Date, Close: money.BRLCents(row.CloseCents), Source: row.Source}
	}
	return out, nil
}

func (r *PostgresRepository) addBond(ctx context.Context, b Bond, now time.Time) error {
	return r.q.InsertBond(ctx, db.InsertBondParams{
		Name: b.Name, Maturity: b.Maturity, Date: b.Date, BuyRateBp: int32(b.BuyRate.Bps()), PriceCents: b.Price.Cents(), FetchedAt: now,
	})
}

func (r *PostgresRepository) latestBonds(ctx context.Context) ([]Bond, error) {
	rows, err := r.q.LatestBonds(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Bond, len(rows))
	for i, row := range rows {
		out[i] = Bond{Name: row.Name, Maturity: row.Maturity, Date: row.Date, BuyRate: money.FromBps(int64(row.BuyRateBp)), Price: money.BRLCents(row.PriceCents)}
	}
	return out, nil
}
