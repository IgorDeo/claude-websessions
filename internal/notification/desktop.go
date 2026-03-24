package notification

import (
	"fmt"
	"os/exec"
	"runtime"
)

// GuiNotifyFunc is a function that sends native GUI notifications.
// Set by the GUI layer when running in --gui mode.
type GuiNotifyFunc func(title, body, id string)

// DesktopSink sends native OS desktop notifications.
// When guiNotify is set, uses GTK/Cocoa notifications (click to focus).
// Otherwise falls back to notify-send (Linux) or osascript (macOS).
type DesktopSink struct {
	guiNotify GuiNotifyFunc
}

func NewDesktopSink(guiNotify GuiNotifyFunc) *DesktopSink {
	return &DesktopSink{guiNotify: guiNotify}
}

func (d *DesktopSink) Send(event SessionEvent) error {
	title := "websessions"
	body := fmt.Sprintf("%s: %s", event.SessionID, string(event.Type))
	if event.Message != "" {
		body = fmt.Sprintf("%s: %s", event.SessionID, event.Message)
	}
	id := "ws-" + event.SessionID + "-" + string(event.Type)

	if d.guiNotify != nil {
		d.guiNotify(title, body, id)
		return nil
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
