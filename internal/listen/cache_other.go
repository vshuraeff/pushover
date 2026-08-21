//go:build !darwin

package listen

import "context"

func convertCachedSound(_ context.Context, mp3Path, _ string) (string, error) {
	return mp3Path, nil
}
