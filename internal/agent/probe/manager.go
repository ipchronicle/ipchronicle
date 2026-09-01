package probe

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/agent/state"
	sharedschedule "github.com/ipchronicle/ipchronicle/internal/schedule"
)

const (
	minimumProbeMemoryBytes  = 64 * 1024 * 1024
	managerResolution        = 250 * time.Millisecond
	probeTaskCleanupInterval = time.Hour
)

type executionRunner interface {
	Run(context.Context, state.Configuration, state.Egress, time.Time) (state.ProbeExecutionOutcome, error)
}

type Manager struct {
	store               *state.Store
	runner              executionRunner
	logger              *log.Logger
	physicalMemoryBytes int64
	now                 func() time.Time

	mu         sync.Mutex
	active     bool
	errors     chan error
	wake       chan struct{}
	uploadWake chan struct{}
	wg         sync.WaitGroup
}

func NewManager(store *state.Store, physicalMemoryBytes int64, logger *log.Logger) *Manager {
	if store == nil {
		panic("probe manager store must not be nil")
	}
	if physicalMemoryBytes < 1 {
		panic("probe manager physical memory must be positive")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Manager{
		store: store, runner: NewRunner(), logger: logger,
		physicalMemoryBytes: physicalMemoryBytes, now: time.Now,
		errors: make(chan error, 1), wake: make(chan struct{}, 1), uploadWake: make(chan struct{}, 1),
	}
}

func (manager *Manager) Wake() <-chan struct{} {
	return manager.wake
}

func (manager *Manager) UploadWake() <-chan struct{} {
	return manager.uploadWake
}

func (manager *Manager) Run(ctx context.Context) error {
	if reconciled, err := manager.store.ReconcileProbeRun(manager.now()); err != nil {
		return fmt.Errorf("reconcile complete-probe run: %w", err)
	} else if reconciled != nil {
		manager.signalWake()
	}
	if err := manager.store.CleanupProbeTasks(manager.now()); err != nil {
		return fmt.Errorf("clean up retained probe tasks: %w", err)
	}
	nextTaskCleanupAt := manager.now().UTC().Add(probeTaskCleanupInterval)

	var scheduleFingerprint string
	var nextScheduledAt *time.Time
	initialized := false
	ticker := time.NewTicker(managerResolution)
	defer ticker.Stop()
	defer manager.wg.Wait()
	for {
		if err := manager.cleanupTasksIfDue(&nextTaskCleanupAt); err != nil {
			return err
		}
		if err := manager.reconcileSchedule(ctx, &scheduleFingerprint, &nextScheduledAt, &initialized); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case err := <-manager.errors:
			return err
		case <-ticker.C:
		}
	}
}

func (manager *Manager) cleanupTasksIfDue(next *time.Time) error {
	now := manager.now().UTC()
	if now.Before(*next) {
		return nil
	}
	if err := manager.store.CleanupProbeTasks(now); err != nil {
		return fmt.Errorf("clean up retained probe tasks: %w", err)
	}
	*next = now.Add(probeTaskCleanupInterval)
	return nil
}

func (manager *Manager) AcceptTask(ctx context.Context, task state.ProbeTaskDelivery) error {
	if _, exists, err := manager.store.ProbeTask(task.ID); err != nil {
		return err
	} else if exists {
		return nil
	}
	if task.Trigger == "" {
		task.Trigger = "manual"
	}
	return manager.tryStart(ctx, task.Trigger, &task, manager.now())
}

func (manager *Manager) reconcileSchedule(ctx context.Context, fingerprint *string, next **time.Time, initialized *bool) error {
	configuration, err := manager.store.Configuration()
	if errors.Is(err, state.ErrNoConfiguration) {
		if *next != nil {
			if err := manager.store.SetNextProbeSchedule(nil); err != nil {
				return err
			}
		}
		*fingerprint = ""
		*next = nil
		*initialized = false
		return nil
	}
	if err != nil {
		return err
	}
	currentFingerprint := strconv.FormatBool(configuration.ProbeSchedule.Enabled) + "\x00" +
		configuration.ProbeSchedule.Cron + "\x00" + configuration.ProbeSchedule.Timezone
	now := manager.now().UTC()
	if !*initialized || *fingerprint != currentFingerprint {
		firstInitialization := !*initialized
		*fingerprint = currentFingerprint
		*initialized = true
		if !configuration.ProbeSchedule.Enabled {
			*next = nil
			return manager.store.SetNextProbeSchedule(nil)
		}
		*next = nil
		if firstInitialization {
			if status, _, statusErr := manager.store.ProbeControlReport(); statusErr != nil {
				return statusErr
			} else if status.NextScheduledAt != nil {
				value := status.NextScheduledAt.UTC()
				*next = &value
			}
		}
		if *next == nil || (*next).After(now) == false {
			if *next != nil && !(*next).After(now) {
				if err := manager.store.RecordSkippedProbe("schedule", "missed", **next); err != nil {
					return err
				}
			}
			value, err := sharedschedule.NextProbe(
				configuration.ProbeSchedule.Cron, configuration.ProbeSchedule.Timezone, now,
			)
			if err != nil {
				return err
			}
			*next = &value
		}
		return manager.store.SetNextProbeSchedule(*next)
	}
	if *next == nil || (*next).After(now) {
		return nil
	}
	occurrence := **next
	if err := manager.tryStart(ctx, "schedule", nil, occurrence); err != nil {
		return err
	}
	value, err := sharedschedule.NextProbe(
		configuration.ProbeSchedule.Cron, configuration.ProbeSchedule.Timezone, now,
	)
	if err != nil {
		return err
	}
	*next = &value
	return manager.store.SetNextProbeSchedule(*next)
}

func (manager *Manager) tryStart(
	ctx context.Context,
	trigger string,
	task *state.ProbeTaskDelivery,
	at time.Time,
) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.active {
		return manager.rejectOccurrence(trigger, task, "busy", at)
	}
	for attempts := 0; attempts < 2; attempts++ {
		configuration, err := manager.store.Configuration()
		if errors.Is(err, state.ErrNoConfiguration) {
			return manager.rejectOccurrence(trigger, task, "disabled", at)
		}
		if err != nil {
			return err
		}
		if reason := manager.ineligibleReason(configuration, task, at); reason != "" {
			return manager.rejectOccurrence(trigger, task, reason, at)
		}
		run, err := manager.store.StartProbeRunAtRevision(
			configuration.Revision, trigger, task, at,
		)
		if errors.Is(err, state.ErrProbeConfigurationChanged) {
			continue
		}
		if errors.Is(err, state.ErrProbeBusy) {
			return manager.rejectOccurrence(trigger, task, "busy", at)
		}
		if errors.Is(err, state.ErrProbeTaskHandled) {
			return nil
		}
		if err != nil {
			return err
		}
		manager.active = true
		manager.signalWake()
		manager.wg.Add(1)
		go manager.execute(ctx, configuration, run)
		return nil
	}
	return errors.New("Agent configuration changed repeatedly while starting a complete-probe run")
}

func (manager *Manager) ineligibleReason(
	configuration state.Configuration,
	task *state.ProbeTaskDelivery,
	at time.Time,
) string {
	if task != nil && !task.ExpiresAt.After(at) {
		return "missed"
	}
	if !configuration.Enabled {
		return "disabled"
	}
	if manager.physicalMemoryBytes < minimumProbeMemoryBytes && !configuration.ProbeLowMemoryOverride {
		return "low-memory"
	}
	enabled := 0
	requested := make(map[string]struct{})
	if task != nil {
		for _, id := range task.PublicAddressIDs {
			requested[id] = struct{}{}
		}
	}
	for _, egress := range configuration.ProbeTargetList() {
		if !egress.Enabled && (task == nil || task.Trigger != "manual") {
			continue
		}
		if task != nil {
			if _, exists := requested[egress.ID]; !exists {
				continue
			}
		}
		enabled++
	}
	if enabled == 0 {
		return "no-egress"
	}
	if task != nil && enabled != len(requested) {
		return "disabled"
	}
	return ""
}

func (manager *Manager) rejectOccurrence(trigger string, task *state.ProbeTaskDelivery, reason string, at time.Time) error {
	if task != nil {
		if _, err := manager.store.RejectProbeTask(*task, reason, at); err != nil && !errors.Is(err, state.ErrProbeTaskHandled) {
			return err
		}
	} else if err := manager.store.RecordSkippedProbe(trigger, reason, at); err != nil {
		return err
	}
	manager.signalWake()
	return nil
}

func (manager *Manager) execute(ctx context.Context, configuration state.Configuration, run state.ProbeRun) {
	defer manager.wg.Done()
	defer func() {
		manager.mu.Lock()
		manager.active = false
		manager.mu.Unlock()
		manager.signalWake()
	}()
	targets := configuration.ProbeTargetList()
	egresses := make(map[string]state.Egress, len(targets))
	for _, egress := range targets {
		egresses[egress.ID] = egress
	}
	for _, manifest := range run.Executions {
		if ctx.Err() != nil {
			if _, err := manager.store.ReconcileProbeRun(manager.now()); err != nil {
				manager.reportError(err)
			}
			return
		}
		egress, exists := egresses[manifest.EgressID]
		if !exists {
			manager.reportError(errors.New("frozen complete-probe egress is missing from its configuration snapshot"))
			return
		}
		startedAt := manager.now().UTC()
		execution, err := manager.store.StartProbeExecution(run.ID, manifest.ID, startedAt)
		if err != nil {
			manager.reportError(err)
			return
		}
		outcome, runErr := manager.runner.Run(ctx, configuration, egress, *execution.StartedAt)
		if outcome.Status != "" {
			if _, err := manager.store.CompleteProbeExecution(run.ID, manifest.ID, outcome); err != nil {
				manager.reportError(err)
				return
			}
			manager.signalWake()
		}
		if runErr != nil {
			manager.reportError(runErr)
			return
		}
	}
	if _, err := manager.store.FinishProbeRun(run.ID, manager.now()); err != nil {
		manager.reportError(err)
		return
	}
	manager.signalWake()
}

func (manager *Manager) reportError(err error) {
	select {
	case manager.errors <- fmt.Errorf("run complete probe: %w", err):
	default:
	}
}

func (manager *Manager) signalWake() {
	select {
	case manager.wake <- struct{}{}:
	default:
	}
	select {
	case manager.uploadWake <- struct{}{}:
	default:
	}
}
