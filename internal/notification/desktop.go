package notification

import (
	"fmt"
	"os/exec"
	"runtime"
)

// DesktopSink sends native OS desktop notifications.
// Uses notify-send on Linux, osascript on macOS.
type DesktopSink struct{}

func NewDesktopSink() *DesktopSink { return &DesktopSink{} }

func (d *DesktopSink) Send(event SessionEvent) error {
	title := "websessions"
	body := fmt.Sprintf("%s: %s", event.SessionID, string(event.Type))
	if event.Message != "" {
		body = fmt.Sprintf("%s: %s", event.SessionID, event.Message)
	}

	switch runtime.GOOS {
	case "linux":
		return d.sendLinux(title, body, event.Type)
	case "darwin":
		return d.sendDarwin(title, body)
	default:
		return nil
	}
}

func (d *DesktopSink) sendLinux(title, body string, eventType EventType) error {
	urgency := "normal"
	icon := "dialog-information"
	switch eventType {
	case EventErrored:
		urgency = "critical"
		icon = "dialog-error"
	case EventWaiting:
		icon = "dialog-warning"
	}

	cmd := exec.Command("notify-send",
		"--app-name=websessions",
		"--urgency="+urgency,
		"--icon="+icon,
		title, body,
	)
	return cmd.Run()
}

func (d *DesktopSink) sendDarwin(title, body string) error {
	script := fmt.Sprintf(`display notification %q with title %q`, body, title)
	return exec.Command("osascript", "-e", script).Run()
}
