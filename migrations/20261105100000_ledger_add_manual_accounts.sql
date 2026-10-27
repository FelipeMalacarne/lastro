-- Accounts outside Open Finance, typed in by me: a wallet ('cash'), whose
-- transactions I add and which counts like any other, and an asset whose
-- balance I update now and then (a private pension, FGTS).

-- +goose Up
ALTER TABLE ledger.accounts DROP CONSTRAINT accounts_type_check;
ALTER TABLE ledger.accounts ADD CONSTRAINT accounts_type_check
    CHECK (type IN ('checking', 'savings', 'credit_card', 'cash', 'asset'));

-- +goose Down
DELETE FROM ledger.transactions WHERE account_id IN (SELECT id FROM ledger.accounts WHERE type IN ('cash', 'asset'));
DELETE FROM ledger.balance_snapshots WHERE account_id IN (SELECT id FROM ledger.accounts WHERE type IN ('cash', 'asset'));
DELETE FROM ledger.accounts WHERE type IN ('cash', 'asset');
ALTER TABLE ledger.accounts DROP CONSTRAINT accounts_type_check;
ALTER TABLE ledger.accounts ADD CONSTRAINT accounts_type_check
    CHECK (type IN ('checking', 'savings', 'credit_card'));
