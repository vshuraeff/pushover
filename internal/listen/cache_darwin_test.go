//go:build darwin

package listen

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vshuraeff/pushover/internal/notify"
)

func TestHelperNotifierDetection(t *testing.T) {
	helper := new(notify.HelperNotifier)
	if !isHelperNotifier(helper) {
		t.Fatal("helper notifier was not detected")
	}
	if isHelperNotifier(&channelNotifier{}) {
		t.Fatal("non-helper notifier was detected as helper")
	}

	soundDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(soundDir, "bike.mp3"), []byte("fake-mp3-data"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cache := newAssetCache(nil, t.TempDir(), soundDir, helper, testLogger())
	converted := 0
	cache.converter = func(context.Context, string, string) (string, error) {
		converted++
		return filepath.Join(soundDir, "povr-bike.caf"), nil
	}
	got := cache.Sound(context.Background(), "bike")
	if converted != 1 || got.name != "povr-bike.caf" {
		t.Fatalf("converted calls = %d, sound = %#v", converted, got)
	}
}
