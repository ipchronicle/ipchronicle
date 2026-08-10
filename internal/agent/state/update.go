package state

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/releaseinfo"
	bolt "go.etcd.io/bbolt"
)

const confirmedAgentUpdateRetention = 24 * time.Hour

var (
	ErrAgentUpdateHandled = errors.New("Agent update task was already handled")
	ErrInvalidAgentUpdate = errors.New("Agent update state is invalid")
)

type AgentUpdateDelivery struct {
	ID            string
	TargetVersion string
	CreatedAt     time.Time
	ExpiresAt     time.Time
}

type AgentUpdateReport struct {
	ID              string
	Status          string
	AcknowledgedAt  time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	PreviousVersion *string
	ResultVersion   *string
	FailureCode     *string
	Diagnostic      *string
}

type AgentUpdate struct {
	AgentUpdateReport
	TargetVersion string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	ConfirmedAt   *time.Time
}

type storedAgentUpdate struct {
	AgentUpdateReport
	TargetVersion string     `json:"targetVersion"`
	CreatedAt     time.Time  `json:"createdAt"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	ConfirmedAt   *time.Time `json:"confirmedAt,omitempty"`
}

func (s *Store) AcceptAgentUpdate(delivery AgentUpdateDelivery, previousVersion string, now time.Time) (AgentUpdateReport, error) {
	if err := validateAgentUpdateDelivery(delivery); err != nil || !validAgentVersion(previousVersion) || now.IsZero() {
		return AgentUpdateReport{}, ErrInvalidAgentUpdate
	}
	now = now.UTC().Truncate(time.Second)
	if now.Before(delivery.CreatedAt) || now.After(delivery.ExpiresAt) {
		return AgentUpdateReport{}, ErrInvalidAgentUpdate
	}
	var report AgentUpdateReport
	err := s.database.Update(func(transaction *bolt.Tx) error {
		bucket := transaction.Bucket(agentUpdatesBucket)
		if encoded := bucket.Get([]byte(delivery.ID)); encoded != nil {
			var existing storedAgentUpdate
			if err := decodeJSON(encoded, &existing, "Agent update"); err != nil {
				return err
			}
			report = existing.AgentUpdateReport
			return ErrAgentUpdateHandled
		}
		if err := ensureNoUnconfirmedAgentUpdate(bucket); err != nil {
			return err
		}
		if transaction.Bucket(probeControlBucket).Get(activeProbeRunKey) != nil {
			failureCode := "probe-active"
			diagnostic := "Agent update was rejected because a complete probe is active"
			report = AgentUpdateReport{
				ID: delivery.ID, Status: "rejected", AcknowledgedAt: now, CompletedAt: cloneTime(&now),
				PreviousVersion: cloneString(&previousVersion), FailureCode: &failureCode, Diagnostic: &diagnostic,
			}
			return putJSON(bucket, []byte(delivery.ID), storedAgentUpdate{
				AgentUpdateReport: report, TargetVersion: delivery.TargetVersion,
				CreatedAt: delivery.CreatedAt.UTC(), ExpiresAt: delivery.ExpiresAt.UTC(),
			})
		}
		report = AgentUpdateReport{ID: delivery.ID, Status: "acknowledged", AcknowledgedAt: now}
		return putJSON(bucket, []byte(delivery.ID), storedAgentUpdate{
			AgentUpdateReport: report, TargetVersion: delivery.TargetVersion,
			CreatedAt: delivery.CreatedAt.UTC(), ExpiresAt: delivery.ExpiresAt.UTC(),
		})
	})
	return report, err
}

func (s *Store) BeginAgentUpdate(id, previousVersion string, now time.Time) (AgentUpdate, error) {
	if !validAgentUpdateID(id) || !validAgentVersion(previousVersion) || now.IsZero() {
		return AgentUpdate{}, ErrInvalidAgentUpdate
	}
	now = now.UTC().Truncate(time.Second)
	return s.changeAgentUpdate(id, func(update *storedAgentUpdate) error {
		if update.Status != "acknowledged" {
			return ErrInvalidAgentUpdate
		}
		update.Status = "verifying"
		update.StartedAt = cloneTime(&now)
		update.PreviousVersion = cloneString(&previousVersion)
		return nil
	})
}

func (s *Store) MarkAgentUpdateInstalling(id string) (AgentUpdate, error) {
	return s.changeAgentUpdate(id, func(update *storedAgentUpdate) error {
		if update.Status != "verifying" || update.StartedAt == nil || update.PreviousVersion == nil {
			return ErrInvalidAgentUpdate
		}
		update.Status = "installing"
		return nil
	})
}

func (s *Store) MarkAgentUpdateRestarting(id string) (AgentUpdate, error) {
	return s.changeAgentUpdate(id, func(update *storedAgentUpdate) error {
		if update.Status != "installing" || update.StartedAt == nil || update.PreviousVersion == nil {
			return ErrInvalidAgentUpdate
		}
		update.Status = "restarting"
		return nil
	})
}

func (s *Store) FailAgentUpdate(id, failureCode, diagnostic string, now time.Time) (AgentUpdate, error) {
	if !validAgentUpdateFailure(failureCode, diagnostic) || now.IsZero() {
		return AgentUpdate{}, ErrInvalidAgentUpdate
	}
	now = now.UTC().Truncate(time.Second)
	return s.changeAgentUpdate(id, func(update *storedAgentUpdate) error {
		if update.Status != "verifying" && update.Status != "installing" {
			return ErrInvalidAgentUpdate
		}
		update.Status = "failed"
		update.CompletedAt = cloneTime(&now)
		update.FailureCode = cloneString(&failureCode)
		update.Diagnostic = optionalString(diagnostic)
		return nil
	})
}

func (s *Store) CommitAgentUpdateHealth(id, runningVersion string, now time.Time) (AgentUpdate, error) {
	if !validAgentUpdateID(id) || !validAgentVersion(runningVersion) || now.IsZero() {
		return AgentUpdate{}, ErrInvalidAgentUpdate
	}
	now = now.UTC().Truncate(time.Second)
	return s.changeAgentUpdate(id, func(update *storedAgentUpdate) error {
		if update.Status != "restarting" || update.TargetVersion != runningVersion {
			return ErrInvalidAgentUpdate
		}
		update.Status = "succeeded"
		update.CompletedAt = cloneTime(&now)
		update.ResultVersion = cloneString(&runningVersion)
		return nil
	})
}

func (s *Store) RollbackAgentUpdate(id, failureCode, diagnostic string, now time.Time) (AgentUpdate, error) {
	if !validAgentUpdateFailure(failureCode, diagnostic) || now.IsZero() {
		return AgentUpdate{}, ErrInvalidAgentUpdate
	}
	now = now.UTC().Truncate(time.Second)
	return s.changeAgentUpdate(id, func(update *storedAgentUpdate) error {
		if (update.Status != "installing" && update.Status != "restarting") || update.PreviousVersion == nil {
			return ErrInvalidAgentUpdate
		}
		update.Status = "rolled-back"
		update.CompletedAt = cloneTime(&now)
		update.ResultVersion = cloneString(update.PreviousVersion)
		update.FailureCode = cloneString(&failureCode)
		update.Diagnostic = optionalString(diagnostic)
		return nil
	})
}

func (s *Store) PendingAgentUpdate() (AgentUpdate, bool, error) {
	var result AgentUpdate
	found := false
	err := s.database.View(func(transaction *bolt.Tx) error {
		return transaction.Bucket(agentUpdatesBucket).ForEach(func(_, encoded []byte) error {
			var record storedAgentUpdate
			if err := decodeJSON(encoded, &record, "Agent update"); err != nil {
				return err
			}
			if record.ConfirmedAt != nil {
				return nil
			}
			if found {
				return errors.New("multiple unconfirmed Agent updates in local state")
			}
			result = agentUpdateFromStored(record)
			found = true
			return nil
		})
	})
	return result, found, err
}

func (s *Store) AgentUpdateControlReport() (*AgentUpdateReport, error) {
	update, found, err := s.PendingAgentUpdate()
	if err != nil || !found {
		return nil, err
	}
	report := update.AgentUpdateReport
	return &report, nil
}

func (s *Store) ConfirmTerminalAgentUpdate(id string, confirmedAt time.Time) (bool, error) {
	if !validAgentUpdateID(id) || confirmedAt.IsZero() {
		return false, ErrInvalidAgentUpdate
	}
	confirmedAt = confirmedAt.UTC().Truncate(time.Second)
	found := false
	err := s.database.Update(func(transaction *bolt.Tx) error {
		bucket := transaction.Bucket(agentUpdatesBucket)
		encoded := bucket.Get([]byte(id))
		if encoded == nil {
			return nil
		}
		found = true
		var record storedAgentUpdate
		if err := decodeJSON(encoded, &record, "Agent update"); err != nil {
			return err
		}
		if !agentUpdateTerminal(record.Status) {
			return ErrInvalidAgentUpdate
		}
		if record.ConfirmedAt == nil {
			record.ConfirmedAt = cloneTime(&confirmedAt)
		}
		return putJSON(bucket, []byte(id), record)
	})
	return found, err
}

func (s *Store) CleanupAgentUpdates(now time.Time) error {
	if now.IsZero() {
		return ErrInvalidAgentUpdate
	}
	cutoff := now.UTC().Add(-confirmedAgentUpdateRetention)
	return s.database.Update(func(transaction *bolt.Tx) error {
		bucket := transaction.Bucket(agentUpdatesBucket)
		var keys [][]byte
		if err := bucket.ForEach(func(key, encoded []byte) error {
			var record storedAgentUpdate
			if err := decodeJSON(encoded, &record, "Agent update"); err != nil {
				return err
			}
			if record.ConfirmedAt != nil && !record.ConfirmedAt.After(cutoff) {
				keys = append(keys, append([]byte(nil), key...))
			}
			return nil
		}); err != nil {
			return err
		}
		for _, key := range keys {
			if err := bucket.Delete(key); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) validateAgentUpdateState() error {
	return s.database.View(func(transaction *bolt.Tx) error {
		unconfirmed := 0
		return transaction.Bucket(agentUpdatesBucket).ForEach(func(_, encoded []byte) error {
			var record storedAgentUpdate
			if err := decodeJSON(encoded, &record, "Agent update"); err != nil {
				return err
			}
			if err := validateStoredAgentUpdate(record); err != nil {
				return err
			}
			if record.ConfirmedAt == nil {
				unconfirmed++
			}
			if unconfirmed > 1 {
				return errors.New("multiple unconfirmed Agent updates in local state")
			}
			return nil
		})
	})
}

func (s *Store) changeAgentUpdate(id string, change func(*storedAgentUpdate) error) (AgentUpdate, error) {
	if !validAgentUpdateID(id) {
		return AgentUpdate{}, ErrInvalidAgentUpdate
	}
	var result AgentUpdate
	err := s.database.Update(func(transaction *bolt.Tx) error {
		bucket := transaction.Bucket(agentUpdatesBucket)
		encoded := bucket.Get([]byte(id))
		if encoded == nil {
			return ErrInvalidAgentUpdate
		}
		var record storedAgentUpdate
		if err := decodeJSON(encoded, &record, "Agent update"); err != nil {
			return err
		}
		if err := change(&record); err != nil {
			return err
		}
		if err := validateStoredAgentUpdate(record); err != nil {
			return err
		}
		if err := putJSON(bucket, []byte(id), record); err != nil {
			return err
		}
		result = agentUpdateFromStored(record)
		return nil
	})
	return result, err
}

func ensureNoUnconfirmedAgentUpdate(bucket *bolt.Bucket) error {
	return bucket.ForEach(func(_, encoded []byte) error {
		var record storedAgentUpdate
		if err := decodeJSON(encoded, &record, "Agent update"); err != nil {
			return err
		}
		if record.ConfirmedAt == nil {
			return errors.New("another Agent update is not yet confirmed")
		}
		return nil
	})
}

func validateStoredAgentUpdate(update storedAgentUpdate) error {
	if !validAgentUpdateID(update.ID) || !validAgentVersion(update.TargetVersion) ||
		update.CreatedAt.IsZero() || !update.ExpiresAt.After(update.CreatedAt) || update.AcknowledgedAt.IsZero() ||
		update.AcknowledgedAt.Before(update.CreatedAt) || update.AcknowledgedAt.After(update.ExpiresAt) {
		return ErrInvalidAgentUpdate
	}
	if update.StartedAt != nil && update.StartedAt.Before(update.AcknowledgedAt) ||
		update.CompletedAt != nil && update.CompletedAt.Before(update.AcknowledgedAt) {
		return ErrInvalidAgentUpdate
	}
	if update.ConfirmedAt != nil && (!agentUpdateTerminal(update.Status) || update.CompletedAt == nil || update.ConfirmedAt.Before(*update.CompletedAt)) {
		return ErrInvalidAgentUpdate
	}
	switch update.Status {
	case "acknowledged":
		if update.StartedAt != nil || update.CompletedAt != nil || update.PreviousVersion != nil || update.ResultVersion != nil ||
			update.FailureCode != nil || update.Diagnostic != nil {
			return ErrInvalidAgentUpdate
		}
	case "verifying", "installing", "restarting":
		if update.StartedAt == nil || !validOptionalAgentVersion(update.PreviousVersion) || update.CompletedAt != nil ||
			update.ResultVersion != nil || update.FailureCode != nil || update.Diagnostic != nil {
			return ErrInvalidAgentUpdate
		}
	case "succeeded":
		if update.StartedAt == nil || update.CompletedAt == nil || !validOptionalAgentVersion(update.PreviousVersion) ||
			!validOptionalAgentVersion(update.ResultVersion) || *update.ResultVersion != update.TargetVersion ||
			update.FailureCode != nil || update.Diagnostic != nil {
			return ErrInvalidAgentUpdate
		}
	case "failed":
		if update.StartedAt == nil || update.CompletedAt == nil || !validOptionalAgentVersion(update.PreviousVersion) ||
			update.ResultVersion != nil || !validOptionalAgentUpdateFailure(update.FailureCode, update.Diagnostic) {
			return ErrInvalidAgentUpdate
		}
	case "rolled-back":
		if update.StartedAt == nil || update.CompletedAt == nil || !validOptionalAgentVersion(update.PreviousVersion) ||
			!validOptionalAgentVersion(update.ResultVersion) || *update.ResultVersion != *update.PreviousVersion ||
			!validOptionalAgentUpdateFailure(update.FailureCode, update.Diagnostic) {
			return ErrInvalidAgentUpdate
		}
	case "rejected":
		if update.StartedAt != nil || update.CompletedAt == nil || !validOptionalAgentVersion(update.PreviousVersion) ||
			update.ResultVersion != nil || !validOptionalAgentUpdateFailure(update.FailureCode, update.Diagnostic) {
			return ErrInvalidAgentUpdate
		}
	default:
		return ErrInvalidAgentUpdate
	}
	return nil
}

func validateAgentUpdateDelivery(delivery AgentUpdateDelivery) error {
	if !validAgentUpdateID(delivery.ID) || !validAgentVersion(delivery.TargetVersion) ||
		delivery.CreatedAt.IsZero() || !delivery.ExpiresAt.After(delivery.CreatedAt) {
		return ErrInvalidAgentUpdate
	}
	return nil
}

func validAgentUpdateID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func validAgentVersion(value string) bool {
	canonical, err := releaseinfo.CanonicalVersion(value)
	return err == nil && canonical == value
}

func validAgentUpdateFailure(code, diagnostic string) bool {
	return len(code) >= 1 && len(code) <= 64 && !strings.ContainsRune(code, '\x00') &&
		len(diagnostic) >= 1 && len(diagnostic) <= 4096 && utf8.ValidString(diagnostic) && !strings.ContainsRune(diagnostic, '\x00')
}

func validOptionalAgentVersion(value *string) bool {
	return value != nil && validAgentVersion(*value)
}

func validOptionalAgentUpdateFailure(code, diagnostic *string) bool {
	return code != nil && diagnostic != nil && validAgentUpdateFailure(*code, *diagnostic)
}

func agentUpdateTerminal(status string) bool {
	return status == "succeeded" || status == "failed" || status == "rolled-back" || status == "rejected"
}

func agentUpdateFromStored(record storedAgentUpdate) AgentUpdate {
	return AgentUpdate{
		AgentUpdateReport: record.AgentUpdateReport, TargetVersion: record.TargetVersion,
		CreatedAt: record.CreatedAt, ExpiresAt: record.ExpiresAt, ConfirmedAt: record.ConfirmedAt,
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return cloneString(&value)
}
