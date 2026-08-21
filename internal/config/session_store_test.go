package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSessionStoreSelection(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		contents string
		want     string
	}{
		{
			name:     "environment file wins over toml keychain",
			env:      "file",
			contents: "session_store = \"keychain\"\n",
			want:     "file",
		},
		{
			name:     "environment keychain wins over toml file",
			env:      "keychain",
			contents: "session_store = \"file\"\n",
			want:     "keychain",
		},
		{
			name:     "toml keychain is used without environment value",
			contents: "session_store = \"keychain\"\n",
			want:     "keychain",
		},
		{
			name: "file is the default",
			want: "file",
		},
		{
			name: "unknown value returns an error",
			env:  "unknown",
			want: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(resetWarnings)
			configDir := t.TempDir()
			t.Setenv("PUSHOVER_CONFIG_DIR", configDir)
			t.Setenv("PUSHOVER_SESSION_STORE", tt.env)
			if tt.contents != "" {
				if err := os.WriteFile(filepath.Join(configDir, tomlConfigFileName), []byte(tt.contents), 0o600); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			}

			store, err := NewSessionStore()
			switch tt.want {
			case "file":
				if err != nil {
					t.Fatalf("NewSessionStore() error = %v", err)
				}
				if _, ok := store.(*FileSessionStore); !ok {
					t.Errorf("NewSessionStore() store = %T, want *FileSessionStore", store)
				}
			case "keychain":
				assertKeychainSessionStore(t, store, err)
			case "unknown":
				if err == nil {
					t.Fatal("NewSessionStore() error = nil, want unknown session_store error")
				}
				if !strings.Contains(err.Error(), "unknown session_store \"unknown\"; expected \"file\" or \"keychain\"") {
					t.Errorf("NewSessionStore() error = %v", err)
				}
			}
		})
	}
}

func assertKeychainSessionStore(t *testing.T, store SessionStore, err error) {
	t.Helper()

	_, wantErr := NewKeychainSessionStore()
	if wantErr != nil {
		if !errors.Is(err, wantErr) {
			t.Errorf("NewSessionStore() error = %v, want %v", err, wantErr)
		}
		return
	}
	if err != nil {
		t.Fatalf("NewSessionStore() error = %v", err)
	}
	if _, ok := store.(*KeychainSessionStore); !ok {
		t.Errorf("NewSessionStore() store = %T, want *KeychainSessionStore", store)
	}
}
