//go:build darwin

package notify

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSelectDarwinDefaultOrder(t *testing.T) {
	binDir := t.TempDir()
	alerterPath := filepath.Join(binDir, "alerter")
	if err := os.WriteFile(alerterPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Run("helper before alerter", func(t *testing.T) {
		home := t.TempDir()
		bundlePath := filepath.Join(home, "Library", "Application Support", "povr", "Povr.app")
		if err := os.MkdirAll(bundlePath, 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		t.Setenv("HOME", home)
		t.Setenv("PATH", binDir)
		t.Setenv(backendEnv, "")

		notifier, err := Select(context.Background())
		if err != nil {
			t.Fatalf("Select() error = %v", err)
		}
		if _, ok := notifier.(*HelperNotifier); !ok {
			t.Fatalf("Select() = %T, want *HelperNotifier", notifier)
		}
	})

	t.Run("alerter before osascript", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("PATH", binDir)
		t.Setenv(backendEnv, "")

		notifier, err := Select(context.Background())
		if err != nil {
			t.Fatalf("Select() error = %v", err)
		}
		if _, ok := notifier.(*AlerterNotifier); !ok {
			t.Fatalf("Select() = %T, want *AlerterNotifier", notifier)
		}
	})

	t.Run("osascript fallback", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("PATH", t.TempDir())
		t.Setenv(backendEnv, "")

		notifier, err := Select(context.Background())
		if err != nil {
			t.Fatalf("Select() error = %v", err)
		}
		if _, ok := notifier.(*AppleScriptNotifier); !ok {
			t.Fatalf("Select() = %T, want *AppleScriptNotifier", notifier)
		}
	})
}
