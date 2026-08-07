package agent

import (
	"errors"
	"testing"
)

func TestCheckRoot(t *testing.T) {
	if err := CheckRoot(0); err != nil {
		t.Fatalf("root should be accepted: %v", err)
	}
	if err := CheckRoot(1000); !errors.Is(err, ErrRootRequired) {
		t.Fatalf("non-root error = %v, want %v", err, ErrRootRequired)
	}
}
