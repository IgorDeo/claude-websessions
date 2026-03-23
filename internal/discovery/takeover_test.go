package discovery_test

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/igor-deoalves/websessions/internal/discovery"
)

func TestKillProcess(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil { t.Fatal(err) }
	pid := cmd.Process.Pid
	err := discovery.KillProcess(pid, 2*time.Second)
	if err != nil { t.Fatalf("kill error: %v", err) }
	proc, err := os.FindProcess(pid)
	if err == nil {
		err = proc.Signal(syscall.Signal(0))
		if err == nil { t.Error("expected process to be dead") }
	}
}

func TestKillProcess_NonExistent(t *testing.T) {
	err := discovery.KillProcess(999999999, 1*time.Second)
	if err == nil { t.Error("expected error for non-existent PID") }
}
