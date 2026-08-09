package notifications

import (
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const signalEventNotifySignal = 0

func workerMemoryFile(name string) (*os.File, error) {
	descriptor, err := unix.MemfdCreate(name, unix.MFD_CLOEXEC)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(descriptor), name), nil
}

// linuxSignalEvent matches the 64-bit Linux sigevent layout used on the
// supported Center architectures.
type linuxSignalEvent struct {
	value  uint64
	signal int32
	notify int32
	data   [48]byte
}

func armWorkerKillTimer(timeout time.Duration) error {
	event := linuxSignalEvent{signal: int32(unix.SIGKILL), notify: signalEventNotifySignal}
	var timerID int32
	if _, _, errno := unix.Syscall(
		unix.SYS_TIMER_CREATE,
		uintptr(unix.CLOCK_MONOTONIC),
		uintptr(unsafe.Pointer(&event)),
		uintptr(unsafe.Pointer(&timerID)),
	); errno != 0 {
		return errno
	}
	specification := unix.ItimerSpec{Value: unix.NsecToTimespec(timeout.Nanoseconds())}
	if _, _, errno := unix.Syscall6(
		unix.SYS_TIMER_SETTIME,
		uintptr(timerID),
		0,
		uintptr(unsafe.Pointer(&specification)),
		0,
		0,
		0,
	); errno != 0 {
		return errno
	}
	return nil
}
