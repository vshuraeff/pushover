//go:build !darwin

package notify

import (
	"context"
	"fmt"
)

func selectPlatformOverride(_ context.Context, backend string) (Notifier, error) {
	switch backend {
	case "helper", "alerter", "osascript":
		return nil, fmt.Errorf("notify: backend %q is only available on darwin", backend)
	default:
		return nil, fmt.Errorf("notify: unknown backend %q (expected helper, alerter, osascript, or file:<path>)", backend)
	}
}

func selectPlatformDefault(_ context.Context) (Notifier, error) {
	return NewStubNotifier()
}
