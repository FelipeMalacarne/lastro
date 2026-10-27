package ingest

import (
	"testing"

	"lastro/modules/ingest/internal/pluggy"
)

func TestInstitutionOf(t *testing.T) {
	meu := pluggy.Item{Connector: pluggy.Connector{ID: 200, Name: "MeuPluggy"}}
	bank := func(transfer, name string) pluggy.Account {
		return pluggy.Account{Type: "BANK", Name: name, BankData: &pluggy.BankData{TransferNumber: transfer}}
	}
	card := pluggy.Account{Type: "CREDIT", Name: "croma-platinum"}
	tests := []struct {
		name     string
		item     pluggy.Item
		accounts []pluggy.Account
		want     string
	}{
		{"a bank's own connector", pluggy.Item{Connector: pluggy.Connector{ID: 612, Name: "Nubank"}}, nil, "Nubank"},
		{"MeuPluggy, by bank code", meu, []pluggy.Account{card, bank("260/0001/12345-6", "Nu Pagamentos S.A.")}, "Nubank"},
		{"a short code", meu, []pluggy.Account{bank("1/1234/5678-9", "Banco do Brasil S.A.")}, "Banco do Brasil"},
		{"an unknown code", meu, []pluggy.Account{bank("999/0001/1-1", "Banco Exemplo S.A.")}, "Banco"},
		{"only a card", meu, []pluggy.Account{card}, "Meu Pluggy"},
	}
	for _, tt := range tests {
		if got := institutionOf(tt.item, tt.accounts); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
