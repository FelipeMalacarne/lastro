package ledger

// TransactionsImported is published after an import commits. It lists the
// transactions that are new or whose data changed, so categorize can look at
// them; removed transactions are not listed.
type TransactionsImported struct {
	TransactionIDs []TransactionID
}

func (TransactionsImported) EventName() string { return "ledger.transactions_imported" }

// TransactionCategorized is published after a transaction's category changes.
type TransactionCategorized struct {
	TransactionID TransactionID
	Category      CategoryID
	Source        Source
}

func (TransactionCategorized) EventName() string { return "ledger.transaction_categorized" }
