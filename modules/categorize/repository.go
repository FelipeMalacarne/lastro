package categorize

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"lastro/modules/categorize/internal/db"
	"lastro/modules/ledger"
)

// Repository is categorize's persistence on the categorize schema. It is
// concrete: nothing swaps it, and its methods are for the service only.
type Repository struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewRepository creates the repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: db.New(pool)}
}

func (r *Repository) categories(ctx context.Context) ([]Category, error) {
	rows, err := r.q.ListCategories(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Category, 0, len(rows))
	for _, row := range rows {
		c, err := categoryFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *Repository) category(ctx context.Context, id ledger.CategoryID) (Category, error) {
	row, err := r.q.GetCategory(ctx, id.String())
	if errors.Is(err, pgx.ErrNoRows) {
		return Category{}, fmt.Errorf("%w: category %s", ErrNotFound, id)
	}
	if err != nil {
		return Category{}, err
	}
	return categoryFromRow(row)
}

func (r *Repository) insertCategory(ctx context.Context, c Category) error {
	err := r.q.InsertCategory(ctx, db.InsertCategoryParams{
		ID: c.ID.String(), Name: c.Name, Color: c.Color.String(), Budgeted: c.Budgeted,
	})
	return nameTaken(err, c.Name)
}

func (r *Repository) updateCategory(ctx context.Context, c Category) error {
	n, err := r.q.UpdateCategory(ctx, db.UpdateCategoryParams{
		ID: c.ID.String(), Name: c.Name, Color: c.Color.String(), Budgeted: c.Budgeted,
	})
	if err != nil {
		return nameTaken(err, c.Name)
	}
	if n == 0 {
		return fmt.Errorf("%w: category %s", ErrNotFound, c.ID)
	}
	return nil
}

// archiveCategory archives a live category and deletes its rules, together.
func (r *Repository) archiveCategory(ctx context.Context, id ledger.CategoryID, at time.Time) (rules int64, err error) {
	err = pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		n, err := q.ArchiveCategory(ctx, db.ArchiveCategoryParams{ID: id.String(), ArchivedAt: &at})
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%w: category %s", ErrNotFound, id)
		}
		rules, err = q.DeleteCategoryRules(ctx, id.String())
		return err
	})
	return rules, err
}

func (r *Repository) rules(ctx context.Context) ([]*Rule, error) {
	rows, err := r.q.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*Rule, 0, len(rows))
	for _, row := range rows {
		rule, err := ruleFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, nil
}

// ruleByPattern returns the rule for this pattern and account, or nil.
func (r *Repository) ruleByPattern(ctx context.Context, p Pattern, account ledger.AccountID) (*Rule, error) {
	row, err := r.q.GetRuleByPattern(ctx, db.GetRuleByPatternParams{
		PatternKind: string(p.Kind()), Pattern: p.Text(), AccountID: accountPtr(account),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ruleFromRow(row)
}

func (r *Repository) saveRule(ctx context.Context, rule *Rule) error {
	return ruleTaken(r.q.SaveRule(ctx, db.SaveRuleParams{
		ID:          string(rule.id),
		PatternKind: string(rule.pattern.kind),
		Pattern:     rule.pattern.text,
		AccountID:   accountPtr(rule.account),
		CategoryID:  rule.category.String(),
		CreatedAt:   rule.createdAt,
		UpdatedAt:   rule.updatedAt,
		Origin:      string(rule.origin),
	}))
}

// rule returns one rule, or ErrRuleNotFound.
func (r *Repository) rule(ctx context.Context, id RuleID) (*Rule, error) {
	row, err := r.q.GetRule(ctx, string(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrRuleNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	return ruleFromRow(row)
}

func (r *Repository) deleteRule(ctx context.Context, id RuleID) error {
	n, err := r.q.DeleteRule(ctx, string(id))
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrRuleNotFound, id)
	}
	return nil
}

// ruleTaken turns the one-rule-per-pattern-and-account violation into ErrRuleTaken.
func ruleTaken(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "rules_pattern_kind_pattern_account_id_key" {
		return fmt.Errorf("%w", ErrRuleTaken)
	}
	return err
}

func categoryFromRow(row db.CategorizeCategory) (Category, error) {
	id, err := ledger.NewCategoryID(row.ID)
	if err != nil {
		return Category{}, err
	}
	color, err := ParseColor(row.Color)
	if err != nil {
		return Category{}, err
	}
	return Category{ID: id, Name: row.Name, Color: color, Budgeted: row.Budgeted, Archived: row.ArchivedAt != nil}, nil
}

func ruleFromRow(row db.CategorizeRule) (*Rule, error) {
	kind, err := ParsePatternKind(row.PatternKind)
	if err != nil {
		return nil, err
	}
	category, err := ledger.NewCategoryID(row.CategoryID)
	if err != nil {
		return nil, err
	}
	rule := &Rule{
		id:        RuleID(row.ID),
		pattern:   Pattern{kind: kind, text: row.Pattern}, // stored normalized
		category:  category,
		origin:    Origin(row.Origin),
		createdAt: row.CreatedAt,
		updatedAt: row.UpdatedAt,
	}
	if row.AccountID != nil {
		rule.account = ledger.AccountID(*row.AccountID)
	}
	return rule, nil
}

func accountPtr(a ledger.AccountID) *string {
	if a == "" {
		return nil
	}
	s := string(a)
	return &s
}

// nameTaken turns the unique name index's violation into ErrNameTaken.
func nameTaken(err error, name string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "categories_name_idx" {
		return fmt.Errorf("%w: %s", ErrNameTaken, name)
	}
	return err
}

// suggestions returns what the model said about each merchant, by pattern.
func (r *Repository) suggestions(ctx context.Context) (map[string]Suggestion, error) {
	rows, err := r.q.ListSuggestions(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Suggestion, len(rows))
	for _, row := range rows {
		s := Suggestion{Pattern: row.Pattern, Confidence: int(row.Confidence), Model: row.Model, AskedAt: row.AskedAt}
		if row.CategoryID != nil {
			if s.Category, err = ledger.NewCategoryID(*row.CategoryID); err != nil {
				return nil, err
			}
		}
		out[row.Pattern] = s
	}
	return out, nil
}

func (r *Repository) saveSuggestion(ctx context.Context, s Suggestion) error {
	var c *string
	if !s.Category.IsZero() {
		id := s.Category.String()
		c = &id
	}
	return r.q.SaveSuggestion(ctx, db.SaveSuggestionParams{
		Pattern: s.Pattern, CategoryID: c, Confidence: int16(s.Confidence), Model: s.Model, AskedAt: s.AskedAt,
	})
}
