package session

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

type ResourceUsage struct {
	RSSKB int64
}

func GetResourceUsage(pid int) (*ResourceUsage, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return nil, err
	}
	idx := strings.LastIndex(string(data), ")")
	if idx < 0 {
		return nil, fmt.Errorf("malformed /proc/stat")
	}
	fields := strings.Fields(string(data)[idx+2:])
	if len(fields) < 22 {
		return nil, fmt.Errorf("not enough fields")
	}
	rssPages, _ := strconv.ParseInt(fields[21], 10, 64)
	return &ResourceUsage{RSSKB: rssPages * int64(os.Getpagesize()) / 1024}, nil
}

func GetTmuxPanePID(tmuxSession string) (int, error) {
	out, err := tmuxRun("display-message", "-t", tmuxSession, "-p", "#{pane_pid}")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(out))
}
