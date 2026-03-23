package discovery

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

func KillProcess(pid int, timeout time.Duration) error {
	proc, err := os.FindProcess(pid)
	if err != nil { return fmt.Errorf("finding process %d: %w", pid, err) }
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("process %d not found: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sending SIGTERM to %d: %w", pid, err)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			proc.Wait() //nolint:errcheck
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		proc.Wait() //nolint:errcheck
		return nil
	}
	time.Sleep(200 * time.Millisecond)
	proc.Wait() //nolint:errcheck
	return nil
}

func Takeover(info ProcessInfo, timeout time.Duration) error {
	if info.PID == 0 { return fmt.Errorf("no PID to take over") }
	return KillProcess(info.PID, timeout)
}
