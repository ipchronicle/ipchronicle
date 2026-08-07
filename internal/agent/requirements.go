package agent

import "errors"

var (
	ErrRootRequired = errors.New("the IPChronicle Agent must run as root")
)

func CheckRoot(effectiveUID int) error {
	if effectiveUID != 0 {
		return ErrRootRequired
	}
	return nil
}
