package config

import (
	"fmt"
	"os"
)

func NewSessionStore() (SessionStore, error) {
	value := sessionStoreName()
	switch value {
	case "file":
		return NewFileSessionStore()
	case "keychain":
		return NewKeychainSessionStore()
	default:
		return nil, fmt.Errorf("unknown session_store %q; expected \"file\" or \"keychain\"", value)
	}
}

func sessionStoreName() string {
	if value := os.Getenv("PUSHOVER_SESSION_STORE"); value != "" {
		return value
	}
	if tomlConfig := endpointTOMLConfig(); tomlConfig != nil && tomlConfig.SessionStore != "" {
		return tomlConfig.SessionStore
	}
	return "file"
}
