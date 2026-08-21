package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSoundsCommand(t *testing.T) {
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
		_, _ = writer.Write([]byte(`{"status":1,"sounds":{"zeta":"Zeta","alpha":"Alpha"}}`))
	}))
	defer server.Close()
	setCLIEnv(t, server.URL)

	root, output := newTestRoot()
	root.SetArgs([]string{"sounds"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	result := output.String()
	if !strings.Contains(result, "alpha:") || !strings.Contains(result, "Zeta") {
		t.Fatalf("output = %q", result)
	}
	if strings.Index(result, "alpha:") > strings.Index(result, "zeta:") {
		t.Fatalf("output is not sorted: %q", result)
	}
}

func TestSoundsCommandJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":1,"sounds":{"custom":"Custom sound"}}`))
	}))
	defer server.Close()
	setCLIEnv(t, server.URL)

	root, output := newTestRoot()
	root.SetArgs([]string{"sounds", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var sounds map[string]string
	if err := json.Unmarshal(output.Bytes(), &sounds); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got := sounds["custom"]; got != "Custom sound" {
		t.Fatalf("custom sound = %q", got)
	}
}
