package schedule

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

var probeParser = cron.NewParser(
	cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
)

func ValidateProbe(expression, timezone string) error {
	if strings.TrimSpace(expression) != expression || len(expression) < 9 || len(expression) > 128 {
		return errors.New("probe Cron expression must be trimmed and between 9 and 128 bytes")
	}
	parsed, err := probeParser.Parse(expression)
	if err != nil {
		return fmt.Errorf("parse six-field probe Cron expression: %w", err)
	}
	if err := ValidateTimezone(timezone); err != nil {
		return err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return err
	}
	if parsed.Next(time.Now().In(location)).IsZero() {
		return errors.New("probe Cron expression never matches a real date")
	}
	return nil
}

func ValidateTimezone(timezone string) error {
	if strings.TrimSpace(timezone) != timezone || timezone == "" || len(timezone) > 128 {
		return errors.New("probe timezone must be a trimmed IANA name")
	}
	location, err := time.LoadLocation(timezone)
	if err != nil || location.String() != timezone {
		return errors.New("probe timezone must be a valid IANA name")
	}
	return nil
}

func NextProbe(expression, timezone string, after time.Time) (time.Time, error) {
	if err := ValidateProbe(expression, timezone); err != nil {
		return time.Time{}, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	parsed, err := probeParser.Parse(expression)
	if err != nil {
		return time.Time{}, err
	}
	next := parsed.Next(after.In(location))
	if next.IsZero() {
		return time.Time{}, errors.New("probe Cron expression has no next occurrence")
	}
	return next.UTC(), nil
}
