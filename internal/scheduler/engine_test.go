package scheduler

import (
	"testing"
	"time"
)

func TestParseTimeMinutes(t *testing.T) {
	if m, err := ParseTimeMinutes("8:05"); err != nil || m != 485 {
		t.Fatalf("ParseTimeMinutes('8:05') = (%d, %v)", m, err)
	}
	if m, err := ParseTimeMinutes("00:00"); err != nil || m != 0 {
		t.Fatalf("ParseTimeMinutes('00:00') = (%d, %v)", m, err)
	}
	for _, bad := range []string{"24:00", "12:60", "-1:00", "abc", "12"} {
		if _, err := ParseTimeMinutes(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestIsWithinSchedule(t *testing.T) {
	mkTime := func(h, m int) time.Time {
		return time.Date(2026, 11, 18, h, m, 0, 0, time.Local)
	}

	cases := []struct {
		name    string
		now     time.Time
		enabled bool
		start   string
		stop    string
		want    bool
	}{
		{"disabled", mkTime(3, 0), false, "08:00", "20:00", true},
		{"equal start stop", mkTime(3, 0), true, "08:00", "08:00", true},
		{"daytime inside", mkTime(12, 0), true, "08:00", "20:00", true},
		{"daytime start boundary", mkTime(8, 0), true, "08:00", "20:00", true},
		{"daytime stop boundary", mkTime(20, 0), true, "08:00", "20:00", false},
		{"overnight 08-00 at 23:59", mkTime(23, 59), true, "08:00", "00:00", true},
		{"overnight 08-00 at 00:00", mkTime(0, 0), true, "08:00", "00:00", false},
		{"overnight 22-06 at 01:30", mkTime(1, 30), true, "22:00", "06:00", true},
		{"overnight 22-06 at 12:00", mkTime(12, 0), true, "22:00", "06:00", false},
	}

	for _, tc := range cases {
		got, err := IsWithinSchedule(tc.now, tc.enabled, tc.start, tc.stop)
		if err != nil {
			t.Fatalf("%s unexpected err: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestNextAlignedTick(t *testing.T) {
	base := time.Date(2026, 11, 18, 12, 0, 15, 0, time.UTC)
	next := nextAlignedTick(base, 60)
	want := time.Date(2026, 11, 18, 12, 1, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("expected %s, got %s", want, next)
	}
}
