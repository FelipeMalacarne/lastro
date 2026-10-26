package money

import "testing"

func TestParsePercent(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"60", 6000, false},
		{"60%", 6000, false},
		{"12.5", 1250, false},
		{"12,5%", 1250, false},
		{"0.25", 25, false},
		{"100 %", 10000, false},
		{"1,234", 0, true},
		{"abc", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParsePercent(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %v", got)
				}
				return
			}
			if err != nil || got.Bps() != tt.want {
				t.Fatalf("ParsePercent(%q) = %d, %v; want %d", tt.in, got.Bps(), err, tt.want)
			}
		})
	}
}

func TestPercentOf(t *testing.T) {
	tests := []struct {
		p    Percent
		m    Money
		want int64
	}{
		{FromBps(6000), BRLCents(400000), 240000},
		{FromBps(1000), BRLCents(5), 1},   // half a cent rounds up
		{FromBps(1000), BRLCents(4), 0},   // 0.4 cent rounds down
		{FromBps(1000), BRLCents(-5), -1}, // half away from zero
		{FromBps(1450), BRLCents(100000), 14500},
	}
	for _, tt := range tests {
		if got := tt.p.Of(tt.m).Cents(); got != tt.want {
			t.Errorf("%v of %v = %d, want %d", tt.p, tt.m, got, tt.want)
		}
	}
}

func TestPercentString(t *testing.T) {
	tests := map[int64]string{6000: "60%", 1250: "12,5%", 25: "0,25%", -150: "-1,5%", 0: "0%"}
	for bps, want := range tests {
		if got := FromBps(bps).String(); got != want {
			t.Errorf("FromBps(%d) = %q, want %q", bps, got, want)
		}
	}
}
