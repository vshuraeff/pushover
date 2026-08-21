// package notify provides desktop-notification backend abstractions.
package notify

import (
	"context"
	"errors"
)

// notification describes a single desktop notification to deliver.
type Notification struct {
	ID             string
	Title          string
	Body           string
	AppName        string
	IconPath       string
	AttachmentPath string
	SoundName      string
	SoundPath      string
	Priority       int
	URL            string
	URLTitle       string
	ThreadID       string
}

// caps describes what a backend is capable of.
type Caps struct {
	Images  bool
	Sound   bool
	Levels  bool
	Actions bool
}

// notifier delivers notifications through some os-specific or fallback mechanism.
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
	Caps() Caps
}

// ErrNotImplemented indicates that notification delivery is unavailable on this platform.
var ErrNotImplemented = errors.New("notify: backend not implemented on this platform")

// level is the delivery urgency level used by notification backends.
type Level string

const (
	LevelPassive       Level = "passive"
	LevelActive        Level = "active"
	LevelTimeSensitive Level = "timeSensitive"
)

// PriorityToLevel maps a pushover priority to a notification level.
func PriorityToLevel(priority int) Level {
	switch {
	case priority <= -1:
		return LevelPassive
	case priority == 0:
		return LevelActive
	default:
		return LevelTimeSensitive
	}
}

// SuppressSoundForLevel reports whether sound should be suppressed for a level.
func SuppressSoundForLevel(level Level) bool {
	return level == LevelPassive
}
