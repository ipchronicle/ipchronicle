package releasetool

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ipchronicle/ipchronicle/internal/releaseinfo"
)

const maximumArtifactBytes = 4 * 1024 * 1024 * 1024

type ArtifactDefinition struct {
	Name       string
	Component  string
	OS         string
	Arch       string
	Executable bool
}

var ArtifactDefinitions = []ArtifactDefinition{
	{Name: ".env.example", Component: "environment"},
	{Name: "LICENSE", Component: "license"},
	{Name: "THIRD_PARTY_NOTICES.md", Component: "notices"},
	{Name: "build-metadata.json", Component: "build-metadata"},
	{Name: "compose.yaml", Component: "compose"},
	{Name: "install-agent.sh", Component: "installer", Executable: true},
	{Name: "ipchronicle-agent-linux-amd64", Component: "agent", OS: "linux", Arch: "amd64", Executable: true},
	{Name: "ipchronicle-agent-linux-amd64.cdx.json", Component: "sbom"},
	{Name: "ipchronicle-agent-linux-arm64", Component: "agent", OS: "linux", Arch: "arm64", Executable: true},
	{Name: "ipchronicle-agent-linux-arm64.cdx.json", Component: "sbom"},
	{Name: "ipchronicle-center-linux-amd64.cdx.json", Component: "sbom"},
	{Name: "ipchronicle-center-linux-amd64.oci.tar", Component: "center-image"},
	{Name: "ipchronicle-center-linux-arm64.cdx.json", Component: "sbom"},
	{Name: "ipchronicle-center-linux-arm64.oci.tar", Component: "center-image"},
}

type CreateOptions struct {
	Directory string
	Version   string
	Revision  string
}

type VerifyOptions struct {
	Directory string
	Version   string
	Revision  string
}

type Summary struct {
	Version       string `json:"version"`
	Revision      string `json:"revision"`
	ArtifactCount int    `json:"artifactCount"`
}

func Create(options CreateOptions) (Summary, error) {
	if options.Directory == "" {
		return Summary{}, errors.New("release directory is required")
	}
	version, err := releaseinfo.CanonicalVersion(options.Version)
	if err != nil {
		return Summary{}, err
	}
	if err := validateDirectoryEntries(options.Directory, false); err != nil {
		return Summary{}, err
	}

	artifacts := make([]releaseinfo.Artifact, 0, len(ArtifactDefinitions))
	for _, definition := range ArtifactDefinitions {
		path := filepath.Join(options.Directory, definition.Name)
		size, digest, err := inspectFile(path, definition.Executable)
		if err != nil {
			return Summary{}, fmt.Errorf("inspect release artifact %q: %w", definition.Name, err)
		}
		artifacts = append(artifacts, releaseinfo.Artifact{
			Name: definition.Name, Component: definition.Component,
			OS: definition.OS, Arch: definition.Arch, Size: size, SHA256: digest,
		})
	}
	slices.SortFunc(artifacts, func(left, right releaseinfo.Artifact) int {
		return strings.Compare(left.Name, right.Name)
	})
	manifest := releaseinfo.Manifest{
		SchemaVersion:     releaseinfo.ManifestSchemaVersion,
		Version:           version,
		Tag:               "v" + version,
		Channel:           channelForVersion(version),
		Revision:          options.Revision,
		SourceURL:         releaseinfo.SourceRepository + "/tree/v" + version,
		AgentCapabilities: slices.Clone(releaseinfo.RequiredAgentCapabilities),
		Artifacts:         artifacts,
	}
	if err := manifest.Validate(); err != nil {
		return Summary{}, err
	}
	encodedManifest, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Summary{}, fmt.Errorf("encode release manifest: %w", err)
	}
	encodedManifest = append(encodedManifest, '\n')
	if err := writeFileAtomic(filepath.Join(options.Directory, releaseinfo.ManifestAssetName), encodedManifest, 0o644); err != nil {
		return Summary{}, err
	}

	checksumNames := make([]string, 0, len(ArtifactDefinitions)+1)
	for _, definition := range ArtifactDefinitions {
		checksumNames = append(checksumNames, definition.Name)
	}
	checksumNames = append(checksumNames, releaseinfo.ManifestAssetName)
	slices.Sort(checksumNames)
	var checksums strings.Builder
	for _, name := range checksumNames {
		_, digest, err := inspectFile(filepath.Join(options.Directory, name), false)
		if err != nil {
			return Summary{}, fmt.Errorf("checksum release file %q: %w", name, err)
		}
		fmt.Fprintf(&checksums, "%s  %s\n", digest, name)
	}
	if err := writeFileAtomic(
		filepath.Join(options.Directory, releaseinfo.ChecksumsAssetName),
		[]byte(checksums.String()), 0o644,
	); err != nil {
		return Summary{}, err
	}
	return Verify(VerifyOptions{Directory: options.Directory, Version: version, Revision: options.Revision})
}

func Verify(options VerifyOptions) (Summary, error) {
	if options.Directory == "" {
		return Summary{}, errors.New("release directory is required")
	}
	if err := validateDirectoryEntries(options.Directory, true); err != nil {
		return Summary{}, err
	}
	manifestFile, err := os.Open(filepath.Join(options.Directory, releaseinfo.ManifestAssetName))
	if err != nil {
		return Summary{}, fmt.Errorf("open release manifest: %w", err)
	}
	manifest, parseErr := releaseinfo.ParseManifest(manifestFile)
	closeErr := manifestFile.Close()
	if parseErr != nil {
		return Summary{}, parseErr
	}
	if closeErr != nil {
		return Summary{}, closeErr
	}
	if options.Version != "" && manifest.Version != options.Version {
		return Summary{}, errors.New("release manifest version does not match the expected version")
	}
	if options.Revision != "" && manifest.Revision != options.Revision {
		return Summary{}, errors.New("release manifest revision does not match the expected revision")
	}
	if !slices.Equal(manifest.AgentCapabilities, releaseinfo.RequiredAgentCapabilities) {
		return Summary{}, errors.New("release manifest capabilities do not exactly match this source")
	}
	if len(manifest.Artifacts) != len(ArtifactDefinitions) {
		return Summary{}, errors.New("release manifest artifact set is incomplete")
	}
	for index, definition := range ArtifactDefinitions {
		artifact := manifest.Artifacts[index]
		if artifact.Name != definition.Name || artifact.Component != definition.Component ||
			artifact.OS != definition.OS || artifact.Arch != definition.Arch {
			return Summary{}, fmt.Errorf("release manifest artifact %d does not match %q", index, definition.Name)
		}
		size, digest, err := inspectFile(filepath.Join(options.Directory, definition.Name), definition.Executable)
		if err != nil {
			return Summary{}, fmt.Errorf("inspect release artifact %q: %w", definition.Name, err)
		}
		if size != artifact.Size || digest != artifact.SHA256 {
			return Summary{}, fmt.Errorf("release artifact %q does not match its manifest", definition.Name)
		}
	}
	checksums, err := parseChecksums(filepath.Join(options.Directory, releaseinfo.ChecksumsAssetName))
	if err != nil {
		return Summary{}, err
	}
	wantNames := make([]string, 0, len(ArtifactDefinitions)+1)
	for _, definition := range ArtifactDefinitions {
		wantNames = append(wantNames, definition.Name)
	}
	wantNames = append(wantNames, releaseinfo.ManifestAssetName)
	slices.Sort(wantNames)
	if len(checksums) != len(wantNames) {
		return Summary{}, errors.New("checksums.txt does not cover the exact release file set")
	}
	for index, name := range wantNames {
		if checksums[index].name != name {
			return Summary{}, fmt.Errorf("checksums.txt entry %d does not identify %q", index, name)
		}
		_, digest, err := inspectFile(filepath.Join(options.Directory, name), false)
		if err != nil {
			return Summary{}, fmt.Errorf("inspect checksummed file %q: %w", name, err)
		}
		if checksums[index].digest != digest {
			return Summary{}, fmt.Errorf("release file %q does not match checksums.txt", name)
		}
	}
	return Summary{Version: manifest.Version, Revision: manifest.Revision, ArtifactCount: len(manifest.Artifacts)}, nil
}

func channelForVersion(version string) string {
	if strings.Contains(version, "-rc.") {
		return "rc"
	}
	return "stable"
}

func validateDirectoryEntries(directory string, includeGenerated bool) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read release directory: %w", err)
	}
	want := make(map[string]struct{}, len(ArtifactDefinitions)+2)
	for _, definition := range ArtifactDefinitions {
		want[definition.Name] = struct{}{}
	}
	if includeGenerated {
		want[releaseinfo.ManifestAssetName] = struct{}{}
		want[releaseinfo.ChecksumsAssetName] = struct{}{}
	} else {
		// Permit replacing output from a previous creation attempt.
		want[releaseinfo.ManifestAssetName] = struct{}{}
		want[releaseinfo.ChecksumsAssetName] = struct{}{}
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if _, ok := want[entry.Name()]; !ok {
			return fmt.Errorf("release directory contains unexpected entry %q", entry.Name())
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect release entry %q: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("release entry %q is not a regular file", entry.Name())
		}
		seen[entry.Name()] = struct{}{}
	}
	for name := range want {
		if !includeGenerated && (name == releaseinfo.ManifestAssetName || name == releaseinfo.ChecksumsAssetName) {
			continue
		}
		if _, ok := seen[name]; !ok {
			return fmt.Errorf("release directory is missing %q", name)
		}
	}
	return nil
}

func inspectFile(path string, executable bool) (int64, string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, "", err
	}
	if !info.Mode().IsRegular() {
		return 0, "", errors.New("not a regular file")
	}
	if executable && info.Mode().Perm()&0o111 == 0 {
		return 0, "", errors.New("file is not executable")
	}
	if info.Size() < 1 || info.Size() > maximumArtifactBytes {
		return 0, "", errors.New("file size is outside the supported range")
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(hash, io.LimitReader(file, maximumArtifactBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return 0, "", copyErr
	}
	if closeErr != nil {
		return 0, "", closeErr
	}
	if written != info.Size() {
		return 0, "", errors.New("file changed while it was inspected")
	}
	return written, hex.EncodeToString(hash.Sum(nil)), nil
}

type checksumEntry struct {
	name   string
	digest string
}

func parseChecksums(path string) ([]checksumEntry, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read checksums.txt: %w", err)
	}
	if len(encoded) == 0 || len(encoded) > 64*1024 || !bytes.HasSuffix(encoded, []byte("\n")) {
		return nil, errors.New("checksums.txt has an invalid size or terminator")
	}
	entries := make([]checksumEntry, 0, len(ArtifactDefinitions)+1)
	scanner := bufio.NewScanner(bytes.NewReader(encoded))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 67 || line[64:66] != "  " {
			return nil, errors.New("checksums.txt contains an invalid line")
		}
		digest, name := line[:64], line[66:]
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size || digest != strings.ToLower(digest) ||
			name == "" || filepath.Base(name) != name || strings.ContainsAny(name, "\\/") {
			return nil, errors.New("checksums.txt contains an invalid entry")
		}
		if len(entries) > 0 && strings.Compare(entries[len(entries)-1].name, name) >= 0 {
			return nil, errors.New("checksums.txt entries are not strictly sorted")
		}
		entries = append(entries, checksumEntry{name: name, digest: digest})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan checksums.txt: %w", err)
	}
	return entries, nil
}

func writeFileAtomic(path string, contents []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".release-output-*")
	if err != nil {
		return fmt.Errorf("create temporary release output: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	return errors.Join(syncErr, closeErr)
}
