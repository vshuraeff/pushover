//go:build darwin

package notify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type soundCommandRunner func(ctx context.Context, name string, args ...string) error

var execSoundCommand soundCommandRunner = func(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

// convertcachedsound converts an audio file into a notification-compatible linear pcm caf file.
func ConvertCachedSound(ctx context.Context, mp3Path, key string) (string, error) {
	home, err := userHomeDir()
	if err != nil {
		return "", fmt.Errorf("notify: locate home directory: %w", err)
	}
	return convertCachedSound(ctx, mp3Path, key, filepath.Join(home, "Library", "Sounds"), execSoundCommand)
}

func convertCachedSound(
	ctx context.Context,
	mp3Path string,
	key string,
	soundsDir string,
	run soundCommandRunner,
) (string, error) {
	if key == "" || filepath.Base(key) != key || key == "." {
		return "", fmt.Errorf("notify: invalid sound cache key %q", key)
	}

	target := filepath.Join(soundsDir, "povr-"+key+".caf")
	if _, err := os.Stat(target); err == nil {
		return target, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("notify: inspect cached sound %q: %w", target, err)
	}

	if err := os.MkdirAll(soundsDir, 0o755); err != nil {
		return "", fmt.Errorf("notify: create sounds directory %q: %w", soundsDir, err)
	}

	// usernotifications accepts linear pcm audio in caf containers for custom sounds.
	args := []string{"-f", "caff", "-d", "LEI16", mp3Path, target}
	if err := run(ctx, "/usr/bin/afconvert", args...); err != nil {
		return "", fmt.Errorf("notify: convert sound %q: %w", mp3Path, err)
	}
	return target, nil
}
