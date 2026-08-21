//go:build darwin

package listen

import (
	"context"

	"github.com/vshuraeff/pushover/internal/notify"
)

func convertCachedSound(ctx context.Context, mp3Path, key string) (string, error) {
	return notify.ConvertCachedSound(ctx, mp3Path, key)
}
