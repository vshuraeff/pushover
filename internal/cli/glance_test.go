package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGlanceCommandSendsExplicitEmptyTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want %s", request.Method, http.MethodPost)
		}
		if request.URL.Path != "/1/glances.json" {
			t.Errorf("path = %s, want /1/glances.json", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		values, ok := request.PostForm["title"]
		if !ok || len(values) != 1 || values[0] != "" {
			t.Errorf("title = %#v, want explicit empty value", values)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()
	setCLIEnv(t, server.URL)

	root, output := newTestRoot()
	root.SetArgs([]string{"glance", "--user", "test-user-key-not-real", "--title="})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got, want := output.String(), "glance sent\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestGlanceCommandOmitsUnchangedTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if values, ok := request.PostForm["title"]; ok {
			t.Errorf("title = %#v, want no title field", values)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()
	setCLIEnv(t, server.URL)

	root, _ := newTestRoot()
	root.SetArgs([]string{"glance", "--user", "test-user-key-not-real"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
}
