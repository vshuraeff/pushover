//go:build !darwin

package notify

import "context"

// StubNotifier reports that notifications are unavailable on this platform.
type StubNotifier struct{}

// NewStubNotifier creates a notifier for unsupported platforms.
func NewStubNotifier() (*StubNotifier, error) {
	return nil, ErrNotImplemented
}

// Notify reports that this platform has no notification backend.
func (*StubNotifier) Notify(context.Context, Notification) error {
	return ErrNotImplemented
}

// Caps returns no supported capabilities.
func (*StubNotifier) Caps() Caps {
	return Caps{}
}
