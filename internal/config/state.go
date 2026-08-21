package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type State struct {
	DeviceID               string `json:"device_id"`
	DeviceName             string `json:"device_name"`
	Secret                 string `json:"secret"`
	LastProcessedMessageID int64  `json:"last_processed_message_id"`
}

type SessionStore interface {
	Load() (*State, error)
	Save(*State) error
}

type FileSessionStore struct {
	statePath string
	lockPath  string
}

var _ SessionStore = (*FileSessionStore)(nil)

func NewFileSessionStore() (*FileSessionStore, error) {
	statePath, err := StateFilePath()
	if err != nil {
		return nil, err
	}
	lockPath, err := LockFilePath()
	if err != nil {
		return nil, err
	}
	return &FileSessionStore{statePath: statePath, lockPath: lockPath}, nil
}

func (s *FileSessionStore) Load() (*State, error) {
	data, err := os.ReadFile(s.statePath)
	if err != nil {
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode session state: %w", err)
	}
	return &state, nil
}

func (s *FileSessionStore) Save(state *State) (err error) {
	if state == nil {
		return errors.New("session state must not be nil")
	}

	dir := filepath.Dir(s.statePath)
	_, statErr := os.Stat(dir)
	createdDir := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !createdDir {
		return fmt.Errorf("stat session state directory: %w", statErr)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create session state directory: %w", err)
	}
	if createdDir {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("set session state directory permissions: %w", err)
		}
	}

	lock, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open session state lock: %w", err)
	}
	defer func() {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		if closeErr := lock.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock session state: %w", err)
	}
	if err := lock.Chmod(0o600); err != nil {
		return fmt.Errorf("set session state lock permissions: %w", err)
	}

	temp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create session state file: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		if temp != nil {
			_ = temp.Close()
		}
		if tempPath != "" {
			_ = os.Remove(tempPath)
		}
	}()

	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("set session state file permissions: %w", err)
	}
	if err := json.NewEncoder(temp).Encode(state); err != nil {
		return fmt.Errorf("encode session state: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync session state: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close session state: %w", err)
	}
	temp = nil

	if err := os.Rename(tempPath, s.statePath); err != nil {
		return fmt.Errorf("replace session state: %w", err)
	}
	tempPath = ""
	if err := os.Chmod(s.statePath, 0o600); err != nil {
		return fmt.Errorf("set session state file permissions: %w", err)
	}
	return nil
}
