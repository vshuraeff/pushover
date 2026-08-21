package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

const tomlConfigFileName = "pushover.conf"

var errMalformedTOMLConfig = errors.New("decode config file")

type warningKey struct {
	path string
	kind string
}

var (
	warningMu    sync.Mutex
	seenWarnings = make(map[warningKey]struct{})
)

type TOMLConfig struct {
	AppToken      string `toml:"app_token"`
	UserKey       string `toml:"user_key"`
	TeamToken     string `toml:"team_token"`
	EncryptionKey string `toml:"encryption_key"`
	APIURL        string `toml:"api_url"`
	WSURL         string `toml:"ws_url"`
}

func LoadTOMLConfig() (*TOMLConfig, error) {
	path, err := tomlConfigPath()
	if err != nil {
		return nil, err
	}
	return loadTOMLConfig(path)
}

func tomlConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, tomlConfigFileName), nil
}

func loadTOMLConfig(path string) (*TOMLConfig, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return &TOMLConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat config file %q: %w", path, err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		warnOnce(path, "permissions", "warning: config file %q has permissions %04o; credentials may be exposed\n", path, info.Mode().Perm())
	}

	var config TOMLConfig
	metadata, err := toml.DecodeFile(path, &config)
	if err != nil {
		return nil, fmt.Errorf("%w %q", errMalformedTOMLConfig, path)
	}
	if keys := metadata.Undecoded(); len(keys) > 0 {
		names := make([]string, 0, len(keys))
		for _, key := range keys {
			names = append(names, key.String())
		}
		warnOnce(path, "unknown-keys", "warning: unknown config keys in %q: %s\n", path, strings.Join(names, ", "))
	}
	return &config, nil
}

func warnOnce(path, kind, format string, args ...any) {
	warningMu.Lock()
	defer warningMu.Unlock()

	key := warningKey{path: path, kind: kind}
	if _, ok := seenWarnings[key]; ok {
		return
	}
	seenWarnings[key] = struct{}{}
	fmt.Fprintf(os.Stderr, format, args...)
}

func resetWarnings() {
	warningMu.Lock()
	defer warningMu.Unlock()

	clear(seenWarnings)
}

func warnMalformedTOMLConfig() {
	path, err := tomlConfigPath()
	if err != nil {
		return
	}
	warnOnce(path, "malformed", "warning: config file %q is malformed and was ignored\n", path)
}

func (c *TOMLConfig) credential(role Role) string {
	if c == nil {
		return ""
	}
	switch role {
	case RoleAppToken:
		return c.AppToken
	case RoleUserKey:
		return c.UserKey
	case RoleTeamToken:
		return c.TeamToken
	case RoleEncryptionKey:
		return c.EncryptionKey
	default:
		return ""
	}
}
