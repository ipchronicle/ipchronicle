package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/agent/state"
)

const defaultHealthTimeout = 3 * time.Minute

type Lifecycle interface {
	StopAgent(context.Context) error
	StartAgent(context.Context) error
}

type SupervisorOptions struct {
	StateDirectory string
	AgentPath      string
	UpdaterPath    string
	InitSystem     string
	HealthTimeout  time.Duration
	Lifecycle      Lifecycle
	Now            func() time.Time
}

func RunSupervisor(ctx context.Context, options SupervisorOptions) error {
	if err := validateSupervisorOptions(options); err != nil {
		return err
	}
	lifecycle := options.Lifecycle
	if lifecycle == nil {
		lifecycle = commandLifecycle{initSystem: options.InitSystem}
	}
	healthTimeout := options.HealthTimeout
	if healthTimeout <= 0 {
		healthTimeout = defaultHealthTimeout
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	if err := lifecycle.StopAgent(ctx); err != nil {
		return fmt.Errorf("stop Agent before update: %w", err)
	}

	updateState, err := readPendingUpdate(options.StateDirectory)
	if err != nil {
		_ = lifecycle.StartAgent(context.Background())
		return err
	}
	if updateState.Status != "installing" && updateState.Status != "restarting" && updateState.Status != "succeeded" {
		_ = lifecycle.StartAgent(context.Background())
		return fmt.Errorf("Agent update %s is in state %s, expected installing, restarting, or succeeded", updateState.ID, updateState.Status)
	}
	checkpoint := checkpointPath(options.StateDirectory, updateState.ID)
	if updateState.Status == "installing" {
		if err := ensureCheckpoint(options.StateDirectory, options.AgentPath, checkpoint); err != nil {
			_ = lifecycle.StartAgent(context.Background())
			return fmt.Errorf("create Agent update checkpoint: %w", err)
		}
	} else if exists, err := HasCheckpoint(options.StateDirectory, updateState.ID); err != nil || !exists {
		_ = lifecycle.StartAgent(context.Background())
		if err != nil {
			return fmt.Errorf("inspect Agent update checkpoint: %w", err)
		}
		return errors.New("Agent update checkpoint is missing after replacement began")
	}
	marker := HealthMarkerPath(options.StateDirectory, updateState.ID)
	if updateState.Status == "restarting" || updateState.Status == "succeeded" {
		if _, err := os.Stat(marker); err == nil {
			if err := finalizeSuccessfulUpdate(options, checkpoint, marker, updateState.ID); err != nil {
				return err
			}
			return lifecycle.StartAgent(context.Background())
		} else if !errors.Is(err, os.ErrNotExist) {
			return rollbackUpdate(options, lifecycle, checkpoint, updateState, "health-marker", err, now())
		}
	} else {
		if err := replaceFile(StagedBinaryPath(options.StateDirectory, updateState.ID), options.AgentPath, 0o755); err != nil {
			return rollbackUpdate(options, lifecycle, checkpoint, updateState, "install-replace", err, now())
		}
		if err := markRestarting(options.StateDirectory, updateState.ID); err != nil {
			return rollbackUpdate(options, lifecycle, checkpoint, updateState, "state-restarting", err, now())
		}
	}
	_ = os.Remove(marker)
	if err := lifecycle.StartAgent(ctx); err != nil {
		return rollbackUpdate(options, lifecycle, checkpoint, updateState, "start-failed", err, now())
	}

	waitContext, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	if err := waitForHealthMarker(waitContext, marker); err != nil {
		return rollbackUpdate(options, lifecycle, checkpoint, updateState, "health-timeout", err, now())
	}
	return finalizeSuccessfulUpdate(options, checkpoint, marker, updateState.ID)
}

func HasCheckpoint(stateDirectory, taskID string) (bool, error) {
	if !filepath.IsAbs(stateDirectory) || taskID == "" || filepath.Base(taskID) != taskID {
		return false, errors.New("Agent update checkpoint identity is invalid")
	}
	checkpoint := checkpointPath(stateDirectory, taskID)
	complete := filepath.Join(checkpoint, "complete")
	info, err := os.Lstat(complete)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return false, errors.New("Agent update checkpoint marker permissions are invalid")
	}
	if err := validateCheckpointTree(checkpoint); err != nil {
		return false, err
	}
	return true, nil
}

func WriteHealthMarker(stateDirectory, taskID string) error {
	if !filepath.IsAbs(stateDirectory) || taskID == "" || filepath.Base(taskID) != taskID {
		return errors.New("health marker path is invalid")
	}
	directory, err := ensureUpdateDirectory(stateDirectory)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".healthy-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := io.WriteString(temporary, taskID+"\n"); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, HealthMarkerPath(stateDirectory, taskID)); err != nil {
		return err
	}
	return syncDirectory(directory)
}

func readPendingUpdate(stateDirectory string) (state.AgentUpdate, error) {
	store, err := state.Open(stateDirectory)
	if err != nil {
		return state.AgentUpdate{}, err
	}
	defer store.Close()
	updateState, found, err := store.PendingAgentUpdate()
	if err != nil {
		return state.AgentUpdate{}, err
	}
	if !found {
		return state.AgentUpdate{}, errors.New("no pending Agent update exists")
	}
	return updateState, nil
}

func markRestarting(stateDirectory, taskID string) error {
	store, err := state.Open(stateDirectory)
	if err != nil {
		return err
	}
	defer store.Close()
	_, err = store.MarkAgentUpdateRestarting(taskID)
	return err
}

func rollbackUpdate(
	options SupervisorOptions,
	lifecycle Lifecycle,
	checkpoint string,
	updateState state.AgentUpdate,
	failureCode string,
	cause error,
	now time.Time,
) error {
	stopContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	stopErr := lifecycle.StopAgent(stopContext)
	cancel()
	if stopErr != nil {
		return errors.Join(cause, fmt.Errorf("stop failed Agent before rollback: %w", stopErr))
	}
	if err := restoreCheckpoint(options.StateDirectory, options.AgentPath, checkpoint); err != nil {
		return errors.Join(cause, fmt.Errorf("restore Agent update checkpoint: %w", err))
	}
	store, err := state.Open(options.StateDirectory)
	if err != nil {
		return errors.Join(cause, err)
	}
	_, markErr := store.RollbackAgentUpdate(updateState.ID, failureCode, boundedDiagnostic(cause), now)
	closeErr := store.Close()
	if markErr != nil || closeErr != nil {
		return errors.Join(cause, markErr, closeErr)
	}
	startContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	startErr := lifecycle.StartAgent(startContext)
	cancel()
	if startErr != nil {
		return errors.Join(cause, fmt.Errorf("restart rolled-back Agent: %w", startErr))
	}
	if err := cleanupUpdateFiles(options.StateDirectory, checkpoint, updateState.ID); err != nil {
		return errors.Join(cause, fmt.Errorf("clean rollback checkpoint: %w", err))
	}
	return nil
}

func finalizeSuccessfulUpdate(options SupervisorOptions, checkpoint, marker, taskID string) error {
	if err := replaceFile(options.AgentPath, options.UpdaterPath, 0o755); err != nil {
		return fmt.Errorf("refresh independent Agent updater: %w", err)
	}
	if err := cleanupUpdateFiles(options.StateDirectory, checkpoint, taskID); err != nil {
		return err
	}
	_ = os.Remove(marker)
	return nil
}

func waitForHealthMarker(ctx context.Context, marker string) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if info, err := os.Stat(marker); err == nil {
			if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
				return errors.New("Agent health marker permissions are invalid")
			}
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func ensureCheckpoint(stateDirectory, agentPath, checkpoint string) error {
	taskID := strings.TrimPrefix(filepath.Base(checkpoint), "checkpoint-")
	if complete, err := HasCheckpoint(stateDirectory, taskID); err != nil {
		return err
	} else if complete {
		return nil
	}
	if err := os.RemoveAll(checkpoint); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(checkpoint, "results"), 0o700); err != nil {
		return err
	}
	for _, name := range []string{"state.db", "master.key"} {
		if err := copyFile(filepath.Join(stateDirectory, name), filepath.Join(checkpoint, name), 0o600); err != nil {
			return err
		}
	}
	if err := copyFile(agentPath, filepath.Join(checkpoint, "agent.previous"), 0o755); err != nil {
		return err
	}
	resultsDirectory := filepath.Join(stateDirectory, "results")
	if err := filepath.WalkDir(resultsDirectory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(resultsDirectory, path)
		if err != nil || relative == "." {
			return err
		}
		destination := filepath.Join(checkpoint, "results", relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Agent result checkpoint encountered non-regular file %s", path)
		}
		return os.Link(path, destination)
	}); err != nil {
		return err
	}
	if err := syncDirectoryTree(filepath.Join(checkpoint, "results")); err != nil {
		return err
	}
	if err := syncDirectory(checkpoint); err != nil {
		return err
	}
	complete := filepath.Join(checkpoint, "complete")
	marker, err := os.OpenFile(complete, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := marker.Sync(); err != nil {
		_ = marker.Close()
		return err
	}
	if err := marker.Close(); err != nil {
		return err
	}
	return syncDirectory(checkpoint)
}

func restoreCheckpoint(stateDirectory, agentPath, checkpoint string) error {
	if _, err := os.Stat(filepath.Join(checkpoint, "complete")); err != nil {
		return errors.New("Agent update checkpoint is incomplete")
	}
	for _, name := range []string{"state.db", "master.key"} {
		if err := replaceFile(filepath.Join(checkpoint, name), filepath.Join(stateDirectory, name), 0o600); err != nil {
			return err
		}
	}
	if err := replaceFile(filepath.Join(checkpoint, "agent.previous"), agentPath, 0o755); err != nil {
		return err
	}
	resultsDirectory := filepath.Join(stateDirectory, "results")
	entries, err := os.ReadDir(resultsDirectory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(resultsDirectory, entry.Name())); err != nil {
			return err
		}
	}
	checkpointResults := filepath.Join(checkpoint, "results")
	if err := filepath.WalkDir(checkpointResults, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(checkpointResults, path)
		if err != nil || relative == "." {
			return err
		}
		destination := filepath.Join(resultsDirectory, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		return os.Link(path, destination)
	}); err != nil {
		return err
	}
	if err := syncDirectoryTree(resultsDirectory); err != nil {
		return err
	}
	return syncDirectory(stateDirectory)
}

func validateCheckpointTree(checkpoint string) error {
	checkpointInfo, err := os.Lstat(checkpoint)
	if err != nil || !checkpointInfo.IsDir() || checkpointInfo.Mode().Perm() != 0o700 {
		return errors.New("Agent update checkpoint directory is invalid")
	}
	for name, mode := range map[string]fs.FileMode{
		"state.db": 0o600, "master.key": 0o600, "agent.previous": 0o755,
	} {
		info, err := os.Lstat(filepath.Join(checkpoint, name))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != mode {
			return fmt.Errorf("Agent update checkpoint file %s is invalid", name)
		}
	}
	results := filepath.Join(checkpoint, "results")
	return filepath.WalkDir(results, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if info.Mode().Perm() != 0o700 {
				return fmt.Errorf("Agent update checkpoint directory %s has invalid permissions", path)
			}
			return nil
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			return fmt.Errorf("Agent update checkpoint result %s is invalid", path)
		}
		return nil
	})
}

func syncDirectoryTree(root string) error {
	var directories []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	}); err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		if err := syncDirectory(directories[index]); err != nil {
			return err
		}
	}
	return nil
}

func cleanupUpdateFiles(stateDirectory, checkpoint, taskID string) error {
	if err := os.RemoveAll(checkpoint); err != nil {
		return err
	}
	for _, path := range []string{StagedBinaryPath(stateDirectory, taskID), HealthMarkerPath(stateDirectory, taskID)} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return syncDirectory(filepath.Join(stateDirectory, "update"))
}

func checkpointPath(stateDirectory, taskID string) string {
	return filepath.Join(stateDirectory, "update", "checkpoint-"+taskID)
}

func replaceFile(source, destination string, mode fs.FileMode) error {
	directory := filepath.Dir(destination)
	temporary, err := os.CreateTemp(directory, ".ipchronicle-replace-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	if _, err := io.Copy(temporary, sourceFile); err != nil {
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return err
	}
	return syncDirectory(directory)
}

func copyFile(source, destination string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	remove := true
	defer func() {
		_ = output.Close()
		if remove {
			_ = os.Remove(destination)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	if err := output.Chmod(mode); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	remove = false
	return nil
}

func validateSupervisorOptions(options SupervisorOptions) error {
	config := Config{InitSystem: options.InitSystem, AgentPath: options.AgentPath, UpdaterPath: options.UpdaterPath}
	if err := config.Validate(); err != nil {
		return err
	}
	clean := filepath.Clean(options.StateDirectory)
	if !filepath.IsAbs(clean) || clean == "/" || clean == "." {
		return errors.New("Agent state directory must be a specific absolute path")
	}
	return nil
}

type commandLifecycle struct {
	initSystem string
}

func (lifecycle commandLifecycle) StopAgent(ctx context.Context) error {
	return lifecycle.run(ctx, "stop")
}

func (lifecycle commandLifecycle) StartAgent(ctx context.Context) error {
	return lifecycle.run(ctx, "start")
}

func (lifecycle commandLifecycle) run(ctx context.Context, action string) error {
	var command *exec.Cmd
	if lifecycle.initSystem == "systemd" {
		command = exec.CommandContext(ctx, "systemctl", action, "ipchronicle-agent.service")
	} else {
		command = exec.CommandContext(ctx, "rc-service", "ipchronicle-agent", action)
	}
	output := &boundedBuffer{limit: 4096}
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s Agent service: %w: %s", action, err, strings.TrimSpace(output.String()))
	}
	return nil
}
