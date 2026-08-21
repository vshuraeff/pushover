//go:build darwin

package notify

import (
	"context"
	"fmt"
	"os/exec"
)

// AlerterNotifier delivers notifications through the alerter command.
type AlerterNotifier struct {
	path string
}

// NewAlerterNotifier creates an alerter-backed notifier when alerter is on PATH.
func NewAlerterNotifier() (*AlerterNotifier, error) {
	path, err := exec.LookPath("alerter")
	if err != nil {
		return nil, fmt.Errorf("notify: alerter not found: %w", err)
	}
	return &AlerterNotifier{path: path}, nil
}

// Notify invokes alerter with the fields it supports.
func (a *AlerterNotifier) Notify(ctx context.Context, n Notification) error {
	args := []string{"-title", n.Title, "-message", n.Body}
	if n.IconPath != "" {
		args = append(args, "-appIcon", n.IconPath)
	}
	if n.AttachmentPath != "" {
		args = append(args, "-contentImage", n.AttachmentPath)
	}
	if n.SoundName != "" && !SuppressSoundForLevel(PriorityToLevel(n.Priority)) {
		args = append(args, "-sound", n.SoundName)
	}
	return runCommand(ctx, a.path, args...)
}

// Caps returns alerter's supported notification fields.
func (*AlerterNotifier) Caps() Caps {
	return Caps{Images: true, Sound: true}
}
