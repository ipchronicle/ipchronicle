package releasetool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyAgentBinaryRejectsUnsupportedAndNonELFInputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(path, []byte("not an ELF binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAgentBinary(path, "riscv64"); err == nil {
		t.Fatal("unsupported Agent architecture unexpectedly accepted")
	}
	if _, err := VerifyAgentBinary(path, "amd64"); err == nil {
		t.Fatal("non-ELF Agent unexpectedly accepted")
	}
}
