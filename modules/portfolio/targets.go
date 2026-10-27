package portfolio

import (
	"time"

	"lastro/money"
)

// Targets is the target allocation and its band: what the planner aims for.
type Targets struct {
	Allocation Allocation
	Band       Band
	UpdatedAt  time.Time
}

// DefaultTargets is where a new portfolio starts until I set mine: a core of
// fixed income and global stocks, with a small Brazilian satellite.
func DefaultTargets() Targets {
	a, _ := NewAllocation(map[Class]money.Percent{
		ClassCDI: money.FromBps(2500), ClassIPCA: money.FromBps(2500), ClassBR: money.FromBps(1000), ClassGlobal: money.FromBps(4000),
	})
	b, _ := NewBand(200)
	return Targets{Allocation: a, Band: b}
}

// Status is where a class stands against its band.
type Status string

const (
	InBand    Status = "in_band"
	BelowBand Status = "below"
	AboveBand Status = "above"
)

// StatusOf compares a class's weight with its target and band.
func (t Targets) StatusOf(c Class, weight money.Percent) Status {
	target := t.Allocation.Of(c).Bps()
	switch {
	case weight.Bps() < target-t.Band.Bps():
		return BelowBand
	case weight.Bps() > target+t.Band.Bps():
		return AboveBand
	}
	return InBand
}
