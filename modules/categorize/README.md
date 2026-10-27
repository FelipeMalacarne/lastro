# categorize

**Responsibility:** choose the category of each transaction and learn from corrections.

**Not its responsibility:** storing a transaction's category. That belongs to `ledger`; `categorize` calls `ledger.Categorize`.

## Concepts

- `Category` (simple CRUD, `category.go`): a name, one of the eight note colours (`Color`), and whether it counts toward the monthly budget. Its id is the `ledger.CategoryID` the ledger stores. The migration seeds the categories of the canvas: Transport (1), Health (2), Leisure (5), Restaurants (10), Shopping (20), Groceries (50), Housing (100), Other (200), and Income (2), which does not count toward the budget.
- `Pattern` (value object, `values.go`): what a rule looks for in a description, `exact` or `contains`, normalized: accents dropped, upper case, anything that is not a letter or a digit read as a space. "Pág*iFood SP" and "PAG IFOOD SP" are the same. `contains` matches whole words in a row: "IFOOD" meets "PAG*IFOOD SP", "BAR" never meets "BARBEARIA".
- `Rule` (aggregate, `rule.go`): a pattern, optionally limited to one ledger account, that leads to a category.
- `Engine` (domain service, `engine.go`): of the rules a transaction meets, picks the most specific (see the invariants).
- `Classifier` (port, from M2's LLM task): minimal interface implemented by the Ollama client and injected by `app`.

## Flow

1. Receives `TransactionsImported`.
2. Tries the rules, from the most specific to the most generic.
3. If no rule matches and the LLM is enabled, asks the LLM.
4. A manual correction in the UI becomes a rule (`Service.Learn`).

Today the module has the categories, the rules and the engine; the handler (step 1) and the LLM (step 3) arrive with the next M2 tasks.

## Invariants

- Two live categories never share a name (case does not matter). Archiving one frees its name.
- A category is never deleted, only archived: it leaves the pickers and the LLM's list, and transactions already in it keep showing it. An archived category does not change, and archiving it deletes its rules, which could no longer lead anywhere.
- A rule never leads to an archived category.
- At most one rule per pattern (kind and text) and account. A correction that meets a taught pattern retargets that rule instead of adding another.
- A pattern has at least 3 letters or digits and at most 100 characters.
- When several rules meet a transaction, the most specific decides: a rule limited to its account before one for every account, then exact before contains, then more words, then more characters, then the most recently taught (my latest correction wins a tie).
- The LLM is only called when no rule matches.
- The LLM answer must be an existing category. If it is not, the transaction is marked "to review".
- A manual correction always becomes a rule, so the same mistake does not happen twice.
- If the LLM is unavailable (zaros is off, for example), the transaction waits; it never gets a guessed category.

## API

- `GET /api/v1/categories`: every category, live ones first, each by name, with `archived`.
- `POST /api/v1/categories` `{"name", "color", "budgeted"}`: `budgeted` defaults to true. 409 when the name is taken.
- `PATCH /api/v1/categories/{id}`: any of `name`, `color` and `budgeted`. 409 for an archived category or a taken name.
- `DELETE /api/v1/categories/{id}`: archives it and answers `{"rulesRemoved": n}`.

## Events

- Consumes: `ledger.TransactionsImported` (from the handler task).
- Publishes: none. `ledger` publishes `TransactionCategorized`.

## Milestone

M2
