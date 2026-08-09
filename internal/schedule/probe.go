package schedule

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const AgentLocalTimezone = "agent-local"

var probeParser = cron.NewParser(
	cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
)

func ValidateProbe(expression, timezone string) error {
	if strings.TrimSpace(expression) != expression || len(expression) < 9 || len(expression) > 128 {
		return errors.New("probe Cron expression must be trimmed and between 9 and 128 bytes")
	}
	if _, err := probeParser.Parse(expression); err != nil {
		return fmt.Errorf("parse six-field probe Cron expression: %w", err)
	}
	if timezone == AgentLocalTimezone {
		return nil
	}
	if strings.TrimSpace(timezone) != timezone || timezone == "" || len(timezone) > 128 {
		return errors.New("probe timezone must be agent-local or a trimmed IANA name")
	}
	location, err := time.LoadLocation(timezone)
	if err != nil || location.String() != timezone {
		return errors.New("probe timezone must be agent-local or a valid IANA name")
	}
	return nil
}

func NextProbe(expression, timezone string, after time.Time, agentLocal *time.Location) (time.Time, error) {
	if err := ValidateProbe(expression, timezone); err != nil {
		return time.Time{}, err
	}
	location := agentLocal
	if timezone != AgentLocalTimezone {
		var err error
		location, err = time.LoadLocation(timezone)
		if err != nil {
			return time.Time{}, err
		}
	}
	if location == nil {
		location = time.Local
	}
	parsed, err := probeParser.Parse(expression)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.Next(after.In(location)).UTC(), nil
}
