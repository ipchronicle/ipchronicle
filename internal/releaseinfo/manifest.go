package releaseinfo

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"golang.org/x/mod/semver"
)

const (
	ManifestSchemaVersion = 1
	ManifestAssetName     = "release-manifest.json"
	ChecksumsAssetName    = "checksums.txt"
	SourceRepository      = "https://github.com/ipchronicle/ipchronicle"
	maxManifestBytes      = 256 * 1024
)

var RequiredAgentCapabilities = []string{
	"address-observation-v1",
	"agent-update-v1",
	"complete-probe-v1",
	"configuration-v6",
	"control-v1",
	"network-inventory-v1",
	"sync-wakeup-v1",
}

type Manifest struct {
	SchemaVersion     int        `json:"schemaVersion"`
	Version           string     `json:"version"`
	Tag               string     `json:"tag"`
	Channel           string     `json:"channel"`
	Revision          string     `json:"revision"`
	SourceURL         string     `json:"sourceUrl"`
	AgentCapabilities []string   `json:"agentCapabilities"`
	Artifacts         []Artifact `json:"artifacts"`
}

type Artifact struct {
	Name      string `json:"name"`
	Component string `json:"component"`
	OS        string `json:"os,omitempty"`
	Arch      string `json:"arch,omitempty"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
}

type BinaryInfo struct {
	Version            string   `json:"version"`
	Revision           string   `json:"revision"`
	Component          string   `json:"component"`
	OS                 string   `json:"os"`
	Arch               string   `json:"arch"`
	Capabilities       []string `json:"capabilities,omitempty"`
	StateSchemaVersion int      `json:"stateSchemaVersion,omitempty"`
}

func ParseBinaryInfo(input io.Reader) (BinaryInfo, error) {
	encoded, err := io.ReadAll(io.LimitReader(input, 64*1024+1))
	if err != nil {
		return BinaryInfo{}, fmt.Errorf("read binary metadata: %w", err)
	}
	if len(encoded) > 64*1024 {
		return BinaryInfo{}, errors.New("binary metadata exceeds 64 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var info BinaryInfo
	if err := decoder.Decode(&info); err != nil {
		return BinaryInfo{}, fmt.Errorf("decode binary metadata: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return BinaryInfo{}, errors.New("binary metadata contains multiple JSON values")
		}
		return BinaryInfo{}, fmt.Errorf("decode binary metadata suffix: %w", err)
	}
	return info, nil
}

func (info BinaryInfo) ValidateAgent(version, revision, architecture string) error {
	canonical, err := CanonicalVersion(info.Version)
	if err != nil || canonical != version || info.Revision != revision || !validRevision(info.Revision) {
		return errors.New("Agent binary version or revision does not match its release")
	}
	if info.Component != "agent" || info.OS != "linux" || info.Arch != architecture ||
		(architecture != "amd64" && architecture != "arm64") || info.StateSchemaVersion < 1 {
		return errors.New("Agent binary platform metadata is invalid")
	}
	if err := validateCapabilities(info.Capabilities); err != nil {
		return err
	}
	return nil
}

func ParseManifest(input io.Reader) (Manifest, error) {
	encoded, err := io.ReadAll(io.LimitReader(input, maxManifestBytes+1))
	if err != nil {
		return Manifest{}, fmt.Errorf("read release manifest: %w", err)
	}
	if len(encoded) > maxManifestBytes {
		return Manifest{}, errors.New("release manifest exceeds 256 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Manifest{}, errors.New("release manifest contains multiple JSON values")
		}
		return Manifest{}, fmt.Errorf("decode release manifest suffix: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (manifest Manifest) Validate() error {
	if manifest.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("unsupported release manifest schema version %d", manifest.SchemaVersion)
	}
	canonical, err := CanonicalVersion(manifest.Version)
	if err != nil || canonical != manifest.Version || manifest.Tag != "v"+manifest.Version {
		return errors.New("release manifest version and tag do not match")
	}
	wantChannel := "stable"
	if semver.Prerelease(manifest.Tag) != "" {
		wantChannel = "rc"
		if !IsReleaseCandidate(manifest.Tag) {
			return errors.New("release manifest contains an unsupported prerelease")
		}
	}
	if manifest.Channel != wantChannel {
		return errors.New("release manifest channel does not match its version")
	}
	if !validRevision(manifest.Revision) {
		return errors.New("release manifest revision must be a lowercase 40-character Git commit")
	}
	if manifest.SourceURL != SourceRepository+"/tree/"+manifest.Tag {
		return errors.New("release manifest source URL does not identify the official tagged source")
	}
	if err := validateCapabilities(manifest.AgentCapabilities); err != nil {
		return err
	}
	if len(manifest.Artifacts) == 0 || len(manifest.Artifacts) > 64 {
		return errors.New("release manifest artifact count is outside the supported range")
	}
	seen := make(map[string]struct{}, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		if err := artifact.Validate(); err != nil {
			return err
		}
		if _, exists := seen[artifact.Name]; exists {
			return fmt.Errorf("release manifest repeats artifact %q", artifact.Name)
		}
		seen[artifact.Name] = struct{}{}
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if _, ok := manifest.AgentArtifact(arch); !ok {
			return fmt.Errorf("release manifest is missing the linux/%s Agent artifact", arch)
		}
	}
	return nil
}

func (artifact Artifact) Validate() error {
	if artifact.Name == "" || strings.ContainsAny(artifact.Name, "/\\") || artifact.Size < 1 {
		return errors.New("release manifest contains an invalid artifact identity")
	}
	if artifact.Component == "agent" {
		if artifact.OS != "linux" || (artifact.Arch != "amd64" && artifact.Arch != "arm64") ||
			artifact.Name != AgentArtifactName(artifact.Arch) {
			return fmt.Errorf("release manifest contains an invalid Agent artifact %q", artifact.Name)
		}
	} else if artifact.OS != "" || artifact.Arch != "" {
		return fmt.Errorf("non-Agent artifact %q declares a platform", artifact.Name)
	}
	digest, err := hex.DecodeString(artifact.SHA256)
	if err != nil || len(digest) != 32 || artifact.SHA256 != strings.ToLower(artifact.SHA256) {
		return fmt.Errorf("release artifact %q has an invalid SHA-256 digest", artifact.Name)
	}
	return nil
}

func (manifest Manifest) AgentArtifact(arch string) (Artifact, bool) {
	for _, artifact := range manifest.Artifacts {
		if artifact.Component == "agent" && artifact.OS == "linux" && artifact.Arch == arch {
			return artifact, true
		}
	}
	return Artifact{}, false
}

func AgentArtifactName(arch string) string {
	return "ipchronicle-agent-linux-" + arch
}

func CanonicalVersion(value string) (string, error) {
	if strings.HasPrefix(value, "v") || strings.TrimSpace(value) != value || value == "" {
		return "", errors.New("product version must omit the v tag prefix")
	}
	tag := "v" + value
	if !semver.IsValid(tag) || semver.Build(tag) != "" {
		return "", errors.New("product version must be a canonical semantic version without build metadata")
	}
	if semver.Prerelease(tag) != "" && !IsReleaseCandidate(tag) {
		return "", errors.New("product prerelease must use the rc.N channel")
	}
	canonical := strings.TrimPrefix(semver.Canonical(tag), "v")
	if canonical != value {
		return "", errors.New("product version is not canonical")
	}
	return canonical, nil
}

func IsReleaseCandidate(tag string) bool {
	prerelease := semver.Prerelease(tag)
	if !strings.HasPrefix(prerelease, "-rc.") || len(prerelease) <= len("-rc.") {
		return false
	}
	for _, character := range prerelease[len("-rc."):] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func SameMajor(left, right string) bool {
	return semver.IsValid(left) && semver.IsValid(right) && semver.Major(left) == semver.Major(right)
}

func validateCapabilities(capabilities []string) error {
	if len(capabilities) == 0 || len(capabilities) > 64 || !slices.IsSorted(capabilities) {
		return errors.New("release manifest Agent capabilities must be a non-empty sorted list")
	}
	for index, capability := range capabilities {
		if capability == "" || len(capability) > 64 || (index > 0 && capability == capabilities[index-1]) {
			return errors.New("release manifest contains an invalid Agent capability")
		}
	}
	for _, required := range RequiredAgentCapabilities {
		if !slices.Contains(capabilities, required) {
			return fmt.Errorf("release manifest Agent capabilities omit %q", required)
		}
	}
	return nil
}

func validRevision(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 20 && value == strings.ToLower(value)
}
