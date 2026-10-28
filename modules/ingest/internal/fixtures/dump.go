// Package fixtures downloads real Pluggy responses, reports what they contain
// without revealing any of it, and anonymizes them into test fixtures.
//
// The layout is the same for a raw dump and for the fixtures in testdata:
//
//	items/<item id>.json           the item
//	accounts/<item id>.json        its accounts
//	transactions/<account id>.json every transaction of the account
//	investments/<item id>.json     its investments
//	investment-transactions/<investment id>.json
//	                               every movement of the investment
//	bills/<account id>.json        a card's closed bills
package fixtures

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"lastro/modules/ingest/internal/pluggy"
)

// Dump writes everything Pluggy returns for the items into dir. The files
// hold real financial data: dir must stay out of git.
func Dump(ctx context.Context, c *pluggy.Client, itemIDs []string, dir string) error {
	for _, id := range itemIDs {
		item, err := c.Item(ctx, id)
		if err != nil {
			return err
		}
		if err := writeJSON(dir, "items", id, item.Raw); err != nil {
			return err
		}
		accounts, err := c.Accounts(ctx, id)
		if err != nil {
			return err
		}
		if err := writeJSON(dir, "accounts", id, raws(accounts, func(a pluggy.Account) json.RawMessage { return a.Raw })); err != nil {
			return err
		}
		for _, a := range accounts {
			txs, err := c.Transactions(ctx, a.ID, time.Time{})
			if err != nil {
				return err
			}
			if err := writeJSON(dir, "transactions", a.ID, raws(txs, func(t pluggy.Transaction) json.RawMessage { return t.Raw })); err != nil {
				return err
			}
			if a.Type != "CREDIT" {
				continue
			}
			// Pluggy documents bills for Open Finance connections only: a card
			// whose bills cannot be read is dumped without them.
			if bills, err := c.Bills(ctx, a.ID); err == nil {
				if err := writeJSON(dir, "bills", a.ID, raws(bills, func(b pluggy.Bill) json.RawMessage { return b.Raw })); err != nil {
					return err
				}
			}
		}
		invs, err := c.Investments(ctx, id)
		if err != nil {
			return err
		}
		if err := writeJSON(dir, "investments", id, raws(invs, func(i pluggy.Investment) json.RawMessage { return i.Raw })); err != nil {
			return err
		}
		for _, inv := range invs {
			txs, err := c.InvestmentTransactions(ctx, inv.ID)
			if err != nil {
				return err
			}
			if err := writeJSON(dir, "investment-transactions", inv.ID, raws(txs, func(t pluggy.InvestmentTransaction) json.RawMessage { return t.Raw })); err != nil {
				return err
			}
		}
	}
	return nil
}

func raws[T any](xs []T, raw func(T) json.RawMessage) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(xs))
	for _, x := range xs {
		out = append(out, raw(x))
	}
	return out
}

func writeJSON(dir, kind, name string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("fixtures: %s/%s: %w", kind, name, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, kind), 0o700); err != nil {
		return fmt.Errorf("fixtures: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, kind, name+".json"), append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("fixtures: %w", err)
	}
	return nil
}

// Set is a dump read back from disk.
type Set struct {
	Items        []pluggy.Item
	Accounts     map[string][]pluggy.Account     // by item id
	Transactions map[string][]pluggy.Transaction // by account id
	Investments  map[string][]pluggy.Investment  // by item id
	// InvestmentTransactions are by investment id.
	InvestmentTransactions map[string][]pluggy.InvestmentTransaction
	// Bills are by card account id; a card missing here had none read.
	Bills map[string][]pluggy.Bill
}

// Load reads a dump or a fixtures directory.
func Load(dir string) (Set, error) {
	s := Set{
		Accounts:     map[string][]pluggy.Account{},
		Transactions: map[string][]pluggy.Transaction{},
		Investments:  map[string][]pluggy.Investment{},

		InvestmentTransactions: map[string][]pluggy.InvestmentTransaction{},
		Bills:                  map[string][]pluggy.Bill{},
	}
	err := eachFile(dir, "items", func(name string, b []byte) error {
		var it pluggy.Item
		if err := json.Unmarshal(b, &it); err != nil {
			return err
		}
		it.Raw = b
		s.Items = append(s.Items, it)
		return nil
	})
	if err == nil {
		err = eachFile(dir, "accounts", func(name string, b []byte) error {
			v, err := decodeList[pluggy.Account](b, func(a *pluggy.Account, r json.RawMessage) { a.Raw = r })
			s.Accounts[name] = v
			return err
		})
	}
	if err == nil {
		err = eachFile(dir, "transactions", func(name string, b []byte) error {
			v, err := decodeList[pluggy.Transaction](b, func(t *pluggy.Transaction, r json.RawMessage) { t.Raw = r })
			s.Transactions[name] = v
			return err
		})
	}
	if err == nil {
		err = eachFile(dir, "investments", func(name string, b []byte) error {
			v, err := decodeList[pluggy.Investment](b, func(i *pluggy.Investment, r json.RawMessage) { i.Raw = r })
			s.Investments[name] = v
			return err
		})
	}
	if err == nil {
		err = eachFile(dir, "investment-transactions", func(name string, b []byte) error {
			v, err := decodeList[pluggy.InvestmentTransaction](b, func(t *pluggy.InvestmentTransaction, r json.RawMessage) { t.Raw = r })
			s.InvestmentTransactions[name] = v
			return err
		})
	}
	if err == nil {
		err = eachFile(dir, "bills", func(name string, b []byte) error {
			v, err := decodeList[pluggy.Bill](b, func(x *pluggy.Bill, r json.RawMessage) { x.Raw = r })
			s.Bills[name] = v
			return err
		})
	}
	if err != nil {
		return Set{}, fmt.Errorf("fixtures: load %s: %w", dir, err)
	}
	return s, nil
}

func decodeList[T any](b []byte, setRaw func(*T, json.RawMessage)) ([]T, error) {
	var rs []json.RawMessage
	if err := json.Unmarshal(b, &rs); err != nil {
		return nil, err
	}
	out := make([]T, len(rs))
	for i, r := range rs {
		if err := json.Unmarshal(r, &out[i]); err != nil {
			return nil, err
		}
		setRaw(&out[i], r)
	}
	return out, nil
}

// eachFile calls fn for every <dir>/<kind>/*.json, in name order. A missing
// kind directory is not an error.
func eachFile(dir, kind string, fn func(name string, b []byte) error) error {
	paths, err := filepath.Glob(filepath.Join(dir, kind, "*.json"))
	if err != nil {
		return err
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		name := filepath.Base(p)
		if err := fn(name[:len(name)-len(".json")], b); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	return nil
}
