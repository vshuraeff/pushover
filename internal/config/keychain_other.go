//go:build !darwin

package config

import "errors"

var ErrKeychainUnsupported = errors.New("keychain session store is only supported on macOS")

type KeychainSessionStore struct{}

var _ SessionStore = (*KeychainSessionStore)(nil)

func NewKeychainSessionStore() (*KeychainSessionStore, error) {
	return nil, ErrKeychainUnsupported
}

func (s *KeychainSessionStore) Load() (*State, error) {
	return nil, ErrKeychainUnsupported
}

func (s *KeychainSessionStore) Save(state *State) error {
	return ErrKeychainUnsupported
}
