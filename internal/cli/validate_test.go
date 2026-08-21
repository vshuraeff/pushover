package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestValidateCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want %s", request.Method, http.MethodPost)
		}
		if request.URL.Path != "/1/users/validate.json" {
			t.Errorf("path = %s, want /1/users/validate.json", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if got := request.PostForm.Get("token"); got != "test-app-token-not-real" {
			t.Errorf("token = %q", got)
		}
		if got := request.PostForm.Get("user"); got != "test-user-key-not-real" {
			t.Errorf("user = %q", got)
		}
		if got := request.PostForm.Get("device"); got != "phone" {
			t.Errorf("device = %q", got)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":1,"devices":["phone"],"licenses":["test-license"]}`))
	}))
	defer server.Close()
	setCLIEnv(t, server.URL)

	root, output := newTestRoot()
	root.SetArgs([]string{"validate", "--user", "test-user-key-not-real", "--device", "phone"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got, want := output.String(), "valid\ndevices: phone\nlicenses: test-license\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestValidateCommandTreatsInvalidKeyAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":0,"errors":{"user":["is invalid"]}}`))
	}))
	defer server.Close()
	setCLIEnv(t, server.URL)

	root, output := newTestRoot()
	root.SetArgs([]string{"validate", "--user", "test-user-key-not-real"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(output.String(), "invalid: pushover api error: user: is invalid") {
		t.Fatalf("output = %q, want invalid result", output.String())
	}
}

func newTestRoot() (*cobra.Command, *bytes.Buffer) {
	root := NewRootCommand()
	Register(root)
	output := &bytes.Buffer{}
	root.SetOut(output)
	root.SetErr(io.Discard)
	return root, output
}

func setCLIEnv(t *testing.T, apiURL string) {
	t.Helper()
	t.Setenv("PUSHOVER_API_URL", apiURL)
	t.Setenv("PUSHOVER_APP_TOKEN", "test-app-token-not-real")
}
