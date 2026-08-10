package state

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

const (
	MaxProbeResultBytes             = 1024 * 1024
	MaxProbeDiagnosticBytes         = 64 * 1024
	maxPendingProbeResultsPerEgress = 30
	confirmedProbeTaskRetention     = 24 * time.Hour
)

var (
	ErrProbeBusy                 = errors.New("a complete-probe run is already active")
	ErrProbeRunNotFound          = errors.New("complete-probe run does not exist")
	ErrProbeTaskHandled          = errors.New("complete-probe task was already handled")
	ErrInvalidProbeState         = errors.New("complete-probe state transition is invalid")
	ErrProbeConfigurationChanged = errors.New("Agent configuration changed before the complete-probe run started")
)

var (
	activeProbeRunKey = []byte("active-run")
	probeStatusKey    = []byte("status")
	activeProcessKey  = []byte("active")
)

type ProbeExecutionManifest struct {
	ID       string `json:"id"`
	EgressID string `json:"egressId"`
	Ordinal  int64  `json:"ordinal"`
	Sequence int64  `json:"sequence"`
}

type ProbeRun struct {
	ID                    string                   `json:"id"`
	ConfigurationRevision int64                    `json:"configurationRevision"`
	HistoryGeneration     string                   `json:"historyGeneration"`
	Trigger               string                   `json:"trigger"`
	TaskID                *string                  `json:"taskId,omitempty"`
	TriggeringEgressID    *string                  `json:"triggeringEgressId,omitempty"`
	StartedAt             time.Time                `json:"startedAt"`
	CompletedAt           *time.Time               `json:"completedAt,omitempty"`
	Status                string                   `json:"status"`
	Executions            []ProbeExecutionManifest `json:"executions"`
	ArtifactRevision      int64                    `json:"artifactRevision"`
	Delivered             bool                     `json:"delivered"`
}

type ProbeExecution struct {
	ID               string     `json:"id"`
	RunID            string     `json:"runId"`
	EgressID         string     `json:"egressId"`
	Ordinal          int64      `json:"ordinal"`
	Sequence         int64      `json:"sequence"`
	Status           string     `json:"status"`
	StartedAt        *time.Time `json:"startedAt,omitempty"`
	CompletedAt      *time.Time `json:"completedAt,omitempty"`
	FailureStage     *string    `json:"failureStage,omitempty"`
	Diagnostic       *string    `json:"diagnostic,omitempty"`
	ResultFile       string     `json:"resultFile,omitempty"`
	ArtifactRevision int64      `json:"artifactRevision"`
	Delivered        bool       `json:"delivered"`
	Evicted          bool       `json:"evicted"`
}

type ProbeExecutionOutcome struct {
	Status       string     `json:"status"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	CompletedAt  time.Time  `json:"completedAt"`
	FailureStage string     `json:"failureStage,omitempty"`
	Diagnostic   string     `json:"diagnostic,omitempty"`
	RawResult    []byte     `json:"rawResult,omitempty"`
}

type ProbeGap struct {
	ID                string    `json:"id"`
	EgressID          string    `json:"egressId"`
	HistoryGeneration string    `json:"historyGeneration"`
	DroppedCount      int64     `json:"droppedCount"`
	FirstSequence     int64     `json:"firstSequence"`
	LastSequence      int64     `json:"lastSequence"`
	FirstObservedAt   time.Time `json:"firstObservedAt"`
	LastObservedAt    time.Time `json:"lastObservedAt"`
	ArtifactRevision  int64     `json:"artifactRevision"`
}

type ProbeArtifact struct {
	ID        string          `json:"id"`
	Revision  int64           `json:"revision"`
	Run       *ProbeRun       `json:"run,omitempty"`
	Execution *ProbeExecution `json:"execution,omitempty"`
	Gap       *ProbeGap       `json:"gap,omitempty"`
	RawResult []byte          `json:"rawResult,omitempty"`
}

type ProbeArtifactReceipt struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

type ProbeTaskDelivery struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type ProbeTaskReport struct {
	ID              string     `json:"id"`
	Status          string     `json:"status"`
	AcknowledgedAt  time.Time  `json:"acknowledgedAt"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	CompletedAt     *time.Time `json:"completedAt,omitempty"`
	RunID           *string    `json:"runId,omitempty"`
	RejectionReason *string    `json:"rejectionReason,omitempty"`
}

type storedProbeTask struct {
	ProbeTaskReport
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   time.Time  `json:"expiresAt"`
	ConfirmedAt *time.Time `json:"confirmedAt,omitempty"`
}

type ProbeStatus struct {
	ActiveRunID                       *string    `json:"activeRunId,omitempty"`
	NextScheduledAt                   *time.Time `json:"nextScheduledAt,omitempty"`
	LastOccurrenceAt                  *time.Time `json:"lastOccurrenceAt,omitempty"`
	LastOccurrenceTrigger             *string    `json:"lastOccurrenceTrigger,omitempty"`
	LastOccurrenceStatus              *string    `json:"lastOccurrenceStatus,omitempty"`
	LastSkipReason                    *string    `json:"lastSkipReason,omitempty"`
	HistoryResetGeneration            *string    `json:"historyResetGeneration,omitempty"`
	HistoryResetAt                    *time.Time `json:"historyResetAt,omitempty"`
	HistoryResetDiscardedAddressItems int64      `json:"historyResetDiscardedAddressItems"`
	HistoryResetDiscardedProbeItems   int64      `json:"historyResetDiscardedProbeItems"`
}

type ProbeProcess struct {
	PID            int       `json:"pid"`
	ProcessGroupID int       `json:"processGroupId"`
	StartTicks     uint64    `json:"startTicks"`
	BootID         string    `json:"bootId"`
	StartedAt      time.Time `json:"startedAt"`
}

type queuedProbeArtifact struct {
	Kind     string `json:"kind"`
	Revision int64  `json:"revision"`
}

func (s *Store) StartProbeRun(trigger string, task *ProbeTaskDelivery, triggeringEgressID *string, startedAt time.Time) (ProbeRun, error) {
	return s.StartProbeRunAtRevision(0, trigger, task, triggeringEgressID, startedAt)
}

func (s *Store) StartProbeRunAtRevision(
	expectedConfigurationRevision int64,
	trigger string,
	task *ProbeTaskDelivery,
	triggeringEgressID *string,
	startedAt time.Time,
) (ProbeRun, error) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if !validProbeTrigger(trigger) || startedAt.IsZero() ||
		(trigger == "manual") != (task != nil) || (trigger == "address-change") != (triggeringEgressID != nil) {
		return ProbeRun{}, ErrInvalidProbeState
	}
	startedAt = startedAt.UTC().Truncate(time.Second)
	var result ProbeRun
	err := s.database.Update(func(transaction *bolt.Tx) error {
		activeUpdate, err := activeAgentUpdate(transaction.Bucket(agentUpdatesBucket))
		if err != nil {
			return err
		}
		if activeUpdate {
			return ErrProbeBusy
		}
		if task != nil {
			if err := validateProbeTaskDelivery(*task); err != nil {
				return err
			}
			if transaction.Bucket(probeTasksBucket).Get([]byte(task.ID)) != nil {
				return ErrProbeTaskHandled
			}
		}
		control := transaction.Bucket(probeControlBucket)
		if control.Get(activeProbeRunKey) != nil {
			return ErrProbeBusy
		}
		configuration, err := configurationFromTransaction(s.masterKey, transaction)
		if err != nil {
			return err
		}
		if expectedConfigurationRevision > 0 && configuration.Revision != expectedConfigurationRevision {
			return ErrProbeConfigurationChanged
		}
		var enabled []Egress
		for _, egress := range configuration.Egresses {
			if egress.Enabled {
				enabled = append(enabled, egress)
			}
		}
		if !configuration.Enabled || len(enabled) == 0 {
			return ErrInvalidProbeState
		}
		runID := uuid.NewString()
		result = ProbeRun{
			ID: runID, ConfigurationRevision: configuration.Revision,
			HistoryGeneration: configuration.HistoryGeneration, Trigger: trigger,
			TriggeringEgressID: cloneString(triggeringEgressID), StartedAt: startedAt,
			Status: "running", ArtifactRevision: 1,
			Executions: make([]ProbeExecutionManifest, 0, len(enabled)),
		}
		if task != nil {
			result.TaskID = cloneString(&task.ID)
		}
		for ordinal, egress := range enabled {
			sequence, err := nextProbeSequence(transaction, egress.ID)
			if err != nil {
				return err
			}
			manifest := ProbeExecutionManifest{
				ID: uuid.NewString(), EgressID: egress.ID, Ordinal: int64(ordinal), Sequence: sequence,
			}
			result.Executions = append(result.Executions, manifest)
			execution := ProbeExecution{
				ID: manifest.ID, RunID: runID, EgressID: egress.ID,
				Ordinal: manifest.Ordinal, Sequence: manifest.Sequence, Status: "pending",
			}
			if err := putJSON(transaction.Bucket(probeExecutionsBucket), []byte(execution.ID), execution); err != nil {
				return err
			}
		}
		if err := putJSON(transaction.Bucket(probeRunsBucket), []byte(result.ID), result); err != nil {
			return err
		}
		if err := putJSON(transaction.Bucket(probeArtifactsBucket), []byte(result.ID), queuedProbeArtifact{Kind: "run", Revision: result.ArtifactRevision}); err != nil {
			return err
		}
		if err := control.Put(activeProbeRunKey, []byte(result.ID)); err != nil {
			return err
		}
		status, err := probeStatusFromTransaction(transaction)
		if err != nil {
			return err
		}
		status.ActiveRunID = cloneString(&result.ID)
		occurrenceStatus := "started"
		status.LastOccurrenceAt = cloneTime(&startedAt)
		status.LastOccurrenceTrigger = cloneString(&trigger)
		status.LastOccurrenceStatus = &occurrenceStatus
		status.LastSkipReason = nil
		if err := putJSON(control, probeStatusKey, status); err != nil {
			return err
		}
		if task != nil {
			runIDValue := result.ID
			taskRecord := storedProbeTask{
				ProbeTaskReport: ProbeTaskReport{
					ID: task.ID, Status: "running", AcknowledgedAt: startedAt,
					StartedAt: cloneTime(&startedAt), RunID: &runIDValue,
				},
				CreatedAt: task.CreatedAt.UTC(), ExpiresAt: task.ExpiresAt.UTC(),
			}
			if err := putJSON(transaction.Bucket(probeTasksBucket), []byte(task.ID), taskRecord); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

func activeAgentUpdate(bucket *bolt.Bucket) (bool, error) {
	active := false
	err := bucket.ForEach(func(_, encoded []byte) error {
		var update storedAgentUpdate
		if err := decodeJSON(encoded, &update, "Agent update"); err != nil {
			return err
		}
		if !agentUpdateTerminal(update.Status) && update.ConfirmedAt == nil {
			active = true
		}
		return nil
	})
	return active, err
}

func (s *Store) RejectProbeTask(task ProbeTaskDelivery, reason string, now time.Time) (ProbeTaskReport, error) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if err := validateProbeTaskDelivery(task); err != nil || !validProbeSkipReason(reason) || now.IsZero() {
		return ProbeTaskReport{}, ErrInvalidProbeState
	}
	now = now.UTC().Truncate(time.Second)
	var report ProbeTaskReport
	err := s.database.Update(func(transaction *bolt.Tx) error {
		bucket := transaction.Bucket(probeTasksBucket)
		if encoded := bucket.Get([]byte(task.ID)); encoded != nil {
			var existing storedProbeTask
			if err := decodeJSON(encoded, &existing, "probe task"); err != nil {
				return err
			}
			report = existing.ProbeTaskReport
			return ErrProbeTaskHandled
		}
		reasonValue := reason
		report = ProbeTaskReport{
			ID: task.ID, Status: "rejected", AcknowledgedAt: now,
			CompletedAt: cloneTime(&now), RejectionReason: &reasonValue,
		}
		return putJSON(bucket, []byte(task.ID), storedProbeTask{
			ProbeTaskReport: report, CreatedAt: task.CreatedAt.UTC(), ExpiresAt: task.ExpiresAt.UTC(),
		})
	})
	return report, err
}

func (s *Store) ProbeTask(id string) (ProbeTaskReport, bool, error) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if _, err := uuid.Parse(id); err != nil {
		return ProbeTaskReport{}, false, ErrInvalidProbeState
	}
	var record storedProbeTask
	err := s.database.View(func(transaction *bolt.Tx) error {
		encoded := transaction.Bucket(probeTasksBucket).Get([]byte(id))
		if encoded == nil {
			return nil
		}
		return decodeJSON(encoded, &record, "probe task")
	})
	return record.ProbeTaskReport, record.ID != "", err
}

func (s *Store) ProbeControlReport() (ProbeStatus, *ProbeTaskReport, error) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	var status ProbeStatus
	var task *ProbeTaskReport
	err := s.database.View(func(transaction *bolt.Tx) error {
		var err error
		status, err = probeStatusFromTransaction(transaction)
		if err != nil {
			return err
		}
		return transaction.Bucket(probeTasksBucket).ForEach(func(_, encoded []byte) error {
			var record storedProbeTask
			if err := decodeJSON(encoded, &record, "probe task"); err != nil {
				return err
			}
			if record.ConfirmedAt != nil {
				return nil
			}
			if task != nil {
				return errors.New("multiple unconfirmed probe tasks in Agent state")
			}
			value := record.ProbeTaskReport
			task = &value
			return nil
		})
	})
	return status, task, err
}

func (s *Store) ConfirmTerminalProbeTask(id string, confirmedAt time.Time) error {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if _, err := uuid.Parse(id); err != nil || confirmedAt.IsZero() {
		return ErrInvalidProbeState
	}
	confirmedAt = confirmedAt.UTC().Truncate(time.Second)
	return s.database.Update(func(transaction *bolt.Tx) error {
		bucket := transaction.Bucket(probeTasksBucket)
		encoded := bucket.Get([]byte(id))
		if encoded == nil {
			return nil
		}
		var record storedProbeTask
		if err := decodeJSON(encoded, &record, "probe task"); err != nil {
			return err
		}
		if !probeTaskTerminal(record.Status) {
			return ErrInvalidProbeState
		}
		if record.ConfirmedAt == nil {
			record.ConfirmedAt = &confirmedAt
		}
		return putJSON(bucket, []byte(id), record)
	})
}

func (s *Store) CleanupProbeTasks(now time.Time) error {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	cutoff := now.UTC().Add(-confirmedProbeTaskRetention)
	return s.database.Update(func(transaction *bolt.Tx) error {
		bucket := transaction.Bucket(probeTasksBucket)
		var keys [][]byte
		if err := bucket.ForEach(func(key, encoded []byte) error {
			var record storedProbeTask
			if err := decodeJSON(encoded, &record, "probe task"); err != nil {
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

func (s *Store) RecordSkippedProbe(trigger, reason string, at time.Time) error {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if !validProbeTrigger(trigger) || !validProbeSkipReason(reason) || at.IsZero() {
		return ErrInvalidProbeState
	}
	at = at.UTC().Truncate(time.Second)
	return s.database.Update(func(transaction *bolt.Tx) error {
		status, err := probeStatusFromTransaction(transaction)
		if err != nil {
			return err
		}
		occurrenceStatus := "skipped"
		status.LastOccurrenceAt = &at
		status.LastOccurrenceTrigger = &trigger
		status.LastOccurrenceStatus = &occurrenceStatus
		status.LastSkipReason = &reason
		return putJSON(transaction.Bucket(probeControlBucket), probeStatusKey, status)
	})
}

func (s *Store) SetNextProbeSchedule(next *time.Time) error {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	return s.database.Update(func(transaction *bolt.Tx) error {
		status, err := probeStatusFromTransaction(transaction)
		if err != nil {
			return err
		}
		status.NextScheduledAt = cloneTime(next)
		return putJSON(transaction.Bucket(probeControlBucket), probeStatusKey, status)
	})
}

func (s *Store) ActiveProbeRun() (ProbeRun, error) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	var run ProbeRun
	err := s.database.View(func(transaction *bolt.Tx) error {
		id := transaction.Bucket(probeControlBucket).Get(activeProbeRunKey)
		if id == nil {
			return ErrProbeRunNotFound
		}
		return decodeProbeRun(transaction.Bucket(probeRunsBucket).Get(id), &run)
	})
	return run, err
}

func (s *Store) ProbeRun(id string) (ProbeRun, []ProbeExecution, error) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	var run ProbeRun
	var executions []ProbeExecution
	err := s.database.View(func(transaction *bolt.Tx) error {
		if err := decodeProbeRun(transaction.Bucket(probeRunsBucket).Get([]byte(id)), &run); err != nil {
			return err
		}
		for _, manifest := range run.Executions {
			var execution ProbeExecution
			if err := decodeProbeExecution(transaction.Bucket(probeExecutionsBucket).Get([]byte(manifest.ID)), &execution); err != nil {
				return err
			}
			executions = append(executions, execution)
		}
		return nil
	})
	return run, executions, err
}

func (s *Store) StartProbeExecution(runID, executionID string, startedAt time.Time) (ProbeExecution, error) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if startedAt.IsZero() {
		return ProbeExecution{}, ErrInvalidProbeState
	}
	startedAt = startedAt.UTC().Truncate(time.Second)
	var result ProbeExecution
	err := s.database.Update(func(transaction *bolt.Tx) error {
		if string(transaction.Bucket(probeControlBucket).Get(activeProbeRunKey)) != runID {
			return ErrInvalidProbeState
		}
		bucket := transaction.Bucket(probeExecutionsBucket)
		if err := decodeProbeExecution(bucket.Get([]byte(executionID)), &result); err != nil {
			return err
		}
		if result.RunID != runID || result.Status != "pending" {
			return ErrInvalidProbeState
		}
		result.Status = "running"
		result.StartedAt = &startedAt
		result.ArtifactRevision++
		if err := putJSON(bucket, []byte(result.ID), result); err != nil {
			return err
		}
		current, err := probeRunUsesCurrentGeneration(s.masterKey, transaction, runID)
		if err != nil {
			return err
		}
		if !current {
			return nil
		}
		return putJSON(transaction.Bucket(probeArtifactsBucket), []byte(result.ID), queuedProbeArtifact{
			Kind: "execution", Revision: result.ArtifactRevision,
		})
	})
	return result, err
}

func (s *Store) CompleteProbeExecution(runID, executionID string, outcome ProbeExecutionOutcome) (ProbeExecution, error) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if err := validateProbeExecutionOutcome(outcome); err != nil {
		return ProbeExecution{}, err
	}
	currentGeneration := false
	if err := s.database.View(func(transaction *bolt.Tx) error {
		var err error
		currentGeneration, err = probeRunUsesCurrentGeneration(s.masterKey, transaction, runID)
		return err
	}); err != nil {
		return ProbeExecution{}, err
	}
	var resultPath string
	if outcome.Status == "succeeded" && currentGeneration {
		var err error
		resultPath, err = s.writeProbeResult(executionID, outcome.RawResult)
		if err != nil {
			return ProbeExecution{}, err
		}
	}
	var result ProbeExecution
	var obsoleteFiles []string
	err := s.database.Update(func(transaction *bolt.Tx) error {
		if string(transaction.Bucket(probeControlBucket).Get(activeProbeRunKey)) != runID {
			return ErrInvalidProbeState
		}
		bucket := transaction.Bucket(probeExecutionsBucket)
		if err := decodeProbeExecution(bucket.Get([]byte(executionID)), &result); err != nil {
			return err
		}
		if result.RunID != runID || (result.Status != "pending" && result.Status != "running") {
			return ErrInvalidProbeState
		}
		if outcome.Status != "skipped" && result.StartedAt == nil && outcome.StartedAt == nil {
			return ErrInvalidProbeState
		}
		if outcome.StartedAt != nil {
			started := outcome.StartedAt.UTC().Truncate(time.Second)
			result.StartedAt = &started
		}
		completed := outcome.CompletedAt.UTC().Truncate(time.Second)
		result.Status = outcome.Status
		result.CompletedAt = &completed
		result.FailureStage = nil
		result.Diagnostic = nil
		if outcome.Status == "succeeded" && currentGeneration {
			result.ResultFile = filepath.Base(resultPath)
		} else {
			if outcome.Status == "succeeded" {
				result.Evicted = true
			} else {
				stage := outcome.FailureStage
				result.FailureStage = &stage
				if outcome.Diagnostic != "" {
					diagnostic := outcome.Diagnostic
					result.Diagnostic = &diagnostic
				}
			}
		}
		result.ArtifactRevision++
		if err := putJSON(bucket, []byte(result.ID), result); err != nil {
			return err
		}
		if !currentGeneration {
			result.Evicted = true
			if err := putJSON(bucket, []byte(result.ID), result); err != nil {
				return err
			}
			return incrementHistoryResetDiscardedProbeItems(transaction, 1)
		}
		if err := putJSON(transaction.Bucket(probeArtifactsBucket), []byte(result.ID), queuedProbeArtifact{
			Kind: "execution", Revision: result.ArtifactRevision,
		}); err != nil {
			return err
		}
		var err error
		obsoleteFiles, err = enforceProbeResultLimit(transaction, result.EgressID)
		return err
	})
	if err != nil {
		if resultPath != "" {
			_ = os.Remove(resultPath)
		}
		return ProbeExecution{}, err
	}
	for _, name := range obsoleteFiles {
		_ = os.Remove(filepath.Join(s.resultDirectory, name))
	}
	return result, nil
}

func (s *Store) FinishProbeRun(runID string, completedAt time.Time) (ProbeRun, error) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if completedAt.IsZero() {
		return ProbeRun{}, ErrInvalidProbeState
	}
	completedAt = completedAt.UTC().Truncate(time.Second)
	var result ProbeRun
	err := s.database.Update(func(transaction *bolt.Tx) error {
		var err error
		result, err = finishProbeRun(s.masterKey, transaction, runID, completedAt)
		return err
	})
	return result, err
}

func (s *Store) ReconcileProbeRun(now time.Time) (*ProbeRun, error) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if now.IsZero() {
		return nil, ErrInvalidProbeState
	}
	now = now.UTC().Truncate(time.Second)
	var reconciled *ProbeRun
	var obsoleteFiles []string
	err := s.database.Update(func(transaction *bolt.Tx) error {
		runID := string(transaction.Bucket(probeControlBucket).Get(activeProbeRunKey))
		if runID == "" {
			return nil
		}
		var run ProbeRun
		if err := decodeProbeRun(transaction.Bucket(probeRunsBucket).Get([]byte(runID)), &run); err != nil {
			return err
		}
		currentGeneration, err := probeRunUsesCurrentGeneration(s.masterKey, transaction, runID)
		if err != nil {
			return err
		}
		for _, manifest := range run.Executions {
			var execution ProbeExecution
			bucket := transaction.Bucket(probeExecutionsBucket)
			if err := decodeProbeExecution(bucket.Get([]byte(manifest.ID)), &execution); err != nil {
				return err
			}
			if probeExecutionTerminal(execution.Status) {
				continue
			}
			execution.CompletedAt = &now
			execution.FailureStage = cloneString(stringPointer("restart"))
			if execution.Status == "running" {
				execution.Status = "interrupted"
				diagnostic := "Agent restarted while the upstream process was active"
				execution.Diagnostic = &diagnostic
			} else {
				execution.Status = "skipped"
			}
			execution.ArtifactRevision++
			if err := putJSON(bucket, []byte(execution.ID), execution); err != nil {
				return err
			}
			if currentGeneration {
				if err := putJSON(transaction.Bucket(probeArtifactsBucket), []byte(execution.ID), queuedProbeArtifact{
					Kind: "execution", Revision: execution.ArtifactRevision,
				}); err != nil {
					return err
				}
				files, err := enforceProbeResultLimit(transaction, execution.EgressID)
				if err != nil {
					return err
				}
				obsoleteFiles = append(obsoleteFiles, files...)
			} else {
				execution.Evicted = true
				if err := putJSON(bucket, []byte(execution.ID), execution); err != nil {
					return err
				}
				if err := incrementHistoryResetDiscardedProbeItems(transaction, 1); err != nil {
					return err
				}
			}
		}
		finished, err := finishProbeRun(s.masterKey, transaction, runID, now)
		if err != nil {
			return err
		}
		reconciled = &finished
		return nil
	})
	for _, name := range obsoleteFiles {
		_ = os.Remove(filepath.Join(s.resultDirectory, name))
	}
	return reconciled, err
}

func (s *Store) reconcileProbeArtifacts() error {
	var obsoleteFiles []string
	err := s.database.Update(func(transaction *bolt.Tx) error {
		queue := transaction.Bucket(probeArtifactsBucket)
		var executionIDs [][]byte
		if err := queue.ForEach(func(key, encoded []byte) error {
			var queued queuedProbeArtifact
			if err := decodeJSON(encoded, &queued, "queued probe artifact"); err != nil {
				return err
			}
			if queued.Kind == "execution" {
				executionIDs = append(executionIDs, append([]byte(nil), key...))
			}
			return nil
		}); err != nil {
			return err
		}
		for _, id := range executionIDs {
			var execution ProbeExecution
			if err := decodeProbeExecution(transaction.Bucket(probeExecutionsBucket).Get(id), &execution); err != nil {
				return err
			}
			if execution.ResultFile == "" {
				continue
			}
			path := filepath.Join(s.resultDirectory, execution.ResultFile)
			if _, err := readBoundedProbeResult(path); err == nil {
				continue
			}
			if err := queue.Delete(id); err != nil {
				return err
			}
			if err := extendProbeGap(transaction, execution); err != nil {
				return err
			}
			obsoleteFiles = append(obsoleteFiles, execution.ResultFile)
			execution.ResultFile = ""
			execution.Evicted = true
			if err := putJSON(transaction.Bucket(probeExecutionsBucket), id, execution); err != nil {
				return err
			}
			if err := cleanupProbeRun(transaction, execution.RunID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, name := range obsoleteFiles {
		if err := os.Remove(filepath.Join(s.resultDirectory, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove unusable complete-probe result: %w", err)
		}
	}
	return s.removeUnreferencedProbeResults()
}

func (s *Store) removeUnreferencedProbeResults() error {
	referenced := make(map[string]struct{})
	if err := s.database.View(func(transaction *bolt.Tx) error {
		return transaction.Bucket(probeExecutionsBucket).ForEach(func(_, encoded []byte) error {
			var execution ProbeExecution
			if err := decodeProbeExecution(encoded, &execution); err != nil {
				return err
			}
			if execution.ResultFile != "" {
				referenced[execution.ResultFile] = struct{}{}
			}
			return nil
		})
	}); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.resultDirectory)
	if err != nil {
		return fmt.Errorf("read Agent result directory: %w", err)
	}
	for _, entry := range entries {
		if _, exists := referenced[entry.Name()]; exists {
			continue
		}
		if err := os.Remove(filepath.Join(s.resultDirectory, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove unreferenced complete-probe result: %w", err)
		}
	}
	return nil
}

func (s *Store) NextProbeArtifact() (ProbeArtifact, error) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	var artifact ProbeArtifact
	var resultFile string
	err := s.database.View(func(transaction *bolt.Tx) error {
		cursor := transaction.Bucket(probeArtifactsBucket).Cursor()
		key, encoded := cursor.First()
		if key == nil {
			return nil
		}
		var queued queuedProbeArtifact
		if err := decodeJSON(encoded, &queued, "queued probe artifact"); err != nil {
			return err
		}
		artifact.ID = string(key)
		artifact.Revision = queued.Revision
		switch queued.Kind {
		case "run":
			var run ProbeRun
			if err := decodeProbeRun(transaction.Bucket(probeRunsBucket).Get(key), &run); err != nil {
				return err
			}
			artifact.Run = &run
		case "execution":
			var execution ProbeExecution
			if err := decodeProbeExecution(transaction.Bucket(probeExecutionsBucket).Get(key), &execution); err != nil {
				return err
			}
			var run ProbeRun
			if err := decodeProbeRun(transaction.Bucket(probeRunsBucket).Get([]byte(execution.RunID)), &run); err != nil {
				return err
			}
			artifact.Run = &run
			artifact.Execution = &execution
			resultFile = execution.ResultFile
		case "gap":
			var gap ProbeGap
			if err := decodeProbeGap(transaction.Bucket(probeGapsBucket).Get(key), &gap); err != nil {
				return err
			}
			artifact.Gap = &gap
		default:
			return errors.New("queued probe artifact has an invalid kind")
		}
		return nil
	})
	if err != nil || artifact.ID == "" || resultFile == "" {
		return artifact, err
	}
	raw, err := readBoundedProbeResult(filepath.Join(s.resultDirectory, resultFile))
	if err != nil {
		return ProbeArtifact{}, err
	}
	artifact.RawResult = raw
	return artifact, nil
}

func (s *Store) AcknowledgeProbeArtifact(receipt ProbeArtifactReceipt) error {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if _, err := uuid.Parse(receipt.ID); err != nil || receipt.Revision < 1 {
		return ErrInvalidProbeState
	}
	var removeFile string
	err := s.database.Update(func(transaction *bolt.Tx) error {
		queue := transaction.Bucket(probeArtifactsBucket)
		encoded := queue.Get([]byte(receipt.ID))
		if encoded == nil {
			return nil
		}
		var queued queuedProbeArtifact
		if err := decodeJSON(encoded, &queued, "queued probe artifact"); err != nil {
			return err
		}
		if queued.Revision > receipt.Revision {
			return nil
		}
		if queued.Revision < receipt.Revision {
			return ErrInvalidProbeState
		}
		if err := queue.Delete([]byte(receipt.ID)); err != nil {
			return err
		}
		switch queued.Kind {
		case "run":
			var run ProbeRun
			if err := decodeProbeRun(transaction.Bucket(probeRunsBucket).Get([]byte(receipt.ID)), &run); err != nil {
				return err
			}
			run.Delivered = true
			if err := putJSON(transaction.Bucket(probeRunsBucket), []byte(run.ID), run); err != nil {
				return err
			}
			return cleanupProbeRun(transaction, run.ID)
		case "execution":
			var execution ProbeExecution
			if err := decodeProbeExecution(transaction.Bucket(probeExecutionsBucket).Get([]byte(receipt.ID)), &execution); err != nil {
				return err
			}
			execution.Delivered = true
			removeFile = execution.ResultFile
			execution.ResultFile = ""
			if err := putJSON(transaction.Bucket(probeExecutionsBucket), []byte(execution.ID), execution); err != nil {
				return err
			}
			return cleanupProbeRun(transaction, execution.RunID)
		case "gap":
			return transaction.Bucket(probeGapsBucket).Delete([]byte(receipt.ID))
		default:
			return ErrInvalidProbeState
		}
	})
	if err == nil && removeFile != "" {
		_ = os.Remove(filepath.Join(s.resultDirectory, removeFile))
	}
	return err
}

func (s *Store) SetProbeProcess(process ProbeProcess) error {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if process.PID < 1 || process.ProcessGroupID < 1 || process.StartTicks < 1 || process.BootID == "" || process.StartedAt.IsZero() {
		return ErrInvalidProbeState
	}
	return s.database.Update(func(transaction *bolt.Tx) error {
		return putJSON(transaction.Bucket(probeProcessBucket), activeProcessKey, process)
	})
}

func (s *Store) ProbeProcess() (*ProbeProcess, error) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	var process ProbeProcess
	err := s.database.View(func(transaction *bolt.Tx) error {
		encoded := transaction.Bucket(probeProcessBucket).Get(activeProcessKey)
		if encoded == nil {
			return nil
		}
		return decodeJSON(encoded, &process, "active probe process")
	})
	if err != nil || process.PID == 0 {
		return nil, err
	}
	return &process, nil
}

func (s *Store) ClearProbeProcess(processGroupID int) error {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	return s.database.Update(func(transaction *bolt.Tx) error {
		bucket := transaction.Bucket(probeProcessBucket)
		encoded := bucket.Get(activeProcessKey)
		if encoded == nil {
			return nil
		}
		var current ProbeProcess
		if err := decodeJSON(encoded, &current, "active probe process"); err != nil {
			return err
		}
		if processGroupID != 0 && current.ProcessGroupID != processGroupID {
			return ErrInvalidProbeState
		}
		return bucket.Delete(activeProcessKey)
	})
}

func finishProbeRun(masterKey [masterKeySize]byte, transaction *bolt.Tx, runID string, completedAt time.Time) (ProbeRun, error) {
	if string(transaction.Bucket(probeControlBucket).Get(activeProbeRunKey)) != runID {
		return ProbeRun{}, ErrInvalidProbeState
	}
	var run ProbeRun
	if err := decodeProbeRun(transaction.Bucket(probeRunsBucket).Get([]byte(runID)), &run); err != nil {
		return ProbeRun{}, err
	}
	succeeded := 0
	for _, manifest := range run.Executions {
		var execution ProbeExecution
		if err := decodeProbeExecution(transaction.Bucket(probeExecutionsBucket).Get([]byte(manifest.ID)), &execution); err != nil {
			return ProbeRun{}, err
		}
		if !probeExecutionTerminal(execution.Status) {
			return ProbeRun{}, ErrInvalidProbeState
		}
		if execution.CompletedAt == nil || execution.CompletedAt.After(completedAt) {
			return ProbeRun{}, ErrInvalidProbeState
		}
		if execution.Status == "succeeded" {
			succeeded++
		}
	}
	switch {
	case succeeded == len(run.Executions):
		run.Status = "succeeded"
	case succeeded > 0:
		run.Status = "partial"
	default:
		run.Status = "failed"
	}
	run.CompletedAt = &completedAt
	run.ArtifactRevision++
	if err := putJSON(transaction.Bucket(probeRunsBucket), []byte(run.ID), run); err != nil {
		return ProbeRun{}, err
	}
	currentGeneration, err := probeRunUsesCurrentGeneration(masterKey, transaction, runID)
	if err != nil {
		return ProbeRun{}, err
	}
	if currentGeneration {
		if err := putJSON(transaction.Bucket(probeArtifactsBucket), []byte(run.ID), queuedProbeArtifact{Kind: "run", Revision: run.ArtifactRevision}); err != nil {
			return ProbeRun{}, err
		}
	} else if err := incrementHistoryResetDiscardedProbeItems(transaction, 1); err != nil {
		return ProbeRun{}, err
	}
	control := transaction.Bucket(probeControlBucket)
	if err := control.Delete(activeProbeRunKey); err != nil {
		return ProbeRun{}, err
	}
	status, err := probeStatusFromTransaction(transaction)
	if err != nil {
		return ProbeRun{}, err
	}
	status.ActiveRunID = nil
	if err := putJSON(control, probeStatusKey, status); err != nil {
		return ProbeRun{}, err
	}
	if run.TaskID != nil {
		bucket := transaction.Bucket(probeTasksBucket)
		var task storedProbeTask
		if err := decodeJSON(bucket.Get([]byte(*run.TaskID)), &task, "probe task"); err != nil {
			return ProbeRun{}, err
		}
		task.Status = run.Status
		task.CompletedAt = &completedAt
		if err := putJSON(bucket, []byte(task.ID), task); err != nil {
			return ProbeRun{}, err
		}
	}
	if !currentGeneration {
		if err := cleanupProbeRun(transaction, run.ID); err != nil {
			return ProbeRun{}, err
		}
	}
	return run, nil
}

func reconcileProbeGeneration(transaction *bolt.Tx, configuration Configuration, previousGeneration string) (int64, error) {
	if previousGeneration == "" || previousGeneration == configuration.HistoryGeneration {
		return 0, nil
	}
	if err := clearBucket(transaction.Bucket(probeSequencesBucket)); err != nil {
		return 0, fmt.Errorf("reset complete-probe sequences: %w", err)
	}
	queue := transaction.Bucket(probeArtifactsBucket)
	var discarded int64

	gapBucket := transaction.Bucket(probeGapsBucket)
	var obsoleteGaps [][]byte
	if err := gapBucket.ForEach(func(key, encoded []byte) error {
		var gap ProbeGap
		if err := decodeProbeGap(encoded, &gap); err != nil {
			return err
		}
		if gap.HistoryGeneration != configuration.HistoryGeneration {
			obsoleteGaps = append(obsoleteGaps, append([]byte(nil), key...))
		}
		return nil
	}); err != nil {
		return 0, err
	}
	for _, key := range obsoleteGaps {
		if queue.Get(key) != nil {
			discarded++
			if err := queue.Delete(key); err != nil {
				return 0, err
			}
		}
		if err := gapBucket.Delete(key); err != nil {
			return 0, err
		}
	}

	runBucket := transaction.Bucket(probeRunsBucket)
	var runIDs [][]byte
	if err := runBucket.ForEach(func(key, encoded []byte) error {
		var run ProbeRun
		if err := decodeProbeRun(encoded, &run); err != nil {
			return err
		}
		if run.HistoryGeneration != configuration.HistoryGeneration {
			runIDs = append(runIDs, append([]byte(nil), key...))
		}
		return nil
	}); err != nil {
		return 0, err
	}
	for _, runKey := range runIDs {
		var run ProbeRun
		if err := decodeProbeRun(runBucket.Get(runKey), &run); err != nil {
			return 0, err
		}
		if queue.Get(runKey) != nil {
			discarded++
			if err := queue.Delete(runKey); err != nil {
				return 0, err
			}
		}
		for _, manifest := range run.Executions {
			key := []byte(manifest.ID)
			var execution ProbeExecution
			if err := decodeProbeExecution(transaction.Bucket(probeExecutionsBucket).Get(key), &execution); err != nil {
				return 0, err
			}
			if queue.Get(key) != nil {
				discarded++
				if err := queue.Delete(key); err != nil {
					return 0, err
				}
			}
			execution.ResultFile = ""
			execution.Evicted = true
			if run.Status == "running" {
				if err := putJSON(transaction.Bucket(probeExecutionsBucket), key, execution); err != nil {
					return 0, err
				}
			} else if err := transaction.Bucket(probeExecutionsBucket).Delete(key); err != nil {
				return 0, err
			}
		}
		if run.Status != "running" {
			if err := runBucket.Delete(runKey); err != nil {
				return 0, err
			}
		}
	}
	return discarded, nil
}

func probeRunUsesCurrentGeneration(masterKey [masterKeySize]byte, transaction *bolt.Tx, runID string) (bool, error) {
	var run ProbeRun
	if err := decodeProbeRun(transaction.Bucket(probeRunsBucket).Get([]byte(runID)), &run); err != nil {
		return false, err
	}
	configuration, err := configurationFromTransaction(masterKey, transaction)
	if err != nil {
		return false, err
	}
	return run.HistoryGeneration == configuration.HistoryGeneration, nil
}

func incrementHistoryResetDiscardedProbeItems(transaction *bolt.Tx, count int64) error {
	if count < 1 {
		return nil
	}
	status, err := probeStatusFromTransaction(transaction)
	if err != nil {
		return err
	}
	status.HistoryResetDiscardedProbeItems += count
	return putJSON(transaction.Bucket(probeControlBucket), probeStatusKey, status)
}

func enforceProbeResultLimit(transaction *bolt.Tx, egressID string) ([]string, error) {
	type queuedExecution struct {
		execution ProbeExecution
	}
	var matching []queuedExecution
	queue := transaction.Bucket(probeArtifactsBucket)
	if err := transaction.Bucket(probeExecutionsBucket).ForEach(func(key, encoded []byte) error {
		var execution ProbeExecution
		if err := decodeProbeExecution(encoded, &execution); err != nil {
			return err
		}
		if execution.EgressID != egressID || !probeExecutionTerminal(execution.Status) {
			return nil
		}
		queued := queue.Get(key)
		if queued == nil {
			return nil
		}
		var item queuedProbeArtifact
		if err := decodeJSON(queued, &item, "queued probe artifact"); err != nil {
			return err
		}
		if item.Kind == "execution" {
			matching = append(matching, queuedExecution{execution: execution})
		}
		return nil
	}); err != nil {
		return nil, err
	}
	slices.SortFunc(matching, func(a, b queuedExecution) int {
		if a.execution.Sequence < b.execution.Sequence {
			return -1
		}
		if a.execution.Sequence > b.execution.Sequence {
			return 1
		}
		return strings.Compare(a.execution.ID, b.execution.ID)
	})
	var files []string
	for len(matching) > maxPendingProbeResultsPerEgress {
		dropped := matching[0].execution
		matching = matching[1:]
		if err := queue.Delete([]byte(dropped.ID)); err != nil {
			return nil, err
		}
		if err := extendProbeGap(transaction, dropped); err != nil {
			return nil, err
		}
		files = append(files, dropped.ResultFile)
		dropped.ResultFile = ""
		dropped.Evicted = true
		if err := putJSON(transaction.Bucket(probeExecutionsBucket), []byte(dropped.ID), dropped); err != nil {
			return nil, err
		}
		if err := cleanupProbeRun(transaction, dropped.RunID); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func extendProbeGap(transaction *bolt.Tx, dropped ProbeExecution) error {
	bucket := transaction.Bucket(probeGapsBucket)
	var existingKey []byte
	var gap ProbeGap
	if err := bucket.ForEach(func(key, encoded []byte) error {
		var item ProbeGap
		if err := decodeProbeGap(encoded, &item); err != nil {
			return err
		}
		if item.EgressID == dropped.EgressID {
			existingKey = append([]byte(nil), key...)
			gap = item
		}
		return nil
	}); err != nil {
		return err
	}
	observedAt := dropped.CompletedAt
	if observedAt == nil {
		return ErrInvalidProbeState
	}
	if existingKey == nil {
		gap = ProbeGap{
			ID: uuid.NewString(), EgressID: dropped.EgressID,
			FirstSequence: dropped.Sequence, LastSequence: dropped.Sequence,
			DroppedCount: 1, FirstObservedAt: *observedAt, LastObservedAt: *observedAt,
			ArtifactRevision: 1,
		}
		var run ProbeRun
		if err := decodeProbeRun(transaction.Bucket(probeRunsBucket).Get([]byte(dropped.RunID)), &run); err != nil {
			return err
		}
		gap.HistoryGeneration = run.HistoryGeneration
	} else {
		gap.DroppedCount++
		if dropped.Sequence < gap.FirstSequence {
			gap.FirstSequence = dropped.Sequence
			gap.FirstObservedAt = *observedAt
		}
		if dropped.Sequence > gap.LastSequence {
			gap.LastSequence = dropped.Sequence
			gap.LastObservedAt = *observedAt
		}
		gap.ArtifactRevision++
	}
	if err := putJSON(bucket, []byte(gap.ID), gap); err != nil {
		return err
	}
	return putJSON(transaction.Bucket(probeArtifactsBucket), []byte(gap.ID), queuedProbeArtifact{
		Kind: "gap", Revision: gap.ArtifactRevision,
	})
}

func cleanupProbeRun(transaction *bolt.Tx, runID string) error {
	var run ProbeRun
	if err := decodeProbeRun(transaction.Bucket(probeRunsBucket).Get([]byte(runID)), &run); err != nil {
		return err
	}
	if run.Status == "running" {
		return nil
	}
	queue := transaction.Bucket(probeArtifactsBucket)
	allExecutionsGone := true
	anyEvicted := false
	for _, manifest := range run.Executions {
		if queue.Get([]byte(manifest.ID)) != nil {
			allExecutionsGone = false
		}
		var execution ProbeExecution
		if err := decodeProbeExecution(transaction.Bucket(probeExecutionsBucket).Get([]byte(manifest.ID)), &execution); err != nil {
			return err
		}
		anyEvicted = anyEvicted || execution.Evicted
	}
	if !allExecutionsGone {
		return nil
	}
	if anyEvicted {
		if err := queue.Delete([]byte(run.ID)); err != nil {
			return err
		}
	}
	if queue.Get([]byte(run.ID)) != nil {
		return nil
	}
	for _, manifest := range run.Executions {
		if err := transaction.Bucket(probeExecutionsBucket).Delete([]byte(manifest.ID)); err != nil {
			return err
		}
	}
	return transaction.Bucket(probeRunsBucket).Delete([]byte(run.ID))
}

func nextProbeSequence(transaction *bolt.Tx, egressID string) (int64, error) {
	bucket := transaction.Bucket(probeSequencesBucket)
	current := bucket.Get([]byte(egressID))
	var value uint64
	if current != nil {
		if len(current) != 8 {
			return 0, errors.New("invalid complete-probe sequence in Agent state")
		}
		value = binary.BigEndian.Uint64(current)
	}
	if value >= uint64(1<<63-1) {
		return 0, errors.New("complete-probe sequence is exhausted")
	}
	value++
	encoded := make([]byte, 8)
	binary.BigEndian.PutUint64(encoded, value)
	if err := bucket.Put([]byte(egressID), encoded); err != nil {
		return 0, err
	}
	return int64(value), nil
}

func probeStatusFromTransaction(transaction *bolt.Tx) (ProbeStatus, error) {
	var status ProbeStatus
	encoded := transaction.Bucket(probeControlBucket).Get(probeStatusKey)
	if encoded == nil {
		return status, nil
	}
	if err := decodeJSON(encoded, &status, "complete-probe status"); err != nil {
		return ProbeStatus{}, err
	}
	return status, nil
}

func (s *Store) validateProbeControlState() error {
	return s.database.View(func(transaction *bolt.Tx) error {
		control := transaction.Bucket(probeControlBucket)
		status, err := probeStatusFromTransaction(transaction)
		if err != nil {
			return err
		}
		if status.NextScheduledAt != nil && status.NextScheduledAt.IsZero() {
			return errors.New("retained complete-probe status contains an invalid next schedule")
		}
		occurrenceFields := status.LastOccurrenceAt != nil && status.LastOccurrenceTrigger != nil && status.LastOccurrenceStatus != nil
		if occurrenceFields != (status.LastOccurrenceAt != nil || status.LastOccurrenceTrigger != nil || status.LastOccurrenceStatus != nil) ||
			status.LastOccurrenceAt != nil && status.LastOccurrenceAt.IsZero() ||
			status.LastOccurrenceTrigger != nil && !validProbeTrigger(*status.LastOccurrenceTrigger) ||
			status.LastOccurrenceStatus != nil && *status.LastOccurrenceStatus != "started" && *status.LastOccurrenceStatus != "skipped" ||
			(status.LastOccurrenceStatus != nil && *status.LastOccurrenceStatus == "skipped") != (status.LastSkipReason != nil) ||
			status.LastSkipReason != nil && !validProbeSkipReason(*status.LastSkipReason) {
			return errors.New("retained complete-probe occurrence status is invalid")
		}
		if (status.HistoryResetGeneration == nil) != (status.HistoryResetAt == nil) ||
			status.HistoryResetAt != nil && status.HistoryResetAt.IsZero() ||
			status.HistoryResetDiscardedAddressItems < 0 || status.HistoryResetDiscardedProbeItems < 0 ||
			status.HistoryResetGeneration == nil && (status.HistoryResetDiscardedAddressItems != 0 || status.HistoryResetDiscardedProbeItems != 0) {
			return errors.New("retained complete-probe history-reset status is invalid")
		}
		if status.HistoryResetGeneration != nil {
			if decoded, err := hex.DecodeString(*status.HistoryResetGeneration); err != nil || len(decoded) != 32 ||
				*status.HistoryResetGeneration != strings.ToLower(*status.HistoryResetGeneration) {
				return errors.New("retained complete-probe history-reset generation is invalid")
			}
		}
		activeID := string(control.Get(activeProbeRunKey))
		if (status.ActiveRunID == nil) != (activeID == "") {
			return errors.New("retained complete-probe active-run pointers disagree")
		}
		if status.ActiveRunID != nil {
			if _, err := uuid.Parse(*status.ActiveRunID); err != nil || *status.ActiveRunID != activeID {
				return errors.New("retained complete-probe active-run pointer is invalid")
			}
			var run ProbeRun
			if err := decodeProbeRun(transaction.Bucket(probeRunsBucket).Get([]byte(activeID)), &run); err != nil {
				return err
			}
			if run.Status != "running" || run.CompletedAt != nil {
				return errors.New("retained active complete-probe run is terminal")
			}
		}
		return nil
	})
}

func (s *Store) writeProbeResult(id string, raw []byte) (string, error) {
	if _, err := uuid.Parse(id); err != nil || len(raw) < 1 || len(raw) > MaxProbeResultBytes || !json.Valid(raw) {
		return "", errors.New("complete-probe result must be valid JSON no larger than 1 MiB")
	}
	temporary, err := os.CreateTemp(s.resultDirectory, ".probe-result-*")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	resultPath := filepath.Join(s.resultDirectory, id+".json")
	published := false
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryPath)
			if published {
				_ = os.Remove(resultPath)
			}
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return "", err
	}
	if _, err := temporary.Write(raw); err != nil {
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if _, err := os.Stat(resultPath); err == nil {
		return "", errors.New("immutable complete-probe result already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.Rename(temporaryPath, resultPath); err != nil {
		return "", err
	}
	published = true
	directory, err := os.Open(s.resultDirectory)
	if err != nil {
		return "", err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil || closeErr != nil {
		return "", errors.Join(syncErr, closeErr)
	}
	keep = true
	return resultPath, nil
}

func readBoundedProbeResult(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open retained complete-probe result: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() < 1 || info.Size() > MaxProbeResultBytes {
		return nil, errors.New("retained complete-probe result has invalid metadata")
	}
	raw, err := io.ReadAll(io.LimitReader(file, MaxProbeResultBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxProbeResultBytes || !json.Valid(raw) {
		return nil, errors.New("retained complete-probe result is invalid")
	}
	return raw, nil
}

func validateProbeTaskDelivery(task ProbeTaskDelivery) error {
	if _, err := uuid.Parse(task.ID); err != nil || task.CreatedAt.IsZero() || !task.ExpiresAt.After(task.CreatedAt) {
		return ErrInvalidProbeState
	}
	return nil
}

func validateProbeExecutionOutcome(outcome ProbeExecutionOutcome) error {
	if outcome.CompletedAt.IsZero() || len(outcome.RawResult) > MaxProbeResultBytes || len([]byte(outcome.Diagnostic)) > MaxProbeDiagnosticBytes ||
		!utf8.ValidString(outcome.Diagnostic) {
		return ErrInvalidProbeState
	}
	if outcome.StartedAt != nil && outcome.CompletedAt.Before(*outcome.StartedAt) {
		return ErrInvalidProbeState
	}
	switch outcome.Status {
	case "succeeded":
		if outcome.FailureStage != "" || outcome.Diagnostic != "" || len(outcome.RawResult) == 0 || !json.Valid(outcome.RawResult) {
			return ErrInvalidProbeState
		}
	case "failed", "interrupted":
		if !validProbeFailureStage(outcome.FailureStage) || len(outcome.RawResult) != 0 {
			return ErrInvalidProbeState
		}
	case "skipped":
		if outcome.StartedAt != nil || !validProbeFailureStage(outcome.FailureStage) || len(outcome.RawResult) != 0 {
			return ErrInvalidProbeState
		}
	default:
		return ErrInvalidProbeState
	}
	return nil
}

func validProbeTrigger(value string) bool {
	return value == "manual" || value == "schedule" || value == "address-change"
}

func validProbeSkipReason(value string) bool {
	switch value {
	case "busy", "disabled", "low-memory", "no-egress", "missed":
		return true
	default:
		return false
	}
}

func validProbeFailureStage(value string) bool {
	switch value {
	case "download", "selector", "adapter", "process", "timeout", "output", "restart":
		return true
	default:
		return false
	}
}

func probeExecutionTerminal(value string) bool {
	return value == "succeeded" || value == "failed" || value == "interrupted" || value == "skipped"
}

func probeTaskTerminal(value string) bool {
	return value == "succeeded" || value == "partial" || value == "failed" || value == "rejected"
}

func putJSON(bucket *bolt.Bucket, key []byte, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return bucket.Put(key, encoded)
}

func decodeJSON(encoded []byte, target any, name string) error {
	if encoded == nil {
		return fmt.Errorf("%s is missing from Agent state", name)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode retained %s: %w", name, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("retained %s contains trailing data", name)
		}
		return fmt.Errorf("decode trailing retained %s data: %w", name, err)
	}
	return nil
}

func decodeProbeRun(encoded []byte, target *ProbeRun) error {
	if encoded == nil {
		return ErrProbeRunNotFound
	}
	if err := decodeJSON(encoded, target, "complete-probe run"); err != nil {
		return err
	}
	if _, err := uuid.Parse(target.ID); err != nil || !validProbeTrigger(target.Trigger) || len(target.Executions) == 0 {
		return errors.New("retained complete-probe run is invalid")
	}
	return nil
}

func decodeProbeExecution(encoded []byte, target *ProbeExecution) error {
	if err := decodeJSON(encoded, target, "complete-probe execution"); err != nil {
		return err
	}
	if _, err := uuid.Parse(target.ID); err != nil || target.Sequence < 1 ||
		(target.ResultFile != "" && target.ResultFile != target.ID+".json") {
		return errors.New("retained complete-probe execution is invalid")
	}
	return nil
}

func decodeProbeGap(encoded []byte, target *ProbeGap) error {
	if err := decodeJSON(encoded, target, "complete-probe gap"); err != nil {
		return err
	}
	if _, err := uuid.Parse(target.ID); err != nil || target.DroppedCount < 1 {
		return errors.New("retained complete-probe gap is invalid")
	}
	return nil
}

func stringPointer(value string) *string {
	return &value
}
