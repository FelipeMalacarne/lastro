package categorize

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"lastro/modules/ledger"
	"lastro/money"
)

var (
	// ErrNotFound means there is no category with that id (or no live one,
	// for a change that needs it live).
	ErrNotFound = errors.New("categorize: not found")
	// ErrNameTaken means a live category already has that name.
	ErrNameTaken = errors.New("categorize: a category with this name exists")
	// ErrRuleNotFound means there is no rule with that id.
	ErrRuleNotFound = errors.New("categorize: no such rule")
	// ErrRuleTaken means another rule already has that pattern and account.
	ErrRuleTaken = errors.New("categorize: a rule with this pattern and account exists")
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
	down := false // once the model fails, stop asking it, but keep its saved answers
	var decided []Decision
	for _, g := range GroupForReview(txs) {
		if g.Pattern.IsZero() {
			continue
		}
		sug, ok := known[g.Pattern.text]
		if !ok {
			if down || asked == maxAsksPerPass {
				continue
			}
			asked++
			var a answer
			if err := s.llm.GenerateJSON(ctx, classifySystem, classifyPrompt(g.Pattern.text, g.Transactions[0], cats, rules), &a); err != nil {
				s.logger.WarnContext(ctx, "categorize: the local model did not answer; its merchants wait in To review", "err", err)
				down = true
				continue
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
	_, err := s.learn(ctx, p, account, category, OriginCorrection)
	return err
}

// learned is the rule a correction taught and its category before it; a
// zero one means the correction created the rule.
type learned struct {
	rule RuleID
	was  ledger.CategoryID
}

func (s *Service) learn(ctx context.Context, p Pattern, account ledger.AccountID, category ledger.CategoryID, origin Origin) (learned, error) {
	c, err := s.repo.category(ctx, category)
	if err != nil {
		return learned{}, fmt.Errorf("categorize: learn: %w", err)
	}
	now := s.now()
	rule, err := s.repo.ruleByPattern(ctx, p, account)
	if err != nil {
		return learned{}, fmt.Errorf("categorize: learn: %w", err)
	}
	var was ledger.CategoryID
	if rule == nil {
		if rule, err = NewRule(p, account, c, origin, now); err != nil {
			return learned{}, err
		}
	} else {
		was = rule.Category()
		changed, err := rule.Retarget(c, now)
		if err != nil {
			return learned{}, err
		}
		if !changed {
			return learned{rule: rule.ID(), was: was}, nil
		}
	}
	if err := s.repo.saveRule(ctx, rule); err != nil {
		return learned{}, fmt.Errorf("categorize: learn: %w", err)
	}
	return learned{rule: rule.ID(), was: was}, nil
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

// Change is what a choice did: how many other transactions it categorized,
// and what puts everything back.
type Change struct {
	Categorized int
	Undo        Undo
}

// Correct settles part of the queue: the pattern becomes a rule (or
// retargets the one it already is), and the rules run again over every
// transaction automation categorized or left waiting (never one I chose by
// hand).
func (s *Service) Correct(ctx context.Context, p Pattern, account ledger.AccountID, category ledger.CategoryID) (Change, error) {
	before, err := s.snapshot(ctx)
	if err != nil {
		return Change{}, fmt.Errorf("categorize: correct: %w", err)
	}
	return s.correct(ctx, before, p, account, category, OriginCorrection)
}

// CreateRule is a rule written by hand on the Categories and rules screen;
// like a correction, the rules then run again.
func (s *Service) CreateRule(ctx context.Context, p Pattern, account ledger.AccountID, category ledger.CategoryID) (Change, error) {
	before, err := s.snapshot(ctx)
	if err != nil {
		return Change{}, fmt.Errorf("categorize: create rule: %w", err)
	}
	return s.correct(ctx, before, p, account, category, OriginHand)
}

func (s *Service) correct(ctx context.Context, before snapshot, p Pattern, account ledger.AccountID, category ledger.CategoryID, origin Origin) (Change, error) {
	l, err := s.learn(ctx, p, account, category, origin)
	if err != nil {
		return Change{}, err
	}
	n, err := s.Reapply(ctx)
	if err != nil {
		return Change{}, err
	}
	after, err := s.snapshot(ctx)
	if err != nil {
		return Change{}, fmt.Errorf("categorize: correct: %w", err)
	}
	return Change{Categorized: n, Undo: Undo{Rule: l.rule, WasRule: l.was, Before: changed(before, after)}}, nil
}

// SetCategory is my choice for one transaction: it always wins over a rule
// or the model. With a pattern, it also teaches that rule and runs it over
// what is still waiting; Categorized counts the others that got one.
func (s *Service) SetCategory(ctx context.Context, tx ledger.TransactionID, category ledger.CategoryID, teach *Pattern) (Change, error) {
	c, err := s.repo.category(ctx, category)
	if err != nil {
		return Change{}, fmt.Errorf("categorize: set category: %w", err)
	}
	if c.Archived {
		return Change{}, fmt.Errorf("%w: %s", ErrArchived, c.Name)
	}
	before, err := s.snapshot(ctx)
	if err != nil {
		return Change{}, fmt.Errorf("categorize: set category: %w", err)
	}
	// A manual category is not in the snapshot: read this one's as it is.
	d, err := s.ledger.Transaction(ctx, tx)
	if err != nil {
		return Change{}, fmt.Errorf("categorize: set category: %w", err)
	}
	if len(d.Transaction.SplitCategories) > 0 {
		return Change{}, fmt.Errorf("%w: the transaction is split; remove the split to give it one category", ErrInvalid)
	}
	before[tx] = beforeOf(tx, d.Transaction.Category)
	if err := s.ledger.Categorize(ctx, tx, category, ledger.SourceManual); err != nil {
		return Change{}, fmt.Errorf("categorize: set category: %w", err)
	}
	if teach == nil {
		return Change{Undo: Undo{Before: []Before{before[tx]}}}, nil
	}
	return s.correct(ctx, before, *teach, "", category, OriginCorrection)
}

// MarkTransfers settles a review group as transfers to accounts lastro does
// not have: every transaction still waiting whose description meets the
// pattern (or, with a zero pattern, equals description). It returns their
// ids, which is what takes it back.
func (s *Service) MarkTransfers(ctx context.Context, p Pattern, description string) ([]ledger.TransactionID, error) {
	txs, err := s.ledger.Uncategorized(ctx)
	if err != nil {
		return nil, fmt.Errorf("categorize: mark transfers: %w", err)
	}
	var ids []ledger.TransactionID
	for _, t := range txs {
		if (!p.IsZero() && p.Matches(t.Description)) || (p.IsZero() && t.Description == description) {
			ids = append(ids, t.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if _, err := s.ledger.MarkTransfers(ctx, ids, true); err != nil {
		return nil, fmt.Errorf("categorize: mark transfers: %w", err)
	}
	return ids, nil
}

// SplitPart is one category's share of a split, in the transaction's
// currency.
type SplitPart struct {
	Category    ledger.CategoryID
	AmountCents int64
}

// Split divides a transaction across live categories (ledger checks the
// parts add up). Like any choice of mine, rules never touch it again.
func (s *Service) Split(ctx context.Context, tx ledger.TransactionID, parts []SplitPart) error {
	d, err := s.ledger.Transaction(ctx, tx)
	if err != nil {
		return fmt.Errorf("categorize: split: %w", err)
	}
	cur := money.Currency(d.Transaction.Currency)
	out := make([]ledger.SplitPart, len(parts))
	for i, p := range parts {
		c, err := s.repo.category(ctx, p.Category)
		if err != nil {
			return fmt.Errorf("categorize: split: %w", err)
		}
		if c.Archived {
			return fmt.Errorf("%w: %s", ErrArchived, c.Name)
		}
		amt, err := money.New(p.AmountCents, cur)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		out[i] = ledger.SplitPart{Category: p.Category, Amount: amt}
	}
	if err := s.ledger.SplitTransaction(ctx, tx, out); err != nil {
		return fmt.Errorf("categorize: split: %w", err)
	}
	return nil
}

// RuleView is a rule as the Categories and rules screen lists it: in the
// order the engine tries them, with how many transactions it meets and how
// many it decides (meets with no more specific rule before it).
type RuleView struct {
	Rule    *Rule
	Order   int // 1 is tried first
	Meets   int
	Decides int
	Last    time.Time // the latest transaction it meets; zero when none
}

// Rules lists every rule in the order they are tried.
func (s *Service) Rules(ctx context.Context) ([]RuleView, error) {
	rules, err := s.repo.rules(ctx)
	if err != nil {
		return nil, fmt.Errorf("categorize: rules: %w", err)
	}
	counts, err := s.ledger.DescriptionCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("categorize: rules: %w", err)
	}
	e := NewEngine(rules)
	views := make([]RuleView, len(e.rules))
	at := make(map[RuleID]int, len(e.rules))
	for i, r := range e.rules {
		views[i] = RuleView{Rule: r, Order: i + 1}
		at[r.id] = i
	}
	for _, c := range counts {
		for _, r := range e.rules {
			if r.Matches(c.Description, c.Account) {
				v := &views[at[r.id]]
				v.Meets += c.Count
				if c.Last.After(v.Last) {
					v.Last = c.Last
				}
			}
		}
		if r := e.Match(c.Description, c.Account); r != nil {
			views[at[r.id]].Decides += c.Count
		}
	}
	return views, nil
}

// RuleChange is an edit of a rule; a nil field stays as it is.
type RuleChange struct {
	Pattern  *Pattern
	Account  *ledger.AccountID // empty: every account
	Category *ledger.CategoryID
}

// UpdateRule edits a rule and runs the rules again over every transaction
// automation categorized, so they follow it. It returns how many changed.
func (s *Service) UpdateRule(ctx context.Context, id RuleID, ch RuleChange) (int, error) {
	r, err := s.repo.rule(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("categorize: update rule: %w", err)
	}
	now := s.now()
	p, account := r.pattern, r.account
	if ch.Pattern != nil {
		p = *ch.Pattern
	}
	if ch.Account != nil {
		account = *ch.Account
	}
	if _, err := r.Reshape(p, account, now); err != nil {
		return 0, err
	}
	if ch.Category != nil {
		c, err := s.repo.category(ctx, *ch.Category)
		if err != nil {
			return 0, fmt.Errorf("categorize: update rule: %w", err)
		}
		if _, err := r.Retarget(c, now); err != nil {
			return 0, err
		}
	}
	if err := s.repo.saveRule(ctx, r); err != nil {
		return 0, fmt.Errorf("categorize: update rule: %w", err)
	}
	return s.Reapply(ctx)
}

// DeleteRule removes a rule. The transactions it categorized keep their
// category until a correction or another rule changes it.
func (s *Service) DeleteRule(ctx context.Context, id RuleID) error {
	if err := s.repo.deleteRule(ctx, id); err != nil {
		return fmt.Errorf("categorize: delete rule: %w", err)
	}
	return nil
}

// Reapply runs the rules over every transaction automation may categorize
// (never one I chose by hand), so a new or changed rule reaches what earlier
// rules or the model decided. It returns how many changed.
func (s *Service) Reapply(ctx context.Context) (int, error) {
	txs, err := s.ledger.Automatable(ctx)
	if err != nil {
		return 0, fmt.Errorf("categorize: reapply: %w", err)
	}
	rules, err := s.repo.rules(ctx)
	if err != nil {
		return 0, fmt.Errorf("categorize: reapply: %w", err)
	}
	current := make(map[ledger.TransactionID]ledger.Categorizable, len(txs))
	for _, t := range txs {
		current[t.ID] = t
	}
	var changed []Decision
	decided := map[ledger.TransactionID]bool{}
	for _, d := range NewEngine(rules).Decide(txs) {
		decided[d.Transaction] = true
		if t := current[d.Transaction]; t.Category != d.Category.String() || t.Source != ledger.SourceRule {
			changed = append(changed, d)
		}
	}
	n, err := s.categorize(ctx, changed, ledger.SourceRule)
	if err != nil {
		return n, fmt.Errorf("categorize: reapply: %w", err)
	}
	// What a rule categorized and no rule meets any more goes back to the
	// review queue, rather than keep a category nothing explains.
	for _, t := range txs {
		if t.Source == ledger.SourceRule && !decided[t.ID] {
			if err := s.ledger.Uncategorize(ctx, t.ID); err != nil {
				return n, fmt.Errorf("categorize: reapply: %w", err)
			}
			n++
		}
	}
	return n, nil
}
