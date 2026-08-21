package config

import (
	"os"
	"path/filepath"
)

const (
	DefaultAPIURL = "https://api.pushover.net"
	DefaultWSURL  = "wss://client.pushover.net/push"
)

func ConfigDir() (string, error) {
	if dir := os.Getenv("PUSHOVER_CONFIG_DIR"); dir != "" {
		return dir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "pushover"), nil
}

func StateFilePath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

func LockFilePath() (string, error) {
	statePath, err := StateFilePath()
	if err != nil {
		return "", err
	}
	return statePath + ".lock", nil
}

func APIURL() string {
	if url := os.Getenv("PUSHOVER_API_URL"); url != "" {
		return url
	}
	if tomlConfig, err := LoadTOMLConfig(); err == nil && tomlConfig.APIURL != "" {
		return tomlConfig.APIURL
	}
	return DefaultAPIURL
}

func WSURL() string {
	if url := os.Getenv("PUSHOVER_WS_URL"); url != "" {
		return url
	}
	if tomlConfig, err := LoadTOMLConfig(); err == nil && tomlConfig.WSURL != "" {
		return tomlConfig.WSURL
	}
	return DefaultWSURL
}
