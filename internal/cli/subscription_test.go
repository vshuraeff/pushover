package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubscriptionMigrateCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want %s", request.Method, http.MethodPost)
		}
		if request.URL.Path != "/1/subscriptions/migrate.json" {
			t.Errorf("path = %s", request.URL.Path)
		}
		for field, want := range map[string]string{
			"token":        "test-fake-app-token-1234",
			"subscription": "test-subscription-id",
			"user":         "test-fake-user-key-1234",
			"device_name":  "Test Phone",
			"sound":        "pushover",
		} {
			if got := request.FormValue(field); got != want {
				t.Errorf("%s = %q, want %q", field, got, want)
			}
		}
		_, _ = writer.Write([]byte(`{"status":1,"subscribed_user_key":"test-subscribed-user-key"}`))
	}))
	defer server.Close()

	t.Setenv("PUSHOVER_API_URL", server.URL)
	t.Setenv("PUSHOVER_APP_TOKEN", "test-fake-app-token-1234")

	var output bytes.Buffer
	command := newTestRootCommand()
	command.SetArgs([]string{"subscription", "migrate", "--subscription", "test-subscription-id", "--user", "test-fake-user-key-1234", "--device-name", "Test Phone", "--sound", "pushover"})
	command.SetOut(&output)
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := output.String(); !strings.Contains(got, "test-subscribed-user-key") {
		t.Errorf("output = %q", got)
	}
}
