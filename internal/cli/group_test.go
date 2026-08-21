package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vshuraeff/pushover/internal/api"
)

const testGroupTokenEnv = "TEST_GROUP_APP_TOKEN"

func TestGroupCommands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(groupCommandHandler(t)))
	defer server.Close()

	for _, test := range []struct {
		name         string
		args         []string
		wantContains string
	}{
		{name: "create", args: []string{"create", "test group"}, wantContains: "test-group-key-000000000000000\n"},
		{name: "list", args: []string{"list"}, wantContains: "test-group-key-000000000000000  test group\n"},
		{name: "show", args: []string{"show", "test-group-key-000000000000000"}, wantContains: "name: test group\nuser: test-user-key-0000000000000000  device: test-device  memo: test memo  disabled: false\n"},
		{name: "add user", args: []string{"add-user", "test-group-key-000000000000000", "test-user-key-0000000000000000", "--device", "test-device", "--memo", "test memo"}, wantContains: "added user test-user-key-0000000000000000 to group test-group-key-000000000000000\n"},
		{name: "remove user", args: []string{"remove-user", "test-group-key-000000000000000", "test-user-key-0000000000000000"}, wantContains: "removed user test-user-key-0000000000000000 in group test-group-key-000000000000000\n"},
		{name: "disable user", args: []string{"disable-user", "test-group-key-000000000000000", "test-user-key-0000000000000000", "--device", "test-device"}, wantContains: "disabled user test-user-key-0000000000000000 in group test-group-key-000000000000000\n"},
		{name: "enable user", args: []string{"enable-user", "test-group-key-000000000000000", "test-user-key-0000000000000000"}, wantContains: "enabled user test-user-key-0000000000000000 in group test-group-key-000000000000000\n"},
		{name: "rename", args: []string{"rename", "test-group-key-000000000000000", "renamed test group"}, wantContains: "renamed group test-group-key-000000000000000\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := executeGroupCommand(t, server.URL, test.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if output != test.wantContains {
				t.Errorf("output = %q, want %q", output, test.wantContains)
			}
		})
	}
}

func TestGroupJSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(groupCommandHandler(t)))
	defer server.Close()

	tests := []struct {
		name  string
		args  []string
		check func(*testing.T, []byte)
	}{
		{
			name: "create",
			args: []string{"create", "test group", "--json"},
			check: func(t *testing.T, data []byte) {
				var result struct {
					Group string `json:"group"`
				}
				if err := json.Unmarshal(data, &result); err != nil {
					t.Fatalf("Unmarshal() error = %v", err)
				}
				if result.Group != "test-group-key-000000000000000" {
					t.Errorf("group = %q", result.Group)
				}
			},
		},
		{
			name: "list",
			args: []string{"list", "--json"},
			check: func(t *testing.T, data []byte) {
				var groups []api.Group
				if err := json.Unmarshal(data, &groups); err != nil {
					t.Fatalf("Unmarshal() error = %v", err)
				}
				if len(groups) != 1 || groups[0].Name != "test group" {
					t.Errorf("groups = %#v", groups)
				}
			},
		},
		{
			name: "show",
			args: []string{"show", "test-group-key-000000000000000", "--json"},
			check: func(t *testing.T, data []byte) {
				var detail api.GroupDetail
				if err := json.Unmarshal(data, &detail); err != nil {
					t.Fatalf("Unmarshal() error = %v", err)
				}
				if detail.Name != "test group" || len(detail.Users) != 1 {
					t.Errorf("detail = %#v", detail)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, err := executeGroupCommand(t, server.URL, test.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			test.check(t, []byte(output))
		})
	}
}

func TestGroupCommandReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/1/groups.json" || request.Method != http.MethodPost {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"status":0,"errors":{"name":["is required"]}}`))
	}))
	defer server.Close()

	_, err := executeGroupCommand(t, server.URL, "create", "test bad group")
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	var apiError *api.APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error type = %T, want *api.APIError", err)
	}
}

func executeGroupCommand(t *testing.T, serverURL string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("PUSHOVER_CONFIG_DIR", t.TempDir())
	t.Setenv(testGroupTokenEnv, "test-app-token-0000000000000000000")

	cmd := NewRootCommand()
	Register(cmd)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(io.Discard)
	baseArgs := []string{"--token-env", testGroupTokenEnv, "group", "--api-base-url", serverURL + "/1"}
	cmd.SetArgs(append(baseArgs, args...))
	err := cmd.Execute()
	return output.String(), err
}

func groupCommandHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm() error = %v", err)
			return
		}
		if got := request.Form.Get("token"); got != "test-app-token-0000000000000000000" {
			t.Errorf("token = %q", got)
		}

		switch request.URL.Path {
		case "/1/groups.json":
			switch request.Method {
			case http.MethodPost:
				if request.Form.Get("name") == "" {
					t.Error("name is empty")
				}
				_, _ = writer.Write([]byte(`{"status":1,"group":"test-group-key-000000000000000"}`))
			case http.MethodGet:
				_, _ = writer.Write([]byte(`{"status":1,"groups":[{"group":"test-group-key-000000000000000","name":"test group"}]}`))
			default:
				t.Errorf("method = %s", request.Method)
			}
		case "/1/groups/test-group-key-000000000000000.json":
			_, _ = writer.Write([]byte(`{"status":1,"name":"test group","users":[{"user":"test-user-key-0000000000000000","device":"test-device","memo":"test memo","disabled":false}]}`))
		case "/1/groups/test-group-key-000000000000000/add_user.json":
			if request.Form.Get("user") == "" || request.Form.Get("device") != "test-device" || request.Form.Get("memo") != "test memo" {
				t.Errorf("add-user form = %#v", request.Form)
			}
			_, _ = writer.Write([]byte(`{"status":1}`))
		case "/1/groups/test-group-key-000000000000000/remove_user.json", "/1/groups/test-group-key-000000000000000/disable_user.json", "/1/groups/test-group-key-000000000000000/enable_user.json":
			if request.Form.Get("user") == "" {
				t.Errorf("user form = %#v", request.Form)
			}
			_, _ = writer.Write([]byte(`{"status":1}`))
		case "/1/groups/test-group-key-000000000000000/rename.json":
			if request.Form.Get("name") != "renamed test group" {
				t.Errorf("rename form = %#v", request.Form)
			}
			_, _ = writer.Write([]byte(`{"status":1}`))
		default:
			t.Errorf("path = %q", request.URL.Path)
		}
	}
}

func TestGroupJSONConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(groupCommandHandler(t)))
	defer server.Close()

	output, err := executeGroupCommand(t, server.URL, "add-user", "test-group-key-000000000000000", "test-user-key-0000000000000000", "--device", "test-device", "--memo", "test memo", "--json")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(output, `"status": "ok"`) {
		t.Errorf("output = %q", output)
	}
}
