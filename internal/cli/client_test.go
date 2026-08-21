package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/config"
	"github.com/vshuraeff/pushover/internal/openclient"
)

func TestClientFetchAndAckRequireSession(t *testing.T) {
	for _, args := range [][]string{{"client", "fetch"}, {"client", "ack", "fake-receipt-123"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Setenv("PUSHOVER_CONFIG_DIR", t.TempDir())
			root, output, stderr := clientRoot(args)
			err := root.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil")
			}
			if !strings.Contains(err.Error(), "povr client login") || !strings.Contains(err.Error(), "povr client register") {
				t.Errorf("Execute() error = %v", err)
			}
			if strings.Contains(output.String(), "fake-secret-123") || strings.Contains(stderr.String(), "fake-secret-123") {
				t.Error("command output contained the fixture secret")
			}
		})
	}
}

func TestClientCommandsUseSessionStoreSelector(t *testing.T) {
	t.Setenv("PUSHOVER_CONFIG_DIR", t.TempDir())
	t.Setenv("PUSHOVER_SESSION_STORE", "invalid")

	for _, args := range [][]string{
		{"client", "login"},
		{"client", "register", "--name", "bad name"},
		{"client", "fetch"},
		{"client", "ack", "fake-receipt-123"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root, _, _ := clientRoot(args)
			root.SetIn(strings.NewReader(""))

			err := root.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil")
			}
			if !strings.Contains(err.Error(), "create session store: unknown session_store \"invalid\"; expected \"file\" or \"keychain\"") {
				t.Errorf("Execute() error = %v", err)
			}
		})
	}
}

func TestClientRegisterRejectsInvalidNameWithoutRequest(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	for _, name := range []string{strings.Repeat("a", 26), "bad name"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("PUSHOVER_API_URL", server.URL)
			t.Setenv("PUSHOVER_CONFIG_DIR", t.TempDir())
			root, _, _ := clientRoot([]string{"client", "register", "--name", name})
			if err := root.Execute(); err == nil {
				t.Fatal("Execute() error = nil")
			}
		})
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("server calls = %d, want 0", got)
	}
}

func TestClientFetchWritesJSONBeforeUpdatingHighest(t *testing.T) {
	t.Setenv("PUSHOVER_CONFIG_DIR", t.TempDir())
	var output *bytes.Buffer
	requestOrder := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/1/messages.json":
			requestOrder = append(requestOrder, "fetch")
			if request.URL.Query().Get("secret") != "fake-secret-123" || request.URL.Query().Get("device_id") != "fake-device-123" {
				t.Error("fetch did not use the stored session")
			}
			_, _ = writer.Write([]byte(`{"status":1,"messages":[{"id":11,"id_str":"11","title":"Title","message":"Body","app":"Example","date":1700000000,"priority":1}]}`))
		case "/1/devices/fake-device-123/update_highest_message.json":
			requestOrder = append(requestOrder, "update")
			if output == nil || !strings.Contains(output.String(), `"id":11`) {
				t.Error("messages were not written before update_highest_message")
			}
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if request.Form.Get("secret") != "fake-secret-123" || request.Form.Get("message") != "11" {
				t.Error("update did not use the stored session or highest id")
			}
			_, _ = writer.Write([]byte(`{"status":1}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("PUSHOVER_API_URL", server.URL)

	store, err := config.NewFileSessionStore()
	if err != nil {
		t.Fatalf("NewFileSessionStore() error = %v", err)
	}
	if err := store.Save(&config.State{Secret: "fake-secret-123", DeviceID: "fake-device-123", DeviceName: "desktop_1"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	root, commandOutput, stderr := clientRoot([]string{"client", "fetch", "--json"})
	output = commandOutput
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var messages []openclient.Message
	if err := json.Unmarshal(commandOutput.Bytes(), &messages); err != nil {
		t.Fatalf("fetch output is not JSON: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != 11 {
		t.Errorf("output messages = %#v", messages)
	}
	if got := strings.Join(requestOrder, ","); got != "fetch,update" {
		t.Errorf("request order = %q", got)
	}
	if strings.Contains(commandOutput.String(), "fake-secret-123") || strings.Contains(stderr.String(), "fake-secret-123") {
		t.Error("command output contained the fixture secret")
	}
}

func clientRoot(args []string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	root := NewRootCommand()
	Register(root)
	output := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(output)
	root.SetErr(stderr)
	root.SetArgs(args)
	return root, output, stderr
}
