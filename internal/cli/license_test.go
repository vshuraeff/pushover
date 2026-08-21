package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
)

func newTestRootCommand() *cobra.Command {
	command := NewRootCommand()
	Register(command)
	return command
}

func TestLicenseAssignRequiresYesForNonTerminalStdin(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	t.Setenv("PUSHOVER_API_URL", server.URL)
	t.Setenv("PUSHOVER_APP_TOKEN", "test-fake-app-token-1234")

	originalStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = originalStdin
		_ = reader.Close()
		_ = writer.Close()
	})

	command := newTestRootCommand()
	command.SetArgs([]string{"license", "assign", "--user", "test-fake-user-key-1234"})
	command.SetIn(bytes.NewBufferString("yes\n"))

	err = command.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error = %q, want mention of --yes", err)
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("requests = %d, want 0", got)
	}
}

func TestLicenseAssignYesSendsSingleRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want %s", request.Method, http.MethodPost)
		}
		if request.URL.Path != "/1/licenses/assign.json" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.FormValue("token"); got != "test-fake-app-token-1234" {
			t.Errorf("token = %q", got)
		}
		if got := request.FormValue("email"); got != "test@example.invalid" {
			t.Errorf("email = %q", got)
		}
		if got := request.FormValue("user"); got != "" {
			t.Errorf("user = %q, want empty", got)
		}
		if got := request.FormValue("os"); got != "Desktop" {
			t.Errorf("os = %q", got)
		}
		_, _ = writer.Write([]byte(`{"status":1,"credits":3}`))
	}))
	defer server.Close()

	t.Setenv("PUSHOVER_API_URL", server.URL)
	t.Setenv("PUSHOVER_APP_TOKEN", "test-fake-app-token-1234")

	var output bytes.Buffer
	command := newTestRootCommand()
	command.SetArgs([]string{"license", "assign", "--yes", "--email", "test@example.invalid", "--os", "Desktop"})
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
	if got := output.String(); !strings.Contains(got, "remaining license credits: 3") {
		t.Errorf("output = %q", got)
	}
}

func TestLicenseCreditsCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/1/licenses.json" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.URL.Query().Get("token"); got != "test-fake-app-token-1234" {
			t.Errorf("token = %q", got)
		}
		_, _ = writer.Write([]byte(`{"status":1,"credits":9}`))
	}))
	defer server.Close()

	t.Setenv("PUSHOVER_API_URL", server.URL)
	t.Setenv("PUSHOVER_APP_TOKEN", "test-fake-app-token-1234")

	var output bytes.Buffer
	command := newTestRootCommand()
	command.SetArgs([]string{"license", "credits"})
	command.SetOut(&output)
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := output.String(); !strings.Contains(got, "remaining license credits: 9") {
		t.Errorf("output = %q", got)
	}
}
