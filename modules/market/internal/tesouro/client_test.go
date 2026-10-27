package tesouro

import (
	"strings"
	"testing"
)

func TestParseKeepsTheLatestDay(t *testing.T) {
	csv := "Tipo Titulo;Data Vencimento;Data Base;Taxa Compra Manha;Taxa Venda Manha;PU Compra Manha;PU Venda Manha;PU Base Manha\n" +
		"Tesouro IPCA+;15/05/2035;24/10/2026;7,20;7,32;2150,10;2120,45;2120,45\n" +
		"Tesouro IPCA+;15/05/2035;23/10/2026;7,18;7,30;2152,00;2122,00;2122,00\n" +
		"Tesouro Selic;01/03/2029;24/10/2026;0,07;0,08;16950,33;16940,12;16940,12\n"
	rows, err := parse(strings.NewReader(csv))
	if err != nil || len(rows) != 2 {
		t.Fatalf("got %+v, %v", rows, err)
	}
	if rows[0].Name != "Tesouro IPCA+" || rows[0].BuyRate != "7,20" || rows[0].Maturity.Year() != 2035 || rows[1].Date.Day() != 24 {
		t.Errorf("rows %+v", rows)
	}
}
