package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestGlanceCommandUsesConfiguredUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if got, want := request.PostForm.Get("user"), "test-user-key-not-real"; got != want {
			t.Errorf("user = %q, want %q", got, want)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()
	setCLIEnv(t, server.URL)
	t.Setenv("PUSHOVER_USER_KEY", "test-user-key-not-real")

	root, output := newTestRoot()
	root.SetArgs([]string{"glance"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got, want := output.String(), "glance sent\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestGlanceCommandRequiresResolvableUser(t *testing.T) {
	setCLIEnv(t, "http://127.0.0.1")
	t.Setenv("PUSHOVER_USER_KEY", "")
	t.Setenv("PUSHOVER_KEY", "")
	t.Setenv("PUSHOVER_CONFIG_DIR", t.TempDir())

	root, _ := newTestRoot()
	root.SetArgs([]string{"glance"})
	err := root.Execute()
	if err == nil {
		t.Fatal("execute succeeded, want missing user credential error")
	}
	if err.Error() == "--user is required" {
		t.Fatalf("error = %q, want credential resolution error", err)
	}
	if !strings.Contains(err.Error(), "missing user_key credential") || !strings.Contains(err.Error(), "PUSHOVER_USER_KEY") {
		t.Fatalf("error = %q, want missing user_key credential with checked env var", err)
	}
}
