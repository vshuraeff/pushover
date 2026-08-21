//go:build darwin

package notify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

var userHomeDir = os.UserHomeDir

func selectPlatformOverride(_ context.Context, backend string) (Notifier, error) {
	switch backend {
	case "helper":
		bundlePath, err := helperBundlePath()
		if err != nil {
			return nil, fmt.Errorf("notify: locate helper bundle: %w", err)
		}
		return NewHelperNotifier(bundlePath)
	case "alerter":
		notifier, err := NewAlerterNotifier()
		if err != nil {
			return nil, fmt.Errorf("notify: select alerter backend: %w", err)
		}
		return notifier, nil
	case "osascript":
		return NewAppleScriptNotifier(), nil
	default:
		return nil, fmt.Errorf("notify: unknown backend %q (expected helper, alerter, osascript, or file:<path>)", backend)
	}
}

func selectPlatformDefault(_ context.Context) (Notifier, error) {
	if bundlePath, ok := installedHelperBundlePath(); ok {
		return NewHelperNotifier(bundlePath)
	}
	if notifier, err := NewAlerterNotifier(); err == nil {
		return notifier, nil
	}
	return NewAppleScriptNotifier(), nil
}

func helperBundlePath() (string, error) {
	home, err := userHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "povr", "Povr.app"), nil
}

func installedHelperBundlePath() (string, bool) {
	bundlePath, err := helperBundlePath()
	if err != nil {
		return "", false
	}
	info, err := os.Stat(bundlePath)
	return bundlePath, err == nil && info.IsDir()
}
