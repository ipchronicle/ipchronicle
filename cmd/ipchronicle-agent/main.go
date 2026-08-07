package main

import (
	"fmt"
	"os"

	"github.com/ipchronicle/ipchronicle/internal/agent"
	"github.com/ipchronicle/ipchronicle/internal/version"
)

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println(version.Value)
		return
	}

	if err := agent.CheckRoot(os.Geteuid()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, agent.ErrRuntimeUnavailable)
	os.Exit(2)
}
