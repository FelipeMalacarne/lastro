# budget

**Responsibility:** monthly budget per category and tracking of spending against it.

**Not its responsibility:** choosing categories or calculating investments.

## Concepts

- `MonthlyBudget` (aggregate): the budget for one `YearMonth`, with a limit per category.
- "Pay yourself first": the month's investment contribution is a fixed budget line, set aside on payday.
- Rollover: money left in a category may carry over to the next month, according to that category's rule.

## Invariants

- There is at most one budget per month.
- The planned contribution is never treated as spending.
- An installment purchase counts toward the budget by the month's installment, not by its total value.

## Events

- Consumes: `ledger.TransactionCategorized`.

## Milestone

M3
