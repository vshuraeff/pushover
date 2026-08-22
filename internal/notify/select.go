package notify

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const backendEnv = "PUSHOVER_NOTIFY_BACKEND"

// Select chooses a notifier from the environment override or platform defaults.
func Select(ctx context.Context) (Notifier, error) {
	return selectNotifier(ctx, os.Getenv)
}

func selectNotifier(ctx context.Context, getenv func(string) string) (Notifier, error) {
	if backend := getenv(backendEnv); backend != "" {
		if path, ok := strings.CutPrefix(backend, "file:"); ok {
			notifier, err := newFileNotifier(path, getenv(consumeFilesEnv) == "1")
			if err != nil {
				return nil, fmt.Errorf("notify: create file backend: %w", err)
			}
			return notifier, nil
		}
		return selectPlatformOverride(ctx, backend)
	}

	return selectPlatformDefault(ctx)
}
