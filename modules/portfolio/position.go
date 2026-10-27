package portfolio

import (
	"fmt"
	"strings"
	"time"

	"lastro/money"
)

// Kind is what an investment is, as the bank reports it.
type Kind string

const (
	KindFixedIncome Kind = "fixed_income" // CDB, RDB, LCI, LCA, debentures
	KindTreasury    Kind = "treasury"     // Tesouro Direto
	KindStock       Kind = "stock"
	KindETF         Kind = "etf"
	KindBDR         Kind = "bdr"
	KindFII         Kind = "fii" // real estate fund
	KindFund        Kind = "fund"
	KindOther       Kind = "other"
)

// Indexer is what a fixed-income position's rate follows.
type Indexer string

const (
	IndexerNone  Indexer = ""
	IndexerCDI   Indexer = "CDI"
	IndexerSelic Indexer = "SELIC"
	IndexerIPCA  Indexer = "IPCA"
	IndexerPre   Indexer = "PRE" // a fixed rate
)

// Report is one investment as a bank reports it on a sync. ingest maps
// Pluggy's investments into it; nothing about Pluggy leaves ingest.
type Report struct {
	ExternalID  string
	Item        string
	Institution string
	Kind        Kind
	Name        string
	Ticker      Ticker // zero for fixed income
	Quantity    Quantity
	Value       money.Money  // net of taxes, as the bank values it
	Invested    *money.Money // what was put in, when the bank says
	Indexer     Indexer
	Rate        money.Percent // 100% of CDI is 10000; IPCA + 6,5% is 650
	Maturity    *time.Time
	AsOf        time.Time
	Active      bool
}

// Position is one investment held: what the bank reported last, what was
// put into it (its basis, for returns), and how I classify it.
type Position struct {
	Report
	basis money.Money // what is still in it, at cost
	// realized is what came out beyond its cost: cash out minus the basis
	// that left with it, over the position's life. Returns count money taken
	// out at what it was worth, not at what it cost.
	realized  money.Money
	override  Class // empty: Classify decides
	emergency bool
	firstSeen time.Time
	closedAt  *time.Time
}

// NewPosition starts following an investment. Its basis is what the bank
// says was invested, or else its value today.
func NewPosition(r Report, now time.Time) (*Position, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	p := &Position{Report: r, firstSeen: now, realized: money.BRLCents(0)}
	p.basis = r.Value
	if r.Invested != nil {
		p.basis = *r.Invested
	}
	if !r.Active {
		p.closedAt = &now
	}
	return p, nil
}

// RestorePosition rebuilds a stored position, for the repository.
func RestorePosition(r Report, basis, realized money.Money, override Class, emergency bool, firstSeen time.Time, closedAt *time.Time) *Position {
	return &Position{Report: r, basis: basis, realized: realized, override: override, emergency: emergency, firstSeen: firstSeen, closedAt: closedAt}
}

func (r Report) validate() error {
	switch {
	case r.ExternalID == "":
		return fmt.Errorf("%w: position without an id", ErrInvalid)
	case r.Value.Currency() != money.BRL:
		return fmt.Errorf("%w: position %s in %s", ErrInvalid, r.ExternalID, r.Value.Currency())
	case r.Value.IsNegative():
		return fmt.Errorf("%w: position %s worth %s", ErrInvalid, r.ExternalID, r.Value)
	}
	return nil
}

// Update takes a new report. Without the bank's own figure, buying more
// adds to the basis at today's unit price and selling takes from it in
// proportion; what a sale brings in beyond that cost is realized. With the
// bank's figure, a smaller invested amount is a redemption, cashed at the
// position's value per real invested before it.
func (p *Position) Update(r Report, now time.Time) error {
	if err := r.validate(); err != nil {
		return err
	}
	if !r.Active {
		// Withdrawn: the bank may report nothing left, so the last value is
		// what came out.
		cash := r.Value
		if cash.IsZero() {
			cash = p.Value
		}
		p.Report = r
		p.close(cash, now)
		return nil
	}
	old := p.basis.Cents()
	switch {
	case r.Invested != nil:
		if in := r.Invested.Cents(); in < old && old > 0 {
			removed := old - in
			cash := removed * p.Value.Cents() / old
			p.realize(cash - removed)
		}
		p.basis = *r.Invested
	case !p.Quantity.IsZero() && r.Quantity != p.Quantity:
		q0, q1 := p.Quantity.Micro(), r.Quantity.Micro()
		if q1 > q0 {
			// The new units cost today's unit price.
			unit := r.Value.Cents() * 1_000_000 / q1
			added := QuantityFromMicro(q1 - q0).Of(money.BRLCents(unit))
			p.basis = money.BRLCents(old + added.Cents())
		} else {
			sold := q0 - q1
			removed := old * sold / q0
			// What the sold units were worth: today's unit price, or the
			// last value when nothing is left to price them by.
			cash := p.Value.Cents() * sold / q0
			if q1 > 0 {
				cash = r.Value.Cents() * sold / q1
			}
			p.realize(cash - removed)
			p.basis = money.BRLCents(old - removed)
		}
	}
	p.Report = r
	p.closedAt = nil
	return nil
}

func (p *Position) realize(cents int64) {
	p.realized = money.BRLCents(p.realized.Cents() + cents)
}

// close takes the whole position out at a value.
func (p *Position) close(cash money.Money, now time.Time) {
	if p.closedAt != nil {
		return
	}
	p.realize(cash.Cents() - p.basis.Cents())
	p.basis = money.BRLCents(0)
	p.Active = false
	p.closedAt = &now
}

// Close marks a position the bank no longer reports, taken out at its last
// value.
func (p *Position) Close(now time.Time) { p.close(p.Value, now) }

func (p *Position) Basis() money.Money    { return p.basis }
func (p *Position) Realized() money.Money { return p.realized }

// NetIn is what went into the position minus what came out, at value: the
// flow a return is measured against.
func (p *Position) NetIn() money.Money {
	return money.BRLCents(p.basis.Cents() - p.realized.Cents())
}
func (p *Position) Emergency() bool      { return p.emergency }
func (p *Position) Override() Class      { return p.override }
func (p *Position) FirstSeen() time.Time { return p.firstSeen }
func (p *Position) ClosedAt() *time.Time { return p.closedAt }
func (p *Position) Open() bool           { return p.closedAt == nil }

// Class is the allocation class it counts in: mine when I chose one,
// Classify's otherwise.
func (p *Position) Class() Class {
	if p.override != "" {
		return p.override
	}
	return Classify(p.Kind, p.Indexer, p.Ticker, p.Name)
}

// Reclassify puts it in another class, or back to Classify's with "".
func (p *Position) Reclassify(c Class) error {
	if c != "" {
		if _, err := ParseClass(string(c)); err != nil {
			return err
		}
	}
	p.override = c
	return nil
}

// MarkEmergency sets it apart as emergency fund, out of the allocation.
func (p *Position) MarkEmergency(on bool) { p.emergency = on }

// globalETFs are B3 ETFs that hold stocks abroad.
var globalETFs = map[string]bool{
	"IVVB11": true, "SPXI11": true, "WRLD11": true, "NASD11": true, "ACWI11": true, "URTH11": true,
	"EURP11": true, "XINA11": true, "ASIA11": true, "SMAC11": true, "TECK11": true, "USTK11": true,
	"BNDX11": true, "GOLD11": true, "HASH11": true, "QBTC11": true, "BITH11": true, "ETHE11": true,
}

// Classify puts an investment in a class by what it is: inflation-linked
// bonds in IPCA+, the rest of fixed income in CDI, ETFs of stocks abroad
// and BDRs in global, and Brazilian stocks, FIIs and ETFs in Brazilian
// stocks. I can always move one by hand.
func Classify(kind Kind, idx Indexer, t Ticker, name string) Class {
	n := strings.ToUpper(name)
	switch kind {
	case KindTreasury:
		if strings.Contains(n, "IPCA") || strings.Contains(n, "RENDA+") || strings.Contains(n, "EDUCA+") || idx == IndexerIPCA {
			return ClassIPCA
		}
		return ClassCDI
	case KindFixedIncome, KindFund, KindOther:
		if idx == IndexerIPCA {
			return ClassIPCA
		}
		return ClassCDI
	case KindBDR:
		return ClassGlobal
	case KindETF:
		if globalETFs[t.String()] {
			return ClassGlobal
		}
		return ClassBR
	case KindStock:
		// BDRs end in 32 to 39.
		if s := t.String(); len(s) == 6 && s[4] == '3' && s[5] >= '2' {
			return ClassGlobal
		}
		return ClassBR
	case KindFII:
		return ClassBR
	}
	return ClassCDI
}
