package operation_setting

import "testing"

func TestIsMinuteInAutoTestChannelTimeRange(t *testing.T) {
	tests := []struct {
		name   string
		range_ string
		minute int
		want   bool
	}{
		{name: "inside normal range", range_: "08:00-23:59", minute: 8 * 60, want: true},
		{name: "outside normal range", range_: "08:00-23:59", minute: 7*60 + 59, want: false},
		{name: "inside cross midnight before midnight", range_: "22:00-06:00", minute: 23 * 60, want: true},
		{name: "inside cross midnight after midnight", range_: "22:00-06:00", minute: 5 * 60, want: true},
		{name: "outside cross midnight", range_: "22:00-06:00", minute: 12 * 60, want: false},
		{name: "same start and end means all day", range_: "00:00-00:00", minute: 12 * 60, want: true},
		{name: "invalid range falls back to allowed", range_: "bad", minute: 12 * 60, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsMinuteInAutoTestChannelTimeRange(tt.minute, tt.range_)
			if got != tt.want {
				t.Fatalf("IsMinuteInAutoTestChannelTimeRange(%d, %q) = %v, want %v", tt.minute, tt.range_, got, tt.want)
			}
		})
	}
}

func TestParseAutoTestChannelTimeRange(t *testing.T) {
	start, end, err := ParseAutoTestChannelTimeRange("08:00-23:59")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start != 8*60 || end != 23*60+59 {
		t.Fatalf("got start=%d end=%d, want start=%d end=%d", start, end, 8*60, 23*60+59)
	}

	if _, _, err := ParseAutoTestChannelTimeRange("24:00-08:00"); err == nil {
		t.Fatal("expected invalid hour error")
	}
}
