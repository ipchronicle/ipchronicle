package probe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/agent/state"
)

const processTerminationGrace = 5 * time.Second

type processRecoveryState interface {
	ProbeProcess() (*state.ProbeProcess, error)
	ClearProbeProcess(int) error
}

func recoverRetainedProcess(ctx context.Context, store processRecoveryState) error {
	retained, err := store.ProbeProcess()
	if err != nil || retained == nil {
		return err
	}
	bootID, err := readBootID()
	if err != nil {
		return err
	}
	if retained.BootID != bootID {
		return store.ClearProbeProcess(retained.ProcessGroupID)
	}
	same, err := sameProcessIdentity(*retained)
	if err != nil {
		return err
	}
	if !same {
		return store.ClearProbeProcess(retained.ProcessGroupID)
	}
	if err := signalProcessGroup(retained.ProcessGroupID, syscall.SIGTERM); err != nil {
		return err
	}
	if waitForProcessGroup(ctx, retained.ProcessGroupID, processTerminationGrace) {
		return store.ClearProbeProcess(retained.ProcessGroupID)
	}
	if err := signalProcessGroup(retained.ProcessGroupID, syscall.SIGKILL); err != nil {
		return err
	}
	if !waitForProcessGroup(ctx, retained.ProcessGroupID, processTerminationGrace) {
		return errors.New("retained probe process group did not terminate")
	}
	return store.ClearProbeProcess(retained.ProcessGroupID)
}

func sameProcessIdentity(process state.ProbeProcess) (bool, error) {
	startTicks, err := readProcessStartTicks(process.PID)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if startTicks != process.StartTicks {
		return false, nil
	}
	processGroupID, err := syscall.Getpgid(process.PID)
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return processGroupID == process.ProcessGroupID, nil
}

func signalProcessGroup(processGroupID int, signal syscall.Signal) error {
	err := syscall.Kill(-processGroupID, signal)
	if err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func waitForProcessGroup(ctx context.Context, processGroupID int, limit time.Duration) bool {
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := syscall.Kill(-processGroupID, 0)
		if errors.Is(err, syscall.ESRCH) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-ticker.C:
		}
	}
}

func currentProcessIdentity(pid, processGroupID int, startedAt time.Time) (state.ProbeProcess, error) {
	startTicks, err := readProcessStartTicks(pid)
	if err != nil {
		return state.ProbeProcess{}, err
	}
	bootID, err := readBootID()
	if err != nil {
		return state.ProbeProcess{}, err
	}
	return state.ProbeProcess{
		PID: pid, ProcessGroupID: processGroupID, StartTicks: startTicks,
		BootID: bootID, StartedAt: startedAt.UTC().Truncate(time.Second),
	}, nil
}

func readProcessStartTicks(pid int) (uint64, error) {
	body, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, fmt.Errorf("read probe process identity: %w", err)
	}
	closing := strings.LastIndexByte(string(body), ')')
	if closing < 0 {
		return 0, errors.New("probe process stat has no command terminator")
	}
	fields := strings.Fields(string(body[closing+1:]))
	if len(fields) <= 19 {
		return 0, errors.New("probe process stat is incomplete")
	}
	value, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || value == 0 {
		return 0, errors.New("probe process stat has an invalid start time")
	}
	return value, nil
}

func readBootID() (string, error) {
	body, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", fmt.Errorf("read Linux boot identity: %w", err)
	}
	value := strings.TrimSpace(string(body))
	if value == "" || len(value) > 128 {
		return "", errors.New("Linux boot identity is invalid")
	}
	return value, nil
}

func terminateProcessGroup(processGroupID int, done <-chan error) error {
	if processGroupID < 1 {
		return errors.New("probe process group is invalid")
	}
	termErr := syscall.Kill(-processGroupID, syscall.SIGTERM)
	if termErr != nil && !errors.Is(termErr, syscall.ESRCH) {
		return termErr
	}
	timer := time.NewTimer(processTerminationGrace)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
	}
	killErr := syscall.Kill(-processGroupID, syscall.SIGKILL)
	if killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
		return killErr
	}
	return <-done
}
