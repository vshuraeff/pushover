package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatePaths(t *testing.T) {
	override := filepath.Join(t.TempDir(), "config")
	tests := []struct {
		name     string
		override string
		want     func(t *testing.T) string
	}{
		{
			name: "default user config directory",
			want: func(t *testing.T) string {
				t.Helper()
				dir, err := os.UserConfigDir()
				if err != nil {
					t.Fatalf("UserConfigDir() error = %v", err)
				}
				return filepath.Join(dir, "povr", "state.json")
			},
		},
		{
			name:     "config directory override",
			override: override,
			want: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(override, "state.json")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("POVR_CONFIG_DIR", tt.override)
			got, err := StateFilePath()
			if err != nil {
				t.Fatalf("StateFilePath() error = %v", err)
			}
			if want := tt.want(t); got != want {
				t.Errorf("StateFilePath() = %q, want %q", got, want)
			}

			lockPath, err := LockFilePath()
			if err != nil {
				t.Fatalf("LockFilePath() error = %v", err)
			}
			if want := got + ".lock"; lockPath != want {
				t.Errorf("LockFilePath() = %q, want %q", lockPath, want)
			}
		})
	}
}

func TestEndpoints(t *testing.T) {
	t.Setenv("PUSHOVER_API_URL", "")
	t.Setenv("PUSHOVER_WS_URL", "")
	if got := APIURL(); got != DefaultAPIURL {
		t.Errorf("APIURL() = %q, want %q", got, DefaultAPIURL)
	}
	if got := WSURL(); got != DefaultWSURL {
		t.Errorf("WSURL() = %q, want %q", got, DefaultWSURL)
	}

	t.Setenv("PUSHOVER_API_URL", "https://test.invalid/api")
	t.Setenv("PUSHOVER_WS_URL", "wss://test.invalid/push")
	if got := APIURL(); got != "https://test.invalid/api" {
		t.Errorf("APIURL() = %q, want override", got)
	}
	if got := WSURL(); got != "wss://test.invalid/push" {
		t.Errorf("WSURL() = %q, want override", got)
	}
}
