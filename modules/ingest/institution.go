package ingest

import (
	"strings"

	"lastro/modules/ingest/internal/pluggy"
)

// meuPluggyConnector is the connector of the items the MeuPluggy connector
// creates. Its name says nothing about the bank behind the item.
const meuPluggyConnector = 200

// bankCodes names the banks by their COMPE code, the first segment of a
// Brazilian account's transfer number ("260/0001/…"). The ones people are
// likely to connect; any other falls back to the account's own name.
var bankCodes = map[string]string{
	"001": "Banco do Brasil",
	"033": "Santander",
	"077": "Inter",
	"102": "XP",
	"104": "Caixa",
	"208": "BTG Pactual",
	"212": "Banco Original",
	"237": "Bradesco",
	"260": "Nubank",
	"290": "PagBank",
	"323": "Mercado Pago",
	"336": "C6 Bank",
	"341": "Itaú",
	"380": "PicPay",
	"422": "Safra",
	"655": "Neon",
	"748": "Sicredi",
	"756": "Sicoob",
}

// institutionOf names the bank behind an item. A direct or Open Finance
// connector is the bank itself. A MeuPluggy item is named after the bank
// code of its first bank account, then the first word of an account's name,
// and only then "Meu Pluggy".
func institutionOf(it pluggy.Item, accounts []pluggy.Account) string {
	if it.Connector.ID != meuPluggyConnector && !strings.EqualFold(it.Connector.Name, "MeuPluggy") {
		return it.Connector.Name
	}
	for _, a := range accounts {
		if a.BankData == nil {
			continue
		}
		code, _, _ := strings.Cut(a.BankData.TransferNumber, "/")
		code = strings.TrimSpace(code)
		if len(code) < 3 {
			code = strings.Repeat("0", 3-len(code)) + code
		}
		if name, ok := bankCodes[code]; ok {
			return name
		}
	}
	for _, a := range accounts {
		if a.Type == "BANK" {
			if f := strings.Fields(a.Name); len(f) > 0 {
				return f[0]
			}
		}
	}
	return "Meu Pluggy"
}
