//go:build !darwin

package notify

import (
	"context"
	"errors"
	"testing"
)

func TestStubNotifierReturnsNotImplemented(t *testing.T) {
	err := NewStubNotifier().Notify(context.Background(), Notification{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Notify() error = %v, want ErrNotImplemented", err)
	}
}
