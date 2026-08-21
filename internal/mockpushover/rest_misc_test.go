package mockpushover

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestGlances(t *testing.T) {
	server := startTestServer(t)
	response, body := doForm(t, server, http.MethodPost, "/1/glances.json", url.Values{
		"token": {testToken}, "user": {testUser}, "device": {"phone"},
		"title": {"Deploy"}, "text": {"Healthy"}, "subtext": {"production"},
		"count": {"7"}, "percent": {"95"},
	})
	requireStatus(t, response, http.StatusOK, body)
	server.store.mu.RLock()
	glance := server.store.glances[glanceKey(testUser, "phone")]
	server.store.mu.RUnlock()
	if glance.Title != "Deploy" || glance.Count != 7 || glance.Percent != 95 {
		t.Fatalf("glance = %#v", glance)
	}

	tests := []struct {
		name   string
		values url.Values
	}{
		{
			name:   "text too long",
			values: url.Values{"token": {testToken}, "user": {testUser}, "text": {strings.Repeat("x", 101)}},
		},
		{
			name:   "percent out of range",
			values: url.Values{"token": {testToken}, "user": {testUser}, "percent": {"101"}},
		},
		{
			name:   "count invalid",
			values: url.Values{"token": {testToken}, "user": {testUser}, "count": {"many"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, body := doForm(t, server, http.MethodPost, "/1/glances.json", test.values)
			requireStatus(t, response, http.StatusBadRequest, body)
		})
	}
}

func TestSoundsLicensesSubscriptionsAndLimits(t *testing.T) {
	server := startTestServer(t)
	t.Run("sounds merge custom entries", func(t *testing.T) {
		response, body := doForm(t, server, http.MethodGet, "/1/sounds.json", url.Values{"token": {testToken}})
		requireStatus(t, response, http.StatusOK, body)
		sounds := decodeObject(t, body)["sounds"].(map[string]any)
		if len(sounds) != len(builtInSounds)+1 || sounds["custom"] != "Custom Sound" || sounds["pushover"] == nil {
			t.Fatalf("sounds = %s", body)
		}
	})

	t.Run("license assignment and exhaustion", func(t *testing.T) {
		assignments := []url.Values{
			{"token": {testToken}, "user": {testUser}, "os": {"Desktop"}},
			{"token": {testToken}, "email": {"pending@example.com"}, "os": {"iOS"}},
		}
		for _, assignment := range assignments {
			response, body := doForm(t, server, http.MethodPost, "/1/licenses/assign.json", assignment)
			requireStatus(t, response, http.StatusOK, body)
		}
		response, body := doForm(t, server, http.MethodGet, "/1/licenses.json", url.Values{"token": {testToken}})
		requireStatus(t, response, http.StatusOK, body)
		payload := decodeObject(t, body)
		if payload["credits"] != float64(0) || len(payload["assigned"].([]any)) != 2 {
			t.Fatalf("licenses = %s", body)
		}
		response, body = doForm(t, server, http.MethodPost, "/1/licenses/assign.json", url.Values{
			"token": {testToken}, "email": {"third@example.com"},
		})
		requireStatus(t, response, http.StatusBadRequest, body)
		response, body = doForm(t, server, http.MethodPost, "/1/licenses/assign.json", url.Values{
			"token": {testToken}, "user": {testUser}, "email": {"both@example.com"},
		})
		requireStatus(t, response, http.StatusBadRequest, body)
	})

	t.Run("subscription migration", func(t *testing.T) {
		response, body := doForm(t, server, http.MethodPost, "/1/subscriptions/migrate.json", url.Values{
			"token": {testToken}, "subscription": {"subscription-code"}, "user": {testUser},
		})
		requireStatus(t, response, http.StatusOK, body)
		key, _ := decodeObject(t, body)["subscribed_user_key"].(string)
		if len(key) != 30 {
			t.Fatalf("migration body = %s", body)
		}
		response, body = doForm(t, server, http.MethodPost, "/1/subscriptions/migrate.json", url.Values{
			"token": {testToken}, "user": {testUser},
		})
		requireStatus(t, response, http.StatusBadRequest, body)
	})

	t.Run("application limits", func(t *testing.T) {
		response, body := doForm(t, server, http.MethodGet, "/1/apps/limits.json", url.Values{"token": {testToken}})
		requireStatus(t, response, http.StatusOK, body)
		payload := decodeObject(t, body)
		if payload["limit"] != float64(5) || payload["remaining"] != float64(5) || payload["reset"] != float64(1_800_000_000) {
			t.Fatalf("limits = %s", body)
		}
		if response.Header.Get("X-Limit-App-Remaining") != "5" {
			t.Fatalf("limit headers = %#v", response.Header)
		}
	})
}

func TestTeams(t *testing.T) {
	server := startTestServer(t)
	teamUsers := func(t *testing.T) []any {
		t.Helper()
		response, body := doForm(t, server, http.MethodGet, "/1/teams.json", url.Values{"token": {testTeamToken}})
		requireStatus(t, response, http.StatusOK, body)
		return decodeObject(t, body)["users"].([]any)
	}
	if users := teamUsers(t); len(users) != 1 {
		t.Fatalf("initial team users = %#v", users)
	}

	response, body := doForm(t, server, http.MethodPost, "/1/teams/add_user.json", url.Values{
		"token": {testTeamToken}, "email": {"invite@example.com"}, "name": {"Invite"}, "admin": {"true"},
	})
	requireStatus(t, response, http.StatusOK, body)
	if users := teamUsers(t); len(users) != 2 {
		t.Fatalf("team users after add = %#v", users)
	}
	response, body = doForm(t, server, http.MethodPost, "/1/teams/revoke_invitation.json", url.Values{
		"token": {testTeamToken}, "email": {"invite@example.com"},
	})
	requireStatus(t, response, http.StatusOK, body)
	if users := teamUsers(t); len(users) != 1 {
		t.Fatalf("team users after revoke = %#v", users)
	}

	response, body = doForm(t, server, http.MethodPost, "/1/teams/add_user.json", url.Values{
		"token": {testTeamToken}, "email": {"remove@example.com"},
	})
	requireStatus(t, response, http.StatusOK, body)
	response, body = doForm(t, server, http.MethodPost, "/1/teams/remove_user.json", url.Values{
		"token": {testTeamToken}, "email": {"remove@example.com"},
	})
	requireStatus(t, response, http.StatusOK, body)
	if users := teamUsers(t); len(users) != 1 {
		t.Fatalf("team users after remove = %#v", users)
	}

	response, body = doForm(t, server, http.MethodGet, "/1/teams.json", url.Values{"token": {"invalid"}})
	requireStatus(t, response, http.StatusBadRequest, body)
}
