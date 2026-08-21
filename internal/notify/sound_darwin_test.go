//go:build darwin

package notify

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConvertCachedSoundSkipsExistingTarget(t *testing.T) {
	soundsDir := t.TempDir()
	target := filepath.Join(soundsDir, "povr-cached.caf")
	if err := os.WriteFile(target, []byte("caf"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	called := false
	got, err := convertCachedSound(context.Background(), "source.mp3", "cached", soundsDir, func(context.Context, string, ...string) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("convertCachedSound() error = %v", err)
	}
	if got != target {
		t.Fatalf("convertCachedSound() = %q, want %q", got, target)
	}
	if called {
		t.Fatal("convertCachedSound() invoked runner for existing target")
	}
}

func TestConvertCachedSoundCommand(t *testing.T) {
	soundsDir := filepath.Join(t.TempDir(), "Sounds")
	var gotName string
	var gotArgs []string
	runner := func(_ context.Context, name string, args ...string) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return nil
	}

	got, err := convertCachedSound(context.Background(), "source.mp3", "alarm", soundsDir, runner)
	if err != nil {
		t.Fatalf("convertCachedSound() error = %v", err)
	}
	wantTarget := filepath.Join(soundsDir, "povr-alarm.caf")
	if got != wantTarget {
		t.Fatalf("convertCachedSound() = %q, want %q", got, wantTarget)
	}
	if gotName != "/usr/bin/afconvert" {
		t.Fatalf("runner name = %q, want /usr/bin/afconvert", gotName)
	}
	wantArgs := []string{"-f", "caff", "-d", "LEI16", "source.mp3", wantTarget}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("runner args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestConvertCachedSoundPropagatesRunnerError(t *testing.T) {
	wantErr := errors.New("conversion failed")
	_, err := convertCachedSound(context.Background(), "source.mp3", "alarm", t.TempDir(), func(context.Context, string, ...string) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("convertCachedSound() error = %v, want wrapped %v", err, wantErr)
	}
}
