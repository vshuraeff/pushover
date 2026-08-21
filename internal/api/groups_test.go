package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testGroupAppToken = "test-app-token-0000000000000000000"

func TestGroupClientMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := request.Form.Get("token"); got != testGroupAppToken {
			t.Errorf("token = %q", got)
		}

		switch request.URL.Path {
		case "/1/groups.json":
			switch request.Method {
			case http.MethodPost:
				if got := request.Form.Get("name"); got != "test group" {
					t.Errorf("name = %q", got)
				}
				_, _ = writer.Write([]byte(`{"status":1,"group":"test-group-key-000000000000000"}`))
			case http.MethodGet:
				_, _ = writer.Write([]byte(`{"status":1,"groups":[{"group":"test-group-key-000000000000000","name":"test group"}]}`))
			default:
				t.Errorf("method = %s", request.Method)
			}
		case "/1/groups/test-group-key-000000000000000.json":
			if request.Method != http.MethodGet {
				t.Errorf("method = %s", request.Method)
			}
			_, _ = writer.Write([]byte(`{"status":1,"name":"test group","users":[{"user":"test-user-key-0000000000000000","device":null,"memo":"test memo","disabled":true}]}`))
		case "/1/groups/test-group-key-000000000000000/add_user.json":
			if request.Method != http.MethodPost {
				t.Errorf("method = %s", request.Method)
			}
			if got := request.Form.Get("user"); got != "test-user-key-0000000000000000" {
				t.Errorf("user = %q", got)
			}
			if got := request.Form.Get("device"); got != "test-device" {
				t.Errorf("device = %q", got)
			}
			if got := request.Form.Get("memo"); got != "test memo" {
				t.Errorf("memo = %q", got)
			}
			_, _ = writer.Write([]byte(`{"status":1}`))
		case "/1/groups/test-group-key-000000000000000/remove_user.json":
			assertGroupUserForm(t, request, false)
			_, _ = writer.Write([]byte(`{"status":1}`))
		case "/1/groups/test-group-key-000000000000000/disable_user.json":
			assertGroupUserForm(t, request, true)
			_, _ = writer.Write([]byte(`{"status":1}`))
		case "/1/groups/test-group-key-000000000000000/enable_user.json":
			assertGroupUserForm(t, request, false)
			_, _ = writer.Write([]byte(`{"status":1}`))
		case "/1/groups/test-group-key-000000000000000/rename.json":
			if got := request.Form.Get("name"); got != "renamed test group" {
				t.Errorf("name = %q", got)
			}
			_, _ = writer.Write([]byte(`{"status":1}`))
		default:
			t.Errorf("path = %q", request.URL.Path)
		}
	}))
	defer server.Close()

	client := New(server.URL+"/1", "0.0.0-test", WithSleep(func(time.Duration) {}))
	ctx := context.Background()

	groupKey, err := client.CreateGroup(ctx, testGroupAppToken, "test group")
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}
	if groupKey != "test-group-key-000000000000000" {
		t.Errorf("group key = %q", groupKey)
	}

	groups, err := client.ListGroups(ctx, testGroupAppToken)
	if err != nil {
		t.Fatalf("ListGroups() error = %v", err)
	}
	if len(groups) != 1 || groups[0] != (Group{Key: groupKey, Name: "test group"}) {
		t.Errorf("groups = %#v", groups)
	}

	detail, err := client.GetGroup(ctx, testGroupAppToken, groupKey)
	if err != nil {
		t.Fatalf("GetGroup() error = %v", err)
	}
	wantDetail := GroupDetail{
		Name: "test group",
		Users: []GroupMember{{
			User: "test-user-key-0000000000000000", Memo: "test memo", Disabled: true,
		}},
	}
	if detail.Name != wantDetail.Name || len(detail.Users) != 1 || detail.Users[0] != wantDetail.Users[0] {
		t.Errorf("detail = %#v", detail)
	}

	if err := client.AddGroupUser(ctx, testGroupAppToken, groupKey, "test-user-key-0000000000000000", "test-device", "test memo"); err != nil {
		t.Errorf("AddGroupUser() error = %v", err)
	}
	if err := client.RemoveGroupUser(ctx, testGroupAppToken, groupKey, "test-user-key-0000000000000000", ""); err != nil {
		t.Errorf("RemoveGroupUser() error = %v", err)
	}
	if err := client.DisableGroupUser(ctx, testGroupAppToken, groupKey, "test-user-key-0000000000000000", "test-device"); err != nil {
		t.Errorf("DisableGroupUser() error = %v", err)
	}
	if err := client.EnableGroupUser(ctx, testGroupAppToken, groupKey, "test-user-key-0000000000000000", ""); err != nil {
		t.Errorf("EnableGroupUser() error = %v", err)
	}
	if err := client.RenameGroup(ctx, testGroupAppToken, groupKey, "renamed test group"); err != nil {
		t.Errorf("RenameGroup() error = %v", err)
	}
}

func TestCreateGroupReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"status":0,"errors":{"name":["is required"]}}`))
	}))
	defer server.Close()

	_, err := New(server.URL+"/1", "0.0.0-test", WithSleep(func(time.Duration) {})).CreateGroup(context.Background(), testGroupAppToken, "")
	if err == nil {
		t.Fatal("CreateGroup() error = nil, want error")
	}
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if got := apiError.Errors["name"]; len(got) != 1 || got[0] != "is required" {
		t.Errorf("name errors = %#v", got)
	}
}

func TestAddGroupUserRejectsLongMemo(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	err := New(server.URL+"/1", "0.0.0-test", WithSleep(func(time.Duration) {})).AddGroupUser(
		context.Background(),
		testGroupAppToken,
		"test-group-key-000000000000000",
		"test-user-key-0000000000000000",
		"",
		strings.Repeat("界", 201),
	)
	if err == nil {
		t.Fatal("AddGroupUser() error = nil, want error")
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("request count = %d, want 0", got)
	}
}

func assertGroupUserForm(t *testing.T, request *http.Request, wantDevice bool) {
	t.Helper()
	if request.Method != http.MethodPost {
		t.Errorf("method = %s", request.Method)
	}
	if got := request.Form.Get("user"); got != "test-user-key-0000000000000000" {
		t.Errorf("user = %q", got)
	}
	_, hasDevice := request.Form["device"]
	if hasDevice != wantDevice {
		t.Errorf("device present = %t, want %t", hasDevice, wantDevice)
	}
}
