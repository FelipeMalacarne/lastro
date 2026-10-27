package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"lastro/modules/budget"
	"lastro/modules/ledger"
	"lastro/money"
)

// The MCP server (ADR-0009): read-only tools over the modules' Services, for
// an LLM client I authorized. Tools answer with what a question needs:
// summaries first, transactions only through filters, and never account
// numbers, Pluggy IDs or anything secret.

const mcpInstructions = `lastro is Felipe's personal finance app: bank accounts and cards synced through Brazilian Open Finance, categorized spending, a monthly budget, an investment portfolio and contribution planning.
Amounts are in cents of BRL (amountCents: 123456 is R$ 1.234,56). Months are "YYYY-MM".
Start from monthly_report or budget for questions about a month, and use transactions only to look at specific spending. Every tool only reads.`

var (
	monthRe = regexp.MustCompile(`^\d{4}-\d{2}$`)
	uuidRe  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

func readOnly(title string) *mcp.ToolAnnotations {
	no := false
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: &no, OpenWorldHint: &no}
}

type monthIn struct {
	Month string `json:"month,omitempty" jsonschema:"the month, YYYY-MM; this month when left out"`
}

type reportIn struct {
	Month string `json:"month,omitempty" jsonschema:"the month, YYYY-MM; last month when left out"`
}

type transactionsIn struct {
	Month    string `json:"month,omitempty" jsonschema:"the month, YYYY-MM; this month when left out"`
	Search   string `json:"search,omitempty" jsonschema:"words in the description, ignoring case and accents"`
	Category string `json:"category,omitempty" jsonschema:"a category id from the categories tool, or none for the uncategorized"`
	Account  string `json:"account,omitempty" jsonschema:"an account id from the accounts tool"`
	Flow     string `json:"flow,omitempty" jsonschema:"in, out or transfer"`
	Tag      string `json:"tag,omitempty" jsonschema:"a tag, without the #"`
}

type planIn struct {
	AmountCents int64 `json:"amountCents,omitempty" jsonschema:"the contribution in cents; this month's pay yourself first when left out"`
}

// mcpAccount is an account without its number.
type mcpAccount struct {
	ID           ledger.AccountID `json:"id"`
	Name         string           `json:"name"`
	Institution  string           `json:"institution"`
	Type         string           `json:"type"`
	Currency     string           `json:"currency"`
	BalanceCents int64            `json:"balanceCents"`
	BalanceAt    time.Time        `json:"balanceAt"`
	Manual       bool             `json:"manual"`
	Card         *ledger.CardView `json:"card"`
}

type mcpCategory struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Budgeted bool   `json:"budgeted"`
	Archived bool   `json:"archived"`
}

// MCPServer builds the server with every tool.
func (a *App) MCPServer() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "lastro", Title: "lastro", Version: a.Version}, &mcp.ServerOptions{Instructions: mcpInstructions, Logger: a.Logger})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "monthly_report",
		Description: "The month in review: income, spending and what was saved, each category against its limit and the month before, and what stood out.",
		Annotations: readOnly("Monthly report"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in reportIn) (*mcp.CallToolResult, any, error) {
		m, err := a.toolMonth(in.Month, -1)
		if err != nil {
			return nil, nil, err
		}
		v, err := a.Report(ctx, m)
		return nil, v, a.toolErr(ctx, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "budget",
		Description: "A month's budget as it stands: pay yourself first, income, each category's limit, spending and pace, and what is left.",
		Annotations: readOnly("Budget"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in monthIn) (*mcp.CallToolResult, any, error) {
		m, err := a.toolMonth(in.Month, 0)
		if err != nil {
			return nil, nil, err
		}
		v, err := a.Budget.Month(ctx, m)
		return nil, v, a.toolErr(ctx, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "transactions",
		Description: "A month's transactions, narrowed by search, category, account, flow or tag, with a summary. Prefer narrowing: a whole month is long.",
		Annotations: readOnly("Transactions"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in transactionsIn) (*mcp.CallToolResult, any, error) {
		f, err := a.toolFilter(in)
		if err != nil {
			return nil, nil, err
		}
		v, err := a.Ledger.Transactions(ctx, f)
		if errors.Is(err, ledger.ErrInvalid) {
			return nil, nil, err
		}
		return nil, v, a.toolErr(ctx, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "accounts",
		Description: "Every account and card with its balance and, for cards, the limit and the bill's dates.",
		Annotations: readOnly("Accounts"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		all, err := a.Ledger.Accounts(ctx)
		if err != nil {
			return nil, nil, a.toolErr(ctx, err)
		}
		out := make([]mcpAccount, len(all))
		for i, v := range all {
			out[i] = mcpAccount{ID: v.ID, Name: v.Name, Institution: v.Institution, Type: v.Type, Currency: v.Currency,
				BalanceCents: v.BalanceCents, BalanceAt: v.BalanceAt, Manual: v.Manual, Card: v.Card}
		}
		return nil, map[string]any{"accounts": out}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "categories",
		Description: "The categories, with their ids for the transactions tool.",
		Annotations: readOnly("Categories"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		cs, err := a.Categorize.Categories(ctx)
		if err != nil {
			return nil, nil, a.toolErr(ctx, err)
		}
		out := make([]mcpCategory, len(cs))
		for i, c := range cs {
			out[i] = mcpCategory{ID: c.ID.String(), Name: c.Name, Budgeted: c.Budgeted, Archived: c.Archived}
		}
		return nil, map[string]any{"categories": out}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "portfolio",
		Description: "The investment portfolio: net worth, each class's value against its target allocation, and the positions.",
		Annotations: readOnly("Portfolio"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		v, err := a.Portfolio.Overview(ctx)
		return nil, v, a.toolErr(ctx, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "contribution_plan",
		Description: "Where a contribution should go, by asset class, to bring the portfolio back toward its targets; the same planner as the Contribute screen.",
		Annotations: readOnly("Contribution plan"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in planIn) (*mcp.CallToolResult, any, error) {
		if in.AmountCents < 0 {
			return nil, nil, errors.New("amountCents must not be negative")
		}
		amount := in.AmountCents
		if amount == 0 {
			v, err := a.Budget.Month(ctx, budget.MonthOf(a.Portfolio.Today()))
			if err != nil {
				return nil, nil, a.toolErr(ctx, err)
			}
			if amount = v.PayYourselfFirst.PlannedCents; amount == 0 {
				return nil, nil, errors.New("pay yourself first is not set this month: pass amountCents")
			}
		}
		v, err := a.Portfolio.Plan(ctx, money.BRLCents(amount))
		return nil, v, a.toolErr(ctx, err)
	})

	return s
}

// MCPHandler serves MCP over streamable HTTP, statelessly: every request
// stands alone, so nothing is kept between calls.
func (a *App) MCPHandler() http.Handler {
	s := a.MCPServer()
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, Logger: a.Logger})
}

// toolMonth reads a month, or takes this month plus offset when it is left out.
func (a *App) toolMonth(s string, offset int) (budget.Month, error) {
	if s == "" {
		return budget.MonthOf(a.Portfolio.Today()).Add(offset), nil
	}
	if !monthRe.MatchString(s) {
		return budget.Month{}, errors.New(`month must look like "2026-10"`)
	}
	return budget.ParseMonth(s)
}

func (a *App) toolFilter(in transactionsIn) (ledger.TransactionFilter, error) {
	m, err := a.toolMonth(in.Month, 0)
	if err != nil {
		return ledger.TransactionFilter{}, err
	}
	f := ledger.TransactionFilter{Month: m.First(), Search: in.Search, Category: in.Category, Account: ledger.AccountID(in.Account)}
	if len([]rune(f.Search)) > 100 {
		return f, errors.New("search is limited to 100 characters")
	}
	if in.Account != "" && !uuidRe.MatchString(in.Account) {
		return f, errors.New("account must be an account id from the accounts tool")
	}
	if in.Category != "" && in.Category != ledger.CategoryNone && !uuidRe.MatchString(in.Category) {
		return f, errors.New(`category must be a category id from the categories tool, or "none"`)
	}
	if f.Flow, err = ledger.ParseFlow(in.Flow); err != nil {
		return f, errors.New(`flow must be "in", "out" or "transfer"`)
	}
	if in.Tag != "" {
		tag, err := ledger.NewTag(in.Tag)
		if err != nil {
			return f, err
		}
		f.Tag = string(tag)
	}
	return f, nil
}

// toolErr keeps internal errors out of the conversation: they go to the log,
// and the model gets a plain message.
func (a *App) toolErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	a.Logger.ErrorContext(ctx, "mcp tool failed", "err", err)
	return fmt.Errorf("lastro could not answer this right now")
}
