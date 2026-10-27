package ledger

import "time"

// ImportBatch is what a source read for one account.
type ImportBatch struct {
	Account ExternalID
	// From is the first day the source read: every transaction the ledger
	// has in the account dated on or after it that the batch leaves out is
	// removed. A zero From, or an empty batch, never removes anything: an
	// empty read is what a lapsed authorization looks like, not proof that
	// everything is gone.
	From         time.Time
	Transactions []ImportedTransaction
}

// ImportResult counts what an import did.
type ImportResult struct {
	Created, Updated, Unchanged, Removed int
}

// reconciliation is what applying a batch decided.
type reconciliation struct {
	result  ImportResult
	save    []*Transaction  // new, changed and removed transactions
	touched []TransactionID // new and changed, for TransactionsImported
	broken  []TransactionID // the other side of pairs that broke; they must leave their pair
}

// reconcile applies a batch to the transactions the ledger already has for
// the account, as Repository.TransactionsForImport returns them:
//
//   - a known external id updates its transaction in place;
//   - a new external id with a known bank reference re-keys that
//     transaction, so its category survives the source recreating it;
//   - any other new external id creates a transaction;
//   - a transaction dated inside the window that the batch leaves out is
//     removed (see ImportBatch.From).
//
// A batch with the same external id twice keeps the last one.
func reconcile(acc *Account, existing []*Transaction, b ImportBatch, now time.Time) (reconciliation, error) {
	var rec reconciliation
	batch := dedupe(b.Transactions)

	byID, byRef := map[ExternalID]*Transaction{}, map[string]*Transaction{}
	for _, t := range existing {
		byID[t.ExternalID()] = t
		if t.BankRef() != "" {
			byRef[t.BankRef()] = t
		}
	}
	inBatch := make(map[ExternalID]bool, len(batch))
	for _, in := range batch {
		inBatch[in.ExternalID] = true
	}

	seen := map[TransactionID]bool{}
	for _, in := range batch {
		t := byID[in.ExternalID]
		if t == nil && in.BankRef != "" {
			// The same bank transaction under a new source id, unless its old
			// id is also in this batch: then they are two transactions.
			if old := byRef[in.BankRef]; old != nil && !inBatch[old.ExternalID()] && !seen[old.ID()] {
				t = old
			}
		}
		if t == nil {
			nt, err := NewTransaction(acc, in)
			if err != nil {
				return reconciliation{}, err
			}
			seen[nt.ID()] = true
			rec.result.Created++
			rec.save, rec.touched = append(rec.save, nt), append(rec.touched, nt.ID())
			continue
		}
		changed, broken, err := t.ApplyImport(in)
		if err != nil {
			return reconciliation{}, err
		}
		seen[t.ID()] = true
		if broken != "" {
			rec.broken = append(rec.broken, broken)
		}
		if !changed {
			rec.result.Unchanged++
			continue
		}
		rec.result.Updated++
		rec.save, rec.touched = append(rec.save, t), append(rec.touched, t.ID())
	}

	if !b.From.IsZero() && len(batch) > 0 {
		from := civilDate(b.From)
		for _, t := range existing {
			if seen[t.ID()] || t.IsRemoved() || t.Date().Before(from) {
				continue
			}
			if broken := t.Remove(now); broken != "" {
				rec.broken = append(rec.broken, broken)
			}
			rec.result.Removed++
			rec.save = append(rec.save, t)
		}
	}

	// A broken pair whose other side is also in this account is handled here.
	inAccount := map[TransactionID]*Transaction{}
	for _, t := range append(existing, rec.save...) {
		inAccount[t.ID()] = t
	}
	var outside []TransactionID
	for _, id := range rec.broken {
		if t := inAccount[id]; t != nil {
			if t.IsPaired() {
				t.LeavePair()
				rec.save = appendOnce(rec.save, t)
			}
			continue
		}
		outside = append(outside, id)
	}
	rec.broken = outside
	return rec, nil
}

func appendOnce(txs []*Transaction, t *Transaction) []*Transaction {
	for _, x := range txs {
		if x == t {
			return txs
		}
	}
	return append(txs, t)
}

// dedupe keeps the last occurrence of each external id, in batch order.
func dedupe(in []ImportedTransaction) []ImportedTransaction {
	last := make(map[ExternalID]int, len(in))
	for i, t := range in {
		last[t.ExternalID] = i
	}
	out := make([]ImportedTransaction, 0, len(last))
	for i, t := range in {
		if last[t.ExternalID] == i {
			out = append(out, t)
		}
	}
	return out
}
