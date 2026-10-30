# categorize

**Responsibility:** choose the category of each transaction and learn from corrections.

**Not its responsibility:** storing a transaction's category. That belongs to `ledger`; `categorize` calls `ledger.Categorize`.

## Concepts

- `Category` (simple CRUD, `category.go`): a name, one of the eight note colours (`Color`), and whether it counts toward the monthly budget. Its id is the `ledger.CategoryID` the ledger stores. The migration seeds the categories of the canvas: Transport (1), Health (2), Leisure (5), Restaurants (10), Shopping (20), Groceries (50), Housing (100), Other (200), and Income (2), which does not count toward the budget.
- `Pattern` (value object, `values.go`): what a rule looks for in a description, `exact` or `contains`, normalized: accents dropped, upper case, anything that is not a letter or a digit read as a space. "Pág*iFood SP" and "PAG IFOOD SP" are the same. `contains` matches whole words in a row: "IFOOD" meets "PAG*IFOOD SP", "BAR" never meets "BARBEARIA".
- `Rule` (aggregate, `rule.go`): a pattern, optionally limited to one ledger account, that leads to a category. Its `Origin` says where it came from: a correction (in To review or on a transaction) or written by hand on the Categories and rules screen. `Teach` is the rule a choice teaches: a new one, or the one with the same pattern and account, retargeted by a correction; a rule written by hand never takes it over.
- `Engine` (domain service, `engine.go`): of the rules a transaction meets, picks the most specific (see the invariants).
- `SuggestPattern` (`values.go`): reads the merchant out of a description for a correction to teach: it skips what banks put before it ("PAG*", "Pix enviado", "ESTORNO") and stops at what they append (a number, a city or state, a site), up to three words. "PAG*IFOOD SP" suggests contains IFOOD.
- `GroupForReview` (domain service, `review.go`): the review queue groups the transactions waiting for a category by the pattern each one suggests, the groups that settle the most first, so one correction settles a whole merchant.
- `LLM` (port, `classifier.go`): the little of the local model categorize needs (`Enabled`, `Model`, `GenerateJSON`); `app` injects the Ollama client. The prompt lists the live categories and the rules I taught as examples; `parseAnswer` accepts only a live category, by name.
- `Suggestion` (`classifier.go`, table `suggestions`): what the model said about a merchant (a suggested pattern), asked once per merchant. From 80% sure it categorizes on its own, as the model's choice; below, it waits in the queue as a suggestion.

## Flow

1. Receives `TransactionsImported` (`OnTransactionsImported`) and asks the ledger which of those transactions it may categorize (`ledger.Categorizable`).
2. Tries the rules, from the most specific to the most generic (`Engine.Decide`), and sends each match to `ledger.Categorize` as a rule's choice.
3. If no rule matches and the LLM is enabled, asks the LLM.
4. A manual correction in the UI becomes a rule (`Service.Correct` and `Service.SetCategory`, through `Learn`), and the rules then run over everything still waiting.

After every sync, `app` calls `CategorizeUncategorized` (on `ingest.SyncCompleted`), which runs steps 2 and 3 over every transaction still without a category: what was imported before a rule existed, or while the rules could not run.

The model runs only in that pass after the sync, never during an import, so a slow or sleeping Ollama never holds a sync up. At most 40 new merchants are asked per pass.

## Invariants

- Two live categories never share a name (case does not matter). Archiving one frees its name.
- A category is never deleted, only archived: it leaves the pickers and the LLM's list, and transactions already in it keep showing it. An archived category does not change, and archiving it deletes its rules, which could no longer lead anywhere. The budget lets it go from the month it was archived on (`CategoryArchived`).
- A rule never leads to an archived category.
- At most one rule per pattern (kind and text) and account. A correction that meets a taught pattern retargets that rule instead of adding another. A rule written by hand never does: one whose pattern, once normalized, and account a rule already has is refused (`ErrRuleTaken`), as an edit into them is.
- A pattern has at least 3 letters or digits and at most 100 characters.
- When several rules meet a transaction, the most specific decides: a rule limited to its account before one for every account, then exact before contains, then more words, then more characters, then the most recently taught (my latest correction wins a tie).
- Only live expenses and income get a category from automation. Transfers and card bill payments never do, and neither does a transaction whose category I chose by hand.
- Categorizing never fails an import: if the rules cannot run, the transactions wait without a category and the pass after the sync tries again.
- The TransferMatcher runs after the imports, so a rule may categorize a transaction that the matcher later pairs as a transfer. It keeps that category; totals by category count only expenses and income, so it never counts.
- The LLM is only called when no rule matches.
- The LLM answer must be an existing category. If it is not, the transaction is marked "to review".
- A manual correction always becomes a rule, so the same mistake does not happen twice.
- If the LLM is unavailable (zaros is off, for example), the transaction waits; it never gets a guessed category.
- Every choice can be undone: it answers with what puts it back (the rule as it was, removed if the choice created it, and each transaction it changed with its earlier category and who chose it), and undoing restores exactly that, a manual choice included, skipping a transaction the bank removed since.

## API

- `GET /api/v1/categories`: every category, live ones first, each by name, with `archived`.
- `POST /api/v1/categories` `{"name", "color", "budgeted"}`: `budgeted` defaults to true. 409 when the name is taken.
- `PATCH /api/v1/categories/{id}`: any of `name`, `color` and `budgeted`. 409 for an archived category or a taken name.
- `DELETE /api/v1/categories/{id}`: archives it, publishes `CategoryArchived` and answers `{"rulesRemoved": n}`.
- `GET /api/v1/review`: `{"waiting", "groups"}`, each group with its suggested `pattern` (or null), `count`, `totalCents`, `latest`, its newest five transactions and the model's `suggestion` (`category`, `confidence` in percent) when it named one.
- `POST /api/v1/review` `{"pattern": {"kind", "text"}, "category"}`: teaches the rule and answers `{"categorized", "undo"}`.
- `PUT /api/v1/transactions/{id}/category` `{"category", "teach"}`: my choice for one transaction (manual, never overwritten); with `teach`, it also becomes a rule. Answers `{"categorized", "undo"}`.
- `PUT /api/v1/transactions/{id}/split` `{"parts": [{"category", "amountCents"}]}`: divides a transaction across live categories (ledger checks the parts add up); 204.
- `POST /api/v1/undo`: takes back a choice; the body is the `undo` it answered with (`rule`, `wasRule`, `before`). Answers `{"restored": n}`.
- `GET /api/v1/patterns/suggest?description=…`: the pattern a correction of that description would teach.
- `GET /api/v1/rules`: every rule in the order the engine tries them, each with `order`, `origin` ("correction" or "hand"), `meets` (transactions its pattern matches), `decides` (transactions it settles now) and `last` (the newest of those).
- `POST /api/v1/rules` `{"pattern", "account", "category"}`: writes a rule by hand (`origin` "hand"); `account` is optional. It runs over what is waiting and what automation categorized, and answers `{"categorized"}`. 409 when a rule has the same pattern and account; the form offers to edit that one.
- `POST /api/v1/review/transfer` `{"pattern"}` or `{"description"}`: marks the waiting transactions that match as transfers between my own accounts (through `ledger`), and answers `{"marked", "ids"}`; unmarking the ids takes it back.
- `PATCH /api/v1/rules/{id}` `{"pattern", "account", "category"}`: reshapes it (an empty `account` means every account) and runs the rules again over what automation categorized: what a rule categorized and no rule meets any more goes back to the review queue. 409 when another rule has the same pattern and account.
- `DELETE /api/v1/rules/{id}`: 204. The transactions it categorized keep their category.

## Events

- Consumes: `ledger.TransactionsImported`. After every sync, `app` also runs `CategorizeUncategorized` on `ingest.SyncCompleted`.
- Publishes: `CategoryArchived` (`events.go`), after an archive commits, with the category and when it was archived; `budget` takes it out of the budget from that month on. `ledger` publishes `TransactionCategorized`.

## Milestone

M2
