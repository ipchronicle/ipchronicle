package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/ipchronicle/ipchronicle/internal/agent"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
	agentupdate "github.com/ipchronicle/ipchronicle/internal/agent/update"
	"github.com/ipchronicle/ipchronicle/internal/releaseinfo"
	"github.com/ipchronicle/ipchronicle/internal/version"
)

const defaultStateDirectory = "/var/lib/ipchronicle-agent"

const (
	defaultAgentPath   = "/usr/local/bin/ipchronicle-agent"
	defaultUpdaterPath = "/usr/local/libexec/ipchronicle-agent-updater"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(arguments []string) error {
	if len(arguments) == 1 && (arguments[0] == "version" || arguments[0] == "--version") {
		fmt.Println(version.Value)
		return nil
	}
	if len(arguments) == 2 && arguments[0] == "version" && arguments[1] == "--json" {
		return json.NewEncoder(os.Stdout).Encode(releaseinfo.BinaryInfo{
			Version: version.Value, Revision: version.Revision, Component: "agent",
			OS: runtime.GOOS, Arch: runtime.GOARCH,
			Capabilities:       append([]string{}, releaseinfo.RequiredAgentCapabilities...),
			StateSchemaVersion: state.SchemaVersion(),
		})
	}
	if err := agent.CheckRoot(os.Geteuid()); err != nil {
		return err
	}
	if len(arguments) == 0 {
		return errors.New("usage: ipchronicle-agent enroll|run|update-supervisor|version")
	}
	switch arguments[0] {
	case "enroll":
		return runEnroll(arguments[1:])
	case "run":
		return runService(arguments[1:])
	case "update-supervisor":
		return runUpdateSupervisor(arguments[1:])
	default:
		return errors.New("usage: ipchronicle-agent enroll|run|update-supervisor|version")
	}
}

func runEnroll(arguments []string) error {
	flags := flag.NewFlagSet("enroll", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	centerURL := flags.String("center-url", "", "center HTTP or HTTPS origin")
	registrationKey := flags.String("registration-key", "", "automatic registration key")
	stateDirectory := flags.String("state-dir", defaultStateDirectory, "root-only Agent state directory")
	updateInit := flags.String("update-init", "", "installed Agent updater init system")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || *centerURL == "" || *registrationKey == "" {
		return errors.New("usage: ipchronicle-agent enroll --center-url URL --registration-key KEY [--state-dir PATH]")
	}
	if !filepath.IsAbs(*stateDirectory) {
		return errors.New("Agent state directory must be absolute")
	}
	store, err := state.Open(*stateDirectory)
	if err != nil {
		return err
	}
	defer store.Close()
	updateCapable := false
	if *updateInit != "" {
		config := agentupdate.Config{InitSystem: *updateInit, AgentPath: defaultAgentPath, UpdaterPath: defaultUpdaterPath}
		if err := config.Validate(); err != nil {
			return err
		}
		updateCapable = true
	}
	identity, err := agent.EnrollWithCapabilities(context.Background(), store, *centerURL, *registrationKey, version.Value, updateCapable)
	if err != nil {
		return err
	}
	fmt.Printf("Agent enrolled as node %s\n", identity.NodeID)
	return nil
}

func runService(arguments []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	stateDirectory := flags.String("state-dir", defaultStateDirectory, "root-only Agent state directory")
	updateInit := flags.String("update-init", "", "installed Agent updater init system")
	agentPath := flags.String("agent-path", defaultAgentPath, "installed Agent executable")
	updaterPath := flags.String("updater-path", defaultUpdaterPath, "independent Agent updater executable")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*stateDirectory) {
		return errors.New("usage: ipchronicle-agent run [--state-dir PATH]")
	}
	store, err := state.Open(*stateDirectory)
	if err != nil {
		return err
	}
	defer store.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	options := agent.RunOptions{}
	if *updateInit != "" {
		config := agentupdate.Config{InitSystem: *updateInit, AgentPath: *agentPath, UpdaterPath: *updaterPath}
		if err := config.Validate(); err != nil {
			return err
		}
		options.UpdateConfig = &config
	}
	return agent.RunWithOptions(ctx, store, version.Value, log.Default(), options)
}

func runUpdateSupervisor(arguments []string) error {
	flags := flag.NewFlagSet("update-supervisor", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	stateDirectory := flags.String("state-dir", defaultStateDirectory, "root-only Agent state directory")
	agentPath := flags.String("agent-path", defaultAgentPath, "installed Agent executable")
	updaterPath := flags.String("updater-path", defaultUpdaterPath, "independent Agent updater executable")
	initSystem := flags.String("update-init", "", "systemd or openrc")
	healthTimeout := flags.Duration("health-timeout", 0, "post-restart health timeout")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || *initSystem == "" {
		return errors.New("usage: ipchronicle-agent update-supervisor --update-init systemd|openrc [--state-dir PATH]")
	}
	return agentupdate.RunSupervisor(context.Background(), agentupdate.SupervisorOptions{
		StateDirectory: *stateDirectory, AgentPath: *agentPath, UpdaterPath: *updaterPath,
		InitSystem: *initSystem, HealthTimeout: *healthTimeout,
	})
}
