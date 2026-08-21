package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAppLimits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/1/apps/limits.json" {
			t.Errorf("path = %s, want /1/apps/limits.json", r.URL.Path)
		}
		if got := r.URL.Query().Get("token"); got != "test-token" {
			t.Error("token was not sent")
		}
		w.Header().Set("X-Limit-App-Limit", "10000")
		w.Header().Set("X-Limit-App-Remaining", "9999")
		w.Header().Set("X-Limit-App-Reset", "1700000000")
		_, _ = w.Write([]byte(`{"status":1,"request":"request-id","limit":10000,"remaining":9999,"reset":1700000000}`))
	}))
	defer server.Close()

	limits, err := New(server.URL, "test").AppLimits(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("AppLimits() error = %v", err)
	}
	if limits.Status != 1 || limits.Request != "request-id" || limits.Limit != 10000 || limits.Remaining != 9999 || limits.Reset != 1_700_000_000 {
		t.Errorf("AppLimits() result = %+v", limits)
	}
	if limits.Header != (RateLimits{Limit: "10000", Remaining: "9999", Reset: "1700000000"}) {
		t.Errorf("headers = %+v", limits.Header)
	}
}
