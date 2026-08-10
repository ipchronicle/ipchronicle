package releasetool

import (
	"debug/buildinfo"
	"debug/elf"
	"errors"
	"fmt"
)

type AgentBinaryInfo struct {
	Arch      string `json:"arch"`
	GoVersion string `json:"goVersion"`
}

func VerifyAgentBinary(path, architecture string) (AgentBinaryInfo, error) {
	wantMachine := elf.EM_NONE
	switch architecture {
	case "amd64":
		wantMachine = elf.EM_X86_64
	case "arm64":
		wantMachine = elf.EM_AARCH64
	default:
		return AgentBinaryInfo{}, errors.New("unsupported Agent architecture")
	}
	file, err := elf.Open(path)
	if err != nil {
		return AgentBinaryInfo{}, fmt.Errorf("open Agent ELF binary: %w", err)
	}
	defer file.Close()
	if file.Class != elf.ELFCLASS64 || file.Type != elf.ET_EXEC || file.Machine != wantMachine {
		return AgentBinaryInfo{}, errors.New("Agent ELF identity does not match its release platform")
	}
	for _, program := range file.Progs {
		if program.Type == elf.PT_INTERP {
			return AgentBinaryInfo{}, errors.New("Agent binary contains a dynamic interpreter")
		}
	}
	libraries, err := file.ImportedLibraries()
	if err != nil {
		return AgentBinaryInfo{}, fmt.Errorf("inspect Agent imported libraries: %w", err)
	}
	if len(libraries) != 0 {
		return AgentBinaryInfo{}, errors.New("Agent binary imports shared libraries")
	}
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return AgentBinaryInfo{}, fmt.Errorf("read Agent Go build metadata: %w", err)
	}
	if info.Path != "github.com/ipchronicle/ipchronicle/cmd/ipchronicle-agent" {
		return AgentBinaryInfo{}, errors.New("Agent Go package identity is invalid")
	}
	settings := make(map[string]string, len(info.Settings))
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if settings["GOOS"] != "linux" || settings["GOARCH"] != architecture ||
		settings["CGO_ENABLED"] != "0" || settings["-trimpath"] != "true" {
		return AgentBinaryInfo{}, errors.New("Agent Go build settings are not release-safe")
	}
	if _, ok := settings["vcs.revision"]; ok {
		return AgentBinaryInfo{}, errors.New("Agent contains non-reproducible VCS build metadata")
	}
	if info.GoVersion == "" {
		return AgentBinaryInfo{}, errors.New("Agent Go toolchain version is missing")
	}
	return AgentBinaryInfo{Arch: architecture, GoVersion: info.GoVersion}, nil
}
