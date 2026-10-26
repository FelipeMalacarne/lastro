# categorize

**Responsibility:** choose the category of each transaction and learn from corrections.

**Not its responsibility:** storing a transaction's category. That belongs to `ledger`; `categorize` calls `ledger.Categorize`.

## Concepts

- `Rule` (aggregate): a pattern (normalized merchant, description fragment, account) that leads to a category.
- `Category` (simple CRUD): name, group and whether it counts toward the budget.
- `Classifier` (port): minimal interface implemented by the Ollama client and injected by `app`.

## Flow

1. Receives `TransactionsImported`.
2. Tries the rules, from the most specific to the most generic.
3. If no rule matches and the LLM is enabled, asks the LLM.
4. A manual correction in the UI becomes a new rule.

## Invariants

- The LLM is only called when no rule matches.
- The LLM answer must be an existing category. If it is not, the transaction is marked "to review".
- A manual correction always becomes a rule, so the same mistake does not happen twice.
- If the LLM is unavailable (zaros is off, for example), the transaction waits; it never gets a guessed category.

## Events

- Consumes: `ledger.TransactionsImported`.
- Publishes: none. `ledger` publishes `TransactionCategorized`.

## Phase

2
