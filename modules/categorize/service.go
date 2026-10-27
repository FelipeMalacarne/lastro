package categorize

import (
	"context"
	"errors"
	"fmt"
	"time"

	"lastro/modules/ledger"
)

var (
	// ErrNotFound means there is no category with that id (or no live one,
	// for a change that needs it live).
	ErrNotFound = errors.New("categorize: not found")
	// ErrNameTaken means a live category already has that name.
	ErrNameTaken = errors.New("categorize: a category with this name exists")
)

// Service is categorize's public facade: the category CRUD the screens use
// and the rules that manual corrections teach.
type Service struct {
	repo *Repository
	now  func() time.Time
}

// New creates the service.
func New(repo *Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// Categories returns every category, live ones first, each by name.
// Archived ones are there so old transactions can still show theirs.
func (s *Service) Categories(ctx context.Context) ([]Category, error) {
	cs, err := s.repo.categories(ctx)
	if err != nil {
		return nil, fmt.Errorf("categorize: categories: %w", err)
	}
	return cs, nil
}

// CreateCategory adds a category. It returns ErrInvalid for a bad name or
// colour and ErrNameTaken when a live category has the name.
func (s *Service) CreateCategory(ctx context.Context, name string, color Color, budgeted bool) (Category, error) {
	c, err := NewCategory(name, color, budgeted)
	if err != nil {
		return Category{}, err
	}
	if err := s.repo.insertCategory(ctx, c); err != nil {
		return Category{}, fmt.Errorf("categorize: create category: %w", err)
	}
	return c, nil
}

// UpdateCategory changes a live category's name, colour or whether it
// counts toward the budget.
func (s *Service) UpdateCategory(ctx context.Context, id ledger.CategoryID, ch CategoryChange) (Category, error) {
	c, err := s.repo.category(ctx, id)
	if err != nil {
		return Category{}, fmt.Errorf("categorize: update category: %w", err)
	}
	if err := c.Change(ch); err != nil {
		return Category{}, err
	}
	if err := s.repo.updateCategory(ctx, c); err != nil {
		return Category{}, fmt.Errorf("categorize: update category: %w", err)
	}
	return c, nil
}

// ArchiveCategory takes a category out of use and deletes its rules, which
// could no longer lead anywhere. Transactions already in it keep it. It
// returns how many rules went with it.
func (s *Service) ArchiveCategory(ctx context.Context, id ledger.CategoryID) (int64, error) {
	n, err := s.repo.archiveCategory(ctx, id, s.now())
	if err != nil {
		return 0, fmt.Errorf("categorize: archive category: %w", err)
	}
	return n, nil
}

// Learn turns a manual correction into a rule: the transactions that meet
// the pattern, in that account or in every one (an empty account), go to
// the category. A rule with the same pattern and account is retargeted
// instead of duplicated.
func (s *Service) Learn(ctx context.Context, p Pattern, account ledger.AccountID, category ledger.CategoryID) error {
	c, err := s.repo.category(ctx, category)
	if err != nil {
		return fmt.Errorf("categorize: learn: %w", err)
	}
	now := s.now()
	rule, err := s.repo.ruleByPattern(ctx, p, account)
	if err != nil {
		return fmt.Errorf("categorize: learn: %w", err)
	}
	if rule == nil {
		if rule, err = NewRule(p, account, c, now); err != nil {
			return err
		}
	} else if changed, err := rule.Retarget(c, now); err != nil || !changed {
		return err
	}
	if err := s.repo.saveRule(ctx, rule); err != nil {
		return fmt.Errorf("categorize: learn: %w", err)
	}
	return nil
}
