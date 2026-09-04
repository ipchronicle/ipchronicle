package releasetool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testRevision = "0123456789abcdef0123456789abcdef01234567"

const testReadinessReport = `# 发布就绪报告

状态：发布前验证中

## 验证结果

<!-- release-evidence:start -->
等待验证。
<!-- release-evidence:end -->
`

func TestCreateAndVerifyRelease(t *testing.T) {
	directory := preparePayload(t)
	summary, err := Create(CreateOptions{Directory: directory, Version: "0.1.0-rc.1", Revision: testRevision})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Version != "0.1.0-rc.1" || summary.Revision != testRevision ||
		summary.ArtifactCount != len(ArtifactDefinitions) {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	checksums, err := os.ReadFile(filepath.Join(directory, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(checksums), "\n"), "\n")
	if len(lines) != len(ArtifactDefinitions)+1 {
		t.Fatalf("unexpected checksum entry count: %d", len(lines))
	}
	if _, err := Verify(VerifyOptions{Directory: directory, Version: "0.1.0-rc.1", Revision: testRevision}); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseArtifactNamesAreUploadSafe(t *testing.T) {
	for _, definition := range ArtifactDefinitions {
		if strings.HasPrefix(definition.Name, ".") || strings.HasSuffix(definition.Name, ".") {
			t.Errorf("release artifact name %q will be rewritten by GitHub", definition.Name)
		}
	}
}

func TestVerifyRejectsTamperingAndUnexpectedFiles(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{
			name: "artifact contents",
			mutate: func(t *testing.T, directory string) {
				writeTestFile(t, filepath.Join(directory, "LICENSE"), "tampered", 0o644)
			},
		},
		{
			name: "unexpected file",
			mutate: func(t *testing.T, directory string) {
				writeTestFile(t, filepath.Join(directory, "unexpected"), "data", 0o644)
			},
		},
		{
			name: "unsorted checksums",
			mutate: func(t *testing.T, directory string) {
				path := filepath.Join(directory, "checksums.txt")
				contents, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				lines := strings.Split(strings.TrimSuffix(string(contents), "\n"), "\n")
				lines[0], lines[1] = lines[1], lines[0]
				writeTestFile(t, path, strings.Join(lines, "\n")+"\n", 0o644)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := preparePayload(t)
			if _, err := Create(CreateOptions{Directory: directory, Version: "0.1.0", Revision: testRevision}); err != nil {
				t.Fatal(err)
			}
			test.mutate(t, directory)
			if _, err := Verify(VerifyOptions{Directory: directory}); err == nil {
				t.Fatal("tampered release unexpectedly passed verification")
			}
		})
	}
}

func TestFinalizeReleaseReport(t *testing.T) {
	directory := preparePayload(t)
	writeTestFile(t, filepath.Join(directory, releaseReadinessName), testReadinessReport, 0o644)
	if _, err := Create(CreateOptions{
		Directory: directory, Version: "0.1.0-rc.1", Revision: testRevision,
	}); err != nil {
		t.Fatal(err)
	}

	summary, err := Finalize(FinalizeOptions{
		Directory:      directory,
		Version:        "0.1.0-rc.1",
		Revision:       testRevision,
		CIRunURL:       "https://github.com/ipchronicle/ipchronicle/actions/runs/123",
		RCRunURL:       "https://github.com/ipchronicle/ipchronicle/actions/runs/456",
		ValidationDate: "2026-08-11",
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Version != "0.1.0-rc.1" || summary.Revision != testRevision {
		t.Fatalf("unexpected finalized summary: %#v", summary)
	}
	report, err := os.ReadFile(filepath.Join(directory, releaseReadinessName))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		readyReportStatus,
		testRevision,
		"https://github.com/ipchronicle/ipchronicle/actions/runs/123",
		"https://github.com/ipchronicle/ipchronicle/actions/runs/456",
		"2026-08-11",
		"分级发布门禁：**通过**",
	} {
		if !strings.Contains(string(report), expected) {
			t.Fatalf("final report does not contain %q", expected)
		}
	}
	if strings.Contains(string(report), pendingReportStatus) {
		t.Fatal("final report still has its pending status")
	}
	if _, err := Verify(VerifyOptions{
		Directory: directory, Version: "0.1.0-rc.1", Revision: testRevision,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := Finalize(FinalizeOptions{
		Directory: directory, CIRunURL: "https://github.com/ipchronicle/ipchronicle/actions/runs/123",
		RCRunURL: "https://github.com/ipchronicle/ipchronicle/actions/runs/456", ValidationDate: "2026-08-11",
	}); err == nil {
		t.Fatal("already-finalized report unexpectedly accepted")
	}
}

func TestFinalizeRejectsInvalidEvidence(t *testing.T) {
	for _, test := range []struct {
		name           string
		ciRunURL       string
		rcRunURL       string
		validationDate string
	}{
		{
			name: "foreign CI URL", ciRunURL: "https://github.com/other/project/actions/runs/123",
			rcRunURL: "https://github.com/ipchronicle/ipchronicle/actions/runs/456", validationDate: "2026-08-11",
		},
		{
			name: "same runs", ciRunURL: "https://github.com/ipchronicle/ipchronicle/actions/runs/123",
			rcRunURL: "https://github.com/ipchronicle/ipchronicle/actions/runs/123", validationDate: "2026-08-11",
		},
		{
			name: "invalid date", ciRunURL: "https://github.com/ipchronicle/ipchronicle/actions/runs/123",
			rcRunURL: "https://github.com/ipchronicle/ipchronicle/actions/runs/456", validationDate: "08/11/2026",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := preparePayload(t)
			writeTestFile(t, filepath.Join(directory, releaseReadinessName), testReadinessReport, 0o644)
			if _, err := Create(CreateOptions{
				Directory: directory, Version: "0.1.0-rc.1", Revision: testRevision,
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := Finalize(FinalizeOptions{
				Directory: directory, CIRunURL: test.ciRunURL, RCRunURL: test.rcRunURL,
				ValidationDate: test.validationDate,
			}); err == nil {
				t.Fatal("invalid release evidence unexpectedly accepted")
			}
		})
	}
}

func TestCreateRejectsMissingNonExecutableAndSymlinkArtifacts(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		directory := preparePayload(t)
		if err := os.Remove(filepath.Join(directory, "LICENSE")); err != nil {
			t.Fatal(err)
		}
		if _, err := Create(CreateOptions{Directory: directory, Version: "0.1.0", Revision: testRevision}); err == nil {
			t.Fatal("missing artifact unexpectedly accepted")
		}
	})
	t.Run("not executable", func(t *testing.T) {
		directory := preparePayload(t)
		if err := os.Chmod(filepath.Join(directory, "install-agent.sh"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Create(CreateOptions{Directory: directory, Version: "0.1.0", Revision: testRevision}); err == nil {
			t.Fatal("non-executable installer unexpectedly accepted")
		}
	})
	t.Run("unexpectedly executable", func(t *testing.T) {
		directory := preparePayload(t)
		if err := os.Chmod(filepath.Join(directory, "ipchronicle-agent-linux-amd64.cdx.json"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := Create(CreateOptions{Directory: directory, Version: "0.1.0", Revision: testRevision}); err == nil {
			t.Fatal("executable SBOM unexpectedly accepted")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		directory := preparePayload(t)
		path := filepath.Join(directory, "LICENSE")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("THIRD_PARTY_NOTICES.md", path); err != nil {
			t.Fatal(err)
		}
		if _, err := Create(CreateOptions{Directory: directory, Version: "0.1.0", Revision: testRevision}); err == nil {
			t.Fatal("symlink artifact unexpectedly accepted")
		}
	})
}

func preparePayload(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	for _, definition := range ArtifactDefinitions {
		mode := os.FileMode(0o644)
		if definition.Executable {
			mode = 0o755
		}
		writeTestFile(t, filepath.Join(directory, definition.Name), "payload for "+definition.Name+"\n", mode)
	}
	return directory
}

func writeTestFile(t *testing.T, path, contents string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}
