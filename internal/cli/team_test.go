package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTeamListCommandUsesTeamToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want %s", request.Method, http.MethodGet)
		}
		if request.URL.Path != "/1/teams.json" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.URL.Query().Get("token"); got != "test-fake-team-token-5678" {
			t.Errorf("token = %q", got)
		}
		_, _ = writer.Write([]byte(`{"status":1,"name":"Test Team","users":[{"id":"team-user-1","name":"Test User","email":"test@example.invalid","administrator":true,"devices":[{"id":"device-1"}]}]}`))
	}))
	defer server.Close()

	t.Setenv("PUSHOVER_API_URL", server.URL)
	t.Setenv("PUSHOVER_TEAM_TOKEN", "test-fake-team-token-5678")

	var output bytes.Buffer
	command := newTestRootCommand()
	command.SetArgs([]string{"team", "list"})
	command.SetOut(&output)
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"team: Test Team", "team-user-1", "devices=1"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output = %q, want %q", output.String(), want)
		}
	}
}

func TestTeamMutationCommandsSendExpectedFields(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		path   string
		fields map[string]string
	}{
		{
			name: "add user",
			args: []string{"team", "add-user", "--email", "test@example.invalid", "--name", "Test User", "--password", "test-password-not-secret", "--instant", "--admin", "--group", "test-group-key"},
			path: "/1/teams/add_user.json",
			fields: map[string]string{
				"email": "test@example.invalid", "name": "Test User", "password": "test-password-not-secret", "instant": "true", "admin": "true", "group": "test-group-key",
			},
		},
		{
			name:   "revoke invitation",
			args:   []string{"team", "revoke-invitation", "--email", "test@example.invalid"},
			path:   "/1/teams/revoke_invitation.json",
			fields: map[string]string{"email": "test@example.invalid"},
		},
		{
			name:   "remove user",
			args:   []string{"team", "remove-user", "--email", "test@example.invalid"},
			path:   "/1/teams/remove_user.json",
			fields: map[string]string{"email": "test@example.invalid"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost {
					t.Errorf("method = %s, want %s", request.Method, http.MethodPost)
				}
				if request.URL.Path != test.path {
					t.Errorf("path = %s, want %s", request.URL.Path, test.path)
				}
				if got := request.FormValue("token"); got != "test-fake-team-token-5678" {
					t.Errorf("token = %q", got)
				}
				for field, want := range test.fields {
					if got := request.FormValue(field); got != want {
						t.Errorf("%s = %q, want %q", field, got, want)
					}
				}
				_, _ = writer.Write([]byte(`{"status":1}`))
			}))
			defer server.Close()

			t.Setenv("PUSHOVER_API_URL", server.URL)
			t.Setenv("PUSHOVER_TEAM_TOKEN", "test-fake-team-token-5678")

			command := newTestRootCommand()
			command.SetArgs(test.args)
			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})
	}
}
