package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSoundsList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want %s", request.Method, http.MethodGet)
		}
		if request.URL.Path != "/1/sounds.json" {
			t.Errorf("path = %s, want /1/sounds.json", request.URL.Path)
		}
		if got := request.URL.Query().Get("token"); got != "test-app-token-not-real" {
			t.Errorf("token = %q", got)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":1,"request":"test-request","sounds":{"bike":"Bike","custom":"Custom sound"}}`))
	}))
	defer server.Close()

	service := NewSoundsService(New(server.URL, "test-version"))
	response, err := service.List(context.Background(), "test-app-token-not-real")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if response.Status != 1 {
		t.Fatalf("status = %d, want 1", response.Status)
	}
	if got := response.Sounds["custom"]; got != "Custom sound" {
		t.Fatalf("custom sound = %q", got)
	}
}
