package schedule

import (
	"testing"
	"time"
)

func TestProbeScheduleUsesSixFieldsAndExplicitTimezone(t *testing.T) {
	if err := ValidateProbe("0 0 0 * * *", AgentLocalTimezone); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProbe("0 0 * * *", AgentLocalTimezone); err == nil {
		t.Fatal("five-field Cron expression was accepted")
	}
	next, err := NextProbe(
		"0 0 0 * * *", "Asia/Shanghai",
		time.Date(2026, 8, 9, 15, 59, 59, 0, time.UTC), time.UTC,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next occurrence = %s, want %s", next, want)
	}
}
