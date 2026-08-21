//go:build !darwin

package notify

import (
	"context"
	"errors"
	"testing"
)

func TestStubNotifierReturnsNotImplemented(t *testing.T) {
	notifier, err := NewStubNotifier()
	if notifier != nil {
		t.Fatalf("NewStubNotifier() notifier = %T, want nil", notifier)
	}
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("NewStubNotifier() error = %v, want ErrNotImplemented", err)
	}

	t.Setenv(backendEnv, "")
	selected, err := Select(context.Background())
	if selected != nil {
		t.Fatalf("Select() notifier = %T, want nil", selected)
	}
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Select() error = %v, want ErrNotImplemented", err)
	}
}
