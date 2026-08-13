package releaseinfo

import (
	"strings"
	"testing"
)

func TestParseManifestAndSelectAgentArtifact(t *testing.T) {
	manifestJSON := `{
      "schemaVersion":1,
      "version":"0.1.0-rc.1",
      "tag":"v0.1.0-rc.1",
      "channel":"rc",
      "revision":"0123456789abcdef0123456789abcdef01234567",
      "sourceUrl":"https://github.com/ipchronicle/ipchronicle/tree/v0.1.0-rc.1",
      "agentCapabilities":["address-observation-v1","agent-update-v1","complete-probe-v1","configuration-v6","control-v1","network-inventory-v1","sync-wakeup-v1"],
      "artifacts":[
        {"name":"ipchronicle-agent-linux-amd64","component":"agent","os":"linux","arch":"amd64","size":123,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
        {"name":"ipchronicle-agent-linux-arm64","component":"agent","os":"linux","arch":"arm64","size":124,"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
      ]
    }`
	manifest, err := ParseManifest(strings.NewReader(manifestJSON))
	if err != nil {
		t.Fatal(err)
	}
	artifact, ok := manifest.AgentArtifact("arm64")
	if !ok || artifact.Size != 124 || artifact.Name != AgentArtifactName("arm64") {
		t.Fatalf("unexpected Agent artifact: %#v, %v", artifact, ok)
	}
}

func TestManifestRejectsMismatchedAndUnknownData(t *testing.T) {
	base := `{"schemaVersion":1,"version":"0.1.0","tag":"v0.1.0","channel":"stable","revision":"0123456789abcdef0123456789abcdef01234567","sourceUrl":"https://github.com/ipchronicle/ipchronicle/tree/v0.1.0","agentCapabilities":["address-observation-v1","agent-update-v1","complete-probe-v1","configuration-v6","control-v1","network-inventory-v1","sync-wakeup-v1"],"artifacts":[{"name":"ipchronicle-agent-linux-amd64","component":"agent","os":"linux","arch":"amd64","size":1,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},{"name":"ipchronicle-agent-linux-arm64","component":"agent","os":"linux","arch":"arm64","size":1,"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}]}`
	for _, input := range []string{
		strings.Replace(base, `"channel":"stable"`, `"channel":"rc"`, 1),
		strings.Replace(base, `"size":1`, `"size":0`, 1),
		strings.TrimSuffix(base, "}") + `,"unknown":true}`,
	} {
		if _, err := ParseManifest(strings.NewReader(input)); err == nil {
			t.Fatalf("invalid manifest unexpectedly accepted: %s", input)
		}
	}
}

func TestCanonicalVersionAndChannels(t *testing.T) {
	for _, value := range []string{"0.1.0", "0.1.0-rc.1", "1.2.3"} {
		if canonical, err := CanonicalVersion(value); err != nil || canonical != value {
			t.Fatalf("canonical version %q = %q, %v", value, canonical, err)
		}
	}
	for _, value := range []string{"v0.1.0", "0.1", "0.1.0+dirty", "0.1.0-beta.1"} {
		if _, err := CanonicalVersion(value); err == nil {
			t.Fatalf("invalid version %q unexpectedly accepted", value)
		}
	}
	if !IsReleaseCandidate("v0.1.0-rc.2") || IsReleaseCandidate("v0.1.0-beta.2") {
		t.Fatal("release candidate classification is incorrect")
	}
}
