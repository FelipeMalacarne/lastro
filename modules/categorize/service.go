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
	llm    LLM
	logger *slog.Logger
	now    func() time.Time
}

// New creates the service. llm and logger may be nil: without a model,
// what no rule meets waits in To review.
func New(repo *Repository, l *ledger.Service, llm LLM, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{repo: repo, ledger: l, llm: llm, logger: logger, now: time.Now}
}

// OnTransactionsImported runs the rules over what an import brought. It
// never fails the import: if the rules cannot run, the transactions wait
// without a category and the pass after the sync (CategorizeUncategorized)
// tries again.
func (s *Service) OnTransactionsImported(ctx context.Context, e ledger.TransactionsImported) error {
	txs, err := s.ledger.Categorizable(ctx, e.TransactionIDs)
	if err == nil {
		_, err = s.applyRules(ctx, txs, false)
	}
	if err != nil {
		s.logger.WarnContext(ctx, "categorize: rules did not run on an import; the pass after the sync retries", "err", err)
	}
	return nil
}

// CategorizeUncategorized runs the rules over every transaction still
// without a category: what was imported before a rule existed, or while the
// rules could not run. Then, if a local model is configured, it asks the
// model about the merchants no rule met (once per merchant) and lets a
// confident answer categorize; an unsure one waits in the queue as a
// suggestion. It returns how many transactions got a category.
func (s *Service) CategorizeUncategorized(ctx context.Context) (int, error) {
	txs, err := s.ledger.Uncategorized(ctx)
	if err != nil {
		return 0, fmt.Errorf("categorize: uncategorized: %w", err)
	}
	n, err := s.applyRules(ctx, txs, true)
	if err != nil {
		return n, fmt.Errorf("categorize: uncategorized: %w", err)
	}
	return n, nil
}

// applyRules gives each transaction a rule meets that rule's category and,
// with ask, sends the rest to the model. It returns how many it categorized.
func (s *Service) applyRules(ctx context.Context, txs []ledger.Categorizable, ask bool) (int, error) {
	if len(txs) == 0 {
		return 0, nil
	}
	rules, err := s.repo.rules(ctx)
	if err != nil {
		return 0, err
	}
	decided := NewEngine(rules).Decide(txs)
	n, err := s.categorize(ctx, decided, ledger.SourceRule)
	if err != nil || !ask || s.llm == nil || !s.llm.Enabled() {
		return n, err
	}
	met := make(map[ledger.TransactionID]bool, len(decided))
	for _, d := range decided {
		met[d.Transaction] = true
	}
	var rest []ledger.Categorizable
	for _, t := range txs {
		if !met[t.ID] {
			rest = append(rest, t)
		}
	}
	m, err := s.askModel(ctx, rest, rules)
	return n + m, err
}

// categorize sends each decision to the ledger and returns how many it
// categorized.
func (s *Service) categorize(ctx context.Context, ds []Decision, src ledger.Source) (int, error) {
	n := 0
	for _, d := range ds {
		err := s.ledger.Categorize(ctx, d.Transaction, d.Category, src)
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

// maxAsksPerPass bounds how many merchants one pass asks the model about,
// so a first pass over years of history never runs for an hour.
const maxAsksPerPass = 40

// askModel asks the model about each merchant no rule met, once (its answer
// is kept), and lets the confident answers categorize. A model that fails
// stops the asking, not the pass: the transactions wait for the next one.
func (s *Service) askModel(ctx context.Context, txs []ledger.Categorizable, rules []*Rule) (int, error) {
	known, err := s.repo.suggestions(ctx)
	if err != nil {
		return 0, err
	}
	cats, err := s.repo.categories(ctx)
	if err != nil {
		return 0, err
	}
	asked := 0
	var decided []Decision
	for _, g := range GroupForReview(txs) {
		if g.Pattern.IsZero() {
			continue
		}
		sug, ok := known[g.Pattern.text]
		if !ok {
			if asked == maxAsksPerPass {
				continue
			}
			asked++
			var a answer
			if err := s.llm.GenerateJSON(ctx, classifySystem, classifyPrompt(g.Pattern.text, g.Transactions[0], cats, rules), &a); err != nil {
				s.logger.WarnContext(ctx, "categorize: the local model did not answer; its merchants wait in To review", "err", err)
				break
			}
			sug = Suggestion{Pattern: g.Pattern.text, Model: s.llm.Model(), AskedAt: s.now()}
			sug.Category, sug.Confidence = parseAnswer(a, cats)
			if err := s.repo.saveSuggestion(ctx, sug); err != nil {
				return 0, err
			}
		}
		if sug.Confident() {
			for _, t := range g.Transactions {
				decided = append(decided, Decision{Transaction: t.ID, Category: sug.Category})
			}
		}
	}
	return s.categorize(ctx, decided, ledger.SourceLLM)
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
	// Suggestions is what the model said about the groups' merchants, by
	// pattern; a group with none was not asked yet, or no model is set up.
	Suggestions map[string]Suggestion
}

// Review returns the queue.
func (s *Service) Review(ctx context.Context) (Review, error) {
	txs, err := s.ledger.Uncategorized(ctx)
	if err != nil {
		return Review{}, fmt.Errorf("categorize: review: %w", err)
	}
	sugs, err := s.repo.suggestions(ctx)
	if err != nil {
		return Review{}, fmt.Errorf("categorize: review: %w", err)
	}
	return Review{Waiting: len(txs), Groups: GroupForReview(txs), Suggestions: sugs}, nil
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
