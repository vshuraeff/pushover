package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMigrateSubscriptionSendsFormAndParsesResponse(t *testing.T) {
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

	userKey, err := newTestClient(server.URL).MigrateSubscription(context.Background(), "test-fake-app-token-1234", "test-subscription-id", "test-fake-user-key-1234", "Test Phone", "pushover")
	if err != nil {
		t.Fatalf("MigrateSubscription() error = %v", err)
	}
	if userKey != "test-subscribed-user-key" {
		t.Errorf("user key = %q", userKey)
	}
}

func TestMigrateSubscriptionPropagatesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"status":0,"errors":["invalid subscription"]}`))
	}))
	defer server.Close()

	_, err := newTestClient(server.URL).MigrateSubscription(context.Background(), "test-fake-app-token-1234", "test-subscription-id", "test-fake-user-key-1234", "", "")
	if err == nil {
		t.Fatal("MigrateSubscription() error = nil, want error")
	}
}
