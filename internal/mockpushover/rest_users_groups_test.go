package mockpushover

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestUserValidationLoginAndDevices(t *testing.T) {
	t.Run("user validation", func(t *testing.T) {
		server := startTestServer(t)
		tests := []struct {
			name   string
			user   string
			device string
			want   int
		}{
			{name: "user", user: testUser, want: http.StatusOK},
			{name: "group", user: testGroup, want: http.StatusOK},
			{name: "device", user: testUser, device: "phone", want: http.StatusOK},
			{name: "bad device", user: testUser, device: "missing", want: http.StatusBadRequest},
			{name: "bad user", user: strings.Repeat("x", 30), want: http.StatusBadRequest},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				response, body := doForm(t, server, http.MethodPost, "/1/users/validate.json", url.Values{
					"token": {testToken}, "user": {test.user}, "device": {test.device},
				})
				requireStatus(t, response, test.want, body)
			})
		}
	})

	t.Run("login and two factor authentication", func(t *testing.T) {
		server := startTestServer(t)
		tests := []struct {
			name     string
			email    string
			password string
			twofa    string
			want     int
		}{
			{name: "normal", email: "one@example.com", password: "password-one", want: http.StatusOK},
			{name: "bad password", email: "one@example.com", password: "wrong", want: http.StatusBadRequest},
			{name: "two factor required", email: "two@example.com", password: "password-two", want: http.StatusPreconditionFailed},
			{name: "two factor accepted", email: "two@example.com", password: "password-two", twofa: "123456", want: http.StatusOK},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				response, body := doForm(t, server, http.MethodPost, "/1/users/login.json", url.Values{
					"email": {test.email}, "password": {test.password}, "twofa": {test.twofa},
				})
				requireStatus(t, response, test.want, body)
				if test.want == http.StatusOK && decodeObject(t, body)["secret"] == "" {
					t.Fatalf("login body = %s", body)
				}
			})
		}
	})

	t.Run("device registration and duplicate name", func(t *testing.T) {
		server := startTestServer(t)
		form := url.Values{"secret": {testBinding}, "name": {"desktop_1"}, "os": {"O"}}
		response, body := doForm(t, server, http.MethodPost, "/1/devices.json", form)
		requireStatus(t, response, http.StatusOK, body)
		if decodeObject(t, body)["id"] == "" {
			t.Fatalf("device body = %s", body)
		}
		response, body = doForm(t, server, http.MethodPost, "/1/devices.json", form)
		requireStatus(t, response, http.StatusBadRequest, body)
		errors := decodeObject(t, body)["errors"].(map[string]any)
		if _, ok := errors["name"]; !ok {
			t.Fatalf("duplicate errors = %s", body)
		}
		response, body = doForm(t, server, http.MethodPost, "/1/devices.json", url.Values{
			"secret": {testBinding}, "name": {"bad name"}, "os": {"O"},
		})
		requireStatus(t, response, http.StatusBadRequest, body)
	})
}

func TestGroupCRUD(t *testing.T) {
	server := startTestServer(t)
	response, body := doForm(t, server, http.MethodPost, "/1/groups.json", url.Values{
		"token": {testToken}, "name": {"Escalation"},
	})
	requireStatus(t, response, http.StatusOK, body)
	groupKey, ok := decodeObject(t, body)["group"].(string)
	if !ok || len(groupKey) != 30 {
		t.Fatalf("create group body = %s", body)
	}

	response, body = doForm(t, server, http.MethodGet, "/1/groups.json", url.Values{"token": {testToken}})
	requireStatus(t, response, http.StatusOK, body)
	if groups := decodeObject(t, body)["groups"].([]any); len(groups) != 2 {
		t.Fatalf("groups = %s", body)
	}

	response, body = doForm(t, server, http.MethodPost, "/1/groups/"+groupKey+"/add_user.json", url.Values{
		"token": {testToken}, "user": {testUser2}, "device": {"tablet"}, "memo": {"secondary"},
	})
	requireStatus(t, response, http.StatusOK, body)
	response, body = doForm(t, server, http.MethodGet, "/1/groups/"+groupKey+".json", url.Values{"token": {testToken}})
	requireStatus(t, response, http.StatusOK, body)
	if users := decodeObject(t, body)["users"].([]any); len(users) != 1 {
		t.Fatalf("group detail = %s", body)
	}

	operations := []struct {
		name     string
		endpoint string
		values   url.Values
	}{
		{
			name: "disable", endpoint: "/1/groups/" + groupKey + "/disable_user.json",
			values: url.Values{"token": {testToken}, "user": {testUser2}},
		},
		{
			name: "enable", endpoint: "/1/groups/" + groupKey + "/enable_user.json",
			values: url.Values{"token": {testToken}, "user": {testUser2}},
		},
		{
			name: "rename", endpoint: "/1/groups/" + groupKey + "/rename.json",
			values: url.Values{"token": {testToken}, "name": {"Renamed"}},
		},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			response, body := doForm(t, server, http.MethodPost, operation.endpoint, operation.values)
			requireStatus(t, response, http.StatusOK, body)
		})
	}

	response, body = doForm(t, server, http.MethodGet, "/1/groups/"+groupKey+".json", url.Values{"token": {testToken}})
	requireStatus(t, response, http.StatusOK, body)
	detail := decodeObject(t, body)
	if detail["name"] != "Renamed" {
		t.Fatalf("renamed group = %s", body)
	}
	member := detail["users"].([]any)[0].(map[string]any)
	if member["disabled"] != false {
		t.Fatalf("enabled member = %#v", member)
	}

	response, body = doForm(t, server, http.MethodPost, "/1/groups/"+groupKey+"/remove_user.json", url.Values{
		"token": {testToken}, "user": {testUser2},
	})
	requireStatus(t, response, http.StatusOK, body)
	response, body = doForm(t, server, http.MethodGet, "/1/groups/"+groupKey+".json", url.Values{"token": {testToken}})
	requireStatus(t, response, http.StatusOK, body)
	if users := decodeObject(t, body)["users"].([]any); len(users) != 0 {
		t.Fatalf("members after remove = %s", body)
	}

	response, body = doForm(t, server, http.MethodPost, "/1/groups/"+testGroup+"/add_user.json", url.Values{
		"token": {testToken}, "user": {testUser2}, "memo": {strings.Repeat("m", 201)},
	})
	requireStatus(t, response, http.StatusBadRequest, body)
}
