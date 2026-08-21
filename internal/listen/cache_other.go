//go:build !darwin

package listen

import (
	"context"

	"github.com/vshuraeff/pushover/internal/notify"
)

func convertCachedSound(_ context.Context, mp3Path, _ string) (string, error) {
	return mp3Path, nil
}

func isHelperNotifier(notify.Notifier) bool {
	return false
}
