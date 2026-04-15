package notify

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Notifier sends system notifications.
type Notifier struct {
	enabled bool
}

// NewNotifier creates a new Notifier.
func NewNotifier(enabled bool) *Notifier {
	return &Notifier{enabled: enabled}
}

// NotifySyncSuccess sends a notification for a successful sync.
func (n *Notifier) NotifySyncSuccess(repoName string, commitsPulled int) {
	if !n.enabled {
		return
	}
	msg := fmt.Sprintf("Successfully synced %s (%d commits pulled)", repoName, commitsPulled)
	n.send("ForkSync", msg)
}

// NotifyConflict sends a notification for sync conflicts.
func (n *Notifier) NotifyConflict(repoName string, conflictCount int) {
	if !n.enabled {
		return
	}
	msg := fmt.Sprintf("Conflicts detected in %s (%d files)", repoName, conflictCount)
	n.send("ForkSync - Conflict", msg)
}

// NotifyError sends a notification for sync errors.
func (n *Notifier) NotifyError(repoName string, errMsg string) {
	if !n.enabled {
		return
	}
	msg := fmt.Sprintf("Error syncing %s: %s", repoName, errMsg)
	n.send("ForkSync - Error", msg)
}

func (n *Notifier) send(title, message string) {
	switch runtime.GOOS {
	case "windows":
		// Use PowerShell toast notification on Windows
		script := fmt.Sprintf(
			"[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null; "+
				"[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom, ContentType = WindowsRuntime] | Out-Null; "+
				"$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02); "+
				"$textNodes = $template.GetElementsByTagName('text'); "+
				"$textNodes.Item(0).AppendChild($template.CreateTextNode(%q)) | Out-Null; "+
				"$textNodes.Item(1).AppendChild($template.CreateTextNode(%q)) | Out-Null; "+
				"$toast = [Windows.UI.Notifications.ToastNotification]::new($template); "+
				"[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('ForkSync').Show($toast)",
			title, message,
		)
		exec.Command("powershell", "-NoProfile", "-Command", script).Run()
	case "darwin":
		// macOS: use osascript
		exec.Command("osascript", "-e", fmt.Sprintf(`display notification %q with title %q`, message, title)).Run()
	case "linux":
		// Linux: use notify-send
		exec.Command("notify-send", escapeNotifySend(title), escapeNotifySend(message)).Run()
	}
}

// escapeNotifySend escapes special shell characters for notify-send arguments.
func escapeNotifySend(s string) string {
	return strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(s)
}
