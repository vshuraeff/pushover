package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshuraeff/pushover/internal/api"
)

func TestSendCommandFullFlags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		for name, want := range map[string]string{
			"user": "direct-user", "message": "message", "device": "phone", "title": "title",
			"url": "https://example.test", "url_title": "example", "priority": "2", "retry": "30",
			"expire": "3600", "callback": "https://example.test/callback", "tags": "release",
			"sound": "siren", "timestamp": "1700000000", "ttl": "120", "html": "1",
			"monospace": "1", "encrypted": "1",
		} {
			if got := r.Form.Get(name); got != want {
				t.Errorf("form %q = %q, want %q", name, got, want)
			}
		}
		if r.Form.Get("token") == "" {
			t.Error("app token was not sent")
		}
		_, _ = w.Write([]byte(`{"status":1,"request":"request-id","receipt":"receipt-id"}`))
	}))
	defer server.Close()
	configureCLITestEnv(t, server.URL)

	output, err := executeCLI(t, nil,
		"send", "--user", "direct-user", "--device", "phone", "--title", "title",
		"--url", "https://example.test", "--url-title", "example", "--priority", "2", "--retry", "30",
		"--expire", "3600", "--callback", "https://example.test/callback", "--tags", "release",
		"--sound", "siren", "--timestamp", "1700000000", "--ttl", "120", "--html", "--monospace",
		"--encrypted", "--json", "message",
	)
	if err != nil {
		t.Fatalf("povr send error = %v", err)
	}
	var result api.SendMessageResponse
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("send output is not JSON: %v", err)
	}
	if result.Request != "request-id" || result.Receipt != "receipt-id" {
		t.Errorf("send JSON result = %+v", result)
	}
}

func TestSendCommandReadsStdin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := r.Form.Get("message"); got != "message from stdin" {
			t.Errorf("message = %q, want message from stdin", got)
		}
		_, _ = w.Write([]byte(`{"status":1,"request":"request-id"}`))
	}))
	defer server.Close()
	configureCLITestEnv(t, server.URL)

	output, err := executeCLI(t, strings.NewReader("message from stdin"), "send", "-")
	if err != nil {
		t.Fatalf("povr send error = %v", err)
	}
	if !strings.Contains(output, "request: request-id") {
		t.Errorf("send output = %q", output)
	}
}

func TestSendCommandSurfacesValidationError(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	configureCLITestEnv(t, server.URL)

	_, err := executeCLI(t, nil, "send", "--user", "direct-user", "--priority", "2", "--retry", "1", "message")
	if err == nil || !strings.Contains(err.Error(), "retry") {
		t.Errorf("povr send error = %v, want retry validation error", err)
	}
	if requests != 0 {
		t.Errorf("requests = %d, want 0", requests)
	}
}

func TestSendCommandAttachment(t *testing.T) {
	attachmentPath := filepath.Join(t.TempDir(), "attachment.txt")
	if err := os.WriteFile(attachmentPath, []byte("attachment data"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		file, header, err := r.FormFile("attachment")
		if err != nil {
			t.Fatalf("FormFile() error = %v", err)
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if header.Filename != "attachment.txt" || string(data) != "attachment data" {
			t.Errorf("attachment filename/content = %q/%q", header.Filename, data)
		}
		_, _ = w.Write([]byte(`{"status":1,"request":"request-id"}`))
	}))
	defer server.Close()
	configureCLITestEnv(t, server.URL)

	output, err := executeCLI(t, nil, "send", "--attachment", attachmentPath, "message")
	if err != nil {
		t.Fatalf("povr send error = %v", err)
	}
	if !strings.Contains(output, "request: request-id") {
		t.Errorf("send output = %q", output)
	}
}

func configureCLITestEnv(t *testing.T, apiURL string) {
	t.Helper()
	t.Setenv("PUSHOVER_CONFIG_DIR", t.TempDir())
	t.Setenv("PUSHOVER_API_URL", apiURL)
	t.Setenv("PUSHOVER_APP_TOKEN", "testtoken1234567890123456789012")
	t.Setenv("PUSHOVER_USER_KEY", "testuserkey12345678901234567890")
}

func executeCLI(t *testing.T, input io.Reader, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCommand()
	Register(cmd)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	if input != nil {
		cmd.SetIn(input)
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return output.String(), err
}
