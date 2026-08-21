package config

import (
	"fmt"
	"os"
	"strings"
)

type Role string

const (
	RoleAppToken  Role = "app_token"
	RoleUserKey   Role = "user_key"
	RoleTeamToken Role = "team_token"
	RoleSession   Role = "session"
)

type RoleRequirement struct {
	Required []Role
	Primary  Role
}

type EnvFlags struct {
	KeyEnv   string
	TokenEnv string
	UserEnv  string
	TeamEnv  string
}

// specific role flags take precedence over --key-env for the primary role.
func Resolve(role, primary Role, flags EnvFlags) (string, error) {
	if !isKnownRole(role) {
		return "", fmt.Errorf("unknown credential role %q", role)
	}
	if !isKnownRole(primary) {
		return "", fmt.Errorf("unknown primary credential role %q", primary)
	}
	if role == RoleSession {
		// session credentials resolve via the session store.
		return "", fmt.Errorf("session credentials resolve via the session store")
	}

	if name := envFlagName(role, primary, flags); name != "" {
		if value := os.Getenv(name); value != "" {
			return value, nil
		}
		return "", fmt.Errorf("environment variable %q for %s is unset or empty", name, role)
	}

	checked := []string{defaultEnvName(role)}
	if role == primary {
		checked = append(checked, "PUSHOVER_KEY")
	}
	for _, name := range checked {
		if value := os.Getenv(name); value != "" {
			return value, nil
		}
	}

	return "", fmt.Errorf("missing %s credential; checked environment variables: %s", role, strings.Join(checked, ", "))
}

func isKnownRole(role Role) bool {
	switch role {
	case RoleAppToken, RoleUserKey, RoleTeamToken, RoleSession:
		return true
	default:
		return false
	}
}

func envFlagName(role, primary Role, flags EnvFlags) string {
	var specific string
	switch role {
	case RoleAppToken:
		specific = flags.TokenEnv
	case RoleUserKey:
		specific = flags.UserEnv
	case RoleTeamToken:
		specific = flags.TeamEnv
	}
	if specific != "" {
		return specific
	}
	if role == primary {
		return flags.KeyEnv
	}
	return ""
}

func defaultEnvName(role Role) string {
	switch role {
	case RoleAppToken:
		return "PUSHOVER_APP_TOKEN"
	case RoleUserKey:
		return "PUSHOVER_USER_KEY"
	case RoleTeamToken:
		return "PUSHOVER_TEAM_TOKEN"
	default:
		return ""
	}
}
