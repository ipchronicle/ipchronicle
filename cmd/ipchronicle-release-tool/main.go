package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/ipchronicle/ipchronicle/internal/releasetool"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "ipchronicle-release-tool: %v\n", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return usageError()
	}
	flags := flag.NewFlagSet(arguments[0], flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	directory := flags.String("directory", "", "release candidate directory")
	path := flags.String("path", "", "release artifact path")
	architecture := flags.String("arch", "", "release artifact architecture")
	version := flags.String("version", "", "canonical product version without the v prefix")
	revision := flags.String("revision", "", "lowercase 40-character Git revision")
	ciRunURL := flags.String("ci-run-url", "", "successful ordinary CI run URL")
	rcRunURL := flags.String("rc-run-url", "", "successful release candidate run URL")
	validationDate := flags.String("validation-date", "", "validation date in YYYY-MM-DD format")
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return usageError()
	}
	var (
		summary releasetool.Summary
		err     error
	)
	switch arguments[0] {
	case "create":
		if *version == "" || *revision == "" {
			return usageError()
		}
		summary, err = releasetool.Create(releasetool.CreateOptions{
			Directory: *directory, Version: *version, Revision: *revision,
		})
	case "verify":
		summary, err = releasetool.Verify(releasetool.VerifyOptions{
			Directory: *directory, Version: *version, Revision: *revision,
		})
	case "finalize":
		if *ciRunURL == "" || *rcRunURL == "" || *validationDate == "" {
			return usageError()
		}
		summary, err = releasetool.Finalize(releasetool.FinalizeOptions{
			Directory: *directory, Version: *version, Revision: *revision,
			CIRunURL: *ciRunURL, RCRunURL: *rcRunURL, ValidationDate: *validationDate,
		})
	case "verify-agent":
		info, verifyErr := releasetool.VerifyAgentBinary(*path, *architecture)
		if verifyErr != nil {
			return verifyErr
		}
		return json.NewEncoder(os.Stdout).Encode(info)
	case "verify-oci":
		if *version == "" || *revision == "" {
			return usageError()
		}
		info, verifyErr := releasetool.VerifyCenterOCI(*path, *architecture, *version, *revision)
		if verifyErr != nil {
			return verifyErr
		}
		return json.NewEncoder(os.Stdout).Encode(info)
	default:
		return usageError()
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(summary)
}

func usageError() error {
	return errors.New("usage: ipchronicle-release-tool create|verify|finalize --directory PATH [release evidence flags]; verify-agent|verify-oci --path PATH --arch ARCH")
}
