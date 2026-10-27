package categorize

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
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

// Service is categorize's public facade: the category CRUD the screens use,
// the rules that manual corrections teach, and running those rules over
// what the ledger imports.
type Service struct {
	repo   *Repository
	ledger *ledger.Service
	logger *slog.Logger
	now    func() time.Time
}

// New creates the service. logger may be nil (tests).
func New(repo *Repository, l *ledger.Service, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{repo: repo, ledger: l, logger: logger, now: time.Now}
}

// OnTransactionsImported runs the rules over what an import brought. It
// never fails the import: if the rules cannot run, the transactions wait
// without a category and the pass after the sync (CategorizeUncategorized)
// tries again.
func (s *Service) OnTransactionsImported(ctx context.Context, e ledger.TransactionsImported) error {
	txs, err := s.ledger.Categorizable(ctx, e.TransactionIDs)
	if err == nil {
		_, err = s.applyRules(ctx, txs)
	}
	if err != nil {
		s.logger.WarnContext(ctx, "categorize: rules did not run on an import; the pass after the sync retries", "err", err)
	}
	return nil
}

// CategorizeUncategorized runs the rules over every transaction still
// without a category: what was imported before a rule existed, or while the
// rules could not run. It returns how many got one.
func (s *Service) CategorizeUncategorized(ctx context.Context) (int, error) {
	txs, err := s.ledger.Uncategorized(ctx)
	if err != nil {
		return 0, fmt.Errorf("categorize: uncategorized: %w", err)
	}
	n, err := s.applyRules(ctx, txs)
	if err != nil {
		return n, fmt.Errorf("categorize: uncategorized: %w", err)
	}
	return n, nil
}

// applyRules gives each transaction a rule meets that rule's category and
// returns how many it categorized.
func (s *Service) applyRules(ctx context.Context, txs []ledger.Categorizable) (int, error) {
	if len(txs) == 0 {
		return 0, nil
	}
	rules, err := s.repo.rules(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range NewEngine(rules).Decide(txs) {
		err := s.ledger.Categorize(ctx, d.Transaction, d.Category, ledger.SourceRule)
		switch {
		case errors.Is(err, ledger.ErrManualCategory), errors.Is(err, ledger.ErrRemoved):
			// Chosen by hand or gone since the list was read: nothing to do.
		case err != nil:
			return n, err
		default:
			n++
		}
	}
	return n, nil
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

// Review is the review queue: every transaction waiting for a category,
// grouped by the rule a correction would teach.
type Review struct {
	Waiting int
	Groups  []ReviewGroup
}

// Review returns the queue.
func (s *Service) Review(ctx context.Context) (Review, error) {
	txs, err := s.ledger.Uncategorized(ctx)
	if err != nil {
		return Review{}, fmt.Errorf("categorize: review: %w", err)
	}
	return Review{Waiting: len(txs), Groups: GroupForReview(txs)}, nil
}

// Correct settles part of the queue: the pattern becomes a rule (or
// retargets the one it already is), and the rules run over everything still
// waiting. It returns how many transactions got a category.
func (s *Service) Correct(ctx context.Context, p Pattern, account ledger.AccountID, category ledger.CategoryID) (int, error) {
	if err := s.Learn(ctx, p, account, category); err != nil {
		return 0, err
	}
	return s.CategorizeUncategorized(ctx)
}

// SetCategory is my choice for one transaction: it always wins over a rule
// or the model. With a pattern, it also teaches that rule and runs it over
// what is still waiting; it returns how many others got a category.
func (s *Service) SetCategory(ctx context.Context, tx ledger.TransactionID, category ledger.CategoryID, teach *Pattern) (int, error) {
	c, err := s.repo.category(ctx, category)
	if err != nil {
		return 0, fmt.Errorf("categorize: set category: %w", err)
	}
	if c.Archived {
		return 0, fmt.Errorf("%w: %s", ErrArchived, c.Name)
	}
	if err := s.ledger.Categorize(ctx, tx, category, ledger.SourceManual); err != nil {
		return 0, fmt.Errorf("categorize: set category: %w", err)
	}
	if teach == nil {
		return 0, nil
	}
	return s.Correct(ctx, *teach, "", category)
}
