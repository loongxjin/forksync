package notify

import (
	"fmt"
	"os/exec"
)

// Notifier sends macOS system notifications.
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
	script := fmt.Sprintf(`display notification %q with title %q`, message, title)
	exec.Command("osascript", "-e", script).Run()
}
