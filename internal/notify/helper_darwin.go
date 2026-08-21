//go:build darwin

package notify

import (
	"context"
	"errors"
)

// HelperRequest is a single line-delimited json request sent to the helper.
type HelperRequest struct {
	ID         string       `json:"id"`
	Title      string       `json:"title"`
	Subtitle   string       `json:"subtitle,omitempty"`
	Body       string       `json:"body"`
	Icon       string       `json:"icon,omitempty"`
	Attachment string       `json:"attachment,omitempty"`
	Sound      *HelperSound `json:"sound,omitempty"`
	Level      string       `json:"level"`
	Thread     string       `json:"thread,omitempty"`
	URL        string       `json:"url,omitempty"`
}

// HelperSound is either a named system sound or a sound file path.
type HelperSound struct {
	Name string `json:"name,omitempty"`
	File string `json:"file,omitempty"`
}

// HelperEvent is a single line-delimited json event received from the helper.
type HelperEvent struct {
	Event string `json:"event"`
	ID    string `json:"id,omitempty"`
	Error string `json:"error,omitempty"`
	URL   string `json:"url,omitempty"`
}

// HelperNotifier is the future client for the Povr helper bundle.
type HelperNotifier struct {
	bundlePath string
}

// NewHelperNotifier creates a notifier for the helper bundle.
func NewHelperNotifier(bundlePath string) (*HelperNotifier, error) {
	if bundlePath == "" {
		return nil, errors.New("notify: helper bundle path is empty")
	}
	return &HelperNotifier{bundlePath: bundlePath}, nil
}

// Notify prepares a helper request until helper ipc is implemented.
func (h *HelperNotifier) Notify(_ context.Context, n Notification) error {
	_ = h.request(n)
	return errors.New("notify: helper backend not yet wired up")
}

func (*HelperNotifier) Caps() Caps {
	return Caps{Images: true, Sound: true, Levels: true, Actions: true}
}

func (h *HelperNotifier) request(n Notification) HelperRequest {
	level := PriorityToLevel(n.Priority)
	request := HelperRequest{
		ID:         n.ID,
		Title:      n.Title,
		Body:       n.Body,
		Icon:       n.IconPath,
		Attachment: n.AttachmentPath,
		Level:      string(level),
		Thread:     n.ThreadID,
		URL:        n.URL,
	}
	if !SuppressSoundForLevel(level) && (n.SoundName != "" || n.SoundPath != "") {
		request.Sound = &HelperSound{Name: n.SoundName, File: n.SoundPath}
	}
	return request
}
