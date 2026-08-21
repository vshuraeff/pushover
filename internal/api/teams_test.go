package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListTeamSendsTokenAndParsesResponse(t *testing.T) {
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
		_, _ = writer.Write([]byte(`{"status":1,"name":"Test Team","users":[{"id":"team-user-1","name":"Test User","email":"test@example.invalid","administrator":true,"devices":[{"id":"device-1","name":"Phone"},"desktop"]}]}`))
	}))
	defer server.Close()

	name, users, err := newTestClient(server.URL).ListTeam(context.Background(), "test-fake-team-token-5678")
	if err != nil {
		t.Fatalf("ListTeam() error = %v", err)
	}
	if name != "Test Team" {
		t.Errorf("name = %q", name)
	}
	if len(users) != 1 {
		t.Fatalf("users = %d, want 1", len(users))
	}
	user := users[0]
	if user.ID != "team-user-1" || user.Name != "Test User" || user.Email != "test@example.invalid" || !user.Administrator {
		t.Errorf("user = %#v", user)
	}
	if len(user.Devices) != 2 {
		t.Errorf("devices = %d, want 2", len(user.Devices))
	}
}

func TestAddTeamUserSendsOptionalFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want %s", request.Method, http.MethodPost)
		}
		if request.URL.Path != "/1/teams/add_user.json" {
			t.Errorf("path = %s", request.URL.Path)
		}
		for field, want := range map[string]string{
			"token":    "test-fake-team-token-5678",
			"email":    "test@example.invalid",
			"name":     "Test User",
			"password": "test-password-not-secret",
			"instant":  "true",
			"admin":    "true",
			"group":    "test-group-key",
		} {
			if got := request.FormValue(field); got != want {
				t.Errorf("%s = %q, want %q", field, got, want)
			}
		}
		_, _ = writer.Write([]byte(`{"status":1,"id":"team-user-1"}`))
	}))
	defer server.Close()

	err := newTestClient(server.URL).AddTeamUser(context.Background(), "test-fake-team-token-5678", "test@example.invalid", TeamUserOptions{
		Name:          "Test User",
		Password:      "test-password-not-secret",
		Instant:       true,
		Administrator: true,
		Group:         "test-group-key",
	})
	if err != nil {
		t.Fatalf("AddTeamUser() error = %v", err)
	}
}

func TestTeamEmailOperationsSendExpectedFields(t *testing.T) {
	paths := []string{
		"/1/teams/revoke_invitation.json",
		"/1/teams/remove_user.json",
	}
	index := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want %s", request.Method, http.MethodPost)
		}
		if index >= len(paths) {
			t.Fatalf("unexpected request path %s", request.URL.Path)
		}
		if request.URL.Path != paths[index] {
			t.Errorf("path = %s, want %s", request.URL.Path, paths[index])
		}
		index++
		if got := request.FormValue("token"); got != "test-fake-team-token-5678" {
			t.Errorf("token = %q", got)
		}
		if got := request.FormValue("email"); got != "test@example.invalid" {
			t.Errorf("email = %q", got)
		}
		_, _ = writer.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	if err := client.RevokeTeamInvitation(context.Background(), "test-fake-team-token-5678", "test@example.invalid"); err != nil {
		t.Fatalf("RevokeTeamInvitation() error = %v", err)
	}
	if err := client.RemoveTeamUser(context.Background(), "test-fake-team-token-5678", "test@example.invalid"); err != nil {
		t.Fatalf("RemoveTeamUser() error = %v", err)
	}
}

func TestListTeamPropagatesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"status":0,"errors":["invalid team token"]}`))
	}))
	defer server.Close()

	_, _, err := newTestClient(server.URL).ListTeam(context.Background(), "test-fake-team-token-5678")
	if err == nil {
		t.Fatal("ListTeam() error = nil, want error")
	}
}
