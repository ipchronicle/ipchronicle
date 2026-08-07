package agent

import "errors"

var (
	ErrRootRequired       = errors.New("the IPChronicle Agent must run as root")
	ErrRuntimeUnavailable = errors.New("the Agent control plane is not available in Phase 0")
)

func CheckRoot(effectiveUID int) error {
	if effectiveUID != 0 {
		return ErrRootRequired
	}
	return nil
}
