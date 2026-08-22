//go:build darwin

package config

import (
	"encoding/base64"
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

type securityRunner func(stdin string, args ...string) (stdout []byte, err error)

type KeychainSessionStore struct {
	run securityRunner
}

var _ SessionStore = (*KeychainSessionStore)(nil)

func NewKeychainSessionStore() (*KeychainSessionStore, error) {
	return &KeychainSessionStore{run: runSecurity}, nil
}

func (s *KeychainSessionStore) Load() (*State, error) {
	data, err := s.run("", "find-generic-password", "-a", keychainAccount, "-s", keychainServiceName, "-w")
	if err != nil {
		if errors.Is(err, ErrKeychainEntryNotFound) || strings.Contains(strings.ToLower(err.Error()), "could not be found") {
			return nil, fmt.Errorf("%w: %w", ErrKeychainEntryNotFound, os.ErrNotExist)
		}
		return nil, fmt.Errorf("find keychain session state: %w", err)
	}
	if decoded, err := base64.StdEncoding.DecodeString(string(data)); err == nil {
		data = decoded
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

	// use base64 because security's interactive command quoting is undocumented.
	script := fmt.Sprintf("add-generic-password -a %s -s %s -w %s -U\n", keychainAccount, keychainServiceName, base64.StdEncoding.EncodeToString(data))
	if _, err := s.run(script, "-iq"); err != nil {
		return fmt.Errorf("save keychain session state: %w", err)
	}
	return nil
}

func runSecurity(stdin string, args ...string) ([]byte, error) {
	command := exec.Command("/usr/bin/security", args...)
	if stdin != "" {
		command.Stdin = strings.NewReader(stdin)
	}
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
