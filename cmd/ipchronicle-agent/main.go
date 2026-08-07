package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ipchronicle/ipchronicle/internal/agent"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
	"github.com/ipchronicle/ipchronicle/internal/version"
)

const defaultStateDirectory = "/var/lib/ipchronicle-agent"

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
	if err := agent.CheckRoot(os.Geteuid()); err != nil {
		return err
	}
	if len(arguments) == 0 {
		return errors.New("usage: ipchronicle-agent enroll|run|version")
	}
	switch arguments[0] {
	case "enroll":
		return runEnroll(arguments[1:])
	case "run":
		return runService(arguments[1:])
	default:
		return errors.New("usage: ipchronicle-agent enroll|run|version")
	}
}

func runEnroll(arguments []string) error {
	flags := flag.NewFlagSet("enroll", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	centerURL := flags.String("center-url", "", "center HTTP or HTTPS origin")
	registrationKey := flags.String("registration-key", "", "automatic registration key")
	stateDirectory := flags.String("state-dir", defaultStateDirectory, "root-only Agent state directory")
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
	identity, err := agent.Enroll(context.Background(), store, *centerURL, *registrationKey, version.Value)
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
	return agent.Run(ctx, store, version.Value, log.Default())
}
