package schedule

import (
	"testing"
	"time"
)

func TestProbeScheduleUsesSixFieldsAndExplicitTimezone(t *testing.T) {
	if err := ValidateProbe("0 0 0 * * *", "UTC"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProbe("0 0 * * *", "UTC"); err == nil {
		t.Fatal("five-field Cron expression was accepted")
	}
	if err := ValidateProbe("0 0 0 * * *", "agent-local"); err == nil {
		t.Fatal("Agent-local timezone sentinel was accepted")
	}
	next, err := NextProbe(
		"0 0 0 * * *", "Asia/Shanghai",
		time.Date(2026, 8, 9, 15, 59, 59, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next occurrence = %s, want %s", next, want)
	}
}

func TestProbeScheduleRejectsExpressionThatNeverMatches(t *testing.T) {
	if err := ValidateProbe("0 0 0 30 2 *", "UTC"); err == nil {
		t.Fatal("impossible calendar date was accepted")
	}
	if _, err := NextProbe("0 0 0 30 2 *", "UTC", time.Now()); err == nil {
		t.Fatal("impossible calendar date produced a next occurrence")
	}
}
