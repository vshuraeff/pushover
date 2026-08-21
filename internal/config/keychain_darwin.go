//go:build darwin

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	keychainServiceName = "net.vshuraeff.povr"
	keychainAccount     = "session-state"
)

var ErrKeychainEntryNotFound = errors.New("keychain session state not found")

type securityRunner func(args ...string) (stdout []byte, err error)

type KeychainSessionStore struct {
	run securityRunner
}

var _ SessionStore = (*KeychainSessionStore)(nil)

func NewKeychainSessionStore() (*KeychainSessionStore, error) {
	return &KeychainSessionStore{run: runSecurity}, nil
}

func (s *KeychainSessionStore) Load() (*State, error) {
	data, err := s.run("find-generic-password", "-a", keychainAccount, "-s", keychainServiceName, "-w")
	if err != nil {
		if errors.Is(err, ErrKeychainEntryNotFound) || strings.Contains(strings.ToLower(err.Error()), "could not be found") {
			return nil, fmt.Errorf("%w: %w", ErrKeychainEntryNotFound, os.ErrNotExist)
		}
		return nil, fmt.Errorf("find keychain session state: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode session state: %w", err)
	}
	return &state, nil
}

func (s *KeychainSessionStore) Save(state *State) error {
	if state == nil {
		return errors.New("session state must not be nil")
	}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode session state: %w", err)
	}
	if _, err := s.run("add-generic-password", "-a", keychainAccount, "-s", keychainServiceName, "-w", string(data), "-U"); err != nil {
		return fmt.Errorf("save keychain session state: %w", err)
	}
	return nil
}

func runSecurity(args ...string) ([]byte, error) {
	command := exec.Command("/usr/bin/security", args...)
	stdout, err := command.Output()
	if err == nil {
		return stdout, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && strings.Contains(strings.ToLower(string(exitErr.Stderr)), "could not be found") {
		return nil, ErrKeychainEntryNotFound
	}
	return nil, err
}
