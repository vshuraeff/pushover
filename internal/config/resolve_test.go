package config

import (
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name        string
		role        Role
		primary     Role
		flags       EnvFlags
		env         map[string]string
		want        string
		wantErr     bool
		errContains []string
		errAbsent   []string
	}{
		{
			name:    "specific app token environment variable",
			role:    RoleAppToken,
			primary: RoleAppToken,
			flags:   EnvFlags{TokenEnv: "TEST_APP_TOKEN"},
			env:     map[string]string{"TEST_APP_TOKEN": "test-fake-app-token-1234"},
			want:    "test-fake-app-token-1234",
		},
		{
			name:    "specific user key environment variable",
			role:    RoleUserKey,
			primary: RoleUserKey,
			flags:   EnvFlags{UserEnv: "TEST_USER_KEY"},
			env:     map[string]string{"TEST_USER_KEY": "test-fake-user-key-1234"},
			want:    "test-fake-user-key-1234",
		},
		{
			name:    "specific team token environment variable",
			role:    RoleTeamToken,
			primary: RoleTeamToken,
			flags:   EnvFlags{TeamEnv: "TEST_TEAM_TOKEN"},
			env:     map[string]string{"TEST_TEAM_TOKEN": "test-fake-team-token-1234"},
			want:    "test-fake-team-token-1234",
		},
		{
			name:        "specific empty environment variable does not fall through",
			role:        RoleAppToken,
			primary:     RoleAppToken,
			flags:       EnvFlags{TokenEnv: "TEST_APP_TOKEN"},
			env:         map[string]string{"PUSHOVER_APP_TOKEN": "test-fake-app-token-1234", "PUSHOVER_KEY": "test-fake-primary-token-1234"},
			wantErr:     true,
			errContains: []string{"TEST_APP_TOKEN", "unset or empty"},
		},
		{
			name:    "default user key environment variable",
			role:    RoleUserKey,
			primary: RoleAppToken,
			env:     map[string]string{"PUSHOVER_USER_KEY": "test-fake-user-key-1234"},
			want:    "test-fake-user-key-1234",
		},
		{
			name:    "default environment variable missing falls through to primary key",
			role:    RoleTeamToken,
			primary: RoleTeamToken,
			env:     map[string]string{"PUSHOVER_KEY": "test-fake-primary-token-1234"},
			want:    "test-fake-primary-token-1234",
		},
		{
			name:    "primary key is not used for non-primary role",
			role:    RoleUserKey,
			primary: RoleAppToken,
			env:     map[string]string{"PUSHOVER_KEY": "test-fake-primary-token-1234"},
			wantErr: true,
			errContains: []string{
				"PUSHOVER_USER_KEY",
			},
			errAbsent: []string{"PUSHOVER_KEY"},
		},
		{
			name:        "final error lists all checked primary names in order",
			role:        RoleAppToken,
			primary:     RoleAppToken,
			wantErr:     true,
			errContains: []string{"PUSHOVER_APP_TOKEN, PUSHOVER_KEY"},
		},
		{
			name:    "key env aliases primary role environment flag",
			role:    RoleUserKey,
			primary: RoleUserKey,
			flags:   EnvFlags{KeyEnv: "TEST_PRIMARY_KEY"},
			env:     map[string]string{"TEST_PRIMARY_KEY": "test-fake-user-key-1234"},
			want:    "test-fake-user-key-1234",
		},
		{
			name:    "specific environment flag wins over key env alias",
			role:    RoleAppToken,
			primary: RoleAppToken,
			flags:   EnvFlags{KeyEnv: "TEST_PRIMARY_KEY", TokenEnv: "TEST_APP_TOKEN"},
			env: map[string]string{
				"TEST_PRIMARY_KEY": "test-fake-primary-token-1234",
				"TEST_APP_TOKEN":   "test-fake-app-token-1234",
			},
			want: "test-fake-app-token-1234",
		},
		{
			name:        "session role is resolved by session store",
			role:        RoleSession,
			primary:     RoleSession,
			wantErr:     true,
			errContains: []string{"session store"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, name := range []string{
				"TEST_APP_TOKEN",
				"TEST_PRIMARY_KEY",
				"TEST_USER_KEY",
				"TEST_TEAM_TOKEN",
				"PUSHOVER_APP_TOKEN",
				"PUSHOVER_USER_KEY",
				"PUSHOVER_TEAM_TOKEN",
				"PUSHOVER_KEY",
			} {
				t.Setenv(name, "")
			}
			for name, value := range tt.env {
				t.Setenv(name, value)
			}

			got, err := Resolve(tt.role, tt.primary, tt.flags)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Resolve() error = nil, want error")
				}
				for _, want := range tt.errContains {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Resolve() error = %q, want it to contain %q", err, want)
					}
				}
				for _, unwanted := range tt.errAbsent {
					if strings.Contains(err.Error(), unwanted) {
						t.Errorf("Resolve() error = %q, must not contain %q", err, unwanted)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got != tt.want {
				t.Error("Resolve() returned an unexpected credential")
			}
		})
	}
}
