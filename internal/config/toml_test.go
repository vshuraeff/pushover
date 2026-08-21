package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTOMLConfigCredentialFallback(t *testing.T) {
	tests := []struct {
		name     string
		role     Role
		primary  Role
		contents string
		env      map[string]string
		want     string
		wantErr  bool
	}{
		{
			name:     "environment value wins over TOML value",
			role:     RoleAppToken,
			primary:  RoleAppToken,
			contents: "app_token = \"test-file-app-token\"\n",
			env:      map[string]string{"PUSHOVER_APP_TOKEN": "test-env-app-token"},
			want:     "test-env-app-token",
		},
		{
			name:     "TOML value is used without environment value",
			role:     RoleUserKey,
			primary:  RoleAppToken,
			contents: "user_key = \"test-file-user-key\"\n",
			want:     "test-file-user-key",
		},
		{
			name:     "session never resolves from TOML",
			role:     RoleSession,
			primary:  RoleSession,
			contents: "app_token = \"test-file-app-token\"\n",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(resetWarnings)
			configDir := t.TempDir()
			t.Setenv("HOME", t.TempDir())
			t.Setenv("PUSHOVER_CONFIG_DIR", configDir)
			for _, name := range []string{
				"PUSHOVER_APP_TOKEN",
				"PUSHOVER_USER_KEY",
				"PUSHOVER_TEAM_TOKEN",
				"PUSHOVER_KEY",
			} {
				t.Setenv(name, "")
			}
			for name, value := range tt.env {
				t.Setenv(name, value)
			}
			if err := os.WriteFile(filepath.Join(configDir, tomlConfigFileName), []byte(tt.contents), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			got, err := Resolve(tt.role, tt.primary, EnvFlags{})
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "session store") {
					t.Error("Resolve() did not return the session store error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got != tt.want {
				t.Error("Resolve() returned an unexpected credential")
			}
		})
	}
}

func TestLoadTOMLConfig(t *testing.T) {
	tests := []struct {
		name            string
		contents        string
		mode            os.FileMode
		writeFile       bool
		wantAppToken    bool
		warningCount    int
		warningContains []string
	}{
		{
			name:         "over-permissive file warns and loads",
			contents:     "app_token = \"test-file-app-token\"\n",
			mode:         0o644,
			writeFile:    true,
			wantAppToken: true,
			warningCount: 1,
			warningContains: []string{
				"0644",
			},
		},
		{
			name:         "unknown key warns and known key loads",
			contents:     "app_token = \"test-file-app-token\"\nunknown_setting = \"ignored\"\n",
			mode:         0o600,
			writeFile:    true,
			wantAppToken: true,
			warningCount: 1,
			warningContains: []string{
				"unknown_setting",
			},
		},
		{
			name:         "missing file returns empty config",
			warningCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(resetWarnings)
			configDir := t.TempDir()
			t.Setenv("HOME", t.TempDir())
			t.Setenv("PUSHOVER_CONFIG_DIR", configDir)
			configPath := filepath.Join(configDir, tomlConfigFileName)
			if tt.writeFile {
				if err := os.WriteFile(configPath, []byte(tt.contents), 0o600); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
				if err := os.Chmod(configPath, tt.mode); err != nil {
					t.Fatalf("Chmod() error = %v", err)
				}
			}

			var config *TOMLConfig
			var loadErr error
			output := captureStderr(t, func() {
				config, loadErr = LoadTOMLConfig()
			})
			if loadErr != nil {
				t.Fatalf("LoadTOMLConfig() error = %v", loadErr)
			}
			if config == nil {
				t.Fatal("LoadTOMLConfig() returned nil config")
			}
			if got := config.AppToken != ""; got != tt.wantAppToken {
				t.Error("LoadTOMLConfig() returned an unexpected app token state")
			}
			if count := strings.Count(output, "warning:"); count != tt.warningCount {
				t.Errorf("warning count = %d, want %d", count, tt.warningCount)
			}
			if tt.warningCount == 1 && len(strings.Split(strings.TrimSpace(output), "\n")) != 1 {
				t.Error("warning output must contain exactly one line")
			}
			for _, want := range tt.warningContains {
				if !strings.Contains(output, want) {
					t.Errorf("warning output does not contain %q", want)
				}
			}
			if tt.warningCount > 0 && !strings.Contains(output, configPath) {
				t.Error("warning output does not identify the config file")
			}
		})
	}
}

func TestLoadTOMLConfigWarningsDeduplicated(t *testing.T) {
	t.Cleanup(resetWarnings)
	configDir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PUSHOVER_CONFIG_DIR", configDir)
	configPath := filepath.Join(configDir, tomlConfigFileName)
	if err := os.WriteFile(configPath, []byte("app_token = \"test-file-app-token\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(configPath, 0o644); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}

	output := captureStderr(t, func() {
		for range 2 {
			if _, err := LoadTOMLConfig(); err != nil {
				t.Fatalf("LoadTOMLConfig() error = %v", err)
			}
		}
	})
	if count := strings.Count(output, "warning:"); count != 1 {
		t.Errorf("warning count = %d, want 1", count)
	}
	if !strings.Contains(output, configPath) {
		t.Error("warning output does not identify the config file")
	}
}

func TestResolveMalformedTOMLConfig(t *testing.T) {
	t.Cleanup(resetWarnings)
	configDir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PUSHOVER_CONFIG_DIR", configDir)
	for _, name := range []string{
		"PUSHOVER_APP_TOKEN",
		"PUSHOVER_USER_KEY",
		"PUSHOVER_TEAM_TOKEN",
		"PUSHOVER_KEY",
	} {
		t.Setenv(name, "")
	}
	if err := os.WriteFile(filepath.Join(configDir, tomlConfigFileName), []byte("app_token = \"ignored\"\n["), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Resolve(RoleAppToken, RoleAppToken, EnvFlags{})
	if err == nil {
		t.Fatal("Resolve() error = nil, want malformed TOML error")
	}
	if !strings.Contains(err.Error(), "decode config file") {
		t.Errorf("Resolve() error = %v, want decode config file error", err)
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	previous := os.Stderr
	os.Stderr = file
	t.Cleanup(func() {
		os.Stderr = previous
		_ = file.Close()
	})

	fn()
	if err := file.Close(); err != nil {
		t.Fatalf("Close(stderr) error = %v", err)
	}
	os.Stderr = previous
	output, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatalf("ReadFile(stderr) error = %v", err)
	}
	return string(output)
}
