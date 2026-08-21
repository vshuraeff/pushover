package notify

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSelectBackendOverride(t *testing.T) {
	home := t.TempDir()
	binDir := t.TempDir()
	alerterPath := filepath.Join(binDir, "alerter")
	if err := os.WriteFile(alerterPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir)

	filePath := filepath.Join(t.TempDir(), "notifications.jsonl")
	tests := []struct {
		name     string
		backend  string
		wantErr  bool
		wantCaps Caps
	}{
		{name: "helper", backend: "helper", wantErr: runtime.GOOS != "darwin", wantCaps: Caps{Images: true, Sound: true, Levels: true, Actions: true}},
		{name: "alerter", backend: "alerter", wantErr: runtime.GOOS != "darwin", wantCaps: Caps{Images: true, Sound: true}},
		{name: "osascript", backend: "osascript", wantErr: runtime.GOOS != "darwin", wantCaps: Caps{Sound: true}},
		{name: "file", backend: "file:" + filePath, wantCaps: Caps{Images: true, Sound: true, Levels: true}},
		{name: "invalid", backend: "unknown", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(backendEnv, test.backend)
			notifier, err := Select(context.Background())
			if test.wantErr {
				if err == nil {
					t.Fatal("Select() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if got := notifier.Caps(); got != test.wantCaps {
				t.Fatalf("Select().Caps() = %#v, want %#v", got, test.wantCaps)
			}
			if fileNotifier, ok := notifier.(*FileNotifier); ok {
				t.Cleanup(func() {
					if err := fileNotifier.Close(); err != nil {
						t.Errorf("Close() error = %v", err)
					}
				})
			}
		})
	}
}
