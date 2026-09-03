package state

import "time"

func boundedTaskTime(createdAt, expiresAt, observedAt time.Time) time.Time {
	createdAt = createdAt.UTC().Truncate(time.Second)
	expiresAt = expiresAt.UTC().Truncate(time.Second)
	observedAt = observedAt.UTC().Truncate(time.Second)
	switch {
	case observedAt.Before(createdAt):
		return createdAt
	case observedAt.After(expiresAt):
		return expiresAt
	default:
		return observedAt
	}
}

func taskProgressTime(observedAt, minimum time.Time) time.Time {
	observedAt = observedAt.UTC().Truncate(time.Second)
	minimum = minimum.UTC().Truncate(time.Second)
	if observedAt.Before(minimum) {
		return minimum
	}
	return observedAt
}

func normalizeTaskReportTimeline(
	createdAt, expiresAt, acknowledgedAt time.Time,
	startedAt, completedAt *time.Time,
) (time.Time, *time.Time, *time.Time) {
	acknowledgedAt = acknowledgedAt.UTC().Truncate(time.Second)
	normalizedAcknowledgedAt := boundedTaskTime(createdAt, expiresAt, acknowledgedAt)
	// Move the whole report timeline by the same offset so retries stay
	// deterministic and phase durations are not distorted by clock skew.
	offset := normalizedAcknowledgedAt.Sub(acknowledgedAt)

	shift := func(value *time.Time, minimum time.Time) *time.Time {
		if value == nil {
			return nil
		}
		normalized := value.UTC().Truncate(time.Second).Add(offset)
		normalized = taskProgressTime(normalized, minimum)
		return &normalized
	}
	normalizedStartedAt := shift(startedAt, normalizedAcknowledgedAt)
	minimumCompletedAt := normalizedAcknowledgedAt
	if normalizedStartedAt != nil {
		minimumCompletedAt = *normalizedStartedAt
	}
	normalizedCompletedAt := shift(completedAt, minimumCompletedAt)
	return normalizedAcknowledgedAt, normalizedStartedAt, normalizedCompletedAt
}
