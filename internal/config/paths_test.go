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
	}{
		{
			name: "default config directory",
		},
		{
			name:     "config directory override",
			override: override,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("PUSHOVER_CONFIG_DIR", tt.override)

			wantDir := filepath.Join(home, ".config", "pushover")
			if tt.override != "" {
				wantDir = tt.override
			}
			gotDir, err := ConfigDir()
			if err != nil {
				t.Fatalf("ConfigDir() error = %v", err)
			}
			if gotDir != wantDir {
				t.Errorf("ConfigDir() = %q, want %q", gotDir, wantDir)
			}

			got, err := StateFilePath()
			if err != nil {
				t.Fatalf("StateFilePath() error = %v", err)
			}
			if want := filepath.Join(wantDir, "state.json"); got != want {
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
	configDir := t.TempDir()
	t.Setenv("PUSHOVER_CONFIG_DIR", configDir)
	t.Setenv("PUSHOVER_API_URL", "")
	t.Setenv("PUSHOVER_WS_URL", "")
	if got := APIURL(); got != DefaultAPIURL {
		t.Errorf("APIURL() = %q, want %q", got, DefaultAPIURL)
	}
	if got := WSURL(); got != DefaultWSURL {
		t.Errorf("WSURL() = %q, want %q", got, DefaultWSURL)
	}

	configPath := filepath.Join(configDir, tomlConfigFileName)
	contents := []byte("api_url = \"https://file.invalid/api\"\nws_url = \"wss://file.invalid/push\"\n")
	if err := os.WriteFile(configPath, contents, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if got := APIURL(); got != "https://file.invalid/api" {
		t.Errorf("APIURL() = %q, want TOML value", got)
	}
	if got := WSURL(); got != "wss://file.invalid/push" {
		t.Errorf("WSURL() = %q, want TOML value", got)
	}

	t.Setenv("PUSHOVER_API_URL", "https://env.invalid/api")
	t.Setenv("PUSHOVER_WS_URL", "wss://env.invalid/push")
	if got := APIURL(); got != "https://env.invalid/api" {
		t.Errorf("APIURL() = %q, want environment override", got)
	}
	if got := WSURL(); got != "wss://env.invalid/push" {
		t.Errorf("WSURL() = %q, want environment override", got)
	}
}
