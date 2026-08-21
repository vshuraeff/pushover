package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const tomlConfigFileName = "pushover.conf"

type TOMLConfig struct {
	AppToken  string `toml:"app_token"`
	UserKey   string `toml:"user_key"`
	TeamToken string `toml:"team_token"`
	APIURL    string `toml:"api_url"`
	WSURL     string `toml:"ws_url"`
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
		fmt.Fprintf(os.Stderr, "warning: config file %q has permissions %04o; credentials may be exposed\n", path, info.Mode().Perm())
	}

	var config TOMLConfig
	metadata, err := toml.DecodeFile(path, &config)
	if err != nil {
		return nil, fmt.Errorf("decode config file %q", path)
	}
	if keys := metadata.Undecoded(); len(keys) > 0 {
		names := make([]string, 0, len(keys))
		for _, key := range keys {
			names = append(names, key.String())
		}
		fmt.Fprintf(os.Stderr, "warning: unknown config keys in %q: %s\n", path, strings.Join(names, ", "))
	}
	return &config, nil
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
	default:
		return ""
	}
}
