package config

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestFileSessionStoreSaveLoadAndPermissions(t *testing.T) {
	tests := []struct {
		name  string
		state State
	}{
		{
			name: "all fields",
			state: State{
				DeviceID:               "test-device-id",
				DeviceName:             "test-device-name",
				Secret:                 "test-fake-session-secret-1234",
				LastProcessedMessageID: 42,
			},
		},
		{
			name: "zero last processed message id",
			state: State{
				DeviceID:   "test-device-id-2",
				DeviceName: "test-device-name-2",
				Secret:     "test-fake-session-secret-5678",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir := filepath.Join(t.TempDir(), "config")
			t.Setenv("PUSHOVER_CONFIG_DIR", configDir)

			store, err := NewFileSessionStore()
			if err != nil {
				t.Fatalf("NewFileSessionStore() error = %v", err)
			}
			if err := store.Save(&tt.state); err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			got, err := store.Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if *got != tt.state {
				t.Error("Load() returned unexpected state")
			}

			dirInfo, err := os.Stat(configDir)
			if err != nil {
				t.Fatalf("Stat(config dir) error = %v", err)
			}
			if got := dirInfo.Mode().Perm(); got != 0o700 {
				t.Errorf("config directory permissions = %o, want 700", got)
			}

			statePath, err := StateFilePath()
			if err != nil {
				t.Fatalf("StateFilePath() error = %v", err)
			}
			stateInfo, err := os.Stat(statePath)
			if err != nil {
				t.Fatalf("Stat(state file) error = %v", err)
			}
			if got := stateInfo.Mode().Perm(); got != 0o600 {
				t.Errorf("state file permissions = %o, want 600", got)
			}
		})
	}
}

func TestFileSessionStoreLoadMissing(t *testing.T) {
	t.Setenv("PUSHOVER_CONFIG_DIR", t.TempDir())
	store, err := NewFileSessionStore()
	if err != nil {
		t.Fatalf("NewFileSessionStore() error = %v", err)
	}

	_, err = store.Load()
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Load() error = %v, want os.ErrNotExist", err)
	}
}

func TestFileSessionStoreSaveWaitsForLock(t *testing.T) {
	t.Setenv("PUSHOVER_CONFIG_DIR", t.TempDir())
	store, err := NewFileSessionStore()
	if err != nil {
		t.Fatalf("NewFileSessionStore() error = %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(store.lockPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	lock, err := os.OpenFile(store.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("OpenFile(lock) error = %v", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("Flock(lock) error = %v", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		done <- store.Save(&State{DeviceID: "test-device-id"})
	}()
	<-started

	select {
	case err := <-done:
		t.Fatalf("Save() completed while lock was held: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatalf("unlock error = %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Save() did not finish after releasing lock")
	}
}

func TestFileSessionStoreSavePreservesExistingDirectoryPermissions(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Chmod(configDir, 0o755); err != nil {
		t.Fatalf("Chmod(config dir) error = %v", err)
	}
	t.Setenv("PUSHOVER_CONFIG_DIR", configDir)

	store, err := NewFileSessionStore()
	if err != nil {
		t.Fatalf("NewFileSessionStore() error = %v", err)
	}
	if err := store.Save(&State{DeviceID: "test-device-id"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("Stat(config dir) error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("config directory permissions = %o, want 755", got)
	}
}
